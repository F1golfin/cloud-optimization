package algorithms

import (
	"fmt"
	"sort"
	"strings"

	"cloud-optimization/scheduler/model"
)

// SelectMaxMin — notre algorithme : « choix des VM par type + Max-Min ».
//
//
// Méthode, en deux niveaux :
//  1. Choix du nombre de VM par type — EXACT : on essaie toutes les
//     combinaisons (0..n_t VM pour chaque type t, sauf « aucune VM »).
//     Ici : e2-standard-2 (0-1) x e2-medium (0-2) x e2-highcpu-4 (0-1)
//     = 2 x 3 x 2 - 1 = 11 combinaisons.
//  2. Répartition — HEURISTIQUE : sur chaque combinaison, Max-Min (le meilleur
//     des algorithmes classiques dans nos simulations).
//
// On garde la combinaison dont le score S est le plus bas parmi celles qui
// respectent le délai (D <= 1,25 x D_RR) et la RAM. Aucun nom de VM, type,
// prix ou durée n'est écrit ici : tout vient des fichiers d'entrée.
//
// Garanties :
//   - la combinaison « toutes les VM » est essayée, donc le résultat est au
//     moins aussi bon que Max-Min seul (dès que Max-Min est admissible) ;
//   - si aucune combinaison n'est admissible, on renvoie Round Robin, qui
//     l'est par définition (D = D_RR).
//
// Passage à l'échelle : le nombre de combinaisons est Π (n_t + 1) - 1. Il
// dépend du nombre de TYPES, pas du nombre de VM : 3 types avec 10 VM chacun
// = 1 330 combinaisons, contre ~10^9 si l'on traitait chaque VM séparément.
func SelectMaxMin(in *model.Instance) model.Allocation {
	results := EvaluateCombinations(in)
	if len(results) > 0 && results[0].Feasible {
		return results[0].Alloc
	}
	return RoundRobin(in)
}

// TypeGroup : les VM d'un même type de machine, dans l'ordre de machines.csv.
type TypeGroup struct {
	Type string
	VMs  []int
}

// GroupByType regroupe les VM par type (ordre de première apparition).
func GroupByType(in *model.Instance) []TypeGroup {
	var groups []TypeGroup
	index := map[string]int{}
	for j, m := range in.Machines {
		k, ok := index[m.Type]
		if !ok {
			k = len(groups)
			index[m.Type] = k
			groups = append(groups, TypeGroup{Type: m.Type})
		}
		groups[k].VMs = append(groups[k].VMs, j)
	}
	return groups
}

// ComboResult : le résultat de Max-Min pour un nombre donné de VM de chaque type.
type ComboResult struct {
	Counts   []int // Counts[t] = nombre de VM allumées du type t (ordre de GroupByType)
	VMs      []int // indices des VM effectivement allumées
	Alloc    model.Allocation
	Metrics  model.Metrics
	Score    float64
	Feasible bool
}

// Label : description lisible, ex. « 0 x e2-standard-2 + 2 x e2-medium + 1 x e2-highcpu-4 ».
// On affiche les VM réellement UTILISÉES : Max-Min peut laisser vide une VM
// autorisée (par ex. 2 tâches seulement), qui ne sera alors pas allumée.
func (r ComboResult) Label(groups []TypeGroup) string {
	parts := make([]string, len(groups))
	for t, g := range groups {
		used := 0
		for _, j := range g.VMs {
			if r.Metrics.PerVM[j].Tasks > 0 {
				used++
			}
		}
		parts[t] = fmt.Sprintf("%d x %s", used, g.Type)
	}
	return strings.Join(parts, " + ")
}

// EvaluateCombinations essaie toutes les combinaisons « nombre de VM par type »
// et les renvoie triées : admissibles d'abord, puis par score S croissant (à
// égalité : moins de VM). Le premier élément est donc le choix de SelectMaxMin.
// Utilisé aussi par « scheduler -explain » pour afficher le tableau complet.
func EvaluateCombinations(in *model.Instance) []ComboResult {
	ref := in.Evaluate(RoundRobin(in))
	groups := GroupByType(in)
	counts := make([]int, len(groups))

	var results []ComboResult
	for {
		// Combinaison suivante, comme un compteur kilométrique : chaque
		// « chiffre » t va de 0 à len(groups[t].VMs). On part de « 0 partout »,
		// qui n'est jamais évalué (il faut au moins une VM).
		t := 0
		for t < len(groups) {
			counts[t]++
			if counts[t] <= len(groups[t].VMs) {
				break
			}
			counts[t] = 0
			t++
		}
		if t == len(groups) {
			break // toutes les combinaisons ont été vues
		}

		// Les VM d'un même type sont interchangeables : on allume les
		// counts[t] premières de ce type (ordre de machines.csv).
		var vms []int
		for t, g := range groups {
			vms = append(vms, g.VMs[:counts[t]]...)
		}
		sort.Ints(vms)

		alloc := minMax(in, vms, true) // Max-Min restreint à ces VM
		met := in.Evaluate(alloc)
		results = append(results, ComboResult{
			Counts:   append([]int(nil), counts...),
			VMs:      vms,
			Alloc:    alloc,
			Metrics:  met,
			Score:    in.Score(met, ref),
			Feasible: in.Feasible(met, ref),
		})
	}

	sort.SliceStable(results, func(a, b int) bool {
		ra, rb := results[a], results[b]
		if ra.Feasible != rb.Feasible {
			return ra.Feasible
		}
		if ra.Score != rb.Score {
			return ra.Score < rb.Score
		}
		return ra.Metrics.UsedVMs < rb.Metrics.UsedVMs
	})
	return results
}
