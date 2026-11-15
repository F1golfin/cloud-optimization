package algorithms

import "cloud-optimization/scheduler/model"

// MET (Minimum Execution Time) : chaque tâche va sur la VM où SA durée est
// la plus courte, sans regarder ce qui est déjà placé sur cette VM.
//
// Défaut attendu : si une VM est (même très légèrement) la plus rapide pour
// la plupart des tâches, tout s'y empile et le makespan explose.
func MET(in *model.Instance) model.Allocation {
	a := make(model.Allocation, len(in.Tasks))
	for i := range in.Tasks {
		best := 0
		for j := 1; j < len(in.Machines); j++ {
			if in.P[i][j] < in.P[i][best] {
				best = j
			}
		}
		a[i] = model.Assignment{Task: i, VM: best}
	}
	return a
}
