// Gantt : transforme une ou plusieurs exécutions RÉELLES de l'orchestrateur
// (results/.../<algo>_r<k>.csv) en diagramme temporel LaTeX (TikZ), prêt à
// coller dans le rapport : une ligne par worker de chaque VM, une barre par
// tâche (début -> fin mesurés par l'orchestrateur), couleur = workload.
// Les VM qui n'ont reçu aucune tâche apparaissent grisées (« éteinte »).
//
//	./bin/gantt -titles "Round Robin,select-maxmin" \
//	    results/exp_real_all/tasks/rr_r1.csv results/exp_real_all/tasks/select-maxmin_r1.csv \
//	    > docs/rapport/figures/gantt_9.tex
//
// Toutes les exécutions passées ensemble partagent la même échelle de temps.
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

type bar struct {
	id, workload, vm string
	worker           int
	start, end       float64
}

type machine struct {
	name    string
	workers int
}

var colors = map[string]string{
	"cpu_task":    "blue!35",
	"memory_task": "orange!50",
	"matrix_task": "green!45",
	"sort_task":   "violet!35",
}

func main() {
	machinesPath := flag.String("machines", "data/machines.csv", "description des VM (nom, vcpu = nombre de workers)")
	titles := flag.String("titles", "", "titres des exécutions, séparés par des virgules")
	width := flag.Float64("width", 12, "largeur de l'axe du temps (cm)")
	xmax := flag.Float64("xmax", 0, "fin de l'axe du temps en s (0 = automatique)")
	flag.Parse()
	files := flag.Args()
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "usage : gantt [-titles a,b] run1.csv [run2.csv ...]")
		os.Exit(2)
	}
	var names []string
	if *titles != "" {
		names = strings.Split(*titles, ",")
	}

	machines := readMachines(*machinesPath)
	runs := make([][]bar, len(files))
	end := 0.0
	for k, f := range files {
		runs[k] = readRun(f)
		for _, b := range runs[k] {
			end = math.Max(end, b.end)
		}
	}
	if *xmax <= 0 {
		*xmax = niceCeil(end)
	}
	scale := *width / *xmax // cm par seconde
	step := niceStep(*xmax)

	fmt.Printf("%% Généré par ./bin/gantt à partir de : %s\n", strings.Join(files, " "))
	fmt.Println("% Durées MESURÉES par l'orchestrateur (start_s / end_s).")
	fmt.Printf("\\begin{tikzpicture}[x=%.4fcm, y=0.42cm, font=\\scriptsize]\n", scale)

	y := 0.0
	for k, run := range runs {
		if k < len(names) {
			fmt.Printf("  \\node[anchor=south west, font=\\small\\bfseries] at (0,%.2f) {%s};\n", y+0.4, tex(strings.TrimSpace(names[k])))
		}
		used := map[string]bool{}
		for _, b := range run {
			used[b.vm] = true
		}
		top := y
		// grille verticale d'abord, pour qu'elle passe SOUS les barres
		bottom := top - 0.2
		for _, m := range machines {
			bottom -= float64(m.workers) + 0.25
		}
		for t := 0.0; t <= *xmax+1e-9; t += step {
			fmt.Printf("  \\draw[gray!25] (%.4f,%.2f) -- (%.4f,%.2f);\n", t, bottom, t, top-0.2)
		}
		for _, m := range machines {
			short := strings.TrimPrefix(m.name, "cloudopt-")
			for w := 1; w <= m.workers; w++ {
				y -= 1
				fmt.Printf("  \\node[anchor=east] at (0,%.2f) {%s w%d};\n", y, tex(short), w)
				if !used[m.name] {
					fmt.Printf("  \\fill[gray!12] (0,%.2f) rectangle (%.4f,%.2f);\n", y-0.38, *xmax, y+0.38)
				}
			}
			if !used[m.name] {
				fmt.Printf("  \\node[gray] at (%.4f,%.2f) {VM éteinte};\n", *xmax/2, y+float64(m.workers-1)/2)
			}
			y -= 0.25 // petit espace entre deux VM
		}
		for _, b := range run {
			by := rowOf(machines, b.vm, b.worker, top)
			fmt.Printf("  \\draw[fill=%s, draw=black!60] (%.4f,%.2f) rectangle (%.4f,%.2f);\n",
				colorOf(b.workload), b.start, by-0.32, b.end, by+0.32)
			if (b.end-b.start)*scale > 0.45 {
				fmt.Printf("  \\node at (%.4f,%.2f) {%s};\n", (b.start+b.end)/2, by, tex(b.id))
			}
		}
		// axe du temps de ce bloc
		y -= 0.2
		fmt.Printf("  \\draw[->] (0,%.2f) -- (%.4f,%.2f) node[right] {s};\n", y, *xmax+step/3, y)
		for t := 0.0; t <= *xmax+1e-9; t += step {
			fmt.Printf("  \\draw (%.4f,%.2f) -- (%.4f,%.2f) node[below] {%s};\n", t, y, t, y-0.15, frNum(t))
		}
		y -= 1.6
	}

	// légende des couleurs
	x := 0.0
	present := map[string]bool{}
	for _, run := range runs {
		for _, b := range run {
			present[b.workload] = true
		}
	}
	for _, w := range []string{"cpu_task", "memory_task", "matrix_task", "sort_task"} {
		if !present[w] {
			continue
		}
		fmt.Printf("  \\draw[fill=%s, draw=black!60] (%.4f,%.2f) rectangle ++(%.4f,0.5);\n", colors[w], x, y, 0.4/scale)
		fmt.Printf("  \\node[anchor=west] at (%.4f,%.2f) {%s};\n", x+0.45/scale, y+0.25, tex(w))
		x += 2.6 / scale
	}
	fmt.Println("\\end{tikzpicture}")
}

// rowOf : ordonnée de la ligne (vm, worker) dans un bloc qui commence en top.
func rowOf(machines []machine, vm string, worker int, top float64) float64 {
	y := top
	for _, m := range machines {
		for w := 1; w <= m.workers; w++ {
			y -= 1
			if m.name == vm && w == worker {
				return y
			}
		}
		y -= 0.25
	}
	fail(fmt.Errorf("VM %s worker %d absente de machines.csv", vm, worker))
	return 0
}

func colorOf(w string) string {
	if c, ok := colors[w]; ok {
		return c
	}
	return "gray!40"
}

func readMachines(path string) []machine {
	rows := readCSV(path)
	var out []machine
	for _, r := range rows {
		n, err := strconv.Atoi(r["vcpu"])
		if err != nil || n <= 0 {
			fail(fmt.Errorf("%s : vcpu invalide pour %s", path, r["vm"]))
		}
		out = append(out, machine{r["vm"], n})
	}
	return out
}

func readRun(path string) []bar {
	var out []bar
	for _, r := range readCSV(path) {
		w, e1 := strconv.Atoi(r["worker"])
		s, e2 := strconv.ParseFloat(r["start_s"], 64)
		e, e3 := strconv.ParseFloat(r["end_s"], 64)
		if e1 != nil || e2 != nil || e3 != nil {
			fail(fmt.Errorf("%s : ligne invalide pour %s", path, r["task_id"]))
		}
		out = append(out, bar{r["task_id"], r["workload"], r["vm"], w, s, e})
	}
	return out
}

func readCSV(path string) []map[string]string {
	f, err := os.Open(path)
	if err != nil {
		fail(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil || len(rows) < 2 {
		fail(fmt.Errorf("%s : fichier vide ou illisible", path))
	}
	var out []map[string]string
	for _, row := range rows[1:] {
		m := map[string]string{}
		for i, h := range rows[0] {
			if i < len(row) {
				m[strings.TrimSpace(h)] = strings.TrimSpace(row[i])
			}
		}
		out = append(out, m)
	}
	return out
}

// niceCeil / niceStep : bornes et graduations « rondes » de l'axe du temps.
func niceStep(max float64) float64 {
	for _, s := range []float64{0.1, 0.2, 0.25, 0.5, 1, 2, 5, 10, 20, 25, 50, 100} {
		if max/s <= 8 {
			return s
		}
	}
	return math.Pow(10, math.Ceil(math.Log10(max/8)))
}

func niceCeil(v float64) float64 {
	s := niceStep(v)
	return math.Ceil(v/s-1e-9) * s
}

// frNum : nombre au format français (virgule décimale).
func frNum(v float64) string {
	s := strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
	return strings.ReplaceAll(s, ".", "{,}")
}

func tex(s string) string { return strings.ReplaceAll(s, "_", "\\_") }

func fail(err error) {
	fmt.Fprintln(os.Stderr, "Erreur :", err)
	os.Exit(1)
}
