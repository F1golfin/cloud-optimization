package algorithms

import (
	"math"
	"sort"

	"cloud-optimization/scheduler/model"
)

// OptimalMaxTasks : au-delà, la recherche exhaustive devient trop longue
// (nombre d'affectations = VM^tâches : 4^9 ≈ 262 000, 4^10 ≈ 1 million).
const OptimalMaxTasks = 10

// Optimal : la MEILLEURE allocation possible selon le modèle, par recherche
// exhaustive. Sert de point de comparaison pour juger select-maxmin sur un
// petit lot ; inutilisable sur un gros lot (renvoie nil au-delà de
// OptimalMaxTasks tâches).
//
// Ce qui est exploré :
//   - TOUTES les affectations tâche -> VM (VM^tâches possibilités) ;
//   - pour chaque VM et chaque ensemble de tâches, le MEILLEUR ordre
//     d'exécution : on essaie toutes les répartitions de ces tâches entre les
//     workers de la VM, et on rejoue chacune dans la file de l'orchestrateur
//     (tâches triées par heure de début). Le temps retenu est celui que
//     l'orchestrateur obtiendrait réellement avec cet ordre.
//
// Critère : même que select-maxmin — S le plus bas parmi les allocations
// admissibles (délai et RAM). À S égal : moins de VM, puis makespan plus court.
// Si aucune n'est admissible, on renvoie Round Robin.
func Optimal(in *model.Instance) model.Allocation {
	n, m := len(in.Tasks), len(in.Machines)
	if n > OptimalMaxTasks {
		return nil
	}
	ref := in.Evaluate(RoundRobin(in))

	// 1. Pour chaque VM et chaque sous-ensemble de tâches (masque de bits) :
	//    meilleur temps de fin, ordre correspondant, et respect de la RAM.
	full := 1 << n
	type vmPlan struct {
		time  float64
		order []int
		ramOK bool
	}
	plans := make([][]vmPlan, m)
	for j := 0; j < m; j++ {
		plans[j] = make([]vmPlan, full)
		plans[j][0] = vmPlan{ramOK: true}
		for mask := 1; mask < full; mask++ {
			var tasks []int
			maxRSS := 0
			for i := 0; i < n; i++ {
				if mask&(1<<i) != 0 {
					tasks = append(tasks, i)
					if in.RSS[i][j] > maxRSS {
						maxRSS = in.RSS[i][j]
					}
				}
			}
			t, order := bestOrder(in, j, tasks)
			mc := in.Machines[j]
			plans[j][mask] = vmPlan{t, order, mc.VCPU*maxRSS <= mc.RAMMB*1024}
		}
	}

	// 2. Toutes les affectations tâche -> VM, par récursion sur les tâches.
	masks := make([]int, m)
	bestMasks := make([]int, m)
	bestS, bestD, bestUsed := math.Inf(1), math.Inf(1), m+1
	found := false
	var rec func(i int)
	rec = func(i int) {
		if i == n {
			met := model.Metrics{RAMOK: true}
			used := 0
			for j, mask := range masks {
				if mask == 0 {
					continue
				}
				p := plans[j][mask]
				used++
				met.RAMOK = met.RAMOK && p.ramOK
				if p.time > met.Makespan {
					met.Makespan = p.time
				}
				met.Cost += math.Max(in.Params.MinBilledS, p.time) * in.Machines[j].PricePerHour / 3600
			}
			if !in.Feasible(met, ref) {
				return
			}
			s := in.Score(met, ref)
			better := s < bestS-1e-12 ||
				(math.Abs(s-bestS) <= 1e-12 && (used < bestUsed || (used == bestUsed && met.Makespan < bestD)))
			if better {
				bestS, bestD, bestUsed, found = s, met.Makespan, used, true
				copy(bestMasks, masks)
			}
			return
		}
		for j := 0; j < m; j++ {
			masks[j] |= 1 << i
			rec(i + 1)
			masks[j] &^= 1 << i
		}
	}
	rec(0)
	if !found {
		return RoundRobin(in)
	}

	var a model.Allocation
	for j, mask := range bestMasks {
		for _, i := range plans[j][mask].order {
			a = append(a, model.Assignment{Task: i, VM: j})
		}
	}
	return a
}

// bestOrder : meilleur ordre d'exécution des tâches sur la VM j.
//
// On essaie toutes les répartitions des tâches entre les workers (la première
// tâche est toujours sur le worker 0 : les workers sont interchangeables).
// Chaque répartition donne une heure de début par tâche ; on trie les tâches
// par heure de début et on rejoue cet ordre dans la file de la VM (Lanes, comme
// l'orchestrateur). Rejouer l'ordre d'un planning ne peut que faire démarrer
// les tâches plus tôt : la répartition optimale est donc bien retrouvée.
func bestOrder(in *model.Instance, j int, tasks []int) (float64, []int) {
	k, w := len(tasks), in.Machines[j].VCPU
	if w > k {
		w = k
	}
	lane := make([]int, k)
	start := make([]float64, k)
	load := make([]float64, w)
	idx := make([]int, k)
	bestT := math.Inf(1)
	var bestOrd []int
	for {
		for x := range load {
			load[x] = 0
		}
		for x, i := range tasks {
			start[x] = load[lane[x]]
			load[lane[x]] += in.P[i][j]
		}
		for x := range idx {
			idx[x] = x
		}
		sort.SliceStable(idx, func(a, b int) bool { return start[idx[a]] < start[idx[b]] })
		l := model.NewLanes(in.Machines[j].VCPU)
		for _, x := range idx {
			l.Add(in.P[tasks[x]][j])
		}
		if t := l.End(); t < bestT-1e-12 {
			bestT = t
			bestOrd = bestOrd[:0]
			for _, x := range idx {
				bestOrd = append(bestOrd, tasks[x])
			}
		}
		// répartition suivante (compteur en base w, la tâche 0 reste sur le worker 0)
		x := 1
		for x < k {
			lane[x]++
			if lane[x] < w {
				break
			}
			lane[x] = 0
			x++
		}
		if x >= k {
			break
		}
	}
	return bestT, bestOrd
}
