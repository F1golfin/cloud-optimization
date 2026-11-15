package algorithms

import "cloud-optimization/scheduler/model"

// MCT (Minimum Completion Time) : on prend les tâches dans l'ordre du fichier ;
// chacune va sur la VM où elle FINIRAIT le plus tôt, compte tenu des tâches
// déjà placées (charge de la VM + durée de la tâche sur cette VM).
//
// Équilibre bien la charge, mais ignore le prix des VM.
func MCT(in *model.Instance) model.Allocation {
	lanes := newLanes(in)
	vms := allVMs(in)
	a := make(model.Allocation, 0, len(in.Tasks))
	for i := range in.Tasks {
		j, _ := bestCompletion(in, lanes, i, vms)
		lanes[j].Add(in.P[i][j])
		a = append(a, model.Assignment{Task: i, VM: j})
	}
	return a
}
