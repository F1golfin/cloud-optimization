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

func TestCombinationCount(t *testing.T) {
	// Types A, B, B, C (comme nos VM : 1 e2-standard-2, 2 e2-medium, 1 e2-highcpu-4)
	// -> (1+1) x (2+1) x (1+1) - 1 = 11 combinaisons.
	in := toy([][]float64{{1, 1, 1, 1}}, 4)
	for j, typ := range []string{"A", "B", "B", "C"} {
		in.Machines[j].Type = typ
	}
	if g := GroupByType(in); len(g) != 3 || len(g[1].VMs) != 2 {
		t.Fatalf("regroupement par type incorrect : %+v", g)
	}
	if n := len(EvaluateCombinations(in)); n != 11 {
		t.Fatalf("%d combinaisons, attendu 11", n)
	}
}

func TestSelectTurnsOffUselessVM(t *testing.T) {
	// 2 tâches de 1 s, 2 VM identiques à 2 workers. RR en met une sur chaque VM
	// (D = 1 s, 2 VM payées 60 s). Une seule VM fait aussi D = 1 s pour moitié
	// prix : select-maxmin doit n'allumer qu'une VM.
	in := toy([][]float64{{1, 1}, {1, 1}}, 2)
	for j := range in.Machines {
		in.Machines[j].Type, in.Machines[j].VCPU = "e2-medium", 2
	}
	ref := in.Evaluate(RoundRobin(in))
	m := in.Evaluate(SelectMaxMin(in))
	if m.UsedVMs != 1 || !near(m.Makespan, 1) {
		t.Fatalf("VM allumées = %d, D = %v ; attendu 1 VM et D = 1", m.UsedVMs, m.Makespan)
	}
	if s := in.Score(m, ref); !near(s, 0.7) { // 0,6 x 1/2 + 0,4 x 1 = 0,7
		t.Fatalf("S = %v, attendu 0,7", s)
	}
}

func TestSelectNeverWorseThanMaxMin(t *testing.T) {
	// select-maxmin essaie aussi « toutes les VM avec Max-Min » : il ne peut
	// pas faire moins bien que Max-Min seul quand celui-ci est admissible.
	rng := rand.New(rand.NewSource(7))
	p := make([][]float64, 400)
	for i := range p {
		base := 0.1 + rng.Float64()*1.5
		p[i] = []float64{base * 1.05, base, base, base * 0.98}
	}
	in := toy(p, 4)
	for j, typ := range []string{"e2-standard-2", "e2-medium", "e2-medium", "e2-highcpu-4"} {
		in.Machines[j].Type, in.Machines[j].VCPU = typ, 2
	}
	in.Machines[0].PricePerHour, in.Machines[3].PricePerHour, in.Machines[3].VCPU = 0.0737, 0.1088, 4
	ref := in.Evaluate(RoundRobin(in))
	mm := in.Evaluate(MaxMin(in))
	sel := in.Evaluate(SelectMaxMin(in))
	if !in.Feasible(sel, ref) {
		t.Fatal("select-maxmin a renvoyé une solution non admissible")
	}
	if in.Feasible(mm, ref) && in.Score(sel, ref) > in.Score(mm, ref)+1e-12 {
		t.Fatalf("select-maxmin (S=%.4f) pire que Max-Min (S=%.4f)", in.Score(sel, ref), in.Score(mm, ref))
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

// permutations : toutes les permutations de xs (petits ensembles uniquement).
func permutations(xs []int) [][]int {
	if len(xs) <= 1 {
		return [][]int{append([]int(nil), xs...)}
	}
	var out [][]int
	for k := range xs {
		rest := append(append([]int(nil), xs[:k]...), xs[k+1:]...)
		for _, p := range permutations(rest) {
			out = append(out, append([]int{xs[k]}, p...))
		}
	}
	return out
}

// Optimal doit trouver le même S que la force brute la plus naïve : toutes les
// affectations ET tous les ordres possibles sur chaque VM.
func TestOptimalMatchesNaiveBruteForce(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for trial := 0; trial < 20; trial++ {
		n := 5
		p := make([][]float64, n)
		for i := range p {
			p[i] = []float64{0.2 + rng.Float64()*2, 0.2 + rng.Float64()*2, 0.2 + rng.Float64()*2}
		}
		in := toy(p, 3)
		in.Machines[1].VCPU, in.Machines[2].VCPU = 2, 3
		in.Machines[1].PricePerHour, in.Machines[2].PricePerHour = 0.07, 0.11
		in.Params.MinBilledS = 1 // pour que la durée compte dans le coût
		ref := in.Evaluate(RoundRobin(in))

		best := math.Inf(1)
		assign := make([]int, n)
		for {
			// tous les ordres : une permutation globale suffit (l'ordre relatif
			// sur chaque VM est celui de la permutation)
			for _, perm := range permutations([]int{0, 1, 2, 3, 4}) {
				var a model.Allocation
				for _, i := range perm {
					a = append(a, model.Assignment{Task: i, VM: assign[i]})
				}
				met := in.Evaluate(a)
				if in.Feasible(met, ref) {
					best = math.Min(best, in.Score(met, ref))
				}
			}
			k := 0
			for k < n {
				assign[k]++
				if assign[k] < 3 {
					break
				}
				assign[k] = 0
				k++
			}
			if k == n {
				break
			}
		}

		a := Optimal(in)
		if err := in.Validate(a); err != nil {
			t.Fatal(err)
		}
		got := in.Score(in.Evaluate(a), ref)
		if !near(got, best) {
			t.Fatalf("essai %d : Optimal S = %.9f, force brute S = %.9f", trial, got, best)
		}
	}
}

// Aucun algorithme ne doit battre l'optimum (même modèle, même critère).
func TestOptimalNeverWorse(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	p := make([][]float64, 8)
	for i := range p {
		p[i] = []float64{0.1 + rng.Float64(), 0.1 + rng.Float64(), 0.1 + rng.Float64(), 0.1 + rng.Float64()}
	}
	in := toy(p, 4)
	in.Machines[1].VCPU, in.Machines[3].VCPU = 2, 4
	in.Params.MinBilledS = 0.5
	ref := in.Evaluate(RoundRobin(in))
	opt := in.Score(in.Evaluate(Optimal(in)), ref)
	for _, a := range All {
		met := in.Evaluate(a.Run(in))
		if in.Feasible(met, ref) && in.Score(met, ref) < opt-1e-9 {
			t.Errorf("%s (S = %.6f) bat l'optimum (S = %.6f)", a.Name, in.Score(met, ref), opt)
		}
	}
}

func TestOptimalRefusesLargeLots(t *testing.T) {
	p := make([][]float64, OptimalMaxTasks+1)
	for i := range p {
		p[i] = []float64{1, 1}
	}
	if Optimal(toy(p, 2)) != nil {
		t.Fatal("Optimal doit refuser un lot de plus de OptimalMaxTasks tâches")
	}
}
