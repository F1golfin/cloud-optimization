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
| 5a. Modèle + RR, MET, MCT, Min-Min, Max-Min | ✅ simulé | `scheduler/`, `data/tasks_*.csv`, `results/simulation/` |
| 5b. Notre algorithme select-maxmin | ✅ simulé | `scheduler/algorithms/select.go` (§ 8) |
| 6. Expériences réelles RR vs select-maxmin | ✅ mesuré | `scripts/exp_real.sh`, `results/exp_real/` (§ 9) |
| 7. Soutenance | ⬜ | |

---

## 2. Reprendre le projet (pour un nouveau membre)

```bash
git clone git@github.com:F1golfin/cloud-optimization.git && cd cloud-optimization
make                                      # workloads locaux + bin/orchestrator, bin/scheduler, bin/gentasks
make test                                 # tests du modèle et des algorithmes
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

## 7. Scheduler : comparaison des algorithmes classiques (SIMULATION)

> ⚠️ Toute cette section est **prédite par le modèle**, rien n'a été exécuté
> sur GCP. Les mesures réelles viendront à l'étape 6.

### Modèle (`scheduler/model`)

- Durée prédite : `p(i,j) = médiane benchmark × contention(j) + 0,10 s`, avec
  `contention` = 1,40 (vm1, vm2, vm3) et 1,32 (highcpu), colonne de
  `data/machines.csv` (§ 6).
- Simulation identique à l'orchestrateur : workers = vCPU, chaque tâche va au
  premier worker libre, dans l'ordre de l'allocation.
- `D` = fin de la dernière VM ; `C` = Σ max(60 s, temps de la VM) × prix, sur les
  VM utilisées ; `S = 0,6·C/C_RR + 0,4·D/D_RR` ; admissible si `D ≤ 1,25·D_RR`
  et RAM respectée.
- **Validation** : appliqué au lot RR réellement exécuté (§ 5), le modèle
  prédit 1,22 s pour 1,32 s mesurées (réel/prédit = 1,08). Le modèle naïf
  (médiane seule, sans contention ni SSH) se trompait d'un facteur 1,65.
  Ce contrôle est un test automatique (`TestPredictionMatchesRealRun`).

### Lots

Générés par `./bin/gentasks -n N -seed 42` : chaque tâche est tirée au hasard,
de façon uniforme, parmi les 12 combinaisons mesurées. Avec la même graine, tout
le monde obtient le même fichier. `data/tasks_{20,100,500,2000}.csv`.

### Résultats (`results/simulation/compare_*.csv`)

**500 tâches, 4 VM** — limite de délai 50,6 s :

| Algo | Coût ($) | Makespan (s) | S | Admissible | Tâches vm1/vm2/vm3/highcpu |
|---|---|---|---|---|---|
| RR | 0,004272 | 40,5 | 1,000 | oui | 125 / 125 / 125 / 125 |
| MET | 0,003657 | 51,9 | 1,026 | **non** | 43 / 0 / 92 / 365 |
| MCT | 0,004272 | 32,5 | 0,921 | oui | 100 / 120 / 104 / 176 |
| Min-Min | 0,004272 | 32,1 | 0,917 | oui | 100 / 96 / 100 / 204 |
| **Max-Min** | 0,004272 | **31,8** | **0,914** | oui | 100 / 96 / 100 / 204 |

**500 tâches, sans highcpu** — limite 68,1 s : tous les algorithmes font comme
RR (S entre 0,995 et 0,999 ; MET non admissible, 124,8 s). Les 3 VM restantes ont
la même vitesse et le même nombre de workers : RR, qui répartit le même nombre de
tâches tirées au hasard, est déjà presque équilibré.

**2000 tâches, 4 VM** — limite 220,5 s :

| Algo | Coût ($) | Makespan (s) | S | Admissible | Tâches vm1/vm2/vm3/highcpu |
|---|---|---|---|---|---|
| RR | 0,009591 | 176,4 | 1,000 | oui | 500 / 500 / 500 / 500 |
| MET | 0,009976 | 236,2 | 1,160 | **non** | 183 / 0 / 333 / 1484 |
| MCT | 0,009701 | 137,1 | 0,918 | oui | 419 / 478 / 423 / 680 |
| Min-Min | 0,009734 | 136,9 | 0,919 | oui | 398 / 387 / 398 / 817 |
| **Max-Min** | 0,009687 | **136,1** | **0,915** | oui | 397 / 388 / 398 / 817 |

**9 tâches (lot du TD3) avec l'allocation manuelle** — limite 1,52 s :

| Algo | Coût ($) | Makespan (s) | S | VM utilisées |
|---|---|---|---|---|
| RR | 0,004272 | 1,216 | 1,000 | 4 |
| MET | 0,002428 | 1,326 | 0,778 | 2 |
| MCT / Max-Min | 0,004272 | 1,211 | 0,998 | 4 |
| Min-Min | 0,004272 | 1,296 | 1,026 | 4 |
| **Manuelle (TD3)** | **0,002458** | 1,216 | **0,745** | 3 (sans highcpu) |

Temps de calcul du scheduler : < 40 ms pour tous les algorithmes, même à 2000 tâches.

### Constats

1. **Les heuristiques classiques optimisent le temps, pas notre objectif.**
   MCT, Min-Min et Max-Min réduisent le makespan d'environ 20 % (500 tâches) et
   22 % (2000 tâches), en envoyant plus de tâches sur highcpu (4 workers). Max-Min
   est le meilleur des algorithmes classiques dans tous les cas (S ≈ 0,91).
2. **Mais elles allument toujours les 4 VM.** Sous 60 s de lot, le coût ne dépend
   que des VM allumées : il reste identique à RR (0,004272 $). Sur 9 tâches,
   l'allocation manuelle du TD3 bat tous les algorithmes (S = 0,745) simplement
   parce qu'elle n'allume pas highcpu (qui n'existait pas encore au TD3).
3. **Au-delà de 60 s, équilibrer coûte un peu plus cher** (2000 tâches : +1 %) :
   avec RR, highcpu finit tôt et s'éteint ; équilibré, la VM la plus chère tourne
   plus longtemps.
4. **MET n'est jamais admissible sur les gros lots** : les VM ayant presque la
   même vitesse, choisir la plus rapide tâche par tâche empile tout sur une seule.
5. **Sans highcpu, il n'y a presque rien à gagner en répartition** : les VM
   restantes sont identiques. Cela contredit l'ancien rapport, où les gains sans
   HighCPU venaient de mesures de VM différentes.

### Ce que doit faire le nouvel algorithme (étape 5b)

Le gain restant ne vient pas de la répartition fine (Max-Min la fait déjà bien)
mais du **choix des VM à allumer** et de **sur quelle VM faire tourner le travail** :

- sous 60 s de lot, chaque VM allumée coûte sa minute : remplir la minute déjà
  payée avant d'en allumer une autre ;
- au-delà, chaque seconde coûte au prix de la VM : privilégier vm2/vm3 (le
  travail le moins cher), highcpu pour le délai, éviter vm1 (dominée, § 6).

→ C'est l'algorithme `select-maxmin` (§ 8).

---

## 8. Notre algorithme : `select-maxmin` (SIMULATION)

> ⚠️ Résultats **prédits par le modèle**. La vérification sur GCP est l'étape 6.

### Principe (`scheduler/algorithms/select.go`)

Deux VM du même type sont interchangeables (vm2 et vm3 sont deux e2-medium) :
on ne se demande pas « quelles VM allumer » mais **« combien de VM de chaque
type allumer »**.

1. **Choix du nombre de VM par type — exact.** On essaie toutes les
   combinaisons : e2-standard-2 (0–1) × e2-medium (0–2) × e2-highcpu-4 (0–1),
   sauf « aucune VM » = **11 combinaisons**.
2. **Répartition — heuristique.** Sur chaque combinaison, **Max-Min**, le
   meilleur algorithme classique (§ 7).
3. On garde le **meilleur S** parmi les combinaisons qui respectent
   `D ≤ 1,25·D_RR` et la RAM. Si aucune ne la respecte : Round Robin.

Propriétés :

- **Aucun nom de VM, type, prix ou durée dans le code** : tout vient de
  `machines.csv` et `vm_performance.csv`. Les mêmes lignes de code éteignent
  2, 1 ou 0 VM selon le lot.
- **Jamais pire que Max-Min** (dès qu'il est admissible) : la combinaison
  « toutes les VM » fait partie des essais. Testé (`TestSelectNeverWorseThanMaxMin`).
- **Passage à l'échelle** : `Π (n_t + 1) − 1` combinaisons, qui dépend du
  nombre de **types** et non du nombre de VM. 3 types × 10 VM = 1 330
  combinaisons (contre ~10⁹ en traitant chaque VM séparément). Temps de calcul
  mesuré : 114 ms pour 2000 tâches.
- **Justification affichable** : `./bin/scheduler -explain -tasks <lot>` affiche
  les 11 combinaisons triées par S.
- **Intégration** : `-algo select-maxmin` affiche aussi les VM à démarrer
  (`VMS="..." ./scripts/manage_vms.sh start`) : les VM non choisies restent éteintes.

### Résultats

| Lot | Max-Min (meilleur classique) | **select-maxmin** | Gain sur S | VM allumées |
|---|---|---|---|---|
| 9 tâches | 0,998 | **0,589** | −41 % | 2 (2 × e2-medium) |
| 500 tâches | 0,914 | **0,819** | −10 % | 3 (sans vm1) |
| 2000 tâches | 0,915 | **0,915** | 0 % | 4 (= Max-Min) |

**9 tâches** (avec l'allocation manuelle du TD3) :

| Algo | Coût ($) | Makespan (s) | S | VM |
|---|---|---|---|---|
| RR | 0,004272 | 1,216 | 1,000 | 4 |
| Max-Min | 0,004272 | 1,211 | 0,998 | 4 |
| Manuelle (TD3) | 0,002458 | 1,216 | 0,745 | 3 |
| **select-maxmin** | **0,001230** | 1,266 | **0,589** | 2 |

Coût −71 % par rapport à RR, −50 % par rapport à l'allocation manuelle, pour un
lot 4 % plus long.

**500 tâches** — `-explain` (limite de délai 50,6 s), extrait :

| VM allumées | Coût ($) | Makespan (s) | S | Admissible |
|---|---|---|---|---|
| **0 × std-2 + 2 × medium + 1 × highcpu** (choisi) | 0,003043 | 39,7 | **0,819** | oui |
| 1 × std-2 + 1 × medium + 1 × highcpu | 0,003657 | 39,7 | 0,906 | oui |
| 1 × std-2 + 2 × medium + 1 × highcpu (= Max-Min) | 0,004272 | 31,8 | 0,914 | oui |
| 0 × std-2 + 1 × medium + 1 × highcpu | 0,002428 | 52,8 | 0,862 | **non** |
| 1 × std-2 + 2 × medium + 0 × highcpu | 0,002458 | 53,8 | 0,877 | **non** |

Même temps que RR (39,7 s contre 40,5 s), coût −29 %. Deux combinaisons non
admissibles ont un meilleur S brut que la combinaison choisie : **sans la
contrainte de délai, l'algorithme choisirait un lot trop lent**. La contrainte
à 125 % joue donc réellement son rôle.

**2000 tâches** — `-explain` (limite 220,5 s), extrait :

| VM allumées | Coût ($) | Makespan (s) | S | Admissible |
|---|---|---|---|---|
| **1 × std-2 + 2 × medium + 1 × highcpu** (choisi) | 0,009687 | 136,1 | **0,9146** | oui |
| 0 × std-2 + 2 × medium + 1 × highcpu | 0,008610 | 169,7 | 0,9235 | oui |
| 1 × std-2 + 1 × medium + 1 × highcpu | 0,010349 | 169,8 | 1,0325 | oui |

### Constats

1. **Plus le lot est petit, plus le choix des VM rapporte** : sous 60 s, chaque
   VM inutile coûte sa minute complète.
2. **Sur un gros lot, éteindre vm1 ne vaut pas le coup**, alors que c'était notre
   intuition (estimation à la main : S ≈ 0,90). Au-delà de 60 s, le travail de vm1
   doit être fait ailleurs : on économise seulement 11 % de coût mais le lot dure
   25 % de plus ; avec 40 % de poids sur le temps, S se dégrade (0,9235 contre
   0,9146). vm1 est la moins rentable par dollar, mais elle ajoute de la capacité.
3. **L'écart est faible à 2000 tâches (0,009)** : le choix dépend des poids.
   Une entreprise plus attentive au coût (`-wcost 0,8`) devrait éteindre vm1 — à
   vérifier (analyse de sensibilité).
4. Mes estimations à la main étaient justes pour 9 et 500 tâches, fausses pour
   2000 : c'est précisément l'intérêt de calculer toutes les combinaisons plutôt
   que d'appliquer une règle intuitive.

---

## 9. Expériences réelles : RR contre select-maxmin sur GCP (MESURES)

Commande : `./scripts/exp_real.sh` (15/11/2026), puis
`LOTS="data/tasks_2000.csv" ./scripts/exp_real.sh`. Pour chaque lot : allocations
et prédictions du scheduler, puis **3 exécutions réelles de chaque algorithme,
alternées** (RR, select, RR, select…) pour qu'une dérive des VM dans le temps
touche les deux de la même façon. Les 4 VM étaient allumées (RR les utilise
toutes) ; select-maxmin n'a envoyé de tâches qu'aux VM qu'il avait choisies.
Coût = même règle que le modèle (fenêtre active par VM, min. 60 s).
S mesuré : référence = RR **mesuré** du même lot.
Fichiers : `results/exp_real/<lot>/` (`summary.csv`, `runs.csv`, une exécution
par fichier, allocations et prédictions).

### Synthèse

| Lot | Algo | Makespan mesuré (s) | Coût mesuré ($) | **S mesuré** | S prédit | VM utilisées |
|---|---|---|---|---|---|---|
| 9 tâches | RR | 1,12 ± 0,09 | 0,004272 | 1,000 | 1,000 | 4 |
| | **select-maxmin** | 1,38 ± 0,15 | **0,001230** | **0,664** | 0,589 | 2 (2 × e2-medium) |
| 500 tâches | RR | 52,2 ± 19,2 * | 0,004320 | 1,000 | 1,000 | 4 |
| | **select-maxmin** | 55,8 ± 16,9 * | **0,003093** | **0,857** | 0,819 | 3 (sans vm1) |
| 2000 tâches | RR | 333,6 ± 6,7 | 0,011819 | 1,000 | 1,000 | 4 |
| | **select-maxmin** | **254,3 ± 2,8** | 0,011453 | **0,886** | 0,915 | 4 (= Max-Min) |

\* écart-type élevé dû au ralentissement de vm3 (voir plus bas).

**select-maxmin bat RR en conditions réelles sur les trois lots.**

| Lot | Gain de coût | Effet sur le temps | Lecture |
|---|---|---|---|
| 9 tâches | **−71 %** | +23 % | 2 VM au lieu de 4 |
| 500 tâches | **−29 %** | ≈ égal (répétition 1 : 41,4 s contre 41,5 s) | vm1 éteinte |
| 2000 tâches | −3 % | **−24 %** | les 4 VM, meilleure répartition |

Répétitions de 500 tâches, une par une :

| Répétition | RR | select-maxmin |
|---|---|---|
| 1 | 41,4 s | 41,5 s |
| 2 | 40,8 s | 51,4 s ⚠️ vm3 |
| 3 | 74,3 s ⚠️ vm3 | 74,5 s ⚠️ vm3 |

Sur la répétition 1 (aucune anomalie), S mesuré ≈ 0,83 pour 0,819 prédit.

### Précision du modèle, VM par VM

Fin de chaque VM, mesurée / prédite (médiane des 3 répétitions) :

| Lot | Algo | highcpu | vm1 | vm2 | vm3 |
|---|---|---|---|---|---|
| 9 | RR | × 0,72 | × 0,89 | × 1,01 | × 0,93 |
| 9 | select | — | — | × 0,92 | × 1,12 |
| 500 | RR | × 1,02 | × 1,09 | × 0,92 | × 0,99 (puis 74 s) |
| 500 | select | × 1,05 | — | × 0,95 | × 1,30 |
| 2000 | RR | × 1,02 | × 1,09 | × 1,15 | **× 1,91** |
| 2000 | select | × 1,05 | × 1,15 | × 0,96 | **× 1,87** |

- **Hors vm3 dégradée, le modèle se trompe de −8 % à +15 %** (highcpu : +2 à +5 %
  partout). Sur 9 tâches, les écarts plus grands sont normaux : 2 ou 3 tâches par VM.
- Le makespan prédit dépend de la VM la plus lente : **une seule VM ralentie suffit à
  fausser la prédiction du lot** (× 1,9 sur 2000 tâches).

### Constat : vm3 a été ralentie d'un facteur ≈ 1,7–1,9

Même type que vm2 (e2-medium), même charge, même session, mais :

- 500 tâches : normale à la répétition 1, puis 51 s et 74 s au lieu de ≈ 40 s ;
- 2000 tâches : ralentie **pendant toute la session**, de façon très régulière
  (326, 337, 338 s pour RR contre 197 s sur vm2). Le **temps de calcul mesuré par les
  workloads eux-mêmes** a doublé : ce n'est pas le réseau, c'est le processeur.

**Hypothèses (non tranchées)** :

1. *Bridage des VM à cœur partagé* : une e2-medium peut utiliser ses 2 vCPU par
   rafales, mais en charge prolongée elle n'a droit qu'à l'équivalent d'environ
   1 vCPU ; avec 2 workers, chaque tâche irait 2 × moins vite (≈ ce qui est observé).
   Mais vm2, identique et autant chargée, n'a pas été ralentie.
2. *Machine physique de vm3 surchargée* (« voisin bruyant ») : cohérent avec un
   ralentissement constant limité à vm3.

**Test pour trancher** : arrêter / redémarrer vm3 (elle peut changer d'hôte) puis
`VMS="cloudopt-vm3" ./scripts/bench_vms.sh`.

### Ce qu'on en retient

1. **Le gain de select-maxmin se confirme en réel** : −71 % et −29 % de coût sur
   les petits lots, −24 % de temps sur le gros, S mesuré entre 0,66 et 0,89.
2. **Le modèle est fiable quand les VM tournent normalement** (≈ ±10 %), contre un
   facteur 3 dans l'ancien rapport.
3. **L'algorithme reste robuste à une VM ralentie** : même avec vm3 dégradée, il
   bat RR, en partie parce qu'il charge moins vm3 (398 tâches au lieu de 500).
4. **Limite principale : la variabilité du cloud.** Une VM « identique » peut être
   1,9 × plus lente, et un planning calculé à l'avance ne s'y adapte pas. Pistes :
   mesurer rapidement chaque VM juste avant de planifier ; ou laisser une VM qui
   a fini prendre des tâches restantes d'une VM lente (*work stealing*), au prix
   d'un écart au planning du scheduler.

---

## 10. Décisions prises

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
| Un seul modèle pour tous les algorithmes | Comparaison équitable ; validé sur le lot réel (§ 7) |
| Lots générés avec une graine fixe | Reproductibles par toute l'équipe |
| Faire varier le nombre de tâches, pas la durée des tâches | Couvre les régimes < 60 s et > 60 s sans refaire le benchmark |
| Ancienne V8 abandonnée, nouvel algorithme à concevoir | Les nouvelles mesures changent le problème (§ 7) |
| select-maxmin : combinaisons de **types** de VM, Max-Min sur chacune | Choix des VM exact et explicable ; passe à l'échelle avec le nombre de types (§ 8) |
| Expériences réelles alternées (RR, select, RR…) | Une dérive des VM pendant la session touche les deux algorithmes (§ 9) |
| S mesuré calculé par rapport à RR mesuré | Compare les deux algorithmes dans les mêmes conditions réelles |

---

## 11. Prochaines étapes

1. ~~Expérience « workers »~~ : faite (§ 6).
2. ~~Modèle + algorithmes classiques~~ : faits (§ 7).
3. ~~Notre algorithme select-maxmin~~ : fait (§ 8).
4. ~~Expériences réelles RR contre select-maxmin~~ : faites (§ 9).
5. *(Recommandé)* Relancer 500 tâches avec des VM « reposées » pour 3 mesures
   sans l'incident vm3 ; tester l'hypothèse sur vm3 (redémarrage + benchmark).
6. *(Optionnel)* Analyse de sensibilité : `-wcost 0.8` / `0.4` sur 500 et 2000 tâches.
7. **Soutenance** : rapport écrit, slides (schéma d'architecture, graphiques),
   démo en direct (9 tâches, select-maxmin : seules vm2 et vm3 allumées), vidéo
   de secours, répétition à trois.

## 12. Règles

- Ne jamais présenter une durée simulée comme mesurée.
- Toujours `./scripts/manage_vms.sh stop` après une session GCP.
- Aucun secret (clé, JSON de compte de service) dans le dépôt.
