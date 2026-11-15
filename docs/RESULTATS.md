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
| 4b. Expérience « workers » | ✅ | `scripts/exp_workers.sh`, `results/exp_workers/` |
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
   → **Confirmé** par l'expérience « workers » (§ 6).
4. **Le coût est dicté par la facturation minimale de 60 s** : le lot dure
   1,3 s mais chaque VM est comptée 60 s. Le coût ne dépend alors que du
   nombre de VM utilisées et de leur prix. Pour comparer les coûts de RR et de
   V8, il faudra des lots dont le makespan approche ou dépasse la minute.

---

## 6. Expérience « workers » : le parallélisme réel de chaque VM

Commande : `./scripts/exp_workers.sh` (15/11/2026). Même lot de **24 tâches
par VM** (2 × les 12 combinaisons du benchmark), exécuté avec 1, 2 et 4
workers par VM, **3 répétitions** par niveau, les 4 VM en même temps.
Fichiers : `results/exp_workers/` (`summary.csv` + un CSV par exécution).

| VM | Workers | Durée du lot (s) | Débit (tâches/s) | Gain vs 1 worker | Ralentissement par tâche* | Surcoût SSH |
|---|---|---|---|---|---|---|
| highcpu (4 vCPU) | 1 | 10,82 | 2,22 | — | 0,89 | 0,086 s |
| | 2 | 6,03 | 3,98 | × 1,79 | 0,94 | 0,084 s |
| | 4 | **4,51** | **5,32** | **× 2,40** | 1,32 | 0,100 s |
| vm1 (2 vCPU) | 1 | 12,21 | 1,96 | — | 0,95 | 0,083 s |
| | 2 | 9,02 | 2,66 | × 1,36 | 1,42 | 0,110 s |
| | 4 | 8,84 | 2,72 | × 1,39 | 2,81 | 0,185 s |
| vm2 (2 vCPU partagés) | 1 | 10,96 | 2,19 | — | 0,90 | 0,086 s |
| | 2 | 8,08 | 2,97 | × 1,36 | 1,36 | 0,107 s |
| | 4 | 7,81 | 3,08 | × 1,41 | 2,61 | 0,175 s |
| vm3 (2 vCPU partagés) | 1 | 10,81 | 2,22 | — | 0,93 | 0,082 s |
| | 2 | 8,04 | 2,99 | × 1,35 | 1,41 | 0,108 s |
| | 4 | 8,38 | 2,86 | × 1,29 | 3,04 | 0,187 s |

\* Ralentissement = temps de calcul mesuré par le workload pendant le lot /
médiane du benchmark (tâche seule), moyenné sur les 24 tâches. Il est
légèrement inférieur à 1 avec 1 worker parce que le benchmark chronométrait
aussi le lancement du processus (quelques ms, non négligeable pour les
petites tâches), alors que `runtime_s` ne mesure que le calcul.

Écart entre les 3 répétitions : < 3 % dans presque tous les cas
(exception : vm3 avec 4 workers, une répétition à 9,8 s au lieu de 7,7 s).

### Constats

1. **Hypothèse confirmée : une VM à 2 vCPU ne vaut que ≈ 1,35 cœur.** Passer
   de 1 à 2 workers n'augmente le débit que de 35 % (et non de 100 %), car
   chaque tâche ralentit d'environ × 1,4. Avec 4 workers, plus aucun gain :
   les tâches se partagent le même cœur (ralentissement × 2,6 à × 3).
   Sur GCP, 1 vCPU = 1 hyperthread ; 2 vCPU = 1 cœur physique.
2. **HighCPU (4 vCPU = 2 cœurs) passe bien à l'échelle** : × 1,79 avec 2
   workers, × 2,40 avec 4.
3. **Nombre de workers retenu = nombre de vCPU** (2, ou 4 pour highcpu) :
   c'est le débit maximal ou quasi maximal partout. Au-delà, on ne gagne
   rien et le surcoût SSH augmente (0,08 → 0,19 s).
4. **Rapport débit / prix** (au nombre de workers retenu) :

   | VM | Débit (tâches/s) | Prix ($/h) | Débit par $/h |
   |---|---|---|---|
   | vm2 / vm3 | 2,97 / 2,99 | 0,0369 | **≈ 81** |
   | highcpu | 5,32 | 0,1088 | 49 |
   | vm1 | 2,66 | 0,0737 | 36 |

   **vm2/vm3 sont de loin les plus économiques, highcpu est la plus rapide,
   et vm1 est dominée** (même débit que vm2 pour 2 × le prix). C'est
   exactement le compromis coût/délai que doit arbitrer le scheduler.
5. **Le processeur d'une VM peut changer d'un démarrage à l'autre.** Pendant
   ce lot, vm1 a calculé `cpu_task` **2 × plus vite** que vm3, mais `sort_task`
   35 % plus lentement, `matrix_task` 20 % et `memory_task` 13 % plus
   lentement. Lors du benchmark (session précédente), vm1 et vm3 étaient
   identiques sur tout. **Hypothèse** : GCP a placé vm1 sur une autre famille
   de processeur (les VM e2 peuvent tourner sur plusieurs générations
   Intel/AMD). `gcloud ... cpuPlatform` affiche « Intel Broadwell » pour toutes
   les VM et ne permet pas de trancher.
   → L'orchestrateur et le benchmark **enregistrent désormais le modèle de
   processeur réel** (`/proc/cpuinfo`, colonne `cpu_model` de `<run>_vms.csv`)
   pour le vérifier à la prochaine session.
   → Conséquence pour le modèle : la durée prédite d'une tâche peut varier de
   ± 30 % selon le processeur obtenu. C'est une limite à présenter à l'oral.

### Conséquences pour le modèle du scheduler

- Durée prédite d'une tâche = médiane du benchmark × facteur de contention de
  la VM (≈ 1,4 pour une VM à 2 vCPU avec 2 workers, ≈ 1,3 pour highcpu avec 4)
  + surcoût SSH (≈ 0,1 s).
- Capacité d'une VM = nombre de workers (= vCPU), chacun vidant sa part de la file.

---

## 7. Décisions prises

| Décision | Raison |
|---|---|
| Compiler une fois en linux/amd64 et copier les binaires | Mac en ARM, VM en x86 ; rien à installer sur les VM |
| Tailles du benchmark = tailles de `data/tasks.csv` | Le scheduler lit directement la durée mesurée, sans extrapoler |
| Médiane plutôt que moyenne | Résiste à une exécution perturbée (vCPU partagés) |
| Une connexion SSH par VM, réutilisée | Surcoût par tâche divisé par 5 à 10 |
| Horodatage sur la machine de contrôle | Une seule horloge, aucune synchronisation entre VM |
| `overhead_s = durée mesurée − runtime_s du workload` | Sépare calcul et coût d'orchestration |
| Workers par VM = nombre de vCPU | Débit maximal mesuré (§ 6) ; au-delà, aucun gain |
| Attente de SSH au démarrage (~2 min max) | Une VM `RUNNING` n'accepte pas encore les connexions |
| Relevé du modèle de processeur à chaque lot | Le CPU d'une VM e2 peut changer entre deux démarrages (§ 6) |

---

## 8. Prochaines étapes

1. ~~Expérience « workers »~~ : faite (§ 6).
2. **Scheduler (étape 5)** : modèle de prédiction = médiane du benchmark ×
   facteur de contention + surcoût SSH mesuré (§ 6) ; implémenter MET, MCT,
   Min-Min, Max-Min et V8 (recherche locale qui minimise directement
   `S = 0,6·C/C_RR + 0,4·D/D_RR` sous `D ≤ 1,25·D_RR`).
3. **Expériences (étape 6)** : lots plus gros (makespan ≥ 60 s), RR vs V8,
   3 répétitions, erreur prédit vs réel.

## 9. Règles

- Ne jamais présenter une durée simulée comme mesurée.
- Toujours `./scripts/manage_vms.sh stop` après une session GCP.
- Aucun secret (clé, JSON de compte de service) dans le dépôt.
