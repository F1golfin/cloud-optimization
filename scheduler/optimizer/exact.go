package main

// Méthode exacte (CM4) : séparation et évaluation (branch and bound) sur le
// MÊME modèle que les heuristiques.
//
//   Brancher : les tâches sont placées une par une (les plus longues d'abord) ;
//              chaque nœud essaie chaque VM pour la tâche suivante.
//   Déduire  : une VM dont la RAM déborderait est écartée (contrainte H5).
//   Borner   : borne inférieure du score de tout ce qui reste à explorer sous
//              ce nœud ; si elle n'est pas meilleure que la meilleure solution
//              connue (la meilleure des heuristiques au départ), on coupe.
//
// Borne inférieure (valide car ajouter une tâche à une VM n'accélère jamais
// les autres, le ralentissement étant croissant avec k) :
//   coût     ≥ coût des VM déjà utilisées (chacune facturée ≥ 60 s) ;
//   makespan ≥ max(occupation des VM déjà remplies,
//                  plus longue durée seule possible d'une tâche restante).
// Si la recherche se termine : la solution est OPTIMALE pour le modèle.
// Si la limite de temps est atteinte : on a une solution et un écart.

import (
	"math"
	"time"
)

type ExactResult struct {
	Alloc    Alloc
	Score    float64
	Status   string  // OPTIMAL ou FEASIBLE (limite de temps)
	Bound    float64 // borne inférieure globale du score (racine)
	Nodes    int64
	Elapsed  time.Duration
	Improved bool // a trouvé mieux que la solution de départ
}

func branchAndBound(in *Instance, incumbent Alloc, limit time.Duration) ExactResult {
	n, m := len(in.Tasks), len(in.VMs)
	ord := order(in, func(t int) float64 { return -meanSolo(in, t) })

	// Pour la borne : plus longue « meilleure durée seule » parmi les tâches restantes à partir de la position i.
	minSolo := make([]float64, n)
	for t := range in.Tasks {
		minSolo[t] = math.Inf(1)
		for v := range in.VMs {
			minSolo[t] = math.Min(minSolo[t], in.Overhead+in.Solo[t][v])
		}
	}
	tailMax := make([]float64, n+1)
	for i := n - 1; i >= 0; i-- {
		tailMax[i] = math.Max(tailMax[i+1], minSolo[ord[i]])
	}
	cheapest := math.Inf(1)
	for _, v := range in.VMs {
		cheapest = math.Min(cheapest, v.PriceH/3600*MinBillS)
	}

	res := ExactResult{Status: "OPTIMAL"}
	best := incumbent.clone()
	bestE := in.Evaluate(best)
	bestS := math.Inf(1)
	if bestE.Feasible() {
		bestS = bestE.Score
	}
	startS := bestS

	a := make(Alloc, n)
	for i := range a {
		a[i] = -1
	}
	group := make([][]int, m)
	busy := make([]float64, m)
	ram := make([]float64, m)
	deadline := MaxSlowdown * in.RefMakespan
	start := time.Now()
	stopped := false

	lowerBound := func(pos int) float64 {
		cost, mk, used := 0.0, tailMax[pos], 0
		for v := 0; v < m; v++ {
			if len(group[v]) > 0 {
				used++
				cost += in.VMs[v].PriceH / 3600 * math.Max(MinBillS, busy[v])
				mk = math.Max(mk, busy[v])
			}
		}
		if used == 0 {
			cost = cheapest
		}
		if mk > deadline+1e-9 {
			return math.Inf(1) // le délai ne peut plus être tenu
		}
		return in.score(cost, mk)
	}
	res.Bound = lowerBound(0)

	var rec func(pos int)
	rec = func(pos int) {
		if stopped {
			return
		}
		res.Nodes++
		if res.Nodes&1023 == 0 && time.Since(start) > limit {
			stopped = true
			return
		}
		if pos == n {
			if e := in.Evaluate(a); e.Feasible() && e.Score < bestS-1e-12 {
				bestS, best = e.Score, a.clone()
			}
			return
		}
		t := ord[pos]
		type child struct {
			v  int
			lb float64
		}
		var kids []child
		for v := 0; v < m; v++ {
			if !in.ramFits(ram, t, v) {
				continue
			}
			oldBusy := busy[v]
			group[v] = append(group[v], t)
			busy[v] = in.simulate(v, group[v])
			lb := lowerBound(pos + 1)
			group[v] = group[v][:len(group[v])-1]
			busy[v] = oldBusy
			if lb < bestS-1e-12 {
				kids = append(kids, child{v, lb})
			}
		}
		// Explorer d'abord les branches les plus prometteuses.
		for i := 1; i < len(kids); i++ {
			for j := i; j > 0 && kids[j].lb < kids[j-1].lb; j-- {
				kids[j], kids[j-1] = kids[j-1], kids[j]
			}
		}
		for _, k := range kids {
			if k.lb >= bestS-1e-12 { // la meilleure solution a pu s'améliorer entre-temps
				continue
			}
			v := k.v
			oldBusy := busy[v]
			a[t] = v
			group[v] = append(group[v], t)
			ram[v] += in.RAM[t]
			busy[v] = in.simulate(v, group[v])
			rec(pos + 1)
			busy[v] = oldBusy
			ram[v] -= in.RAM[t]
			group[v] = group[v][:len(group[v])-1]
			a[t] = -1
		}
	}
	rec(0)

	if stopped {
		res.Status = "FEASIBLE (limite de temps)"
	}
	res.Alloc, res.Score = best, bestS
	res.Elapsed = time.Since(start)
	res.Improved = bestS < startS-1e-12
	return res
}
