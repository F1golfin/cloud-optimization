package algorithms

import "cloud-optimization/scheduler/model"

// RoundRobin : tâche 1 -> VM 1, tâche 2 -> VM 2, ... puis on recommence.
// Ignore les durées, les prix et la charge : c'est la référence (S = 1).
func RoundRobin(in *model.Instance) model.Allocation {
	a := make(model.Allocation, len(in.Tasks))
	for i := range in.Tasks {
		a[i] = model.Assignment{Task: i, VM: i % len(in.Machines)}
	}
	return a
}
