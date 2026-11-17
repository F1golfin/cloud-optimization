// modelcheck : compare l'heure de fin PRÉDITE de chaque VM (modèle choisi par
// -machines / -perf) à l'heure de fin MESURÉE lors des exécutions réelles
// déjà faites (dossiers produits par scripts/exp_real.sh).
//
//	./bin/modelcheck results/exp_real/*/ results/exp_real_all/*/                    (modèle V1)
//	./bin/modelcheck -ref cloudopt-vm2 results/exp_real/*/ results/exp_real_all/*/  (modèle V2)
//
// Une ligne par (exécution, VM) : dossier, algo, répétition, VM, nombre de
// tâches, fin prédite, fin mesurée, processeur, et « bridee » si cette VM est
// une e2-medium nettement plus lente que l'autre (rapport > 1,3, au moins
// 100 tâches chacune) : ces cas ne relèvent pas du modèle.
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"cloud-optimization/scheduler/model"
)

func main() {
	machines := flag.String("machines", "data/machines.csv", "description des VM")
	perf := flag.String("perf", "data/vm_performance.csv", "benchmark")
	p := model.DefaultParams()
	flag.StringVar(&p.RefVM, "ref", "", "modèle V2 : VM de référence (vide = modèle V1)")
	flag.Parse()

	w := csv.NewWriter(os.Stdout)
	w.Write([]string{"dir", "algo", "rep", "vm", "machine_type", "tasks", "pred_s", "meas_s", "cpu", "bridee"})
	for _, dir := range flag.Args() {
		dir = strings.TrimSuffix(dir, "/")
		lot := lotOf(filepath.Base(dir))
		in, err := model.Load(lot, *machines, *perf, nil, p)
		check(err)
		allocs, _ := filepath.Glob(filepath.Join(dir, "alloc_*.csv"))
		for _, ap := range allocs {
			algo := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(ap), "alloc_"), ".csv")
			a, err := in.LoadAllocation(ap)
			check(err)
			met := in.Evaluate(a)
			for rep := 1; ; rep++ {
				rows, err := readCSV(filepath.Join(dir, fmt.Sprintf("%s_r%d_vms.csv", algo, rep)))
				if err != nil {
					break
				}
				slow := throttled(rows)
				for _, r := range rows {
					j := vmIndex(in, r["vm"])
					if j < 0 || r["tasks"] == "0" {
						continue
					}
					w.Write([]string{dir, algo, strconv.Itoa(rep), r["vm"], in.Machines[j].Type, r["tasks"],
						fmt.Sprintf("%.4f", met.PerVM[j].Time), r["last_end_s"], r["cpu_model"],
						strconv.FormatBool(slow[r["vm"]])})
				}
			}
		}
	}
	w.Flush()
	check(w.Error())
}

// throttled : la plus lente des deux e2-medium, si elle l'est nettement.
func throttled(rows []map[string]string) map[string]bool {
	out := map[string]bool{}
	var med []map[string]string
	for _, r := range rows {
		n, _ := strconv.Atoi(r["tasks"])
		if (strings.HasSuffix(r["vm"], "vm2") || strings.HasSuffix(r["vm"], "vm3")) && n >= 100 {
			med = append(med, r)
		}
	}
	if len(med) == 2 {
		a, _ := strconv.ParseFloat(med[0]["active_s"], 64)
		b, _ := strconv.ParseFloat(med[1]["active_s"], 64)
		if a > 1.3*b {
			out[med[0]["vm"]] = true
		} else if b > 1.3*a {
			out[med[1]["vm"]] = true
		}
	}
	return out
}

func lotOf(name string) string {
	switch {
	case strings.HasPrefix(name, "tasks_2000"):
		return "data/tasks_2000.csv"
	case strings.HasPrefix(name, "tasks_500"):
		return "data/tasks_500.csv"
	default:
		return "data/tasks.csv"
	}
}

func vmIndex(in *model.Instance, name string) int {
	for j, m := range in.Machines {
		if m.Name == name {
			return j
		}
	}
	return -1
}

func readCSV(path string) ([]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil || len(rows) < 2 {
		return nil, fmt.Errorf("%s : illisible", path)
	}
	var out []map[string]string
	for _, row := range rows[1:] {
		m := map[string]string{}
		for i, h := range rows[0] {
			m[h] = row[i]
		}
		out = append(out, m)
	}
	return out, nil
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "Erreur :", err)
		os.Exit(1)
	}
}
