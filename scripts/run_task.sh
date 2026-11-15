#!/usr/bin/env bash
# Exécute UNE tâche sur UNE VM (TD3) et récupère sa sortie sur le contrôleur.
#
# Usage : ./scripts/run_task.sh TASK_ID WORKLOAD SIZE VM ZONE
#
# Sorties (dans $RESULTS_DIR, par défaut results/) :
#   TASK_ID.out / TASK_ID.err   sortie standard / erreur de la tâche distante
#   timings.csv                 une ligne par tâche :
#                               task_id,workload,size,vm,start_epoch,end_epoch,exit_code
#   (start/end mesurés sur le contrôleur : le temps de connexion SSH est inclus,
#    c'est le temps réellement vécu par l'orchestrateur)
set -euo pipefail

if [ "$#" -ne 5 ]; then
    echo "Usage: $0 TASK_ID WORKLOAD SIZE VM ZONE" >&2
    exit 1
fi
TASK_ID="$1"; WORKLOAD="$2"; SIZE="$3"; VM="$4"; ZONE="$5"

cd "$(dirname "$0")/.."
source scripts/config.sh          # PROJECT, REMOTE_DIR (projet propre à chaque membre)
RESULTS_DIR="${RESULTS_DIR:-results}"
mkdir -p "$RESULTS_DIR"

now() { perl -MTime::HiRes=time -e 'printf "%.3f\n", time'; }

echo "[$TASK_ID] Starting $WORKLOAD --size $SIZE on $VM..."
start="$(now)"
status=0
gcloud compute ssh "$VM" --project="$PROJECT" --zone="$ZONE" --quiet \
    --command="cd ~/$REMOTE_DIR && ./bin/$WORKLOAD --size $SIZE" \
    > "$RESULTS_DIR/$TASK_ID.out" 2> "$RESULTS_DIR/$TASK_ID.err" || status=$?
end="$(now)"

# Une seule ligne par écriture : les tâches lancées en parallèle n'entremêlent pas leurs lignes.
echo "$TASK_ID,$WORKLOAD,$SIZE,$VM,$start,$end,$status" >> "$RESULTS_DIR/timings.csv"

if [ "$status" -eq 0 ]; then
    echo "[$TASK_ID] Completed successfully."
else
    echo "[$TASK_ID] Failed (exit code: $status)." >&2
    exit "$status"
fi
