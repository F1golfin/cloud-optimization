// Orchestrateur : exécute une allocation sur les VM et collecte les résultats.
//
//	allocation.csv ──► une file de tâches par VM ──► N workers par VM (SSH) ──► results.csv
//
// Principes :
//   - Le scheduler DÉCIDE (allocation.csv), l'orchestrateur EXÉCUTE : il ne
//     déplace jamais une tâche vers une autre VM que celle choisie.
//   - Chaque VM a sa propre file ; toutes les VM travaillent en parallèle, et
//     sur une VM, N workers (par défaut N = nombre de vCPU) vident la file.
//   - Une seule connexion SSH par VM, réutilisée par toutes ses tâches
//     (multiplexage OpenSSH ControlMaster) : on ne paie pas une poignée de
//     main SSH (~0,5–1 s) par tâche.
//   - Les horodatages début/fin sont pris SUR LA MACHINE DE CONTRÔLE, avec une
//     seule horloge : pas de problème de synchronisation entre VM. La durée
//     mesurée inclut donc le surcoût réseau/SSH, que l'on isole en le
//     comparant au runtime_s renvoyé par le workload lui-même.
//
// Usage :
//
//	./bin/orchestrator -alloc data/allocation_rr.csv -out results/run_rr.csv
//	./bin/orchestrator -alloc data/allocation_rr.csv -workers 1
//	./bin/orchestrator -alloc data/allocation_rr.csv -local   # test sans GCP
package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Seuls ces programmes peuvent être lancés : le nom vient d'un CSV et finit
// dans une commande shell distante, on refuse donc toute autre valeur.
var allowedWorkloads = map[string]bool{
	"cpu_task": true, "memory_task": true, "matrix_task": true, "sort_task": true,
}

// Facturation minimale par VM active, identique au modèle du scheduler.
const minBilledSeconds = 60.0

type Task struct {
	ID       string
	Workload string
	Size     int
	VM       string
}

type Machine struct {
	Name         string
	Type         string
	VCPU         int
	PricePerHour float64
}

type Result struct {
	Task
	Worker        int
	Start, End    time.Duration // relatifs au lancement du lot (t0)
	RemoteRuntime float64       // runtime_s affiché par le workload, -1 si absent
	ExitCode      int
	Err           string
}

func (r Result) Duration() float64 { return (r.End - r.Start).Seconds() }

// Runner : la façon d'exécuter une tâche sur une VM (SSH réel ou local pour tester).
type Runner interface {
	Prepare(vms []string) error
	Run(ctx context.Context, vm string, t Task) (stdout string, err error)
	Close()
}

func main() {
	allocPath := flag.String("alloc", "", "allocation à exécuter (task_id,workload,size,vm) [obligatoire]")
	machinesPath := flag.String("machines", "data/machines.csv", "description des VM (vm,machine_type,vcpu,ram_mb,price_per_hour)")
	outPath := flag.String("out", "results/results.csv", "fichier de résultats par tâche")
	workers := flag.Int("workers", 0, "workers par VM (0 = nombre de vCPU de la VM)")
	local := flag.Bool("local", false, "exécute les tâches sur cette machine au lieu des VM (test)")
	binDir := flag.String("bin", "bin/local", "dossier des workloads en mode -local")
	project := flag.String("project", envOr("PROJECT", "cloud-opt-511106"), "projet GCP")
	zone := flag.String("zone", envOr("ZONE", "europe-west1-b"), "zone GCP")
	user := flag.String("user", os.Getenv("USER"), "utilisateur SSH sur les VM")
	key := flag.String("key", filepath.Join(os.Getenv("HOME"), ".ssh", "google_compute_engine"), "clé SSH créée par gcloud")
	remoteDir := flag.String("remote-dir", "cloudopt", "dossier de déploiement sur les VM (cf. scripts/deploy.sh)")
	timeout := flag.Duration("timeout", 5*time.Minute, "durée maximale d'une tâche")
	flag.Parse()

	if *allocPath == "" {
		fmt.Fprintln(os.Stderr, "Erreur : -alloc est obligatoire")
		flag.Usage()
		os.Exit(2)
	}

	tasks, err := readAllocation(*allocPath)
	check(err)
	machines, err := readMachines(*machinesPath)
	check(err)

	// Regroupe les tâches par VM en gardant l'ordre du CSV (= ordre de la file).
	queues := map[string][]Task{}
	var vms []string
	for _, t := range tasks {
		if _, ok := machines[t.VM]; !ok {
			check(fmt.Errorf("tâche %s : VM %q absente de %s", t.ID, t.VM, *machinesPath))
		}
		if _, seen := queues[t.VM]; !seen {
			vms = append(vms, t.VM)
		}
		queues[t.VM] = append(queues[t.VM], t)
	}
	sort.Strings(vms)

	var runner Runner
	if *local {
		runner = &localRunner{binDir: *binDir}
	} else {
		runner = &sshRunner{project: *project, zone: *zone, user: *user, key: *key, remoteDir: *remoteDir}
	}
	fmt.Printf("Allocation %s : %d tâches sur %d VM (%s)\n", *allocPath, len(tasks), len(vms), strings.Join(vms, ", "))
	fmt.Println("Préparation des connexions...")
	check(runner.Prepare(vms))
	defer runner.Close()

	// ------------------------------------------------------------ exécution
	var (
		mu      sync.Mutex
		results []Result
		wg      sync.WaitGroup
	)
	t0 := time.Now()
	for _, vm := range vms {
		n := *workers
		if n <= 0 {
			n = machines[vm].VCPU
		}
		queue := make(chan Task, len(queues[vm]))
		for _, t := range queues[vm] {
			queue <- t
		}
		close(queue)
		fmt.Printf("  %-17s %3d tâches, %d workers\n", vm, len(queues[vm]), n)

		for w := 1; w <= n; w++ {
			wg.Add(1)
			go func(vm string, worker int) {
				defer wg.Done()
				for t := range queue { // chaque worker prend la tâche suivante de la file de SA VM
					r := execute(runner, vm, worker, t, t0, *timeout)
					mu.Lock()
					results = append(results, r)
					status := "ok"
					if r.ExitCode != 0 {
						status = fmt.Sprintf("ÉCHEC (code %d) %s", r.ExitCode, r.Err)
					}
					fmt.Printf("  [%7.2fs] %-6s %-12s %-9d %-17s w%d %6.2fs  %s\n",
						r.End.Seconds(), t.ID, t.Workload, t.Size, vm, worker, r.Duration(), status)
					mu.Unlock()
				}
			}(vm, w)
		}
	}
	wg.Wait()

	sort.Slice(results, func(i, j int) bool { return results[i].Start < results[j].Start })
	check(writeResults(*outPath, results))
	failed := summarize(results, machines, vms, *outPath)
	if failed > 0 {
		os.Exit(1)
	}
}

// execute lance une tâche et mesure début/fin avec l'horloge de la machine de contrôle.
func execute(runner Runner, vm string, worker int, t Task, t0 time.Time, timeout time.Duration) Result {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	r := Result{Task: t, Worker: worker, RemoteRuntime: -1}
	r.Start = time.Since(t0)
	stdout, err := runner.Run(ctx, vm, t)
	r.End = time.Since(t0)

	if err != nil {
		r.ExitCode = 1
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			r.ExitCode = ee.ExitCode() // 255 = erreur SSH, sinon code du workload
		}
		if ctx.Err() == context.DeadlineExceeded {
			r.Err = "timeout"
		} else {
			r.Err = strings.TrimSpace(err.Error())
		}
		return r
	}
	// Le workload affiche une ligne JSON contenant son propre temps de calcul.
	var out struct {
		RuntimeS *float64 `json:"runtime_s"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(stdout)), &out) == nil && out.RuntimeS != nil {
		r.RemoteRuntime = *out.RuntimeS
	}
	return r
}

// ------------------------------------------------------------ SSH vers GCP

type sshRunner struct {
	project, zone, user, key, remoteDir string
	hosts                               map[string]hostInfo
	ctlDir                              string
}

type hostInfo struct{ id, ip string }

// Prepare récupère l'IP de chaque VM via gcloud (elle change à chaque
// démarrage) puis ouvre une connexion SSH maître par VM, réutilisée ensuite.
func (s *sshRunner) Prepare(vms []string) error {
	out, err := exec.Command("gcloud", "compute", "instances", "list",
		"--project="+s.project, "--zones="+s.zone,
		"--format=csv[no-heading](name,id,status,networkInterfaces[0].accessConfigs[0].natIP)").Output()
	if err != nil {
		return fmt.Errorf("gcloud compute instances list : %v", err)
	}
	s.hosts = map[string]hostInfo{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Split(line, ",")
		if len(f) == 4 && f[2] == "RUNNING" && f[3] != "" {
			s.hosts[f[0]] = hostInfo{id: f[1], ip: f[3]}
		}
	}
	for _, vm := range vms {
		if _, ok := s.hosts[vm]; !ok {
			return fmt.Errorf("%s n'est pas démarrée (./scripts/manage_vms.sh start)", vm)
		}
	}

	// Dossier court pour les sockets de contrôle (limite ~100 caractères).
	if s.ctlDir, err = os.MkdirTemp("/tmp", "cossh"); err != nil {
		return err
	}
	var wg sync.WaitGroup
	errs := make([]error, len(vms))
	for i, vm := range vms {
		wg.Add(1)
		go func(i int, vm string) {
			defer wg.Done()
			// Juste après « manage_vms.sh start », la VM est RUNNING mais son
			// serveur SSH démarre encore (« Connection refused ») : on réessaie
			// pendant ~2 minutes. Une erreur de clé, elle, ne se corrige pas
			// toute seule : on abandonne aussitôt.
			var out []byte
			var err error
			for attempt := 1; attempt <= 24; attempt++ {
				out, err = exec.Command("ssh", append(s.sshArgs(vm), "true")...).CombinedOutput()
				msg := string(out)
				if err == nil || !(strings.Contains(msg, "Connection refused") ||
					strings.Contains(msg, "timed out") || strings.Contains(msg, "No route to host")) {
					break
				}
				if attempt == 1 {
					fmt.Printf("  %s : SSH pas encore prêt, attente...\n", vm)
				}
				time.Sleep(5 * time.Second)
			}
			if err != nil {
				errs[i] = fmt.Errorf("connexion SSH à %s impossible : %v %s\n"+
					"  (si c'est la première fois : gcloud compute ssh %s --zone %s --command true)",
					vm, err, strings.TrimSpace(string(out)), vm, s.zone)
			}
		}(i, vm)
	}
	wg.Wait()
	return errors.Join(errs...)
}

// sshArgs : mêmes clé et fichier known_hosts que gcloud compute ssh.
// gcloud enregistre la clé d'hôte de chaque VM sous l'alias « compute.<id> »,
// ce qui permet une vérification stricte même si l'IP change.
func (s *sshRunner) sshArgs(vm string) []string {
	h := s.hosts[vm]
	return []string{
		"-i", s.key,
		"-o", "HostKeyAlias=compute." + h.id,
		"-o", "UserKnownHostsFile=" + filepath.Join(filepath.Dir(s.key), "google_compute_known_hosts"),
		"-o", "StrictHostKeyChecking=yes",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=20",
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=" + filepath.Join(s.ctlDir, "%C"),
		"-o", "ControlPersist=300",
		s.user + "@" + h.ip,
	}
}

func (s *sshRunner) Run(ctx context.Context, vm string, t Task) (string, error) {
	// Workload et taille sont validés à la lecture du CSV : la commande est sûre.
	remote := fmt.Sprintf("cd ~/%s && ./bin/%s --size %d", s.remoteDir, t.Workload, t.Size)
	return runCmd(ctx, "ssh", append(s.sshArgs(vm), remote)...)
}

func (s *sshRunner) Close() {
	// Ferme les connexions maîtres (sinon elles restent ouvertes ControlPersist secondes).
	for vm := range s.hosts {
		args := s.sshArgs(vm)
		dest := args[len(args)-1]
		_ = exec.Command("ssh", append(args[:len(args)-1], "-O", "exit", dest)...).Run()
	}
	if s.ctlDir != "" {
		os.RemoveAll(s.ctlDir)
	}
}

// ------------------------------------------------------------ mode local (test)

type localRunner struct{ binDir string }

func (l *localRunner) Prepare(vms []string) error {
	for w := range allowedWorkloads {
		if _, err := os.Stat(filepath.Join(l.binDir, w)); err != nil {
			return fmt.Errorf("%s/%s introuvable (lancer : make)", l.binDir, w)
		}
	}
	return nil
}

func (l *localRunner) Run(ctx context.Context, _ string, t Task) (string, error) {
	return runCmd(ctx, filepath.Join(l.binDir, t.Workload), "--size", strconv.Itoa(t.Size))
}

func (l *localRunner) Close() {}

func runCmd(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil && stderr.Len() > 0 {
		err = fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return string(out), err
}

// ------------------------------------------------------------ entrées / sorties

func readCSV(path string, want []string) ([]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%s : %v", path, err)
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("%s : fichier vide", path)
	}
	col := map[string]int{}
	for i, h := range rows[0] {
		col[strings.TrimSpace(h)] = i
	}
	for _, w := range want {
		if _, ok := col[w]; !ok {
			return nil, fmt.Errorf("%s : colonne %q manquante", path, w)
		}
	}
	var out []map[string]string
	for _, row := range rows[1:] {
		m := map[string]string{}
		for _, w := range want {
			m[w] = strings.TrimSpace(row[col[w]])
		}
		out = append(out, m)
	}
	return out, nil
}

func readAllocation(path string) ([]Task, error) {
	rows, err := readCSV(path, []string{"task_id", "workload", "size", "vm"})
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var tasks []Task
	for i, r := range rows {
		size, err := strconv.Atoi(r["size"])
		switch {
		case r["task_id"] == "" || r["vm"] == "":
			return nil, fmt.Errorf("%s ligne %d : task_id ou vm vide", path, i+2)
		case seen[r["task_id"]]:
			return nil, fmt.Errorf("%s : task_id %s en double", path, r["task_id"])
		case !allowedWorkloads[r["workload"]]:
			return nil, fmt.Errorf("%s ligne %d : workload inconnu %q", path, i+2, r["workload"])
		case err != nil || size <= 0:
			return nil, fmt.Errorf("%s ligne %d : taille invalide %q", path, i+2, r["size"])
		}
		seen[r["task_id"]] = true
		tasks = append(tasks, Task{ID: r["task_id"], Workload: r["workload"], Size: size, VM: r["vm"]})
	}
	return tasks, nil
}

func readMachines(path string) (map[string]Machine, error) {
	rows, err := readCSV(path, []string{"vm", "machine_type", "vcpu", "price_per_hour"})
	if err != nil {
		return nil, err
	}
	ms := map[string]Machine{}
	for _, r := range rows {
		vcpu, err1 := strconv.Atoi(r["vcpu"])
		price, err2 := strconv.ParseFloat(r["price_per_hour"], 64)
		if err1 != nil || err2 != nil || vcpu <= 0 {
			return nil, fmt.Errorf("%s : ligne invalide pour %s", path, r["vm"])
		}
		ms[r["vm"]] = Machine{Name: r["vm"], Type: r["machine_type"], VCPU: vcpu, PricePerHour: price}
	}
	return ms, nil
}

func writeResults(path string, results []Result) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	w.Write([]string{"task_id", "workload", "size", "vm", "worker", "start_s", "end_s",
		"duration_s", "remote_runtime_s", "overhead_s", "exit_code", "error"})
	for _, r := range results {
		remote, overhead := "", ""
		if r.RemoteRuntime >= 0 {
			remote = f6(r.RemoteRuntime)
			overhead = f6(r.Duration() - r.RemoteRuntime)
		}
		w.Write([]string{r.ID, r.Workload, strconv.Itoa(r.Size), r.VM, strconv.Itoa(r.Worker),
			f6(r.Start.Seconds()), f6(r.End.Seconds()), f6(r.Duration()), remote, overhead,
			strconv.Itoa(r.ExitCode), r.Err})
	}
	w.Flush()
	return w.Error()
}

// summarize affiche makespan, charge et coût par VM, et les écrit dans <out>_vms.csv.
func summarize(results []Result, machines map[string]Machine, vms []string, outPath string) (failed int) {
	type agg struct {
		n                 int
		first, last       float64
		busy, remote, ovh float64
		nRemote           int
	}
	per := map[string]*agg{}
	makespan := 0.0
	for _, r := range results {
		a := per[r.VM]
		if a == nil {
			a = &agg{first: r.Start.Seconds()}
			per[r.VM] = a
		}
		a.n++
		a.busy += r.Duration()
		if s := r.Start.Seconds(); s < a.first {
			a.first = s
		}
		if e := r.End.Seconds(); e > a.last {
			a.last = e
		}
		if r.RemoteRuntime >= 0 {
			a.remote += r.RemoteRuntime
			a.ovh += r.Duration() - r.RemoteRuntime
			a.nRemote++
		}
		if r.ExitCode != 0 {
			failed++
		}
		if e := r.End.Seconds(); e > makespan {
			makespan = e
		}
	}

	summaryPath := strings.TrimSuffix(outPath, filepath.Ext(outPath)) + "_vms.csv"
	f, err := os.Create(summaryPath)
	check(err)
	defer f.Close()
	w := csv.NewWriter(f)
	w.Write([]string{"vm", "tasks", "first_start_s", "last_end_s", "active_s", "busy_s",
		"remote_runtime_s", "mean_overhead_s", "billed_s", "cost_usd"})

	fmt.Printf("\n%-17s %6s %9s %9s %9s %10s %10s\n", "VM", "tâches", "actif(s)", "occupé(s)", "calcul(s)", "surcoût/t", "coût($)")
	totalCost := 0.0
	for _, vm := range vms {
		a := per[vm]
		active := a.last - a.first
		billed := active
		if billed < minBilledSeconds {
			billed = minBilledSeconds
		}
		cost := billed * machines[vm].PricePerHour / 3600
		totalCost += cost
		meanOvh := 0.0
		if a.nRemote > 0 {
			meanOvh = a.ovh / float64(a.nRemote)
		}
		fmt.Printf("%-17s %6d %9.2f %9.2f %9.2f %9.3fs %10.6f\n", vm, a.n, active, a.busy, a.remote, meanOvh, cost)
		w.Write([]string{vm, strconv.Itoa(a.n), f6(a.first), f6(a.last), f6(active), f6(a.busy),
			f6(a.remote), f6(meanOvh), f6(billed), fmt.Sprintf("%.8f", cost)})
	}
	w.Flush()

	fmt.Printf("\nMakespan mesuré : %.3f s | coût (modèle, min. %.0f s/VM) : %.6f $ | tâches : %d ok, %d en échec\n",
		makespan, minBilledSeconds, totalCost, len(results)-failed, failed)
	fmt.Printf("Résultats : %s  |  par VM : %s\n", outPath, summaryPath)
	return failed
}

func f6(x float64) string { return strconv.FormatFloat(x, 'f', 6, 64) }

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "Erreur :", err)
		os.Exit(1)
	}
}
