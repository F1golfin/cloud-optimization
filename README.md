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
scheduler/                le(s) scheduler(s)
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
./scripts/manage_vms.sh create   # crée / corrige les 4 VM (scripts/config.sh)
./scripts/manage_vms.sh start
./scripts/deploy.sh              # make linux + copie des binaires sur chaque VM
./scripts/bench_vms.sh           # benchmark sur chaque VM -> data/vm_performance.csv
./scripts/manage_vms.sh stop     # TOUJOURS à la fin
```

`data/machines.csv` décrit les VM (type, vCPU, RAM, prix horaire) ; il est lu
par le scheduler et par l'orchestrateur.

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

## Modèle d'optimisation (résumé)

- Score : `S = 0,6 · C/C_RR + 0,4 · D/D_RR` (plus bas = mieux ; Round Robin = 1).
  `C` = coût modélisé (facturation minimale de 60 s par VM active), `D` = makespan.
- Contrainte : `D ≤ 1,25 · D_RR`, sinon la solution n'est pas admissible.
- Les durées issues du simulateur sont des **prédictions** ; seules les sorties
  de l'orchestrateur sont des **mesures**.

## Responsabilité cloud

Toujours arrêter les VM après les tests (le script sera ajouté à l'étape GCP).
Aucune clé ni aucun fichier d'identifiants ne doit être commité.

## Avancement

État détaillé, mesures et constats : **[docs/RESULTATS.md](docs/RESULTATS.md)**.

- [x] Étape 1 : mise en place (module Go, Makefile, cross-compilation)
- [x] Étape 2 : benchmark local
- [x] Étape 3 : VM GCP + benchmark sur chaque VM (`data/vm_performance.csv`)
- [x] Étape 4 : orchestrateur Go (testé sur GCP, `results/run_rr.csv`)
- [ ] Étape 5 : schedulers (RR, MET, MCT, Min-Min, Max-Min, V8)
- [ ] Étape 6 : expériences réelles RR vs V8
- [ ] Étape 7 : soutenance
