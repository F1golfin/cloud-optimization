package main

// Amélioration d'allocations complètes (CM5) : recherche locale par
// déplacements et échanges, multi-départ, recuit simulé.
// Tout se passe DANS LE CALCUL, avant l'exécution : on ne déplace jamais une
// tâche déjà lancée sur une VM.

import (
	"math"
	"math/rand"
)

// localSearch : descente « meilleure amélioration ».
// Voisinage :
//   - déplacement : une tâche change de VM ;
//   - échange : deux tâches de VM différentes échangent leurs VM.
// On applique le meilleur voisin tant qu'il améliore l'objectif (score +
// pénalités) ; on s'arrête sur un optimum local.
func localSearch(in *Instance, start Alloc) (Alloc, float64) {
	cur := start.clone()
	curF := in.objective(in.Evaluate(cur))
	n, m := len(in.Tasks), len(in.VMs)
	for {
		bestF := curF
		var best Alloc
		try := func(a Alloc) {
			if f := in.objective(in.Evaluate(a)); f < bestF-1e-12 {
				bestF, best = f, a.clone()
			}
		}
		for t := 0; t < n; t++ { // déplacements
			old := cur[t]
			for v := 0; v < m; v++ {
				if v != old {
					cur[t] = v
					try(cur)
				}
			}
			cur[t] = old
		}
		for t1 := 0; t1 < n; t1++ { // échanges
			for t2 := t1 + 1; t2 < n; t2++ {
				if cur[t1] != cur[t2] {
					cur[t1], cur[t2] = cur[t2], cur[t1]
					try(cur)
					cur[t1], cur[t2] = cur[t2], cur[t1]
				}
			}
		}
		if best == nil {
			return cur, curF
		}
		cur, curF = best, bestF
	}
}

// simulatedAnnealing (CM5) : part d'une allocation, tire au hasard un
// déplacement ou un échange ; accepte toujours une amélioration, et une
// dégradation Δ avec la probabilité exp(-Δ/T). T décroît géométriquement.
// Graine fixe => résultat reproductible. On termine par une recherche locale.
func simulatedAnnealing(in *Instance, start Alloc, iters int, seed int64) (Alloc, float64) {
	rng := rand.New(rand.NewSource(seed))
	n, m := len(in.Tasks), len(in.VMs)
	cur := start.clone()
	curF := in.objective(in.Evaluate(cur))
	best, bestF := cur.clone(), curF
	t0, t1 := 0.05, 0.0005 // températures en « unités de score »
	for i := 0; i < iters; i++ {
		temp := t0 * math.Pow(t1/t0, float64(i)/float64(iters))
		next := cur.clone()
		if rng.Intn(2) == 0 || n < 2 {
			next[rng.Intn(n)] = rng.Intn(m)
		} else {
			a, b := rng.Intn(n), rng.Intn(n)
			next[a], next[b] = next[b], next[a]
		}
		f := in.objective(in.Evaluate(next))
		if f <= curF || rng.Float64() < math.Exp(-(f-curF)/temp) {
			cur, curF = next, f
			if f < bestF-1e-12 {
				best, bestF = next.clone(), f
			}
		}
	}
	return localSearch(in, best)
}
