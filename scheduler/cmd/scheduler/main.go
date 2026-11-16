// Scheduler : décide sur quelle VM exécuter chaque tâche et écrit allocation.csv.
//
// Un algorithme :
//
//	./bin/scheduler -algo mct -tasks data/tasks_100.csv -out data/allocation_mct_100.csv
//
// Comparer tous les algorithmes sur le même lot (simulation, rien n'est exécuté) :
//
//	./bin/scheduler -compare -tasks data/tasks_500.csv
//	./bin/scheduler -compare -tasks data/tasks_500.csv -exclude cloudopt-highcpu
//	./bin/scheduler -compare -tasks data/tasks.csv -eval data/allocation_manual.csv
//
// Justifier le choix de select-maxmin (résultat de chaque combinaison de types) :
//
//	./bin/scheduler -explain -tasks data/tasks_500.csv
//
// Sur un petit lot (10 tâches au plus), -compare ajoute « optimal » : la
// meilleure allocation possible selon le modèle (recherche exhaustive).
//
// Les durées, coûts et scores affichés sont des PRÉDICTIONS du modèle
// (package model), pas des mesures : seule l'exécution par l'orchestrateur mesure.
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"cloud-optimization/scheduler/algorithms"
	"cloud-optimization/scheduler/model"
)

func main() {
	algo := flag.String("algo", "rr", "algorithme : rr, met, mct, minmin, maxmin, select-maxmin, optimal (10 tâches max)")
	explain := flag.Bool("explain", false, "affiche le résultat de Max-Min pour chaque combinaison « nombre de VM par type »")
	tasks := flag.String("tasks", "", "lot de tâches (task_id,workload,size) [obligatoire]")
	machines := flag.String("machines", "data/machines.csv", "description des VM")
	perf := flag.String("perf", "data/vm_performance.csv", "médianes du benchmark par VM")
	out := flag.String("out", "data/allocation.csv", "allocation produite (mode -algo)")
	exclude := flag.String("exclude", "", "VM à retirer, séparées par des virgules (ex. cloudopt-highcpu)")
	compare := flag.Bool("compare", false, "compare tous les algorithmes sur le lot")
	eval := flag.String("eval", "", "allocations existantes à inclure dans la comparaison (ex. data/allocation_manual.csv)")
	saveDir := flag.String("save-dir", "", "mode -compare : dossier où écrire l'allocation de chaque algorithme")
	csvOut := flag.String("csv", "", "mode -compare : fichier CSV du tableau de comparaison")
	p := model.DefaultParams()
	flag.Float64Var(&p.OverheadS, "overhead", p.OverheadS, "surcoût SSH par tâche (s)")
	flag.Float64Var(&p.WeightCost, "wcost", p.WeightCost, "poids du coût dans S (le temps pèse 1 - wcost)")
	flag.Float64Var(&p.DeadlineFactor, "deadline", p.DeadlineFactor, "contrainte D <= deadline x D_RR")
	flag.Parse()

	if *tasks == "" {
		fmt.Fprintln(os.Stderr, "Erreur : -tasks est obligatoire")
		flag.Usage()
		os.Exit(2)
	}
	var excl []string
	if *exclude != "" {
		excl = strings.Split(*exclude, ",")
	}
	in, err := model.Load(*tasks, *machines, *perf, excl, p)
	check(err)

	// La référence de S et de la contrainte de délai est toujours Round Robin.
	ref := in.Evaluate(algorithms.RoundRobin(in))

	names := make([]string, len(in.Machines))
	for j, m := range in.Machines {
		names[j] = m.Name
	}
	fmt.Printf("Lot %s : %d tâches | VM : %s\n", *tasks, len(in.Tasks), strings.Join(names, ", "))
	fmt.Printf("Modèle : durée = médiane benchmark x contention + %.2f s | S = %.1f·C/C_RR + %.1f·D/D_RR | D <= %.2f x D_RR = %.3f s\n\n",
		p.OverheadS, p.WeightCost, 1-p.WeightCost, p.DeadlineFactor, p.DeadlineFactor*ref.Makespan)

	if *explain {
		// Tableau de justification de select-maxmin : une ligne par combinaison
		// « nombre de VM de chaque type », Max-Min sur chacune, triées par S.
		groups := algorithms.GroupByType(in)
		results := algorithms.EvaluateCombinations(in)
		fmt.Printf("%d combinaisons « nombre de VM par type » (Max-Min sur chacune) :\n\n", len(results))
		fmt.Printf("%-55s %12s %13s %8s %10s\n", "VM allumées", "Coût ($)", "Makespan (s)", "S", "Admissible")
		for k, r := range results {
			feasible, mark := "oui", ""
			if !r.Feasible {
				feasible = "non"
			}
			if k == 0 && r.Feasible {
				mark = "  <- choisi"
			}
			fmt.Printf("%-55s %12.6f %13.3f %8.4f %10s%s\n",
				r.Label(groups), r.Metrics.Cost, r.Metrics.Makespan, r.Score, feasible, mark)
		}
		fmt.Println("\nPrédictions du modèle (pas des mesures).")
		return
	}

	if !*compare {
		a, ok := algorithms.Get(*algo)
		if !ok {
			check(fmt.Errorf("algorithme inconnu %q", *algo))
		}
		alloc := a.Run(in)
		if alloc == nil {
			check(fmt.Errorf("%s : lot trop grand (%d tâches, %d au plus)", a.Name, len(in.Tasks), algorithms.OptimalMaxTasks))
		}
		check(in.Validate(alloc))
		check(os.MkdirAll(filepath.Dir(*out), 0o755))
		check(in.WriteAllocation(*out, alloc))
		met := in.Evaluate(alloc)
		printHeader()
		printRow(in, a.Name, met, ref, 0)

		// Seules les VM qui reçoivent des tâches doivent être démarrées.
		var used []string
		for j, s := range met.PerVM {
			if s.Tasks > 0 {
				used = append(used, in.Machines[j].Name)
			}
		}
		fmt.Printf("\nAllocation écrite : %s\n", *out)
		fmt.Printf("VM à démarrer     : VMS=\"%s\" ./scripts/manage_vms.sh start\n", strings.Join(used, " "))
		fmt.Printf("Puis              : ./bin/orchestrator -alloc %s\n", *out)
		return
	}

	type row struct {
		name    string
		m       model.Metrics
		elapsed time.Duration
	}
	var rows []row
	algos := algorithms.All
	if len(in.Tasks) <= algorithms.OptimalMaxTasks {
		algos = append(algos[:len(algos):len(algos)], algorithms.Extra...)
	}
	for _, a := range algos {
		start := time.Now()
		alloc := a.Run(in)
		elapsed := time.Since(start)
		check(in.Validate(alloc))
		rows = append(rows, row{a.Name, in.Evaluate(alloc), elapsed})
		if *saveDir != "" {
			check(os.MkdirAll(*saveDir, 0o755))
			check(in.WriteAllocation(filepath.Join(*saveDir, "allocation_"+a.Name+".csv"), alloc))
		}
	}
	if *eval != "" {
		for _, path := range strings.Split(*eval, ",") {
			alloc, err := in.LoadAllocation(strings.TrimSpace(path))
			check(err)
			name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			rows = append(rows, row{name, in.Evaluate(alloc), 0})
		}
	}

	printHeader()
	for _, r := range rows {
		printRow(in, r.name, r.m, ref, r.elapsed)
	}
	fmt.Println("\nPrédictions du modèle (pas des mesures). « non » = makespan au-delà de la limite de délai.")

	if *csvOut != "" {
		check(os.MkdirAll(filepath.Dir(*csvOut), 0o755))
		f, err := os.Create(*csvOut)
		check(err)
		w := csv.NewWriter(f)
		header := []string{"algo", "tasks", "vms_used", "makespan_s", "cost_usd", "score", "feasible", "ram_ok", "runtime_ms"}
		header = append(header, names...)
		w.Write(header)
		for _, r := range rows {
			rec := []string{r.name, strconv.Itoa(len(in.Tasks)), strconv.Itoa(r.m.UsedVMs),
				fmt.Sprintf("%.6f", r.m.Makespan), fmt.Sprintf("%.8f", r.m.Cost),
				fmt.Sprintf("%.6f", in.Score(r.m, ref)), strconv.FormatBool(in.Feasible(r.m, ref)),
				strconv.FormatBool(r.m.RAMOK), fmt.Sprintf("%.3f", float64(r.elapsed.Microseconds())/1000)}
			for _, s := range r.m.PerVM {
				rec = append(rec, strconv.Itoa(s.Tasks))
			}
			w.Write(rec)
		}
		w.Flush()
		check(w.Error())
		f.Close()
		fmt.Println("Tableau écrit :", *csvOut)
	}
}

func printHeader() {
	fmt.Printf("%-18s %12s %13s %8s %10s %9s  %s\n", "Algo", "Coût ($)", "Makespan (s)", "S", "Admissible", "Calcul", "Tâches par VM")
}

func printRow(in *model.Instance, name string, m, ref model.Metrics, elapsed time.Duration) {
	feasible := "oui"
	if !in.Feasible(m, ref) {
		feasible = "non"
	}
	var dist []string
	for j, s := range m.PerVM {
		if s.Tasks > 0 {
			short := strings.TrimPrefix(in.Machines[j].Name, "cloudopt-")
			dist = append(dist, fmt.Sprintf("%s:%d", short, s.Tasks))
		}
	}
	calc := "-"
	if elapsed > 0 {
		calc = fmt.Sprintf("%.1f ms", float64(elapsed.Microseconds())/1000)
	}
	fmt.Printf("%-18s %12.6f %13.3f %8.4f %10s %9s  %s\n",
		name, m.Cost, m.Makespan, in.Score(m, ref), feasible, calc, strings.Join(dist, " "))
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "Erreur :", err)
		os.Exit(1)
	}
}
