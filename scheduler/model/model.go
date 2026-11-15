// Package model est le modèle commun à tous les algorithmes de scheduling :
// il lit les données, PRÉDIT la durée de chaque tâche sur chaque VM, simule
// l'exécution d'une allocation comme le ferait l'orchestrateur, et calcule
// le makespan D, le coût C et le score S.
//
// Tous les algorithmes sont jugés avec ce même modèle : la comparaison est
// équitable. Rien ici n'exécute de tâche ni ne contacte GCP.
package model

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Task : une tâche à placer (ligne de tasks.csv).
type Task struct {
	ID       string
	Workload string
	Size     int
}

// Machine : une VM candidate (ligne de machines.csv).
type Machine struct {
	Name         string
	Type         string
	VCPU         int     // = nombre de workers lancés par l'orchestrateur
	RAMMB        int     // mémoire de la VM
	PricePerHour float64 // $/h
	Contention   float64 // ralentissement mesuré quand tous les workers tournent (expérience workers)
}

// Params : hypothèses du modèle, toutes issues de mesures ou de nos choix de projet.
type Params struct {
	OverheadS      float64 // surcoût SSH par tâche, mesuré par l'orchestrateur (~0,1 s)
	MinBilledS     float64 // facturation minimale par VM allumée (règle GCP : 60 s)
	WeightCost     float64 // poids du coût dans S (0,6) ; le temps pèse 1 - WeightCost
	DeadlineFactor float64 // contrainte D <= DeadlineFactor x D_RR (1,25)
}

func DefaultParams() Params {
	return Params{OverheadS: 0.10, MinBilledS: 60, WeightCost: 0.6, DeadlineFactor: 1.25}
}

// Instance : un problème complet, prêt à être résolu.
type Instance struct {
	Tasks    []Task
	Machines []Machine
	Params   Params
	// P[i][j] : durée PRÉDITE de la tâche i sur la VM j, en secondes
	//   = médiane du benchmark x contention de la VM + surcoût SSH
	P [][]float64
	// RSS[i][j] : mémoire max mesurée (Ko) de la tâche i sur la VM j
	RSS [][]int
}

// Assignment : la tâche Task (indice dans Tasks) va sur la VM VM (indice dans Machines).
// Une Allocation est ORDONNÉE : sur chaque VM, l'orchestrateur exécute les
// tâches dans l'ordre de la liste. L'ordre fait donc partie de la décision.
type Assignment struct{ Task, VM int }
type Allocation []Assignment

// ------------------------------------------------------------ chargement

// Load lit tasks.csv, machines.csv et vm_performance.csv. Les VM dont le nom
// est dans exclude sont retirées (scénario « VM indisponible »).
func Load(tasksPath, machinesPath, perfPath string, exclude []string, p Params) (*Instance, error) {
	in := &Instance{Params: p}

	rows, err := readCSV(tasksPath, "task_id", "workload", "size")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for n, r := range rows {
		size, err := strconv.Atoi(r["size"])
		if err != nil || size <= 0 || r["task_id"] == "" || seen[r["task_id"]] {
			return nil, fmt.Errorf("%s ligne %d : tâche invalide ou en double", tasksPath, n+2)
		}
		seen[r["task_id"]] = true
		in.Tasks = append(in.Tasks, Task{ID: r["task_id"], Workload: r["workload"], Size: size})
	}

	skip := map[string]bool{}
	for _, e := range exclude {
		skip[strings.TrimSpace(e)] = true
	}
	rows, err = readCSV(machinesPath, "vm", "machine_type", "vcpu", "ram_mb", "price_per_hour", "contention")
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if skip[r["vm"]] {
			continue
		}
		vcpu, e1 := strconv.Atoi(r["vcpu"])
		ram, e2 := strconv.Atoi(r["ram_mb"])
		price, e3 := strconv.ParseFloat(r["price_per_hour"], 64)
		cont, e4 := strconv.ParseFloat(r["contention"], 64)
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil || vcpu <= 0 || cont <= 0 {
			return nil, fmt.Errorf("%s : ligne invalide pour %s", machinesPath, r["vm"])
		}
		in.Machines = append(in.Machines, Machine{Name: r["vm"], Type: r["machine_type"],
			VCPU: vcpu, RAMMB: ram, PricePerHour: price, Contention: cont})
	}
	if len(in.Machines) == 0 {
		return nil, fmt.Errorf("aucune VM disponible")
	}

	type key struct {
		vm, workload string
		size         int
	}
	median := map[key]float64{}
	rss := map[key]int{}
	rows, err = readCSV(perfPath, "machine", "workload", "size", "median_s", "max_rss_kb")
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		size, _ := strconv.Atoi(r["size"])
		m, err := strconv.ParseFloat(r["median_s"], 64)
		if err != nil {
			return nil, fmt.Errorf("%s : médiane invalide", perfPath)
		}
		k := key{r["machine"], r["workload"], size}
		median[k] = m
		rss[k], _ = strconv.Atoi(r["max_rss_kb"])
	}

	in.P = make([][]float64, len(in.Tasks))
	in.RSS = make([][]int, len(in.Tasks))
	for i, t := range in.Tasks {
		in.P[i] = make([]float64, len(in.Machines))
		in.RSS[i] = make([]int, len(in.Machines))
		for j, m := range in.Machines {
			k := key{m.Name, t.Workload, t.Size}
			med, ok := median[k]
			if !ok {
				return nil, fmt.Errorf("pas de mesure pour %s %d sur %s (taille absente du benchmark)", t.Workload, t.Size, m.Name)
			}
			in.P[i][j] = med*m.Contention + p.OverheadS
			in.RSS[i][j] = rss[k]
		}
	}
	return in, nil
}

// LoadAllocation lit un allocation.csv existant (RR, manuel...) pour l'évaluer.
func (in *Instance) LoadAllocation(path string) (Allocation, error) {
	rows, err := readCSV(path, "task_id", "vm")
	if err != nil {
		return nil, err
	}
	taskIdx := map[string]int{}
	for i, t := range in.Tasks {
		taskIdx[t.ID] = i
	}
	vmIdx := map[string]int{}
	for j, m := range in.Machines {
		vmIdx[m.Name] = j
	}
	var a Allocation
	for _, r := range rows {
		i, ok1 := taskIdx[r["task_id"]]
		j, ok2 := vmIdx[r["vm"]]
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("%s : tâche %s ou VM %s inconnue", path, r["task_id"], r["vm"])
		}
		a = append(a, Assignment{Task: i, VM: j})
	}
	return a, in.Validate(a)
}

// ------------------------------------------------------------ simulation

// Lanes : l'heure de fin prévue de chaque worker d'une VM. Une tâche est
// confiée au worker qui se libère le premier, exactement comme dans
// l'orchestrateur (N workers qui vident la file de la VM).
type Lanes []float64

func NewLanes(workers int) Lanes { return make(Lanes, workers) }

func (l Lanes) earliest() int {
	best := 0
	for w := 1; w < len(l); w++ {
		if l[w] < l[best] {
			best = w
		}
	}
	return best
}

// FinishIfAdded : heure de fin de la tâche si on l'ajoutait maintenant.
func (l Lanes) FinishIfAdded(p float64) float64 { return l[l.earliest()] + p }

// Add ajoute une tâche de durée p et renvoie son heure de fin.
func (l Lanes) Add(p float64) float64 {
	w := l.earliest()
	l[w] += p
	return l[w]
}

// End : heure de fin de la VM (son dernier worker).
func (l Lanes) End() float64 {
	end := 0.0
	for _, t := range l {
		if t > end {
			end = t
		}
	}
	return end
}

// ------------------------------------------------------------ évaluation

type VMStat struct {
	Tasks    int
	Time     float64 // fin prévue de la VM (s)
	BilledS  float64 // max(MinBilledS, Time) si la VM est utilisée, 0 sinon
	Cost     float64 // $
	MaxRSSKB int     // plus grosse tâche placée sur la VM
	RAMOK    bool    // workers x plus grosse tâche <= RAM
}

type Metrics struct {
	Makespan float64 // D (s)
	Cost     float64 // C ($)
	UsedVMs  int
	RAMOK    bool
	PerVM    []VMStat
}

// Validate : chaque tâche apparaît exactement une fois, sur une VM existante.
func (in *Instance) Validate(a Allocation) error {
	count := make([]int, len(in.Tasks))
	for _, x := range a {
		if x.Task < 0 || x.Task >= len(in.Tasks) || x.VM < 0 || x.VM >= len(in.Machines) {
			return fmt.Errorf("affectation hors limites : %+v", x)
		}
		count[x.Task]++
	}
	for i, c := range count {
		if c != 1 {
			return fmt.Errorf("tâche %s affectée %d fois", in.Tasks[i].ID, c)
		}
	}
	return nil
}

// Evaluate simule l'allocation et calcule D, C et les statistiques par VM.
func (in *Instance) Evaluate(a Allocation) Metrics {
	lanes := make([]Lanes, len(in.Machines))
	for j, m := range in.Machines {
		lanes[j] = NewLanes(m.VCPU)
	}
	met := Metrics{PerVM: make([]VMStat, len(in.Machines)), RAMOK: true}
	for _, x := range a {
		lanes[x.VM].Add(in.P[x.Task][x.VM])
		s := &met.PerVM[x.VM]
		s.Tasks++
		if r := in.RSS[x.Task][x.VM]; r > s.MaxRSSKB {
			s.MaxRSSKB = r
		}
	}
	for j, m := range in.Machines {
		s := &met.PerVM[j]
		s.RAMOK = m.VCPU*s.MaxRSSKB <= m.RAMMB*1024
		met.RAMOK = met.RAMOK && s.RAMOK
		if s.Tasks == 0 {
			continue // VM non utilisée : on ne l'allume pas, elle ne coûte rien
		}
		s.Time = lanes[j].End()
		s.BilledS = s.Time
		if s.BilledS < in.Params.MinBilledS {
			s.BilledS = in.Params.MinBilledS
		}
		s.Cost = s.BilledS * m.PricePerHour / 3600
		met.Cost += s.Cost
		met.UsedVMs++
		if s.Time > met.Makespan {
			met.Makespan = s.Time
		}
	}
	return met
}

// Score : S = w·C/C_ref + (1-w)·D/D_ref. Plus bas = mieux ; la référence (RR) vaut 1.
func (in *Instance) Score(m, ref Metrics) float64 {
	w := in.Params.WeightCost
	return w*m.Cost/ref.Cost + (1-w)*m.Makespan/ref.Makespan
}

// Feasible : contrainte de délai D <= 1,25·D_ref et contrainte de RAM.
func (in *Instance) Feasible(m, ref Metrics) bool {
	return m.RAMOK && m.Makespan <= in.Params.DeadlineFactor*ref.Makespan+1e-9
}

// ------------------------------------------------------------ sortie

// WriteAllocation écrit l'allocation au format lu par l'orchestrateur.
func (in *Instance) WriteAllocation(path string, a Allocation) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	w.Write([]string{"task_id", "workload", "size", "vm"})
	for _, x := range a {
		t := in.Tasks[x.Task]
		w.Write([]string{t.ID, t.Workload, strconv.Itoa(t.Size), in.Machines[x.VM].Name})
	}
	w.Flush()
	return w.Error()
}

// ------------------------------------------------------------ CSV

func readCSV(path string, want ...string) ([]map[string]string, error) {
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
	out := make([]map[string]string, 0, len(rows)-1)
	for _, row := range rows[1:] {
		m := map[string]string{}
		for _, w := range want {
			m[w] = strings.TrimSpace(row[col[w]])
		}
		out = append(out, m)
	}
	return out, nil
}
