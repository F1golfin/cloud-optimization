# Journal des résultats et état du projet

Document de passation pour l'équipe : où en est le projet, ce qui a été
**mesuré**, ce que l'on en conclut, et comment reprendre le travail.
Mis à jour à chaque étape. Dernière mise à jour : 16/11/2026.

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
| 6b. Expériences réelles, tous les algorithmes | ✅ mesuré | `results/exp_real_all/` (§ 9 bis) |
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
   Vérifié (analyse de sensibilité, simulation) : avec `-wcost 0.8`, select-maxmin
   éteint vm1 sur 2000 tâches (3 VM, 0,008610 $, 169,7 s) au lieu de garder les 4
   (0,009687 $, 136,1 s). Même code, décision différente selon les priorités.
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

### Constat : vm3 a été ralentie d'un facteur ≈ 1,7–1,9 (session 1)

Même type que vm2 (e2-medium), même charge, même session, mais :

- 500 tâches : normale à la répétition 1, puis 51 s et 74 s au lieu de ≈ 40 s ;
- 2000 tâches : ralentie **pendant toute la session**, de façon très régulière
  (326, 337, 338 s pour RR contre 197 s sur vm2). Le **temps de calcul mesuré par les
  workloads eux-mêmes** a doublé : ce n'est pas le réseau, c'est le processeur.

Deux hypothèses au départ : (1) *bridage des VM à cœur partagé* (une e2-medium
peut utiliser ses 2 vCPU par rafales, mais n'a droit en moyenne qu'à une part du
processeur) ; (2) *machine physique de vm3 défaillante* (« voisin bruyant »).
Pour trancher, nous avons **remplacé vm3** (suppression puis recréation : nouvel
identifiant, probablement un autre hôte) et refait des mesures.

### Investigation : remplacement de vm3 et sessions supplémentaires

**a) Nouvelle vm3, charge courte (≈ 2 min)** — `results/session_vm3_neuve/` :
benchmark proche du premier (+3 à +9 %) et expérience workers identique à vm2
(débit 2,16 / 2,94 / 3,07 tâches/s pour 1 / 2 / 4 workers, contre 2,00 / 2,85 /
3,09 pour vm2), sans dégradation sur 9 exécutions. Pendant cette session, c'est
**vm2** qui était un peu plus lente et plus irrégulière que d'habitude (cpu_task
+19 %, écarts-types de 3 à 10 % au lieu de < 1 %).

**b) Trois sessions de 2000 tâches** (makespan en secondes) :

| Session | VM3 | RR r1 / r2 / r3 | select-maxmin r1 / r2 / r3 |
|---|---|---|---|
| 1 (`tasks_2000`) | ancienne | 325,9 / 336,9 / 338,2 | 256,8 / 254,6 / 251,3 |
| 2 (`tasks_2000_session2`) | ancienne | 241,5 / 340,9 / 339,4 | 255,5 / 253,7 / 256,2 |
| 3 (`tasks_2000_vm3neuve`) | **nouvelle** | 241,4 / 347,6 / ~~437,3~~ † | 263,9 / 262,2 / **148,1** |

† *RR r3 de la session 3 est exclue* : le surcoût SSH a bondi en même temps sur les
4 VM (0,5 à 1,0 s par tâche au lieu de ≈ 0,11 s) alors que le temps de calcul de
highcpu était normal. Le problème venait de la machine de contrôle ou du réseau
(veille du Mac, Wi-Fi…), pas des VM.

**c) La nouvelle vm3 se comporte comme l'ancienne** sous charge prolongée : 1,5 à
2 × plus lente que vm2 pendant plusieurs exécutions (temps de calcul sur vm3 :
401 à 578 s contre ≈ 220 à 330 s sur vm2), **puis redevenue normale** à la
dernière exécution (148 s mesurées pour 136 s prédites, soit +9 %). vm2 a aussi eu
un épisode lent (172 s au lieu de 131 s en session 3, r1).

**Conclusion de l'investigation :** ce n'était **pas un hôte défaillant propre à
l'ancienne vm3**. C'est un comportement des **e2-medium à cœur partagé** : leur
vitesse varie fortement sous charge prolongée (plusieurs minutes), puis remonte.
**La documentation GCP confirme le mécanisme** (*Machines à usage général →
CPU bursting*) : chacun des 2 vCPU d'une e2-medium n'a droit qu'à **50 % du temps
processeur**, soit un vCPU au total ; au-delà, la VM fonctionne par **rafales**
(≈ 120 s à pleine charge). Privée de rafale, elle tourne donc ≈ 2 × moins vite,
ce que nous mesurons (× 1,7 à 1,9).

**Mais le mécanisme n'explique pas tout** : lors de la session « tous les
algorithmes » sur 2000 tâches (§ 9 bis), vm3 a été bridée dans **17 exécutions sur
18** alors que **vm2, même type et même charge, ne l'a jamais été**. Le bridage
dépend donc aussi de facteurs que nous ne contrôlons pas (probablement
l'occupation de la machine physique). highcpu a été stable dans toutes nos
mesures ; **vm1** (e2-standard-2, pas à cœur partagé) a en revanche ralenti lors
d'une session (47 à 66 s au lieu de ≈ 31 s sur 500 tâches, § 9 bis) : aléa
ponctuel, non expliqué par les rafales.

**Bilan sur 2000 tâches (8 paires d'exécutions valides)** : select-maxmin est plus
rapide dans **6 paires sur 8**. Dans les 2 autres, vm3 était moins bridée pendant
l'exécution de RR. Avec ces lots longs, **les deux makespans sont fixés par
l'e2-medium la plus bridée**, quel que soit l'algorithme : la variabilité de ces VM
est du même ordre de grandeur que le gain de l'algorithme.

### Ce qu'on en retient

1. **Le gain de select-maxmin se confirme en réel sur les petits lots** : −71 % et
   −29 % de coût (9 et 500 tâches). **Sur 2000 tâches, le gain est réel mais
   bruité** : plus rapide dans 6 paires d'exécutions sur 8.
2. **Le modèle est fiable quand les VM tournent normalement** (≈ ±10 %, par exemple
   148 s mesurées pour 136 s prédites), contre un facteur 3 dans l'ancien rapport.
   Les écarts × 1,6 à × 1,9 viennent du bridage des e2-medium, que le modèle,
   calibré sur des mesures courtes, ne connaît pas.
3. **L'algorithme est plus prévisible que RR** sur 2000 tâches (sessions 1 et 2 :
   251 à 257 s contre 241 à 341 s), en partie parce qu'il charge moins vm3
   (398 tâches au lieu de 500).
4. **Limite principale : la variabilité des VM à cœur partagé.** Les e2-medium sont
   les moins chères, mais imprévisibles en charge prolongée ; un planning calculé à
   l'avance ne s'y adapte pas. Pistes : donner deux vitesses aux e2-medium dans le
   modèle (pleine vitesse pendant la rafale, ≈ moitié ensuite) ; mesurer la vitesse
   réelle pendant l'exécution et rééquilibrer (*work stealing*), au prix d'un écart
   au planning du scheduler.

---

## 9 bis. Expériences réelles : tous les algorithmes (MESURES)

Commandes (16/11/2026, même protocole qu'au § 9 : 3 exécutions alternées par
algorithme, S mesuré par rapport à RR mesuré du même lot) :

```bash
OUT=results/exp_real_all ALGOS="rr met mct minmin maxmin select-maxmin manual" \
    LOTS="data/tasks.csv" ./scripts/exp_real.sh
OUT=results/exp_real_all ALGOS="rr met mct minmin maxmin select-maxmin" \
    LOTS="data/tasks_500.csv" ./scripts/exp_real.sh
OUT=results/exp_real_all ALGOS="rr met mct minmin maxmin select-maxmin" \
    LOTS="data/tasks_2000.csv" ./scripts/exp_real.sh   # ≈ 1 h 30
```

`manual` = l'allocation faite à la main au TD3 (`data/allocation_manual.csv`),
valable seulement pour le lot de 9 tâches. Fichiers : `results/exp_real_all/<lot>/`
(`summary.csv`, `runs.csv`, une exécution par fichier ; log du lot de 2000 :
`tasks_2000/run.log`).

| Lot | Algo | Makespan mesuré (s) | Coût mesuré ($) | **S mesuré** | S prédit |
|---|---|---|---|---|---|
| 9 tâches | **select-maxmin** | 1,20 ± 0,05 | **0,001230** | **0,541** | 0,589 |
| | manuelle (TD3) | 1,25 ± 0,13 | 0,002458 | 0,731 | 0,745 |
| | MET | 1,37 ± 0,04 | 0,002428 | 0,762 | 0,778 |
| | Min-Min | 0,87 ± 0,03 | 0,004272 | 0,868 | 1,026 |
| | Max-Min | 1,08 ± 0,07 | 0,004272 | 0,931 | 0,998 |
| | MCT | 1,10 ± 0,06 | 0,004272 | 0,937 | 0,998 |
| | RR | 1,30 ± 0,30 | 0,004272 | 1,000 | 1,000 |
| 500 tâches | **select-maxmin** | 65,5 ± 9,3 | **0,003118** | **0,864** | 0,819 |
| | Min-Min | 46,7 ± 12,2 | 0,004272 | 0,899 | 0,917 |
| | Max-Min | 47,6 ± 9,0 | 0,004272 | 0,904 | 0,914 |
| | MCT | 51,8 ± 17,0 | 0,004315 | 0,938 | 0,921 |
| | RR | 60,3 ± 20,1 | 0,004353 | 1,000 | 1,000 |
| | MET | 73,1 ± 18,6 | 0,003820 | 1,011 | 1,026 |
| 2000 tâches ‡ | **select-maxmin** | 162,6 ± 2,6 | 0,007263 | **0,969** | 0,915 |
| | MCT | 165,4 ± 1,2 | 0,007204 | 0,970 | 0,918 |
| | Min-Min | 160,6 ± 3,1 | 0,007360 | 0,973 | 0,919 |
| | Max-Min | 164,3 ± 2,1 | 0,007318 | 0,977 | 0,915 |
| | RR | 182,4 ± 60,4 | 0,007117 | 1,000 | 1,000 |
| | MET | 230,3 ± 14,5 | 0,008019 | 1,181 | 1,160 |

‡ **Processeur différent** : pour ce lot, GCP a démarré les 4 VM sur **AMD EPYC
7B12** au lieu d'**Intel Xeon @ 2.20GHz** (toutes les autres sessions, relevé dans
la colonne `cpu_model` des `*_vms.csv`). Sur AMD, les VM non bridées sont ≈ 35 %
plus rapides (highcpu, 817 tâches : 92–93 s contre 142–145 s sur Intel). Les durées
ne sont donc **pas comparables** aux autres sessions ; les algorithmes, eux, sont
comparés entre eux dans les mêmes conditions.

### Constats

1. **Sur 9 et 500 tâches, select-maxmin a le meilleur score mesuré**, devant les
   cinq heuristiques et devant l'allocation manuelle : il éteint des VM, ce que les
   autres ne font pas. Sur 500 tâches son lot est plus long (vm3 bridée sur 2 de ses
   3 exécutions, 71 s au lieu de ≈ 40 s), mais l'économie de 28 % compense.
2. **Sur 2000 tâches, select-maxmin = Max-Min** : il garde les 4 VM, donc la même
   allocation (817 / 397 / 388 / 398 tâches). L'écart 0,969 / 0,977 est du bruit ;
   MCT et Min-Min sont aussi à moins de 1 %. On écrit « à égalité ». Tous battent RR
   et MET.
3. **RR a l'écart-type le plus grand** sur 2000 tâches : r1 = 112,8 s (vm3 pas encore
   bridée, meilleure exécution de toutes), puis 212,8 s et 221,5 s.
4. **Le classement mesuré suit le classement prédit**, sauf Min-Min sur 9 tâches
   (prédit le plus lent, en réalité le plus rapide) : le modèle applique la
   contention (× 1,4) à toute VM utilisée, même quand elle n'exécute qu'une tâche à
   la fois. Il est trop pessimiste pour les VM peu chargées.
5. **vm3 bridée, vm2 jamais** (2000 tâches) : 17 exécutions sur 18 avec vm3 lente
   (≈ 160 s contre ≈ 90 s pour vm2 sur le même nombre de tâches) ; seule RR r1 était
   normale. Sur 500 tâches, vm1 a aussi ralenti sur plusieurs r3 (48 à 66 s au lieu
   de ≈ 31 s).
6. **Le modèle est calibré sur Intel** : sur AMD il surestime les durées des VM non
   bridées (≈ × 0,65). Réel/prédit ≈ 1,2 sur 2000 tâches uniquement parce que vm3
   bridée compense. Piste : benchmark par plateforme, ou mesure des VM juste avant
   de planifier.

### Écart à l'optimum (SIMULATION, 9 tâches)

`./bin/scheduler -compare -tasks data/tasks.csv` inclut maintenant `optimal`
(`scheduler/algorithms/optimal.go`) : recherche exhaustive des 4⁹ = 262 144
affectations, avec le meilleur ordre d'exécution sur chaque VM (78 ms).
Testé contre une force brute naïve (affectations × permutations).

| Algo | Coût ($) | Makespan (s) | S prédit | VM |
|---|---|---|---|---|
| optimal | 0,001230 | 1,257 | **0,5865** | vm2:6 vm3:3 |
| select-maxmin | 0,001230 | 1,266 | 0,5893 | vm2:5 vm3:4 |
| allocation manuelle | 0,002458 | 1,216 | 0,7453 | vm1:3 vm2:3 vm3:3 |

select-maxmin choisit **les mêmes VM** que l'optimum ; il n'est qu'à **0,5 %**
de S (9 ms de makespan), écart bien inférieur au bruit de mesure (± 50 ms).
Inutile de l'exécuter sur GCP : la différence ne serait pas mesurable.

### Diagramme temporel (MESURE)

`./bin/gantt` trace les exécutions réelles (début / fin de chaque tâche relevés
par l'orchestrateur) : `docs/rapport/figures/gantt_9.tex` (RR r1 et
select-maxmin r1 sur 9 tâches, `results/exp_real_all/tasks/`).

### Sensibilité aux poids (SIMULATION, `-explain -wcost 0.8`, 2000 tâches)

| Poids coût / temps | Combinaison choisie | Coût ($) | Makespan (s) |
|---|---|---|---|
| 60 / 40 | 4 VM | 0,009687 | 136,1 |
| 80 / 20 | 3 VM (sans vm1) | 0,008610 | 169,7 |

---

## 9 quater. Modèle V2 « par caractéristiques » (VALIDATION SUR MESURES RÉELLES)

**Critique reçue à l'oral (17/11/2026)** : le modèle est calibré sur les VM
mêmes qui servent aux tests (risque de surapprentissage) ; il ne sait pas
prédire une VM ou un groupe de VM jamais mesuré.

**Modèle V2** (`-ref cloudopt-vm2`) : seule vm2 est mesurée (benchmark +
contention). vm1, vm3 et highcpu sont prédites **uniquement à partir de leur
fiche** (`machines.csv` : vCPU = workers, RAM, prix, `speed` = 1 pour la famille
e2). Pour V2, les trois autres VM sont donc **hors échantillon**.

**Validation** (`./bin/modelcheck`, sans nouvelle exécution GCP) : fin prédite /
fin mesurée de chaque VM, sur les 315 (exécution, VM) des dossiers
`results/exp_real/` et `results/exp_real_all/`.

| Sous-ensemble | n | V1 : médiane mesuré/prédit, erreur moyenne | V2 : médiane, erreur moyenne |
|---|---|---|---|
| Lots 500/2000, VM non bridées, Intel | 131 | 1,04 · 12,4 % | 0,98 · **11,4 %** |
| — e2-medium | 56 | 0,98 · 15,4 % | 0,97 · 14,8 % |
| — e2-standard-2 (vm1, hors échantillon en V2) | 33 | 1,09 · 14,5 % | 1,06 · 13,5 % |
| — e2-highcpu-4 (hors échantillon en V2) | 42 | 1,04 · 6,7 % | 0,98 · 5,3 % |
| Lots de 9 tâches | 90 | 0,91 · 14,3 % | 0,87 · 16,6 % |
| Session AMD (2000), VM non bridées | 55 | 0,67 · 33 % | 0,65 · 35 % |
| VM bridées (pour info) | 39 | 1,78 · 61 % | 1,72 · 57 % |
| Makespan par exécution (Intel, sans VM bridée) | 17 | 1,21 · 29,9 % | 1,14 · 26,1 % |

**Décisions de select-maxmin** : identiques en V1 et en V2 (mêmes VM choisies,
répartitions à quelques tâches près) sur les lots de 9, 200, 500 et 2000 tâches.

**Constats.**
1. **Prédire une VM jamais mesurée à partir de sa fiche est aussi précis que de
   la mesurer** : les VM hors échantillon (vm1, highcpu) sont même un peu mieux
   prédites en V2. Le modèle ne colle donc pas aux particularités des VM
   mesurées ; ce qui compte, c'est le type (vCPU, famille).
2. Ce résultat repose sur un fait mesuré : **dans la famille e2, la vitesse par
   tâche ne dépend pas du type** (±5 %). Pour une autre famille (n2, c3, ARM…),
   le facteur `speed` doit venir des scores publiés par Google ou d'un
   mini-benchmark : **non testé ici**.
3. Les erreurs résiduelles ne viennent pas du choix V1/V2 mais des VM elles-mêmes
   (bridage des e2-medium, processeur AMD choisi par Google) : elles sont les
   mêmes dans les deux modèles.

**Limite du test** : pas de nouvelle exécution sur un groupe de VM recréé ; la
validation réutilise les exécutions passées (mais avec un modèle qui n'a vu
qu'une seule des quatre VM).

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
| Lot de 2000 « tous algorithmes » gardé malgré le passage sur AMD | Tous les algorithmes ont tourné dans les mêmes conditions ; signalé partout (§ 9 bis) |

---

## 11. Prochaines étapes

1. ~~Expérience « workers »~~ : faite (§ 6).
2. ~~Modèle + algorithmes classiques~~ : faits (§ 7).
3. ~~Notre algorithme select-maxmin~~ : fait (§ 8).
4. ~~Expériences réelles RR contre select-maxmin~~ : faites (§ 9).
5. ~~Investigation vm3~~ : faite (§ 9) — rafales des e2-medium (doc GCP), pas un
   hôte défaillant ; reste inexpliqué : pourquoi vm3 et pas vm2.
6. ~~Analyse de sensibilité~~ : faite (`-wcost 0.8`, § 9 bis).
6 bis. ~~Comparaison réelle de tous les algorithmes~~ : faite (§ 9 bis), intégrée au
   rapport LaTeX.
7. **Soutenance** : rapport écrit, slides (schéma d'architecture, graphiques),
   démo en direct (9 tâches, select-maxmin : seules vm2 et vm3 allumées), vidéo
   de secours, répétition à trois.

## 12. Règles

- Ne jamais présenter une durée simulée comme mesurée.
- Toujours `./scripts/manage_vms.sh stop` après une session GCP.
- Aucun secret (clé, JSON de compte de service) dans le dépôt.
