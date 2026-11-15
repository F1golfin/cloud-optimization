package main

// Modèle de prédiction et fonction objectif.
//
// Hypothèses (à énoncer en soutenance) :
//   H1. Les tâches d'une même VM sont lancées en même temps (& + wait, TD3)
//       et se partagent la VM : avec k tâches actives, une tâche avance
//       1/ralentissement(k) fois moins vite que seule. Le ralentissement est
//       MESURÉ (results/concurrency.csv) ; au-delà des k mesurés, on suppose
//       un partage proportionnel ; sans mesure, ralentissement = max(1, k/vCPU).
//   H2. La durée d'une tâche seule est la médiane du benchmark ; une taille non
//       mesurée est interpolée en échelle log-log.
//   H3. Chaque tâche paie un surcoût fixe de lancement (connexion SSH) : Overhead.
//   H4. Une VM utilisée est facturée prix × max(60 s, durée d'occupation)
//       (facturation à la seconde avec une minute minimum, comme GCP) ;
//       une VM sans tâche n'est pas comptée (on ne l'allume pas).
//   H5. Toutes les tâches d'une VM peuvent être actives ensemble : la somme de
//       leurs RAM (RSS max mesuré) doit rester ≤ 85 % de la RAM de la VM.

import (
	"fmt"
	"math"
)

const (
	MinBillS    = 60.0 // facturation minimale d'une VM (s)
	RAMFraction = 0.85 // part de la RAM utilisable par les tâches
	WCost       = 0.6  // poids du coût dans le score
	WTime       = 0.4  // poids du makespan dans le score
	MaxSlowdown = 1.25 // makespan autorisé ≤ 1,25 × makespan du RR
)

// Instance : tout ce qu'il faut pour évaluer une allocation.
type Instance struct {
	Tasks    []Task
	VMs      []VM
	Solo     [][]float64 // [tâche][vm] durée seule prédite (s)
	RAM      []float64   // [tâche] RAM max (Mo)
	slow     Slowdown
	Overhead float64 // surcoût de lancement par tâche (s)

	// Référence (RR) : sert à normaliser le score. 0 tant qu'elle n'est pas fixée.
	RefCost, RefMakespan float64
}

func NewInstance(tasks []Task, vms []VM, perf Perf, slow Slowdown, overhead float64) (*Instance, error) {
	in := &Instance{Tasks: tasks, VMs: vms, slow: slow, Overhead: overhead}
	in.Solo = make([][]float64, len(tasks))
	in.RAM = make([]float64, len(tasks))
	for i, t := range tasks {
		in.Solo[i] = make([]float64, len(vms))
		for j, v := range vms {
			pts := perf[v.Name][t.Workload]
			if len(pts) == 0 {
				return nil, fmt.Errorf("aucune mesure de %s sur %s (benchmark manquant)", t.Workload, v.Name)
			}
			in.Solo[i][j] = interp(pts, float64(t.Size), func(p perfPoint) float64 { return p.median })
			in.RAM[i] = math.Max(in.RAM[i], interp(pts, float64(t.Size), func(p perfPoint) float64 { return p.rssMB }))
		}
	}
	return in, nil
}

// slowdown : ralentissement d'une tâche du workload w quand k tâches sont actives sur la VM v.
func (in *Instance) slowdown(v int, w string, k int) float64 {
	if k <= 1 {
		return 1
	}
	if byK := in.slow[in.VMs[v].Name][w]; len(byK) > 0 {
		if f, ok := byK[k]; ok {
			return f
		}
		// k non mesuré : on part du plus grand k mesuré et on suppose un partage proportionnel.
		kmax := 0
		for kk := range byK {
			if kk > kmax && kk < k {
				kmax = kk
			}
		}
		if kmax >= 1 {
			return byK[kmax] * float64(k) / float64(kmax)
		}
	}
	return math.Max(1, float64(k)/float64(in.VMs[v].VCPU))
}

// simulate prédit la date de fin de chaque tâche d'une VM (H1) par simulation
// à événements : tant qu'il reste des tâches, chacune avance à la vitesse
// 1/ralentissement(k) ; on avance jusqu'à la prochaine fin de tâche.
// Renvoie la durée d'occupation de la VM (fin de sa dernière tâche).
func (in *Instance) simulate(v int, tasks []int) float64 {
	if len(tasks) == 0 {
		return 0
	}
	if len(tasks) == 1 {
		return in.Overhead + in.Solo[tasks[0]][v]
	}
	rem := make([]float64, len(tasks)) // travail restant, en secondes « seule »
	for i, t := range tasks {
		rem[i] = in.Solo[t][v]
	}
	active := len(tasks)
	now := 0.0
	for active > 0 {
		dt := math.Inf(1)
		for i, t := range tasks {
			if rem[i] > 0 {
				speed := 1 / in.slowdown(v, in.Tasks[t].Workload, active)
				dt = math.Min(dt, rem[i]/speed)
			}
		}
		now += dt
		for i, t := range tasks {
			if rem[i] > 0 {
				rem[i] -= dt / in.slowdown(v, in.Tasks[t].Workload, active)
				if rem[i] <= 1e-9 {
					rem[i] = 0
				}
			}
		}
		active = 0
		for _, r := range rem {
			if r > 0 {
				active++
			}
		}
	}
	return in.Overhead + now
}

// Alloc : alloc[i] = indice de la VM de la tâche i.
type Alloc []int

func (a Alloc) clone() Alloc { return append(Alloc(nil), a...) }

// Eval : résultat de l'évaluation d'une allocation par le modèle.
type Eval struct {
	Cost     float64   // $ (H4)
	Makespan float64   // s : fin de la dernière tâche, toutes VM confondues
	Busy     []float64 // durée d'occupation de chaque VM (0 = VM non utilisée)
	RAMUsed  []float64 // RAM réservée sur chaque VM (Mo)
	RAMOK    bool      // H5 respectée sur toutes les VM
	TimeOK   bool      // makespan ≤ 1,25 × makespan du RR
	Score    float64   // 0,6·C/C_RR + 0,4·D/D_RR (plus petit = meilleur)
	UsedVMs  int
}

func (e Eval) Feasible() bool { return e.RAMOK && e.TimeOK }

func (in *Instance) groups(a Alloc) [][]int {
	g := make([][]int, len(in.VMs))
	for t, v := range a {
		if v >= 0 {
			g[v] = append(g[v], t)
		}
	}
	return g
}

// Evaluate calcule coût, makespan, contraintes et score d'une allocation
// (éventuellement partielle : les tâches à -1 sont ignorées).
func (in *Instance) Evaluate(a Alloc) Eval {
	e := Eval{Busy: make([]float64, len(in.VMs)), RAMUsed: make([]float64, len(in.VMs)), RAMOK: true}
	for v, ts := range in.groups(a) {
		for _, t := range ts {
			e.RAMUsed[v] += in.RAM[t]
		}
		if e.RAMUsed[v] > RAMFraction*in.VMs[v].RAMMB {
			e.RAMOK = false
		}
		e.Busy[v] = in.simulate(v, ts)
		if len(ts) > 0 {
			e.UsedVMs++
			e.Cost += in.VMs[v].PriceH / 3600 * math.Max(MinBillS, e.Busy[v])
		}
		e.Makespan = math.Max(e.Makespan, e.Busy[v])
	}
	e.TimeOK = in.RefMakespan == 0 || e.Makespan <= MaxSlowdown*in.RefMakespan+1e-9
	e.Score = in.score(e.Cost, e.Makespan)
	return e
}

func (in *Instance) score(cost, makespan float64) float64 {
	if in.RefCost == 0 || in.RefMakespan == 0 {
		return math.Inf(1)
	}
	return WCost*cost/in.RefCost + WTime*makespan/in.RefMakespan
}

// objective : valeur minimisée par la recherche locale et la méthode exacte :
// le score, avec une forte pénalité pour chaque contrainte violée (pour que
// la recherche puisse traverser des allocations invalides puis en sortir).
func (in *Instance) objective(e Eval) float64 {
	pen := 0.0
	for v, r := range e.RAMUsed {
		if over := r - RAMFraction*in.VMs[v].RAMMB; over > 0 {
			pen += 10 + over/100
		}
	}
	if !e.TimeOK {
		pen += 10 + (e.Makespan/(MaxSlowdown*in.RefMakespan) - 1)
	}
	return e.Score + pen
}

// ramFits : la tâche t peut-elle s'ajouter à la VM v (contrainte H5) ?
func (in *Instance) ramFits(used []float64, t, v int) bool {
	return used[v]+in.RAM[t] <= RAMFraction*in.VMs[v].RAMMB+1e-9
}
