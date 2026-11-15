package algorithms

import "cloud-optimization/scheduler/model"

// MinMin : à chaque tour, on calcule pour chaque tâche restante sa meilleure
// heure de fin possible (sur sa meilleure VM), puis on place la tâche dont
// cette heure est la PLUS PETITE. Les petites tâches passent d'abord.
func MinMin(in *model.Instance) model.Allocation { return minMax(in, false) }

// MaxMin : même principe, mais on place la tâche dont la meilleure heure de
// fin est la PLUS GRANDE. Les grosses tâches passent d'abord, ce qui évite
// qu'une grosse tâche placée en dernier allonge le lot.
func MaxMin(in *model.Instance) model.Allocation { return minMax(in, true) }

// minMax : coût O(n² · m) — environ 16 millions d'évaluations pour 2 000
// tâches et 4 VM, soit bien moins d'une seconde.
func minMax(in *model.Instance, pickLargest bool) model.Allocation {
	lanes := newLanes(in)
	remaining := make([]int, len(in.Tasks))
	for i := range remaining {
		remaining[i] = i
	}
	a := make(model.Allocation, 0, len(in.Tasks))

	for len(remaining) > 0 {
		chosen, chosenVM := -1, -1
		chosenFinish := 0.0
		for k, i := range remaining {
			j, f := bestCompletion(in, lanes, i)
			better := chosen < 0 ||
				(!pickLargest && f < chosenFinish) ||
				(pickLargest && f > chosenFinish)
			if better {
				chosen, chosenVM, chosenFinish = k, j, f
			}
		}
		i := remaining[chosen]
		lanes[chosenVM].Add(in.P[i][chosenVM])
		a = append(a, model.Assignment{Task: i, VM: chosenVM})
		// suppression en conservant l'ordre : le résultat reste déterministe
		remaining = append(remaining[:chosen], remaining[chosen+1:]...)
	}
	return a
}
