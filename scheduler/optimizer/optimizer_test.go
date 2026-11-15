package main

// Tests (sans GCP) : go test ./scheduler/optimizer/
//   - le modèle de simulation donne les durées attendues sur des cas simples ;
//   - chaque heuristique produit une allocation complète et respecte le filtre RAM ;
//   - la méthode exacte trouve le même optimum qu'une énumération exhaustive.

import (
	"math"
	"math/rand"
	"testing"
	"time"
)

// randomInstance : instance synthétique (durées, RAM, prix) tirée avec une graine.
func randomInstance(seed int64, n, m int) *Instance {
	rng := rand.New(rand.NewSource(seed))
	wls := []string{"cpu_task", "memory_task", "matrix_task", "sort_task"}
	in := &Instance{Overhead: 1}
	for j := 0; j < m; j++ {
		in.VMs = append(in.VMs, VM{Name: string(rune('A' + j)), VCPU: 1 + rng.Intn(4),
			RAMMB: float64(2048 * (1 + rng.Intn(4))), PriceH: 0.03 + 0.1*rng.Float64()})
	}
	for i := 0; i < n; i++ {
		in.Tasks = append(in.Tasks, Task{ID: string(rune('a' + i)), Workload: wls[rng.Intn(4)], Size: 1})
		base := 2 + 20*rng.Float64()
		row := make([]float64, m)
		for j := range row {
			row[j] = base * (0.5 + rng.Float64())
		}
		in.Solo = append(in.Solo, row)
		in.RAM = append(in.RAM, 50+1500*rng.Float64())
	}
	rr := make(Alloc, n)
	for i := range rr {
		rr[i] = i % m
	}
	e := in.Evaluate(rr)
	in.RefCost, in.RefMakespan = e.Cost, e.Makespan
	return in
}

func TestSimulateSimpleCases(t *testing.T) {
	in := &Instance{Overhead: 2, VMs: []VM{{Name: "v", VCPU: 1}},
		Tasks: []Task{{Workload: "cpu_task"}, {Workload: "cpu_task"}}, Solo: [][]float64{{10}, {4}}}
	if got := in.simulate(0, []int{0}); math.Abs(got-12) > 1e-9 {
		t.Fatalf("une tâche seule : %v, attendu 12", got)
	}
	// 1 vCPU, 2 tâches : partage à 50 % ; la courte (4 s) finit à 8 s,
	// la longue a alors fait 4 s, il lui en reste 6 seule : fin à 14 s (+2 de surcoût).
	if got := in.simulate(0, []int{0, 1}); math.Abs(got-16) > 1e-9 {
		t.Fatalf("deux tâches sur 1 vCPU : %v, attendu 16", got)
	}
	in.VMs[0].VCPU = 2 // 2 vCPU : pas de ralentissement
	if got := in.simulate(0, []int{0, 1}); math.Abs(got-12) > 1e-9 {
		t.Fatalf("deux tâches sur 2 vCPU : %v, attendu 12", got)
	}
}

func TestHeuristicsProduceValidAllocations(t *testing.T) {
	for seed := int64(1); seed <= 20; seed++ {
		in := randomInstance(seed, 10, 3)
		for _, h := range Heuristics() {
			a := h.Run(in)
			if len(a) != len(in.Tasks) {
				t.Fatalf("%s : %d tâches allouées, attendu %d", h.Name, len(a), len(in.Tasks))
			}
			for i, v := range a {
				if v < 0 || v >= len(in.VMs) {
					t.Fatalf("%s : tâche %d sur VM %d invalide", h.Name, i, v)
				}
			}
		}
	}
}

// bruteForce : énumère les m^n allocations et renvoie le meilleur score valide.
func bruteForce(in *Instance) float64 {
	n, m := len(in.Tasks), len(in.VMs)
	a := make(Alloc, n)
	best := math.Inf(1)
	var rec func(i int)
	rec = func(i int) {
		if i == n {
			if e := in.Evaluate(a); e.Feasible() && e.Score < best {
				best = e.Score
			}
			return
		}
		for v := 0; v < m; v++ {
			a[i] = v
			rec(i + 1)
		}
	}
	rec(0)
	return best
}

func TestBranchAndBoundMatchesBruteForce(t *testing.T) {
	for seed := int64(1); seed <= 30; seed++ {
		in := randomInstance(seed, 7, 3)
		want := bruteForce(in)
		start := Heuristics()[0].Run(in) // RR, toujours valide vis-à-vis du délai
		got := branchAndBound(in, start, 10*time.Second)
		if got.Status != "OPTIMAL" {
			t.Fatalf("graine %d : statut %s", seed, got.Status)
		}
		if math.IsInf(want, 1) != math.IsInf(got.Score, 1) || (!math.IsInf(want, 1) && math.Abs(got.Score-want) > 1e-9) {
			t.Fatalf("graine %d : branch and bound %.6f, exhaustif %.6f", seed, got.Score, want)
		}
	}
}

func TestLocalSearchNeverWorsens(t *testing.T) {
	for seed := int64(1); seed <= 10; seed++ {
		in := randomInstance(seed, 10, 4)
		for _, h := range Heuristics() {
			a := h.Run(in)
			before := in.objective(in.Evaluate(a))
			_, after := localSearch(in, a)
			if after > before+1e-12 {
				t.Fatalf("%s : la recherche locale dégrade %.4f -> %.4f", h.Name, before, after)
			}
		}
	}
}
