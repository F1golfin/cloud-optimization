# Contexte du projet : synthèse au 14/11/2026

Ce document résume :

1. le sujet du projet et le barème ;
2. ce que demande chaque TD ;
3. le cours (CM1 à CM6) ;
4. l'analyse de l'ancien projet, codé par un camarade avec une IA ;
5. les décisions prises et la suite.

Les PDF originaux sont dans `docs/`. Ils restent en local et sont exclus du dépôt par `.gitignore`.

---

## 1. Le sujet (`docs/Projet_Semestriel.pdf`)

**Question centrale :** étant donné un ensemble de tâches et de VM, sur quelle VM exécuter chaque tâche ?

- Équipe de **3**. Chacun doit pouvoir expliquer tout le projet.
- Langage principal **compilé** : Go, C, C++ ou Rust. Le shell est autorisé pour les scripts d'aide (benchmark, déploiement).
- GCP Compute Engine, piloté avec `gcloud`. Kubernetes et les conteneurs ne sont pas requis.
- **Méthode d'optimisation libre** : heuristique, méthode exacte, solveur ou algorithme maison. Elle doit être :
  - expliquée ;
  - reproductible ;
  - reliée au cours ;
  - capable de produire une allocation valide ;
  - comparée à une référence (*baseline*).
- Le projet sépare 4 concepts :
  - **Workload** : un programme compilé ;
  - **Infrastructure** : les VM ;
  - **Scheduler** : il décide où chaque tâche s'exécute ;
  - **Orchestrateur** : il lance les tâches et collecte les résultats.
- Flux : `tâches + machines + mesures → SCHEDULER → allocation.csv → ORCHESTRATEUR → VM1/VM2/VM3 → results.csv`

**Livrables :**

- un dépôt propre ;
- les instructions pour compiler et exécuter ;
- les fichiers de configuration (tâches, machines) ;
- les résultats de benchmark ;
- le scheduler ;
- l'orchestrateur ;
- les allocations manuelle et optimisée ;
- les résultats d'exécution finaux ;
- un **rapport technique** ;
- une **présentation orale avec démo**.

**Notation :** 100 % à la soutenance du TD6. Les TD1 à TD5 ne sont pas notés. La soutenance dure 12 à 15 min de présentation (démo comprise), suivies de 8 à 10 min de questions. Les 3 membres parlent.

### Barème /20 (1 point par critère)

| Bloc | Critères |
|---|---|
| Cadrage et architecture (4) | définition du problème ; modèle des tâches et des VM ; clarté de l'architecture ; choix justifiés (compromis) |
| Exécution distribuée (4) | déploiement GCP ; exécution réellement parallèle sur plusieurs VM ; lancement et collecte des résultats ; qualité et reproductibilité |
| Ordonnancement et optimisation (5) | formulation (entrées, variables) ; contraintes ; objectif ; justification reliée au cours ; allocation **directement exécutable** par l'orchestrateur |
| Preuves et analyse (4) | référence ; métriques cohérentes ; comparaison chiffrée référence contre optimisé ; interprétation (erreurs de prédiction, variabilité, limites) |
| Soutenance (3) | clarté et respect du temps ; démo et visuels ; maîtrise par les 3 membres |

> « A sophisticated optimization method does not compensate for a non-functional distributed system… **Results must be supported by actual measurements.** »

---

## 2. Les TD

| TD | Objectif | Ce qu'il faut avoir à la fin |
|---|---|---|
| **TD1**, local | Préparer Linux, compiler les 4 workloads, caractériser la machine, écrire un script de benchmark automatique | `data/machine_local.txt`, `scripts/benchmark.sh`, `data/benchmark_local.csv` (format `workload,size,run,runtime_s,max_rss_kb`, mesures **brutes**, 3 tailles × 3 répétitions), un README et une analyse |
| **TD2**, Compute Engine | Comprendre projet, région, zone, type de machine, image, disque, IP, pare-feu, SSH, scp, cycle de vie (stop ≠ delete) | 1 VM au moins, `data/machine_cloud_vm1.txt`, `data/benchmark_cloud_vm1.csv`, une comparaison local / cloud, les VM arrêtées |
| **TD3**, orchestration | 3 VM, mêmes binaires partout, `allocation_manual.csv` (`task_id,workload,size,vm`), `run_task.sh` (1 tâche → 1 VM → `results/Tx.out/.err`), `run_all.sh` (lance chaque ligne avec `&`, puis **un seul `wait`**) | Toutes les tâches lancées indépendamment ; **plusieurs tâches en même temps sur la même VM** ; échecs visibles |
| **TD4**, Round-Robin | Séparer `tasks.csv` et `machines.csv` ; RR compilé (`scheduler/round_robin.go`, code fourni par le prof) → `allocation_rr.csv` ; `run_all.sh` prend l'allocation en argument ; cas de test A–E | RR fonctionnel, allocation valide, tests, exécution distribuée de l'allocation RR, paragraphe « pourquoi le RR ne suffit pas » |
| **TD5**, optimisation II | Améliorer la stratégie, intégrer le compromis performance / coût, brancher le scheduler sur la chaîne d'exécution | Pour la séance : **fonction objectif + avancement + résultats préliminaires** |
| **TD6** | Soutenance | Comparaison réelle référence contre optimisé, rapport, slides, démo |

### ⚠️ Règles importantes des TD3 et TD4

- **« Do not create one sequential queue per VM »** : chaque tâche est un SSH indépendant lancé avec `&`, et les tâches d'une même VM tournent **en même temps**. C'est l'OS de la VM qui partage le CPU et la RAM.
- **Le Round-Robin est obligatoire comme référence, mais n'est pas une méthode d'optimisation.** Le renommer ne compte pas.
- Garder le format `task_id,workload,size,vm` : l'orchestrateur ne doit pas changer.
- Ne pas allouer plus de RAM qu'une VM ne peut en supporter quand plusieurs `memory_task` tournent ensemble.
- Arrêter ou supprimer les VM inutilisées.

---

## 3. Le cours

| CM | Idée clé | Lien avec le projet |
|---|---|---|
| **CM1–CM2** | VM, région / zone, cycle de vie ; **coût = taille × temps allumé** ; une VM plus grosse va plus vite mais coûte plus cher | Le compromis coût / temps |
| **CM3** | Formuler : **décision** (quelle VM), **objectif** (makespan, temps moyen, utilisation, coût), **contraintes** (une tâche une fois, RAM). Heuristiques : RR, FIFO, SPT, LPT, Min-Min, Max-Min, MET, MCT, First / Best Fit. Exemple : RR 18 min, LPT 17, optimum 15 | Votre méthode principale (glouton) et les comparaisons |
| **CM4** | Méthodes exactes. **ILP** : x_ij ∈ {0,1}, Σ_j x_ij = 1, min T avec T ≥ L_j. **Bornes inf / sup**, *branch and bound*, statuts OPTIMAL / FEASIBLE / INFEASIBLE. CSP / CP-SAT. OR-Tools | Bonus : calculer l'optimum pour mesurer l'écart |
| **CM5** (à venir) | Recherche locale : déplacements (une tâche change de VM) et échanges (deux tâches s'inversent) ; optimum local ; recuit simulé, tabou, multi-départ | Bonus : améliorer le glouton |
| **CM6** | Scheduling ≠ placement ≠ exécution. Cycle **filtrer → noter → assigner → exécuter → observer** (comme Kubernetes). Concurrence (même VM) ≠ parallélisme (VM différentes). Le modèle « une tâche à la fois » des CM3 / CM4 n'est **pas** celui du projet, d'où la boucle **estimer → choisir → exécuter → mesurer → ajuster**. Au moins **3 runs**. Problème offline / statique | La validation par la mesure ; énoncer les hypothèses ; un compromis raisonnable est accepté |

Le détail de ce que le prof a dit à l'oral est dans `docs/NOTES_CM6_ORAL.md`. Deux points nouveaux :

- il propose un score normalisé entre 0 et 1 ;
- quand aucune VM n'a assez de place, une tâche peut rester **en attente** : « Running tasks in batches is a valid option for your project ».

Message du prof au CM6 : énoncer clairement les hypothèses. Un compromis est acceptable s'il est raisonnable (« moins cher et un peu plus lent », oui ; « 1 h au lieu de 1 min », non).

---

## 4. L'ancien projet (camarade + IA)

Il se trouve dans `/Users/steven/Documents/ESILV/cloud/Cloud_Optimization_V8_Projet_Complet_20261110_1809/cloud-optimization`. Le superflu y a été rangé dans `archive/` le 14/11, et une sauvegarde intacte se trouve dans le dossier voisin `cloud-optimization_SAUVEGARDE_originale`.

### Architecture

```
workloads/*.go              4 tâches (cpu, memory, matrix, sort)
scheduler/round_robin.go    RR du TD4
scheduler/optimization/
  weighted_greedy_v6.go     optimiseur principal (726 lignes)
  compare_algorithms.go     RR, MET, MCT, Min-Min, Max-Min, V4 (860 lignes ; les 260 premières sont identiques à la V6)
scripts/run_task.sh, run_all.sh         orchestration TD3 (1 SSH par tâche, & + wait)
scripts/test_round_robin_20.sh          run complet GCP : start → scp → exécution → récupération → stop (trap)
scripts/remote_batch_runner.py          sur la VM : pool de N workers ⚠️ = file par VM, contraire au TD3/TD4
scripts/benchmark_cloud.py, benchmark_vm*.sh   benchmark (5 répétitions)
data/tasks_20.csv, machine_specs.csv, machine_prices.csv, benchmark_measured_catalog.csv
results/experiments/        les seules mesures réelles : RR 20 tâches (1 run, ~5,5 s) et highcpu seule
```

VM :

| VM | Type | CPU / RAM | Prix |
|---|---|---|---|
| vm1 | e2-standard-2 | 2 vCPU / 8 Go | 0,0737 $/h |
| vm2, vm3 | e2-medium | 2 vCPU / 4 Go | 0,0369 $/h |
| highcpu | e2-highcpu-4 | 4 vCPU / 4 Go | 0,1088 $/h |

Zone europe-west1-b, projet `cloud-optimization-511107`.

### Comment fonctionne la V6

1. `predict` (l.112) estime la durée de chaque couple (tâche, VM) à partir des benchmarks.
2. `evaluate` (l.186) simule une file par vCPU, vérifie que la RAM reste ≤ 85 %, calcule le coût = prix × max(60 s, durée) pour chaque VM utilisée, puis le score = 0,6·C/C_RR + 0,4·D/D_RR.
3. Le RR sert de référence ; la limite de temps vaut D ≤ 1,25 × D_RR.
4. **Glouton** (l.440) : les tâches sont prises dans l'**ordre du fichier, sans tri**, et chacune va sur la VM au meilleur score. C'est un « MCT avec le score 60/40 ».
5. Recherche locale : déplacements (l.465), extinction d'une VM (l.487), échanges (l.548).
6. ⚠️ Dernière étape (l.594) : elle **change d'objectif**. Elle minimise le temps seul, avec un budget de coût ×1,10, et ne suit donc plus le 60/40.
7. Elle plante sans `-performance data/observed_vm_performance_measured.csv`.

**Résultat (modèle, 20 tâches) :**

| | Coût | Makespan | Score |
|---|---|---|---|
| RR | 0,00246 $ | 3,08 s | 1,000 |
| Glouton seul | 0,00062 $ | 8,12 s | 1,204 |
| V6 | 0,00246 $ | 2,27 s | 0,895 |

À cause du minimum de 60 s, le glouton empile tout sur la VM la moins chère ; c'est la recherche locale qui rattrape. Au final, la V6 coûte autant que le RR : le gain vient uniquement du temps.

### Bilan de l'ancien projet

- ✅ TD1 à TD4 corrects, orchestration propre, VM arrêtées automatiquement.
- ❌ vm3 jamais benchmarkée (copie de vm2).
- ❌ **Aucun run optimisé réel.**
- ❌ Tâches trop courtes (~0,2 s, contre 2 à 5 s pour un SSH).
- ❌ Erreur de prédiction ×2,8 non analysée.
- ❌ 8 versions empilées, pas de README TD4/TD5, pas de rapport.

---

## 5. Décisions prises et suite

### Objectif

```
min  0,6 · Coût/Coût_RR + 0,4 · Makespan/Makespan_RR
s.c. chaque tâche est placée sur exactement une VM
     RAM utilisée ≤ 85 % de la RAM de la VM
     Makespan ≤ 1,25 · Makespan_RR
```

### Méthode

- **Méthode principale** : un glouton clair (filtrer → noter → assigner, CM3 + CM6), que les 3 membres savent expliquer.
- **Bonus facultatifs** :
  - recherche locale multi-départ depuis les heuristiques du cours (CM5) ;
  - ILP OR-Tools pour l'écart à l'optimum (CM4).
- Les déplacements et échanges se font **dans le calcul**, avant l'exécution. On ne déplace jamais une tâche en cours sur une VM.

### Priorités

Les **mesures réelles** passent en premier :

- 3 runs RR + 3 runs optimisés, avec le même orchestrateur ;
- coût réel = prix × durée ;
- analyse de l'écart entre prédit et mesuré.

Avant les mesures finales :

- **allonger les tâches** pour qu'elles durent 10 à 30 s ;
- **refaire le benchmark** des 4 VM avec ces nouvelles tailles.

### Points à trancher dans ce dépôt

- `data/machines.csv` liste 3 VM, mais highcpu a été benchmarkée : faut-il 3 ou 4 VM ?
- Le README prévoit un orchestrateur avec « une file par VM », **contraire aux TD3 et TD4**. Il faut garder l'exécution concurrente (`&` + `wait`), ou demander l'accord du prof au TD5.

**Toujours demander avant toute action GCP : cela coûte de l'argent.**
