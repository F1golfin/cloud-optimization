package model

import (
	"encoding/csv"
	"math"
	"os"
	"strconv"
	"testing"
)

// toy : instance construite à la main, sans fichier, pour tester le modèle seul.
// p[i][j] = durée de la tâche i sur la VM j.
func toy(p [][]float64, machines ...Machine) *Instance {
	in := &Instance{Params: DefaultParams(), Machines: machines, P: p}
	in.RSS = make([][]int, len(p))
	for i := range p {
		in.Tasks = append(in.Tasks, Task{ID: "T" + strconv.Itoa(i), Workload: "cpu_task", Size: 1})
		in.RSS[i] = make([]int, len(machines))
	}
	return in
}

func vm(name string, workers int, pricePerHour float64) Machine {
	return Machine{Name: name, VCPU: workers, RAMMB: 4096, PricePerHour: pricePerHour, Contention: 1}
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestLanes(t *testing.T) {
	// 4 tâches de 1 s sur 2 workers : 2 vagues de 2 tâches -> fin à 2 s.
	l := NewLanes(2)
	for k := 0; k < 4; k++ {
		l.Add(1)
	}
	if !near(l.End(), 2) {
		t.Fatalf("fin = %v, attendu 2", l.End())
	}
	// Une tâche de 3 s puis deux de 1 s : la 2e et la 3e se partagent le worker libre.
	l = NewLanes(2)
	l.Add(3)
	l.Add(1)
	if f := l.FinishIfAdded(1); !near(f, 2) {
		t.Fatalf("FinishIfAdded = %v, attendu 2", f)
	}
}

func TestCostMinimumBilling(t *testing.T) {
	// Prix 36 $/h = 0,01 $/s. VM a : 10 s de travail -> facturée 60 s -> 0,6 $.
	// VM b : aucune tâche -> pas allumée -> 0 $.
	in := toy([][]float64{{10, 10}}, vm("a", 1, 36), vm("b", 1, 36))
	m := in.Evaluate(Allocation{{Task: 0, VM: 0}})
	if !near(m.Cost, 0.6) || m.UsedVMs != 1 || !near(m.Makespan, 10) {
		t.Fatalf("coût=%v VM=%d D=%v ; attendu 0,6 $, 1 VM, 10 s", m.Cost, m.UsedVMs, m.Makespan)
	}
	// Au-delà de 60 s, facturation à la seconde : 100 s -> 1,0 $.
	in = toy([][]float64{{100}}, vm("a", 1, 36))
	if m := in.Evaluate(Allocation{{0, 0}}); !near(m.Cost, 1.0) {
		t.Fatalf("coût=%v, attendu 1,0 $", m.Cost)
	}
}

func TestScoreAndDeadline(t *testing.T) {
	in := toy([][]float64{{1}}, vm("a", 1, 36))
	ref := Metrics{Makespan: 10, Cost: 1, RAMOK: true}
	m := Metrics{Makespan: 12.5, Cost: 0.5, RAMOK: true}
	// S = 0,6·0,5/1 + 0,4·12,5/10 = 0,3 + 0,5 = 0,8
	if s := in.Score(m, ref); !near(s, 0.8) {
		t.Fatalf("S = %v, attendu 0,8", s)
	}
	if !in.Feasible(m, ref) { // 12,5 = 1,25 x 10 : juste à la limite, admissible
		t.Fatal("12,5 s devrait être admissible (limite 12,5 s)")
	}
	m.Makespan = 12.6
	if in.Feasible(m, ref) {
		t.Fatal("12,6 s ne devrait pas être admissible")
	}
}

func TestValidate(t *testing.T) {
	in := toy([][]float64{{1}, {1}}, vm("a", 1, 36))
	if in.Validate(Allocation{{0, 0}, {1, 0}}) != nil {
		t.Fatal("allocation correcte refusée")
	}
	if in.Validate(Allocation{{0, 0}}) == nil {
		t.Fatal("tâche manquante non détectée")
	}
	if in.Validate(Allocation{{0, 0}, {0, 0}, {1, 0}}) == nil {
		t.Fatal("tâche en double non détectée")
	}
}

// TestPredictionMatchesRealRun : le modèle, chargé avec NOS mesures, doit
// prédire le makespan du lot RR réellement exécuté sur GCP (results/run_rr.csv)
// à 15 % près. C'est le lien entre le modèle et la réalité.
func TestPredictionMatchesRealRun(t *testing.T) {
	in, err := Load("../../data/tasks.csv", "../../data/machines.csv", "../../data/vm_performance.csv", nil, DefaultParams())
	if err != nil {
		t.Fatal(err)
	}
	alloc, err := in.LoadAllocation("../../data/allocation_rr.csv")
	if err != nil {
		t.Fatal(err)
	}
	predicted := in.Evaluate(alloc).Makespan

	f, err := os.Open("../../results/run_rr.csv")
	if err != nil {
		t.Skip("results/run_rr.csv absent :", err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	real := 0.0
	for _, r := range rows[1:] {
		if end, _ := strconv.ParseFloat(r[6], 64); end > real { // colonne end_s
			real = end
		}
	}
	ratio := real / predicted
	t.Logf("makespan réel %.3f s / prédit %.3f s = %.2f", real, predicted, ratio)
	if ratio < 0.85 || ratio > 1.15 {
		t.Fatalf("écart prédit/réel trop grand : %.2f", ratio)
	}
}

// Modèle V2 (par caractéristiques) : la VM de référence garde exactement ses
// durées V1 ; les autres reprennent les médianes et la contention de la
// référence, divisées par leur colonne « speed ».
func TestSpecModelUsesReference(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := dir + "/" + name
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	tasks := write("tasks.csv", "task_id,workload,size\nT1,cpu_task,100\n")
	machines := write("machines.csv", "vm,machine_type,vcpu,ram_mb,price_per_hour,contention,speed\n"+
		"ref,e2-medium,2,4096,0.04,1.5,1\n"+
		"fast,n2-standard-2,2,8192,0.10,1.2,2\n"+
		"new,e2-medium,2,4096,0.04,9.9,\n") // VM jamais benchmarkée, speed vide = 1
	perf := write("perf.csv", "machine,workload,size,runs,median_s,mean_s,std_s,max_rss_kb\n"+
		"ref,cpu_task,100,5,2.0,2.0,0,1000\n"+
		"fast,cpu_task,100,5,0.5,0.5,0,1000\n")

	// V1 : la VM « new » n'a pas de benchmark -> erreur.
	if _, err := Load(tasks, machines, perf, nil, DefaultParams()); err == nil {
		t.Fatal("V1 devrait refuser une VM sans benchmark")
	}

	p := DefaultParams()
	p.RefVM = "ref"
	in, err := Load(tasks, machines, perf, nil, p)
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{
		2.0*1.5 + 0.1,   // ref : ses propres mesures
		2.0/2*1.5 + 0.1, // fast : 2 fois plus rapide, contention de la référence
		2.0*1.5 + 0.1,   // new : jamais mesurée, même type -> comme la référence
	}
	for j, w := range want {
		if !near(in.P[0][j], w) {
			t.Errorf("%s : durée %.4f, attendu %.4f", in.Machines[j].Name, in.P[0][j], w)
		}
	}

	p.RefVM = "absente"
	if _, err := Load(tasks, machines, perf, nil, p); err == nil {
		t.Fatal("une VM de référence absente doit être refusée")
	}
}
