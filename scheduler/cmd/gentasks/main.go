// gentasks : génère un lot de tâches aléatoire mais REPRODUCTIBLE.
//
//	./bin/gentasks -n 500 -seed 42 -out data/tasks_500.csv
//
// Les combinaisons (workload, taille) sont lues dans vm_performance.csv :
// on ne génère que des tâches dont la durée a été mesurée sur les VM.
// Chaque combinaison a la même probabilité. Avec la même graine, tout le
// monde obtient exactement le même fichier.
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

type combo struct {
	workload string
	size     int
}

func main() {
	n := flag.Int("n", 100, "nombre de tâches")
	seed := flag.Int64("seed", 42, "graine du générateur aléatoire")
	perf := flag.String("perf", "data/vm_performance.csv", "benchmark des VM (source des combinaisons)")
	out := flag.String("out", "", "fichier de sortie (défaut : data/tasks_<n>.csv)")
	flag.Parse()

	if *n <= 0 {
		fail(fmt.Errorf("-n doit être > 0"))
	}
	if *out == "" {
		*out = fmt.Sprintf("data/tasks_%d.csv", *n)
	}

	combos, err := readCombos(*perf)
	if err != nil {
		fail(err)
	}

	rng := rand.New(rand.NewSource(*seed))
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fail(err)
	}
	f, err := os.Create(*out)
	if err != nil {
		fail(err)
	}
	w := csv.NewWriter(f)
	w.Write([]string{"task_id", "workload", "size"})
	width := len(strconv.Itoa(*n))
	count := map[string]int{}
	for i := 1; i <= *n; i++ {
		c := combos[rng.Intn(len(combos))]
		count[c.workload]++
		w.Write([]string{fmt.Sprintf("T%0*d", width, i), c.workload, strconv.Itoa(c.size)})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		fail(err)
	}
	f.Close()

	fmt.Printf("%s : %d tâches (graine %d) |", *out, *n, *seed)
	for _, wl := range []string{"cpu_task", "memory_task", "matrix_task", "sort_task"} {
		fmt.Printf(" %s=%d", wl, count[wl])
	}
	fmt.Println()
}

// readCombos : les couples (workload, taille) distincts du benchmark, dans un ordre fixe.
func readCombos(path string) ([]combo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil || len(rows) < 2 {
		return nil, fmt.Errorf("%s illisible ou vide", path)
	}
	col := map[string]int{}
	for i, h := range rows[0] {
		col[h] = i
	}
	wi, ok1 := col["workload"]
	si, ok2 := col["size"]
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("%s : colonnes workload/size manquantes", path)
	}
	seen := map[combo]bool{}
	var combos []combo
	for _, r := range rows[1:] {
		size, err := strconv.Atoi(r[si])
		if err != nil {
			return nil, fmt.Errorf("%s : taille invalide %q", path, r[si])
		}
		c := combo{r[wi], size}
		if !seen[c] {
			seen[c] = true
			combos = append(combos, c)
		}
	}
	// Tri pour que le résultat ne dépende pas de l'ordre des lignes du fichier.
	sort.Slice(combos, func(a, b int) bool {
		if combos[a].workload != combos[b].workload {
			return combos[a].workload < combos[b].workload
		}
		return combos[a].size < combos[b].size
	})
	return combos, nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "Erreur :", err)
	os.Exit(1)
}
