#!/usr/bin/env bash
# Test de généralisation du modèle sur des VM JAMAIS MESURÉES, d'autres types
# et d'autres familles que notre parc (critique de l'oral : surapprentissage).
#
# Parc de test : data/machines_newvms.csv
#   - cloudopt-vm2 (e2-medium) : la RÉFÉRENCE, déjà benchmarkée, non démarrée ;
#   - 4 nouvelles VM : e2-standard-4 (même famille, 4 vCPU), n2-standard-2
#     (Intel), n2d-standard-2 (AMD), t2d-standard-2 (AMD, 1 vCPU = 1 cœur).
#
# Étapes (toutes automatiques) :
#   1. création des 4 VM + déploiement des workloads ;
#   2. benchmark complet des 4 VM -> data/vm_performance_newvms.csv
#      (sert UNIQUEMENT à l'analyse : prédicteurs « 1 tâche » et « complet ») ;
#   3. exécutions réelles Round Robin sur les 4 VM : 500 puis 2000 tâches,
#      3 répétitions -> results/exp_newvms/ ;
#   4. arrêt des VM (même en cas d'erreur).
# Puis : ./scripts/newvms_report.sh  (analyse, sans GCP).
#
# Durée ≈ 25 min. Coût ≈ 0,40 $/h pour les 4 VM.
# Supprimer les VM ensuite : MACHINES_CSV=data/machines_newvms.csv VMS="<les 4>" ./scripts/manage_vms.sh delete
set -euo pipefail
cd "$(dirname "$0")/.."

export MACHINES_CSV="${MACHINES_CSV:-data/machines_newvms.csv}"
REF="${REF:-cloudopt-vm2}"
# Les VM du fichier, sauf la référence (déjà mesurée, inutile de la démarrer).
VMS="$(awk -F, -v ref="$REF" 'NR > 1 && $1 != ref { print $1 }' "$MACHINES_CSV" | xargs)"
export VMS
echo "Parc de test : $VMS   (référence : $REF)"

trap 'echo; echo "Arrêt des VM de test..."; ./scripts/manage_vms.sh stop || true' EXIT

make tools linux >/dev/null

echo; echo "##### 1. Création et déploiement #####"
./scripts/manage_vms.sh create
./scripts/manage_vms.sh start
./scripts/deploy.sh

echo; echo "##### 2. Benchmark des VM de test #####"
PERF_OUT=data/vm_performance_newvms.csv ./scripts/bench_vms.sh

echo; echo "##### 3. Exécutions réelles (Round Robin) #####"
OUT=results/exp_newvms ALGOS=rr REPS="${REPS:-3}" \
    LOTS="${LOTS:-data/tasks_500.csv data/tasks_2000.csv}" \
    SCHED_FLAGS="-ref $REF -exclude $REF" \
    ./scripts/exp_real.sh

echo; echo "Terminé. Analyse : ./scripts/newvms_report.sh"
