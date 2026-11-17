#!/usr/bin/env bash
# Expérience « workers » : combien de tâches une VM exécute-t-elle vraiment
# en parallèle sans se ralentir ?
#
# Protocole :
#   - le MÊME lot est donné à chaque VM : 2 tâches de chacune des 12
#     combinaisons workload/taille du benchmark = 24 tâches par VM ;
#   - on l'exécute avec 1, 2 puis 4 workers par VM (toutes les VM en même
#     temps : elles sont indépendantes), 3 répétitions par niveau ;
#   - pour chaque VM et chaque niveau : durée du lot, débit (tâches/s) et
#     ralentissement moyen = runtime mesuré / médiane du benchmark (tâche seule).
#
# Usage : ./scripts/exp_workers.sh            (VM démarrées, binaires déployés)
#         LEVELS="1 2" REPS=1 ./scripts/exp_workers.sh
#         ORCH_FLAGS=-local ./scripts/exp_workers.sh   (test sans GCP)
#
# Sorties : results/exp_workers/w<k>_r<r>.csv (+ _vms.csv) et
#           results/exp_workers/summary.csv
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/config.sh

LEVELS="${LEVELS:-1 2 4}"
REPS="${REPS:-3}"
OUT_DIR="results/exp_workers"
ALLOC="$OUT_DIR/allocation.csv"
PERF="data/vm_performance.csv"

[[ -x bin/orchestrator ]] || make tools
mkdir -p "$OUT_DIR"

# allocation
# 24 tâches par VM, ordre alterné (petites et grandes mélangées).
{
    echo "task_id,workload,size,vm"
    for vm in $VMS; do
        awk -F, -v vm="$vm" 'NR > 1 && $1 == vm { c[++n] = $2 "," $3 }
            END { for (rep = 1; rep <= 2; rep++) for (i = 1; i <= n; i++)
                      printf "%s-%02d,%s,%s\n", vm, (rep - 1) * n + i, c[i], vm }' "$PERF"
    done
} > "$ALLOC"
echo "Allocation : $ALLOC ($(($(wc -l < "$ALLOC") - 1)) tâches)"

# exécutions
for k in $LEVELS; do
    for r in $(seq 1 "$REPS"); do
        echo
        echo "===== workers=$k  répétition $r/$REPS ====="
        ./bin/orchestrator ${ORCH_FLAGS:-} -alloc "$ALLOC" -workers "$k" -out "$OUT_DIR/w${k}_r${r}.csv" | tail -8
    done
done

# synthèse
# Ralentissement : on joint chaque tâche à la médiane du benchmark de SA VM.
{
    echo "vm,workers,reps,batch_s_mean,throughput_tasks_per_s,slowdown_mean,overhead_s_mean"
    for k in $LEVELS; do
        for r in $(seq 1 "$REPS"); do
            awk -F, -v k="$k" 'FNR == 1 { next }
                FILENAME ~ /performance/ { med[$1 "," $2 "," $3] = $5; next }
                FILENAME ~ /_vms\.csv$/  { printf "B,%s,%s,%s\n", $1, k, $5; next }
                $9 != "" { printf "T,%s,%s,%.6f,%s\n", $4, k, $9 / med[$4 "," $2 "," $3], $10 }' \
                "$PERF" "$OUT_DIR/w${k}_r${r}_vms.csv" "$OUT_DIR/w${k}_r${r}.csv"
        done
    done | awk -F, -v reps="$REPS" '
        $1 == "B" { key = $2 "," $3; batch[key] += $4; nb[key]++; if (!(key in seen)) { seen[key] = 1; order[++n] = key } }
        $1 == "T" { key = $2 "," $3; slow[key] += $4; ovh[key] += $5; nt[key]++ }
        END {
            for (i = 1; i <= n; i++) {
                k = order[i]; b = batch[k] / nb[k]; tasks = nt[k] / nb[k]
                printf "%s,%d,%.3f,%.2f,%.2f,%.3f\n", k, nb[k], b, tasks / b, slow[k] / nt[k], ovh[k] / nt[k]
            }
        }' | sort -t, -k1,1 -k2,2n
} > "$OUT_DIR/summary.csv"

echo
echo "Synthèse ($OUT_DIR/summary.csv) :"
awk -F, '{ printf "%-17s %8s %5s %13s %12s %11s %12s\n", $1, $2, $3, $4, $5, $6, $7 }' "$OUT_DIR/summary.csv"
echo
echo "Pensez à arrêter les VM : ./scripts/manage_vms.sh stop"
