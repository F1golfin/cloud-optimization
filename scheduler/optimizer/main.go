// optimizer : lance tous les algorithmes d'allocation du cours sur le même
// scénario, les évalue avec le même modèle et la même fonction objectif,
// les compare au Round-Robin et écrit la meilleure allocation.
//
//   min  0,6 · Coût/Coût_RR + 0,4 · Makespan/Makespan_RR
//   s.c. chaque tâche sur exactement une VM ;
//        RAM des tâches d'une VM ≤ 85 % de sa RAM ;
//        Makespan ≤ 1,25 × Makespan_RR.
//
// Build : go build -o bin/local/optimizer ./scheduler/optimizer   (ou make)
// Usage : ./bin/local/optimizer                       compare tout, écrit data/allocation_opt.csv
//         ./bin/local/optimizer -eval data/allocation_rr.csv   détaille la prédiction d'une allocation
//
// Les allocations produites ont exactement le format du TD3
// (task_id,workload,size,vm) : l'orchestrateur les exécute sans modification.
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type row struct {
	Name, Course, Note string
	Alloc              Alloc
	E                  Eval
	CPU                time.Duration
}

func main() {
	var (
		tasksF    = flag.String("tasks", "data/tasks.csv", "tâches (task_id,workload,size)")
		machF     = flag.String("machines", "data/machines.csv", "VM disponibles (vm)")
		specsF    = flag.String("specs", "data/vm_specs.csv", "caractéristiques des VM")
		perfF     = flag.String("perf", "data/vm_performance.csv", "benchmark (une tâche seule)")
		concF     = flag.String("conc", "results/concurrency.csv", "benchmark de concurrence (facultatif)")
		out       = flag.String("out", "data/allocation_opt.csv", "allocation retenue (meilleur score valide)")
		allDir    = flag.String("all", "data/allocations", "dossier où écrire l'allocation de chaque algorithme")
		report    = flag.String("report", "results/compare.csv", "tableau comparatif (CSV)")
		overhead  = flag.Float64("overhead", 3.0, "surcoût de lancement d'une tâche, connexion SSH comprise (s)")
		exactTime = flag.Duration("exact-time", 60*time.Second, "limite de temps de la méthode exacte")
		saIters   = flag.Int("sa-iters", 200000, "itérations du recuit simulé")
		seed      = flag.Int64("seed", 42, "graine du recuit simulé (reproductibilité)")
		evalF     = flag.String("eval", "", "évaluer seulement cette allocation et afficher le détail par VM")
	)
	flag.Parse()
	if err := run(*tasksF, *machF, *specsF, *perfF, *concF, *out, *allDir, *report, *evalF,
		*overhead, *exactTime, *saIters, *seed); err != nil {
		fmt.Fprintln(os.Stderr, "Erreur :", err)
		os.Exit(1)
	}
}

func run(tasksF, machF, specsF, perfF, concF, out, allDir, report, evalF string,
	overhead float64, exactTime time.Duration, saIters int, seed int64) error {
	tasks, err := loadTasks(tasksF)
	if err != nil {
		return err
	}
	vms, err := loadVMs(machF, specsF)
	if err != nil {
		return err
	}
	perf, err := loadPerf(perfF)
	if err != nil {
		return err
	}
	slow, err := loadConcurrency(concF)
	if err != nil {
		return err
	}
	in, err := NewInstance(tasks, vms, perf, slow, overhead)
	if err != nil {
		return err
	}

	// Référence : le Round-Robin normalise le score (score du RR = 1).
	heur := Heuristics()
	rr := heur[0].Run(in)
	rrE := in.Evaluate(rr)
	in.RefCost, in.RefMakespan = rrE.Cost, rrE.Makespan

	if evalF != "" {
		return evaluateFile(in, evalF)
	}

	fmt.Printf("Scénario : %d tâches, %d VM | concurrence : %s | surcoût de lancement : %.1f s\n",
		len(tasks), len(vms), map[bool]string{true: "mesurée", false: "théorique (vCPU)"}[slow != nil], overhead)
	fmt.Printf("Référence RR : coût %.6f $, makespan %.1f s ; délai max = %.1f s\n\n",
		in.RefCost, in.RefMakespan, MaxSlowdown*in.RefMakespan)

	var rows []row
	add := func(name, course, note string, a Alloc, d time.Duration) {
		rows = append(rows, row{name, course, note, a, in.Evaluate(a), d})
	}

	// 1. Heuristiques constructives (CM3).
	for _, h := range heur {
		t0 := time.Now()
		a := h.Run(in)
		add(h.Name, h.Course, h.Rule, a, time.Since(t0))
	}

	// 2. Recherche locale multi-départ (CM5) : une descente depuis chaque heuristique.
	t0 := time.Now()
	var lsBest Alloc
	lsBestF, lsFrom := 1e18, ""
	for _, r := range rows {
		a, f := localSearch(in, r.Alloc)
		if f < lsBestF-1e-12 {
			lsBest, lsBestF, lsFrom = a, f, r.Name
		}
	}
	add("Recherche locale", "CM5", fmt.Sprintf("déplacements + échanges, %d départs (meilleur : %s)", len(rows), lsFrom),
		lsBest, time.Since(t0))

	// 3. Recuit simulé (CM5), depuis le meilleur optimum local.
	t0 = time.Now()
	saA, _ := simulatedAnnealing(in, lsBest, saIters, seed)
	add("Recuit simulé", "CM5", fmt.Sprintf("%d itérations, graine %d, puis recherche locale", saIters, seed), saA, time.Since(t0))

	// 4. Méthode exacte (CM4), avec la meilleure solution valide comme point de départ.
	inc := bestFeasible(rows)
	ex := branchAndBound(in, inc.Alloc, exactTime)
	note := fmt.Sprintf("%s, %d nœuds, borne inf. %.4f", ex.Status, ex.Nodes, ex.Bound)
	rows = append(rows, row{"Branch & bound", "CM4", note, ex.Alloc, in.Evaluate(ex.Alloc), ex.Elapsed})

	printTable(in, rows)
	if err := writeReport(report, rows); err != nil {
		return err
	}
	if err := os.MkdirAll(allDir, 0o755); err != nil {
		return err
	}
	for _, r := range rows {
		name := strings.NewReplacer(" ", "_", "&", "and", "/", "-").Replace(strings.ToLower(r.Name))
		if err := writeAlloc(filepath.Join(allDir, name+".csv"), in, r.Alloc); err != nil {
			return err
		}
	}
	best := bestFeasible(rows)
	if err := writeAlloc(out, in, best.Alloc); err != nil {
		return err
	}
	fmt.Printf("\nMeilleure allocation valide : %s (score %.4f, coût %+.1f %%, makespan %+.1f %% vs RR) -> %s\n",
		best.Name, best.E.Score, 100*(best.E.Cost/in.RefCost-1), 100*(best.E.Makespan/in.RefMakespan-1), out)
	fmt.Printf("Allocation de chaque algorithme : %s/ ; tableau : %s\n", allDir, report)
	return nil
}

// bestFeasible : la ligne valide de plus petit score (à égalité, la première = la plus simple).
func bestFeasible(rows []row) row {
	best := rows[0]
	for _, r := range rows {
		if r.E.Feasible() && (!best.E.Feasible() || r.E.Score < best.E.Score-1e-12) {
			best = r
		}
	}
	return best
}

func printTable(in *Instance, rows []row) {
	sorted := append([]row(nil), rows...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].E.Feasible() != sorted[j].E.Feasible() {
			return sorted[i].E.Feasible()
		}
		return sorted[i].E.Score < sorted[j].E.Score
	})
	fmt.Printf("%-17s %-7s %11s %9s %8s %8s %8s %4s %4s %5s %9s\n",
		"Algorithme", "Cours", "Coût ($)", "vs RR", "Makesp.", "vs RR", "Score", "VM", "RAM", "Délai", "Calcul")
	for _, r := range sorted {
		ok := func(b bool) string {
			if b {
				return "ok"
			}
			return "NON"
		}
		fmt.Printf("%-17s %-7s %11.6f %+8.1f%% %7.1fs %+7.1f%% %8.4f %4d %4s %5s %9s\n",
			r.Name, r.Course, r.E.Cost, 100*(r.E.Cost/in.RefCost-1), r.E.Makespan,
			100*(r.E.Makespan/in.RefMakespan-1), r.E.Score, r.E.UsedVMs, ok(r.E.RAMOK), ok(r.E.TimeOK),
			r.CPU.Round(time.Microsecond))
	}
	fmt.Println("\nDétail des méthodes :")
	for _, r := range rows {
		fmt.Printf("  %-17s %s\n", r.Name, r.Note)
	}
}

func writeReport(path string, rows []row) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	w.Write([]string{"algorithm", "course", "pred_cost_usd", "pred_makespan_s", "score", "used_vms", "ram_ok", "time_ok", "compute_time_s", "note"})
	for _, r := range rows {
		w.Write([]string{r.Name, r.Course, fmt.Sprintf("%.8f", r.E.Cost), fmt.Sprintf("%.3f", r.E.Makespan),
			fmt.Sprintf("%.5f", r.E.Score), fmt.Sprint(r.E.UsedVMs), fmt.Sprint(r.E.RAMOK), fmt.Sprint(r.E.TimeOK),
			fmt.Sprintf("%.6f", r.CPU.Seconds()), r.Note})
	}
	w.Flush()
	return w.Error()
}

// writeAlloc écrit une allocation au format du TD3 (écriture atomique, comme le RR du TD4).
func writeAlloc(path string, in *Instance, a Alloc) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".alloc-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	w := csv.NewWriter(tmp)
	w.Write([]string{"task_id", "workload", "size", "vm"})
	for i, t := range in.Tasks {
		w.Write([]string{t.ID, t.Workload, fmt.Sprint(t.Size), in.VMs[a[i]].Name})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// evaluateFile : prédiction détaillée d'une allocation existante (pour la comparer au mesuré).
func evaluateFile(in *Instance, path string) error {
	rows, err := readCSV(path, []string{"task_id", "workload", "size", "vm"})
	if err != nil {
		return err
	}
	idx := map[string]int{}
	for i, t := range in.Tasks {
		idx[t.ID] = i
	}
	vmIdx := map[string]int{}
	for j, v := range in.VMs {
		vmIdx[v.Name] = j
	}
	a := make(Alloc, len(in.Tasks))
	for i := range a {
		a[i] = -1
	}
	for _, r := range rows {
		t, ok1 := idx[r[0]]
		v, ok2 := vmIdx[r[3]]
		if !ok1 || !ok2 {
			return fmt.Errorf("%s : tâche ou VM inconnue (%s -> %s)", path, r[0], r[3])
		}
		if a[t] != -1 {
			return fmt.Errorf("%s : tâche %s affectée deux fois", path, r[0])
		}
		a[t] = v
	}
	for i, v := range a {
		if v == -1 {
			return fmt.Errorf("%s : tâche %s absente", path, in.Tasks[i].ID)
		}
	}
	e := in.Evaluate(a)
	fmt.Printf("Prédiction pour %s\n", path)
	for v, vm := range in.VMs {
		var ids []string
		for t, x := range a {
			if x == v {
				ids = append(ids, in.Tasks[t].ID)
			}
		}
		fmt.Printf("  %-17s %-14s %5.1f s  RAM %5.0f/%5.0f Mo  %s\n", vm.Name, vm.Type, e.Busy[v],
			e.RAMUsed[v], RAMFraction*vm.RAMMB, strings.Join(ids, " "))
	}
	fmt.Printf("Coût %.6f $ | makespan %.1f s | score %.4f | RAM %v | délai %v\n",
		e.Cost, e.Makespan, e.Score, e.RAMOK, e.TimeOK)
	return nil
}
