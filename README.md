# Cloud Optimization – Distributed Task Scheduling on GCP

Projet semestriel ESILV A5 CCC (équipe de 3).

**Question centrale :** étant donné un lot de tâches et un ensemble de VM,
sur quelle VM exécuter chaque tâche pour minimiser le coût sans trop
dégrader le délai ?

## Architecture

```
tasks.csv + machines.csv + vm_performance.csv (mesures réelles)
                    │
                    ▼
               SCHEDULER  (RR, MET, MCT, Min-Min, Max-Min, V8)
                    │
                    ▼
             allocation.csv   (task_id,workload,size,vm)
                    │
                    ▼
             ORCHESTRATEUR  (une file par VM, exécution via SSH, en parallèle)
          ┌─────────┼─────────┬──────────┐
          ▼         ▼         ▼          ▼
         vm1       vm2       vm3      highcpu
          └─────────┴────┬────┴──────────┘
                         ▼
                    results.csv   (task_id,vm,start,end,duration,exit_code)
```

| Composant | Rôle | Emplacement |
|---|---|---|
| Workloads | Programmes Go compilés (CPU, mémoire, matrice, tri) | `workloads/` |
| Infrastructure | 4 VM Compute Engine, europe-west1-b | `scripts/` |
| Scheduler | Décide la VM de chaque tâche | `scheduler/` |
| Orchestrateur | Lance les tâches et collecte les résultats | `orchestrator/` |

## Organisation du dépôt

```
workloads/<nom>/main.go   les 4 workloads de référence (un dossier par programme)
orchestrator/             l'orchestrateur (exécution sur les VM via SSH)
scheduler/model/          modèle commun : prédiction, simulation, coût, score S
scheduler/algorithms/     RR, MET, MCT, Min-Min, Max-Min, select-maxmin, optimal
scheduler/cmd/            programmes scheduler, gentasks et gantt
scripts/                  scripts gcloud (création/arrêt des VM, déploiement)
data/                     entrées : tâches, machines, allocations, mesures
results/                  sorties : benchmarks, résultats d'exécution
docs/                     énoncé et rapport (PDF, non versionnés)
bin/                      binaires compilés (non versionnés, `make` les recrée)
```

## Prérequis

- Go ≥ 1.22 (`go version`)
- Google Cloud SDK (`gcloud`) authentifié sur le projet, uniquement pour les étapes GCP
- `make`

## Compilation

```bash
make          # binaires pour la machine locale -> bin/local/
make linux    # binaires pour les VM GCP (linux/amd64) -> bin/linux-amd64/
make vet      # vérification statique
make test     # tests automatiques (modèle et algorithmes)
```

Les VM GCP `e2-*` sont en **x86_64** : il faut leur envoyer les binaires de
`bin/linux-amd64/`, pas ceux de `bin/local/` (ARM sur Mac Apple Silicon).

## Exécution d'un workload

```bash
./bin/local/cpu_task --size 500000
./bin/local/memory_task --size 256
./bin/local/matrix_task --size 400
./bin/local/sort_task --size 5000000
```

Chaque programme affiche une ligne JSON, par exemple :

```json
{"task":"cpu_task","size":500000,"runtime_s":0.241,"primes_found":41538}
```

## Benchmark

Le même script mesure les workloads en local et sur chaque VM :
4 workloads × 3 tailles (celles de `data/tasks.csv`) × 5 répétitions,
avec une exécution de chauffe non mesurée avant chaque taille.

```bash
make && ./scripts/benchmark.sh -m local        # -> results/benchmark_local.csv
```

Format : `machine,workload,size,run,runtime_s,max_rss_kb`. La colonne `machine`
permet de fusionner les fichiers de toutes les VM en un seul tableau pour le
scheduler. Le benchmark local a été fait sous Linux (VM OrbStack, Ubuntu arm64).

## Infrastructure GCP

```bash
./scripts/manage_vms.sh create   # crée / corrige les VM de data/machines.csv
./scripts/manage_vms.sh start
./scripts/deploy.sh              # make linux + copie des binaires sur chaque VM
./scripts/bench_vms.sh           # benchmark sur chaque VM -> data/vm_performance.csv
./scripts/manage_vms.sh stop     # TOUJOURS à la fin
```

`data/machines.csv` est le **seul endroit** où les VM sont décrites (nom, type GCP,
vCPU, RAM, prix horaire, contention). Il est lu par le scheduler (modèle) et par
les scripts (`scripts/config.sh` : création, démarrage, arrêt, déploiement).
Ajouter, retirer ou changer le type d'une VM = modifier une ligne de ce fichier.
`scripts/config.sh` ne contient plus que le projet GCP et la zone.

## Orchestrateur

```bash
make                                   # construit bin/orchestrator
./bin/orchestrator -alloc data/allocation_rr.csv -out results/run_rr.csv
./bin/orchestrator -alloc data/allocation_rr.csv -workers 1   # 1 tâche à la fois par VM
./bin/orchestrator -alloc data/allocation_rr.csv -local       # test sans GCP
```

- Une **file par VM**, dans l'ordre de `allocation.csv` ; toutes les VM
  travaillent en parallèle, et sur chaque VM **N workers** vident la file
  (par défaut N = nombre de vCPU, option `-workers`).
- **Une connexion SSH par VM**, réutilisée par toutes ses tâches (multiplexage
  OpenSSH `ControlMaster`), avec la clé et la vérification d'hôte de gcloud.
- **Horodatage sur la machine de contrôle** (une seule horloge) : `start_s`,
  `end_s` relatifs au lancement du lot. `overhead_s = duration_s - remote_runtime_s`
  isole le surcoût SSH/réseau du temps de calcul mesuré par le workload.

Sorties :

| Fichier | Contenu |
|---|---|
| `results/<run>.csv` | `task_id,workload,size,vm,worker,start_s,end_s,duration_s,remote_runtime_s,overhead_s,exit_code,error` |
| `results/<run>_vms.csv` | par VM : tâches, fenêtre active, temps occupé, surcoût moyen, durée facturée (min. 60 s), coût |

Le code retour est non nul si une tâche échoue ; les autres tâches continuent.

## Scheduler

```bash
./bin/gentasks -n 500 -seed 42                       # -> data/tasks_500.csv (reproductible)
./bin/scheduler -algo select-maxmin -tasks data/tasks_500.csv -out data/allocation_sel_500.csv
./bin/scheduler -explain -tasks data/tasks_500.csv   # justification : les 11 combinaisons de types de VM
./bin/scheduler -compare -tasks data/tasks_500.csv   # tous les algorithmes, même modèle
./bin/scheduler -compare -tasks data/tasks_500.csv -exclude cloudopt-highcpu
./bin/scheduler -compare -tasks data/tasks.csv -eval data/allocation_manual.csv   # + « optimal » (≤ 10 tâches)
```

Algorithmes : `rr` (référence), `met`, `mct`, `minmin`, `maxmin` et
**`select-maxmin`** (notre algorithme) : il essaie chaque combinaison « nombre de
VM de chaque type » (11 pour notre parc), répartit avec Max-Min et garde le
meilleur score S qui respecte le délai. En mode `-algo`, il affiche aussi les VM
à démarrer : les autres restent éteintes.
`optimal` : meilleure allocation possible selon le modèle, par recherche
exhaustive (toutes les affectations et le meilleur ordre sur chaque VM) ; lots de
10 tâches au plus, ajouté automatiquement au mode compare. Sert à mesurer l'écart
de select-maxmin à l'optimum.
Options du mode compare : `-csv <fichier>` (tableau), `-save-dir <dossier>`
(allocation de chaque algorithme, directement exécutable par l'orchestrateur).

### Modèle (package `scheduler/model`)

- Durée prédite de la tâche i sur la VM j :
  `p(i,j) = médiane du benchmark (data/vm_performance.csv) × contention(j) + 0,1 s`.
  `contention` (colonne de `data/machines.csv`) = ralentissement mesuré quand
  tous les workers de la VM tournent ; 0,1 s = surcoût SSH mesuré.
- Simulation : chaque VM a autant de workers que de vCPU ; chaque tâche, dans
  l'ordre de l'allocation, va au premier worker libre (comme l'orchestrateur).
- Makespan `D` = fin de la dernière VM. Coût `C` = Σ max(60 s, temps de la VM)
  × prix horaire, sur les VM utilisées.
- Score : `S = 0,6 · C/C_RR + 0,4 · D/D_RR` (plus bas = mieux ; Round Robin = 1).
- Contraintes : `D ≤ 1,25 · D_RR` (délai) et workers × plus grosse tâche ≤ RAM.
- Les sorties du scheduler sont des **prédictions** ; seules celles de
  l'orchestrateur sont des **mesures**.

### Modèle V2 : prédire des VM jamais mesurées (option `-ref`)

Par défaut (V1), chaque VM a son propre benchmark. Avec `-ref <vm>`, seule cette
VM de référence est mesurée ; les autres sont prédites à partir de leur fiche
dans `data/machines.csv` :

- durée = médiane de la référence ÷ `speed` × contention de la référence + 0,1 s ;
- `speed` (colonne optionnelle, 1 par défaut) = vitesse par tâche relative à la
  référence : 1 pour toute la famille e2 (mesuré : ±5 %), sinon le rapport des
  scores de benchmark publiés par Google pour chaque type de machine ;
- workers = `vcpu`, mémoire = `ram_mb`, coût = `price_per_hour`, comme en V1.

```bash
./bin/scheduler -algo select-maxmin -tasks data/tasks_500.csv -ref cloudopt-vm2
./bin/modelcheck -ref cloudopt-vm2 results/exp_real/*/ results/exp_real_all/*/ > v2.csv
```

`modelcheck` compare, VM par VM, la fin prédite à la fin mesurée dans toutes
les exécutions réelles passées (validation : `docs/RESULTATS.md` § 9 quater).

## Expérience réelle : RR contre select-maxmin

```bash
./scripts/manage_vms.sh start
./scripts/exp_real.sh                              # 9 et 500 tâches, 3 répétitions alternées
LOTS="data/tasks_2000.csv" ./scripts/exp_real.sh   # 2000 tâches
./scripts/manage_vms.sh stop
```

Pour chaque lot : allocations et prédictions du scheduler, exécutions réelles
alternées, puis `results/exp_real/<lot>/summary.csv` (makespan et coût mesurés,
moyenne ± écart-type, S mesuré, erreur réel/prédit). Résultats et analyse :
[docs/RESULTATS.md § 9](docs/RESULTATS.md).

Précision du modèle, exécution par exécution (makespan prédit contre mesuré,
et repérage des e2-medium bridées) :

```bash
./scripts/precision.sh results/exp_real/*/            # select-maxmin par défaut
ALGO=rr ./scripts/precision.sh results/exp_real/*/
LATEX=1 ./scripts/precision.sh results/exp_real/*/    # lignes de tableau pour le rapport
```

Diagramme temporel (Gantt) d'exécutions **réelles**, au format TikZ pour le rapport :

```bash
./bin/gantt -titles "Round Robin,select-maxmin" \
    results/exp_real_all/tasks/rr_r1.csv results/exp_real_all/tasks/select-maxmin_r1.csv \
    > docs/rapport/figures/gantt_9.tex
```

## Responsabilité cloud

Toujours arrêter les VM après les tests : `./scripts/manage_vms.sh stop`.
Aucune clé ni aucun fichier d'identifiants ne doit être commité.

## Avancement

État détaillé, mesures et constats : **[docs/RESULTATS.md](docs/RESULTATS.md)**.

- [x] Étape 1 : mise en place (module Go, Makefile, cross-compilation)
- [x] Étape 2 : benchmark local
- [x] Étape 3 : VM GCP + benchmark sur chaque VM (`data/vm_performance.csv`)
- [x] Étape 4 : orchestrateur Go (testé sur GCP, `results/run_rr.csv`)
- [x] Étape 5a : modèle + RR, MET, MCT, Min-Min, Max-Min + mode compare
- [x] Étape 5b : notre algorithme `select-maxmin` (simulé)
- [x] Étape 6 : expériences réelles RR vs select-maxmin (9, 500, 2000 tâches)
- [ ] Étape 7 : soutenance
