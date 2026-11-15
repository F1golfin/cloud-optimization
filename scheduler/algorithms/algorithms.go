// Package algorithms regroupe les heuristiques de placement. Chacune reçoit
// une instance (tâches, VM, durées prédites) et renvoie une allocation
// ORDONNÉE ; toutes sont évaluées ensuite avec le même modèle (package model).
//
// Ce sont les heuristiques classiques de « list scheduling » sur machines
// hétérogènes (cours d'ordonnancement) :
//
//	RR      affectation cyclique, sans regarder les durées      (référence)
//	MET     Minimum Execution Time : la VM où la tâche seule est la plus rapide
//	MCT     Minimum Completion Time : la VM où la tâche finirait le plus tôt
//	Min-Min à chaque tour, place la tâche qui peut finir le plus tôt
//	Max-Min à chaque tour, place la tâche dont la meilleure fin est la plus tardive
package algorithms

import "cloud-optimization/scheduler/model"

type Algo struct {
	Name        string
	Description string
	Run         func(in *model.Instance) model.Allocation
}

// All : ordre d'affichage dans le mode compare. RR en premier : c'est la référence.
var All = []Algo{
	{"rr", "Round Robin (référence)", RoundRobin},
	{"met", "Minimum Execution Time", MET},
	{"mct", "Minimum Completion Time", MCT},
	{"minmin", "Min-Min", MinMin},
	{"maxmin", "Max-Min", MaxMin},
}

func Get(name string) (Algo, bool) {
	for _, a := range All {
		if a.Name == name {
			return a, true
		}
	}
	return Algo{}, false
}

// newLanes : l'état « heure de fin de chaque worker » de chaque VM, vide au départ.
func newLanes(in *model.Instance) []model.Lanes {
	l := make([]model.Lanes, len(in.Machines))
	for j, m := range in.Machines {
		l[j] = model.NewLanes(m.VCPU)
	}
	return l
}

// bestCompletion : la VM sur laquelle la tâche i finirait le plus tôt, et à quelle heure.
// En cas d'égalité, la première VM de machines.csv l'emporte (résultat déterministe).
func bestCompletion(in *model.Instance, lanes []model.Lanes, i int) (vm int, finish float64) {
	vm = -1
	for j := range in.Machines {
		f := lanes[j].FinishIfAdded(in.P[i][j])
		if vm < 0 || f < finish {
			vm, finish = j, f
		}
	}
	return vm, finish
}
