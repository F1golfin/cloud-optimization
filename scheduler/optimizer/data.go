package main

// Lecture des données d'entrée : tâches, VM, caractéristiques des VM,
// mesures de benchmark (une tâche seule) et mesures de concurrence.

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Task : une ligne de tasks.csv.
type Task struct {
	ID       string
	Workload string
	Size     int
}

// VM : une machine disponible (machines.csv) et ses caractéristiques (vm_specs.csv).
type VM struct {
	Name   string
	Type   string
	VCPU   int
	RAMMB  float64
	PriceH float64 // $/heure
}

// readCSV lit un CSV, vérifie l'en-tête et renvoie les lignes de données.
func readCSV(path string, header []string) ([][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%s : %w", path, err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%s est vide", path)
	}
	if len(rows[0]) < len(header) {
		return nil, fmt.Errorf("%s : colonnes attendues %s", path, strings.Join(header, ","))
	}
	for i, h := range header {
		if strings.TrimSpace(rows[0][i]) != h {
			return nil, fmt.Errorf("%s : la colonne %d doit être %q", path, i+1, h)
		}
	}
	for _, r := range rows[1:] {
		for i := range r {
			r[i] = strings.TrimSpace(r[i])
		}
	}
	return rows[1:], nil
}

func loadTasks(path string) ([]Task, error) {
	rows, err := readCSV(path, []string{"task_id", "workload", "size"})
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var tasks []Task
	for i, r := range rows {
		if r[0] == "" || r[1] == "" || r[2] == "" {
			return nil, fmt.Errorf("%s : champ vide, ligne %d", path, i+2)
		}
		if seen[r[0]] {
			return nil, fmt.Errorf("%s : task_id en double %q", path, r[0])
		}
		seen[r[0]] = true
		size, err := strconv.Atoi(r[2])
		if err != nil || size <= 0 {
			return nil, fmt.Errorf("%s : taille invalide %q, ligne %d", path, r[2], i+2)
		}
		tasks = append(tasks, Task{ID: r[0], Workload: r[1], Size: size})
	}
	if len(tasks) == 0 {
		return nil, fmt.Errorf("%s : aucune tâche", path)
	}
	return tasks, nil
}

// loadVMs lit machines.csv (noms, dans l'ordre) puis les caractéristiques de chaque VM.
func loadVMs(machinesPath, specsPath string) ([]VM, error) {
	names, err := readCSV(machinesPath, []string{"vm"})
	if err != nil {
		return nil, err
	}
	specRows, err := readCSV(specsPath, []string{"machine", "machine_type", "vcpu", "ram_mb", "price_usd_h"})
	if err != nil {
		return nil, err
	}
	specs := map[string]VM{}
	for _, r := range specRows {
		vcpu, e1 := strconv.Atoi(r[2])
		ram, e2 := strconv.ParseFloat(r[3], 64)
		price, e3 := strconv.ParseFloat(r[4], 64)
		if e1 != nil || e2 != nil || e3 != nil || vcpu <= 0 || ram <= 0 || price <= 0 {
			return nil, fmt.Errorf("%s : ligne invalide pour %s", specsPath, r[0])
		}
		specs[r[0]] = VM{Name: r[0], Type: r[1], VCPU: vcpu, RAMMB: ram, PriceH: price}
	}
	var vms []VM
	seen := map[string]bool{}
	for _, r := range names {
		if r[0] == "" || seen[r[0]] {
			return nil, fmt.Errorf("%s : VM vide ou en double (%q)", machinesPath, r[0])
		}
		seen[r[0]] = true
		s, ok := specs[r[0]]
		if !ok {
			return nil, fmt.Errorf("VM %s absente de %s", r[0], specsPath)
		}
		vms = append(vms, s)
	}
	if len(vms) == 0 {
		return nil, fmt.Errorf("%s : aucune VM disponible", machinesPath)
	}
	return vms, nil
}

// perfPoint : mesure (médiane) d'un workload à une taille donnée sur une VM.
type perfPoint struct {
	size   float64
	median float64 // s
	rssMB  float64
}

// Perf : mesures du benchmark « une tâche seule », par VM et par workload.
type Perf map[string]map[string][]perfPoint // vm -> workload -> points triés par taille

func loadPerf(path string) (Perf, error) {
	rows, err := readCSV(path, []string{"machine", "workload", "size", "runs", "median_s", "mean_s", "std_s", "max_rss_kb"})
	if err != nil {
		return nil, err
	}
	p := Perf{}
	for _, r := range rows {
		size, e1 := strconv.ParseFloat(r[2], 64)
		med, e2 := strconv.ParseFloat(r[4], 64)
		rss, e3 := strconv.ParseFloat(r[7], 64)
		if e1 != nil || e2 != nil || e3 != nil {
			return nil, fmt.Errorf("%s : ligne invalide %v", path, r)
		}
		if p[r[0]] == nil {
			p[r[0]] = map[string][]perfPoint{}
		}
		p[r[0]][r[1]] = append(p[r[0]][r[1]], perfPoint{size, med, rss / 1024})
	}
	for _, byW := range p {
		for w := range byW {
			sort.Slice(byW[w], func(i, j int) bool { return byW[w][i].size < byW[w][j].size })
		}
	}
	return p, nil
}

// interp estime une valeur à la taille x par interpolation (ou extrapolation)
// linéaire en échelle log-log entre les deux points mesurés les plus proches.
// Si la taille a été mesurée, renvoie directement la mesure.
func interp(pts []perfPoint, x float64, get func(perfPoint) float64) float64 {
	for _, p := range pts {
		if p.size == x {
			return get(p)
		}
	}
	if len(pts) == 1 {
		return get(pts[0]) * x / pts[0].size
	}
	i := sort.Search(len(pts), func(i int) bool { return pts[i].size >= x })
	if i == 0 {
		i = 1
	}
	if i >= len(pts) {
		i = len(pts) - 1
	}
	a, b := pts[i-1], pts[i]
	ya, yb := math.Max(get(a), 1e-9), math.Max(get(b), 1e-9)
	slope := (math.Log(yb) - math.Log(ya)) / (math.Log(b.size) - math.Log(a.size))
	return ya * math.Exp(slope*(math.Log(x)-math.Log(a.size)))
}

// Slowdown : facteur de ralentissement mesuré quand k tâches identiques
// tournent en même temps sur une VM : durée(k) / durée(1).
type Slowdown map[string]map[string]map[int]float64 // vm -> workload -> k -> facteur

// loadConcurrency lit results/concurrency.csv. Fichier absent : renvoie nil
// (le modèle utilisera alors le partage théorique des vCPU).
func loadConcurrency(path string) (Slowdown, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil
	}
	rows, err := readCSV(path, []string{"machine", "workload", "size", "k", "copy", "runtime_s"})
	if err != nil {
		return nil, err
	}
	sum := map[string]float64{}
	cnt := map[string]float64{}
	for _, r := range rows {
		t, err := strconv.ParseFloat(r[5], 64)
		if err != nil {
			return nil, fmt.Errorf("%s : durée invalide %q", path, r[5])
		}
		key := r[0] + "|" + r[1] + "|" + r[3]
		sum[key] += t
		cnt[key]++
	}
	s := Slowdown{}
	for key := range sum {
		parts := strings.Split(key, "|")
		k, _ := strconv.Atoi(parts[2])
		base := parts[0] + "|" + parts[1] + "|1"
		if cnt[base] == 0 {
			continue
		}
		f := (sum[key] / cnt[key]) / (sum[base] / cnt[base])
		if s[parts[0]] == nil {
			s[parts[0]] = map[string]map[int]float64{}
		}
		if s[parts[0]][parts[1]] == nil {
			s[parts[0]][parts[1]] = map[int]float64{}
		}
		s[parts[0]][parts[1]][k] = math.Max(f, 1) // jamais plus rapide qu'une tâche seule
	}
	// Le bruit de mesure peut donner f(3) < f(2) : on impose un ralentissement
	// croissant avec k (ajouter une tâche n'accélère pas les autres), ce que
	// suppose la borne de la méthode exacte.
	for _, byW := range s {
		for _, byK := range byW {
			var ks []int
			for k := range byK {
				ks = append(ks, k)
			}
			sort.Ints(ks)
			for i := 1; i < len(ks); i++ {
				byK[ks[i]] = math.Max(byK[ks[i]], byK[ks[i-1]])
			}
		}
	}
	return s, nil
}
