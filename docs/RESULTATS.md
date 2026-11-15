# Journal des résultats et état du projet

Document de passation pour l'équipe : où en est le projet, ce qui a été
**mesuré**, ce que l'on en conclut, et comment reprendre le travail.
Mis à jour à chaque étape. Dernière mise à jour : 15/11/2026.

> Règle : tout chiffre de ce document est une **mesure réelle**, sauf mention
> explicite « hypothèse » ou « prédiction ».

---

## 1. Avancement

| Étape | État | Livrable |
|---|---|---|
| 1. Mise en place | ✅ | `go.mod`, `Makefile` (local + linux/amd64), `.gitignore` |
| 2. Benchmark local (Linux) | ✅ | `scripts/benchmark.sh`, `results/benchmark_local.csv` |
| 3. VM GCP + benchmark par VM | ✅ | `scripts/{config,manage_vms,deploy,bench_vms}.sh`, `data/vm_performance.csv` |
| 4. Orchestrateur Go | ✅ testé sur GCP | `orchestrator/main.go`, `results/run_rr*.csv` |
| 4b. Expérience « workers » | ⏳ en cours | parallélisme réel de chaque VM |
| 5. Schedulers (RR, MET, MCT, Min-Min, Max-Min, V8) | ⬜ | RR seul existe (`scheduler/roundrobin`) |
| 6. Expériences réelles RR vs V8 | ⬜ | |
| 7. Soutenance | ⬜ | |

---

## 2. Reprendre le projet (pour un nouveau membre)

```bash
git clone git@github.com:F1golfin/cloud-optimization.git && cd cloud-optimization
make                                      # workloads locaux + bin/orchestrator + bin/roundrobin
./bin/orchestrator -local -alloc data/allocation_rr.csv -out /tmp/test.csv   # test sans GCP
```

Pour GCP : Google Cloud SDK installé, `gcloud auth login`, accès au projet
`cloud-opt-511106`. Ensuite (voir le README) :

```bash
./scripts/manage_vms.sh start
./scripts/deploy.sh                       # une fois, ou après modification des workloads
./bin/orchestrator -alloc data/allocation_rr.csv -out results/mon_run.csv
./scripts/manage_vms.sh stop              # TOUJOURS
```

La première connexion d'un nouveau poste à une VM peut demander
`gcloud compute ssh <vm> --zone europe-west1-b --command true` (enregistre la clé).

---

## 3. Infrastructure

| VM | Type | vCPU | RAM | Prix (modèle) |
|---|---|---|---|---|
| cloudopt-vm1 | e2-standard-2 | 2 | 8 Go | 0,0737 $/h |
| cloudopt-vm2 | e2-medium | 2 (partagés) | 4 Go | 0,0369 $/h |
| cloudopt-vm3 | e2-medium | 2 (partagés) | 4 Go | 0,0369 $/h |
| cloudopt-highcpu | e2-highcpu-4 | 4 | 4 Go | 0,1088 $/h |

Zone `europe-west1-b`, Ubuntu 24.04 x86_64. Source : `data/machines.csv`.
Machine de contrôle : VM Linux OrbStack (Ubuntu arm64) sur le Mac.

---

## 4. Benchmark : une tâche seule

Protocole : 4 workloads × 3 tailles × 5 répétitions, une exécution de chauffe
non mesurée avant chaque taille, une seule tâche à la fois. Valeur = **médiane**
(s). Écart-type < 1 % dans presque tous les cas.

| Workload | Taille | Local (OrbStack) | vm1 | vm2 | vm3 | highcpu |
|---|---|---|---|---|---|---|
| cpu_task | 200 000 | 0,003 | 0,045 | 0,047 | 0,045 | 0,045 |
| cpu_task | 500 000 | 0,008 | 0,148 | 0,150 | 0,147 | 0,148 |
| cpu_task | 1 000 000 | 0,019 | 0,378 | 0,383 | 0,378 | 0,379 |
| matrix_task | 200 | 0,004 | 0,018 | 0,020 | 0,019 | 0,018 |
| matrix_task | 400 | 0,023 | 0,101 | 0,105 | 0,101 | 0,104 |
| matrix_task | 600 | 0,071 | 0,320 | 0,328 | 0,317 | 0,318 |
| memory_task | 128 Mo | 0,039 | 0,206 | 0,224 | 0,207 | 0,222 |
| memory_task | 256 Mo | 0,078 | 0,406 | 0,435 | 0,402 | 0,427 |
| memory_task | 512 Mo | 0,149 | 0,797 | 0,854 | 0,793 | 0,846 |
| sort_task | 1 M | 0,055 | 0,137 | 0,140 | 0,138 | 0,138 |
| sort_task | 5 M | 0,294 | 0,720 | 0,733 | 0,719 | 0,726 |
| sort_task | 10 M | 0,612 | 1,498 | 1,515 | 1,496 | 1,507 |

Fichiers : `results/benchmark_*.csv` (brut), `data/vm_performance.csv` (médianes,
moyenne, écart-type, RSS max), `results/bench_logs/`.

### Constats

1. **Les 4 VM ont la même vitesse pour une tâche seule (à 5 % près).** Un
   workload est mono-thread et toutes les VM e2 utilisent la même famille de
   processeurs. HighCPU n'a pas des cœurs plus rapides : elle en a plus.
   ⚠️ Cela **contredit l'ancien rapport**, où HighCPU semblait dominer les
   autres VM tâche par tâche.
2. **vm1 coûte 2× vm2/vm3 pour la même vitesse et le même nombre de vCPU.**
   Son seul avantage est la RAM (8 Go). Elle ne devrait être choisie que pour
   ajouter de la capacité.
3. Le local (Mac M-series) est 2,5 à 20× plus rapide : il ne sert pas à
   prédire les durées sur GCP.
4. Les tâches durent 0,02 à 1,5 s sur les VM, donc le surcoût d'orchestration
   compte (voir § 5).

---

## 5. Premier lot réel : Round Robin, 9 tâches, 4 VM

Commande : `./bin/orchestrator -alloc data/allocation_rr.csv -out results/run_rr.csv`
(workers par défaut = vCPU : 2, ou 4 pour highcpu). Exécuté le 15/11/2026.

| VM | Tâches | Fenêtre active (s) | Temps de calcul (s) | Surcoût SSH moyen / tâche | Coût modèle ($) |
|---|---|---|---|---|---|
| highcpu | 2 | 0,54 | 0,58 | 0,077 s | 0,001813 |
| vm1 | 3 | 1,31 | 1,68 | 0,085 s | 0,001228 |
| vm2 | 2 | 0,83 | 1,45 | 0,070 s | 0,000615 |
| vm3 | 2 | 0,68 | 0,94 | 0,099 s | 0,000615 |

**Makespan mesuré : 1,316 s. 9/9 tâches OK. Coût modèle : 0,004272 $.**
Fichiers : `results/run_rr.csv` (par tâche), `results/run_rr_vms.csv` (par VM).

### Constats

1. **L'exécution distribuée fonctionne** : 4 files en parallèle sur 4 VM,
   résultats collectés, aucune perte.
2. **Surcoût SSH : 0,06 à 0,11 s par tâche**, grâce à la connexion maître
   réutilisée (multiplexage OpenSSH). Avec une connexion par tâche, on paie
   environ 0,5 à 1 s (ordre de grandeur de l'ancien test du rapport).
3. **Deux tâches simultanées sur une VM à 2 vCPU se ralentissent fortement.**
   Temps de calcul pendant le lot / médiane du benchmark (tâche seule) :

   | VM | Tâches simultanées | Ralentissement |
   |---|---|---|
   | vm1 (2 vCPU) | T1+T5, puis T9+T5 | × 1,5 à × 2,2 |
   | vm2 (2 vCPU partagés) | T2+T6 | × 2,0 |
   | vm3 (2 vCPU partagés) | T3+T7 | × 1,5 à × 2,3 |
   | highcpu (4 vCPU) | T4+T8 | × 1,06 à × 1,17 |

   **Hypothèse** : sur GCP, 2 vCPU = 1 cœur physique en hyperthreading ; deux
   tâches se partagent donc un cœur. HighCPU (4 vCPU = 2 cœurs) n'a presque pas
   ralenti. C'est probablement la principale cause de l'écart prédit/réel de
   l'ancien rapport (1,1 s prédit, 3–4 s mesuré).
   ⚠️ 9 tâches seulement, juste après le démarrage des VM : **à confirmer** par
   l'expérience « workers » avant de l'intégrer au modèle du scheduler.
4. **Le coût est dicté par la facturation minimale de 60 s** : le lot dure
   1,3 s mais chaque VM est comptée 60 s. Le coût ne dépend alors que du
   nombre de VM utilisées et de leur prix. Pour comparer les coûts de RR et de
   V8, il faudra des lots dont le makespan approche ou dépasse la minute.

---

## 6. Décisions prises

| Décision | Raison |
|---|---|
| Compiler une fois en linux/amd64 et copier les binaires | Mac en ARM, VM en x86 ; rien à installer sur les VM |
| Tailles du benchmark = tailles de `data/tasks.csv` | Le scheduler lit directement la durée mesurée, sans extrapoler |
| Médiane plutôt que moyenne | Résiste à une exécution perturbée (vCPU partagés) |
| Une connexion SSH par VM, réutilisée | Surcoût par tâche divisé par 5 à 10 |
| Horodatage sur la machine de contrôle | Une seule horloge, aucune synchronisation entre VM |
| `overhead_s = durée mesurée − runtime_s du workload` | Sépare calcul et coût d'orchestration |

---

## 7. Prochaines étapes

1. **Expérience « workers »** : même lot sur chaque VM avec 1, 2 (et 4 pour
   highcpu) workers, et comparaison du débit. On en déduit le bon nombre de
   workers par VM et un facteur de ralentissement pour le modèle.
2. **Scheduler (étape 5)** : modèle de prédiction = médiane du benchmark ×
   facteur de contention + surcoût SSH mesuré ; implémenter MET, MCT,
   Min-Min, Max-Min et V8 (recherche locale qui minimise directement
   `S = 0,6·C/C_RR + 0,4·D/D_RR` sous `D ≤ 1,25·D_RR`).
3. **Expériences (étape 6)** : lots plus gros (makespan ≥ 60 s), RR vs V8,
   3 répétitions, erreur prédit vs réel.

## 8. Règles

- Ne jamais présenter une durée simulée comme mesurée.
- Toujours `./scripts/manage_vms.sh stop` après une session GCP.
- Aucun secret (clé, JSON de compte de service) dans le dépôt.
