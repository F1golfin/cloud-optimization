package algorithms

import (
	"math"
	"math/rand"
	"strconv"
	"testing"

	"cloud-optimization/scheduler/model"
)

// toy : 1 worker par VM, mêmes prix ; p[i][j] = durée de la tâche i sur la VM j.
func toy(p [][]float64, nVM int) *model.Instance {
	in := &model.Instance{Params: model.DefaultParams(), P: p}
	for j := 0; j < nVM; j++ {
		in.Machines = append(in.Machines, model.Machine{Name: "vm" + strconv.Itoa(j), VCPU: 1, RAMMB: 4096, PricePerHour: 0.04, Contention: 1})
	}
	in.RSS = make([][]int, len(p))
	for i := range p {
		in.Tasks = append(in.Tasks, model.Task{ID: "T" + strconv.Itoa(i), Workload: "cpu_task", Size: 1})
		in.RSS[i] = make([]int, nVM)
	}
	return in
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// Chaque algorithme doit produire une allocation valide (chaque tâche une
// seule fois, sur une VM existante) sur un gros lot aléatoire hétérogène.
func TestAllAlgorithmsValid(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	p := make([][]float64, 300)
	for i := range p {
		p[i] = []float64{rng.Float64() * 2, rng.Float64() * 2, rng.Float64() * 2, rng.Float64() * 2}
	}
	in := toy(p, 4)
	in.Machines[3].VCPU = 4
	for _, a := range All {
		if err := in.Validate(a.Run(in)); err != nil {
			t.Errorf("%s : %v", a.Name, err)
		}
	}
}

func TestRoundRobinCycles(t *testing.T) {
	in := toy(make([][]float64, 7), 3)
	for i := range in.P {
		in.P[i] = []float64{1, 1, 1}
	}
	for k, x := range RoundRobin(in) {
		if x.VM != k%3 {
			t.Fatalf("tâche %d sur VM %d, attendu %d", k, x.VM, k%3)
		}
	}
}

func TestMETPicksFastestVM(t *testing.T) {
	// La VM 1 est la plus rapide pour toutes les tâches : MET y met tout.
	in := toy([][]float64{{2, 1}, {3, 2}, {5, 4}}, 2)
	for _, x := range MET(in) {
		if x.VM != 1 {
			t.Fatalf("MET a placé la tâche %d sur la VM %d", x.Task, x.VM)
		}
	}
}

func TestMCTBalancesLoad(t *testing.T) {
	// Durées 3, 3, 2 sur 2 VM identiques : 3 -> vm0, 3 -> vm1, 2 -> vm0 (égalité -> 1re VM). D = 5.
	in := toy([][]float64{{3, 3}, {3, 3}, {2, 2}}, 2)
	m := in.Evaluate(MCT(in))
	if !near(m.Makespan, 5) {
		t.Fatalf("MCT : D = %v, attendu 5", m.Makespan)
	}
}

func TestMinMinVersusMaxMin(t *testing.T) {
	// Tâches 5, 1, 1 sur 2 VM identiques.
	// Min-Min place les petites d'abord : 1 -> vm0, 1 -> vm1, puis 5 -> vm0 : D = 6.
	// Max-Min place la grosse d'abord : 5 -> vm0, 1 -> vm1, 1 -> vm1 : D = 5.
	in := toy([][]float64{{5, 5}, {1, 1}, {1, 1}}, 2)
	if d := in.Evaluate(MinMin(in)).Makespan; !near(d, 6) {
		t.Fatalf("Min-Min : D = %v, attendu 6", d)
	}
	if d := in.Evaluate(MaxMin(in)).Makespan; !near(d, 5) {
		t.Fatalf("Max-Min : D = %v, attendu 5", d)
	}
}
