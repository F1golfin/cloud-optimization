#!/usr/bin/env bash
# Exécution distribuée d'une allocation (TD3 / TD4).
#
# Usage : ./scripts/run_all.sh [ALLOCATION.csv]     (défaut : data/allocation_manual.csv)
#         RESULTS_DIR=results/runs/rr_1 ./scripts/run_all.sh data/allocation_rr.csv
#
# Chaque ligne task_id,workload,size,vm est lancée par run_task.sh en
# arrière-plan (&) : toutes les tâches partent en même temps, y compris
# plusieurs sur la même VM (pas de file par VM). Un seul `wait` à la fin.
#
# Sorties dans $RESULTS_DIR (défaut : results/) :
#   Tx.out / Tx.err, timings.csv (voir run_task.sh)
#   results.csv   une ligne par tâche : début/fin relatifs au lancement, durée, statut
#   summary.csv   makespan mesuré, occupation et coût réel de chaque VM
#                 (coût = prix × max(60 s, occupation), même règle que le modèle)
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/config.sh

CSV="${1:-data/allocation_manual.csv}"
export RESULTS_DIR="${RESULTS_DIR:-results}"
SPECS="${SPECS:-data/vm_specs.csv}"

[[ -f "$CSV" ]] || { echo "Erreur : $CSV introuvable" >&2; exit 1; }
mkdir -p "$RESULTS_DIR"
rm -f "$RESULTS_DIR/timings.csv"
cp "$CSV" "$RESULTS_DIR/allocation.csv"

t0="$(perl -MTime::HiRes=time -e 'printf "%.3f\n", time')"
pids=()
task_ids=()
echo "Starting distributed execution of $CSV -> $RESULTS_DIR"

while IFS=, read -r task_id workload size vm; do
    [[ "$task_id" == "task_id" || -z "$task_id" ]] && continue
    ./scripts/run_task.sh "$task_id" "$workload" "$size" "$vm" "$ZONE" &
    pids+=("$!")
    task_ids+=("$task_id")
done < "$CSV"

failed=0
for i in "${!pids[@]}"; do
    if wait "${pids[$i]}"; then
        echo "[${task_ids[$i]}] SUCCESS"
    else
        echo "[${task_ids[$i]}] FAILED"
        failed=1
    fi
done

# ---------------------------------------------------------------- mesures
{
    echo "task_id,workload,size,vm,start_s,end_s,duration_s,exit_code"
    awk -F, -v t0="$t0" '{ printf "%s,%s,%s,%s,%.3f,%.3f,%.3f,%s\n", $1,$2,$3,$4,$5-t0,$6-t0,$6-$5,$7 }' \
        "$RESULTS_DIR/timings.csv" | sort -t, -k1,1V
} > "$RESULTS_DIR/results.csv"

awk -F, '
    FNR == 1 { next }
    FILENAME == ARGV[1] { price[$1] = $5; next }
    {
        vm = $4
        if (!(vm in first) || $5 < first[vm]) first[vm] = $5
        if ($6 > last[vm]) last[vm] = $6
        if ($6 > makespan) makespan = $6
        n[vm]++
    }
    END {
        print "vm,tasks,busy_s,billed_s,cost_usd"
        for (vm in n) {
            busy = last[vm]          # VM occupée du lancement à la fin de sa dernière tâche
            billed = (busy < 60) ? 60 : busy
            c = price[vm] / 3600 * billed
            total += c
            printf "%s,%d,%.3f,%.3f,%.8f\n", vm, n[vm], busy, billed, c
        }
        printf "TOTAL,,%.3f,,%.8f\n", makespan, total
    }' "$SPECS" "$RESULTS_DIR/results.csv" > "$RESULTS_DIR/summary.csv"

echo
awk -F, 'NR > 1 { printf "  %-17s %3s tâche(s)  occupation %7s s  coût %s $\n", $1, $2, $3, $5 }' "$RESULTS_DIR/summary.csv"

if [[ "$failed" -eq 0 ]]; then
    echo "All tasks completed successfully!"
else
    echo "Some tasks failed." >&2
    exit 1
fi
