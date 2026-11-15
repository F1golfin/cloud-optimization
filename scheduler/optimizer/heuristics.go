package main

// Heuristiques constructives du cours (CM3) + notre glouton (CM3 + CM6).
//
// Toutes, sauf RR, suivent le cycle du CM6 : FILTRER (VM dont la RAM peut
// encore accueillir la tâche) puis CHOISIR parmi les VM restantes selon la
// règle de l'heuristique. RR reste la référence brute du TD4 (aucun filtre).
//
// Adaptation au projet : sur une VM, les tâches tournent en même temps (TD3) ;
// « la VM qui se libère le plus tôt » des CM3 devient donc « la VM dont la
// durée d'occupation prédite (modèle H1) est la plus faible ».

import (
	"math"
	"sort"
)

// builder : état d'une allocation en construction.
type builder struct {
	in    *Instance
	a     Alloc
	group [][]int   // tâches déjà placées sur chaque VM
	busy  []float64 // durée d'occupation prédite de chaque VM
	ram   []float64 // RAM déjà réservée sur chaque VM
}

func newBuilder(in *Instance) *builder {
	b := &builder{in: in, a: make(Alloc, len(in.Tasks)), group: make([][]int, len(in.VMs)),
		busy: make([]float64, len(in.VMs)), ram: make([]float64, len(in.VMs))}
	for i := range b.a {
		b.a[i] = -1
	}
	return b
}

func (b *builder) place(t, v int) {
	b.a[t] = v
	b.group[v] = append(b.group[v], t)
	b.ram[v] += b.in.RAM[t]
	b.busy[v] = b.in.simulate(v, b.group[v])
}

// busyIf : durée d'occupation de v si on y ajoutait t (« completion time »).
func (b *builder) busyIf(t, v int) float64 {
	return b.in.simulate(v, append(append([]int(nil), b.group[v]...), t))
}

// candidates : VM qui passent le filtre RAM. Si aucune ne passe, on garde
// toutes les VM (l'allocation sera signalée invalide par l'évaluation).
func (b *builder) candidates(t int) []int {
	var c []int
	for v := range b.in.VMs {
		if b.in.ramFits(b.ram, t, v) {
			c = append(c, v)
		}
	}
	if len(c) == 0 {
		for v := range b.in.VMs {
			c = append(c, v)
		}
	}
	return c
}

// argmin renvoie la VM candidate de plus petite valeur (égalité : ordre de machines.csv).
func argmin(c []int, f func(v int) float64) int {
	best, bv := -1, math.Inf(1)
	for _, v := range c {
		if x := f(v); x < bv-1e-12 {
			best, bv = v, x
		}
	}
	return best
}

// Heuristic : un algorithme qui produit une allocation complète.
type Heuristic struct {
	Name, Course, Rule string
	Run                func(*Instance) Alloc
}

// order renvoie les indices des tâches triés selon key (stable : égalité = ordre du fichier).
func order(in *Instance, key func(t int) float64) []int {
	idx := make([]int, len(in.Tasks))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(i, j int) bool { return key(idx[i]) < key(idx[j]) })
	return idx
}

func fileOrder(in *Instance) []int { return order(in, func(int) float64 { return 0 }) }

// meanSolo : durée seule moyenne d'une tâche sur les VM (sert à trier pour SPT / LPT).
func meanSolo(in *Instance, t int) float64 {
	s := 0.0
	for _, d := range in.Solo[t] {
		s += d
	}
	return s / float64(len(in.Solo[t]))
}

// listSchedule : prend les tâches dans l'ordre donné et place chacune sur la
// VM candidate qui minimise choose(t, v).
func listSchedule(in *Instance, ord []int, choose func(b *builder, t, v int) float64) Alloc {
	b := newBuilder(in)
	for _, t := range ord {
		v := argmin(b.candidates(t), func(v int) float64 { return choose(b, t, v) })
		b.place(t, v)
	}
	return b.a
}

// earliestFree : la VM la moins occupée pour l'instant (règle des CM3 pour FCFS/SPT/LPT).
func earliestFree(b *builder, t, v int) float64 { return b.busy[v] }

// completion : durée d'occupation de la VM après ajout de la tâche (MCT).
func completion(b *builder, t, v int) float64 { return b.busyIf(t, v) }

// minMinMaxMin : Min-Min (pickMax=false) ou Max-Min (pickMax=true), CM3.
// À chaque étape : pour chaque tâche restante, sa meilleure date de fin
// possible ; on place la tâche dont cette date est la plus petite (Min-Min)
// ou la plus grande (Max-Min), sur la VM qui la réalise.
func minMinMaxMin(in *Instance, pickMax bool) Alloc {
	b := newBuilder(in)
	left := map[int]bool{}
	for t := range in.Tasks {
		left[t] = true
	}
	for len(left) > 0 {
		bestT, bestV := -1, -1
		bestC := math.Inf(1)
		if pickMax {
			bestC = math.Inf(-1)
		}
		for _, t := range fileOrder(in) {
			if !left[t] {
				continue
			}
			v := argmin(b.candidates(t), func(v int) float64 { return b.busyIf(t, v) })
			c := b.busyIf(t, v)
			if (!pickMax && c < bestC-1e-12) || (pickMax && c > bestC+1e-12) {
				bestT, bestV, bestC = t, v, c
			}
		}
		b.place(bestT, bestV)
		delete(left, bestT)
	}
	return b.a
}

// fit : First Fit / Best Fit sur la RAM (placement, CM3). Les VM sont
// parcourues dans l'ordre de machines.csv. Si une tâche ne rentre nulle part,
// elle va sur la VM la moins remplie (allocation alors signalée invalide).
func fit(in *Instance, ord []int, best bool) Alloc {
	b := newBuilder(in)
	for _, t := range ord {
		v := -1
		for _, c := range b.candidates(t) {
			if !in.ramFits(b.ram, t, c) {
				continue
			}
			if !best {
				v = c
				break
			}
			left := RAMFraction*in.VMs[c].RAMMB - b.ram[c] - in.RAM[t]
			if v == -1 || left < RAMFraction*in.VMs[v].RAMMB-b.ram[v]-in.RAM[t]-1e-9 {
				v = c
			}
		}
		if v == -1 {
			v = argmin(b.candidates(t), func(v int) float64 { return b.ram[v] / in.VMs[v].RAMMB })
		}
		b.place(t, v)
	}
	return b.a
}

// greedy6040 : notre glouton (CM3 « filtrer → noter → assigner » + CM6).
//  1. Ordre LPT : les tâches les plus longues d'abord (elles fixent le makespan).
//  2. Filtre : VM où la RAM rentre ET dont la durée d'occupation prédite
//     resterait ≤ 1,25 × makespan du RR (contrainte de temps).
//  3. Score de chaque VM restante : le score 60/40 de l'allocation partielle
//     si on y place la tâche (coût et makespan normalisés par ceux du RR).
//  4. On garde la VM de plus petit score.
// Le filtre de temps empêche l'écueil d'un glouton naïf : tout empiler sur la
// VM la moins chère parce que la minute de facturation est déjà payée.
func greedy6040(in *Instance) Alloc {
	b := newBuilder(in)
	limit := MaxSlowdown * in.RefMakespan
	for _, t := range order(in, func(t int) float64 { return -meanSolo(in, t) }) {
		cands := b.candidates(t)
		var ok []int
		for _, v := range cands {
			if b.busyIf(t, v) <= limit+1e-9 {
				ok = append(ok, v)
			}
		}
		if len(ok) == 0 { // aucune VM ne tient le délai : on limite les dégâts
			b.place(t, argmin(cands, func(v int) float64 { return b.busyIf(t, v) }))
			continue
		}
		v := argmin(ok, func(v int) float64 {
			b.a[t] = v
			s := in.Evaluate(b.a).Score
			b.a[t] = -1
			return s
		})
		b.place(t, v)
	}
	return b.a
}

// Heuristics : tous les algorithmes constructifs, dans l'ordre d'affichage.
func Heuristics() []Heuristic {
	return []Heuristic{
		{"RR", "TD4", "tâche i -> VM i mod m, sans filtre (référence)", func(in *Instance) Alloc {
			a := make(Alloc, len(in.Tasks))
			for i := range a {
				a[i] = i % len(in.VMs)
			}
			return a
		}},
		{"FCFS", "CM3", "ordre du fichier, VM la moins occupée", func(in *Instance) Alloc {
			return listSchedule(in, fileOrder(in), earliestFree)
		}},
		{"SPT", "CM3", "plus courtes d'abord, VM la moins occupée", func(in *Instance) Alloc {
			return listSchedule(in, order(in, func(t int) float64 { return meanSolo(in, t) }), earliestFree)
		}},
		{"LPT", "CM3", "plus longues d'abord, VM la moins occupée", func(in *Instance) Alloc {
			return listSchedule(in, order(in, func(t int) float64 { return -meanSolo(in, t) }), earliestFree)
		}},
		{"MET", "CM3", "VM où la tâche seule est la plus rapide", func(in *Instance) Alloc {
			return listSchedule(in, fileOrder(in), func(b *builder, t, v int) float64 { return in.Solo[t][v] })
		}},
		{"MCT", "CM3", "ordre du fichier, VM qui finit le plus tôt avec la tâche", func(in *Instance) Alloc {
			return listSchedule(in, fileOrder(in), completion)
		}},
		{"Min-Min", "CM3", "tâche de plus petite fin possible d'abord", func(in *Instance) Alloc {
			return minMinMaxMin(in, false)
		}},
		{"Max-Min", "CM3", "tâche de plus grande fin possible d'abord", func(in *Instance) Alloc {
			return minMinMaxMin(in, true)
		}},
		{"FirstFit", "CM3", "1re VM où la RAM rentre (ordre du fichier)", func(in *Instance) Alloc {
			return fit(in, fileOrder(in), false)
		}},
		{"BestFit", "CM3", "VM qui laisse le moins de RAM libre", func(in *Instance) Alloc {
			return fit(in, fileOrder(in), true)
		}},
		{"FFD", "CM3", "First Fit, tâches triées par RAM décroissante", func(in *Instance) Alloc {
			return fit(in, order(in, func(t int) float64 { return -in.RAM[t] }), false)
		}},
		{"Glouton-60/40", "CM3+CM6", "LPT, filtre RAM + délai, VM de meilleur score 60/40", greedy6040},
	}
}
