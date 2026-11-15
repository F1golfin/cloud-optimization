#!/usr/bin/env bash
# Benchmark de CONCURRENCE : comment la durée d'une tâche augmente quand
# k tâches identiques tournent en même temps sur la même VM (TD3 : les tâches
# d'une VM partagent son CPU et sa RAM).
#
# Pour chaque VM, chaque workload (taille moyenne) et k = 1, 2, 3, 4 :
# on lance k copies en parallèle (&) et on mesure la durée réelle de chacune.
# Les 4 VM sont mesurées en même temps (une VM = une machine indépendante).
#
# Usage : ./scripts/bench_concurrency.sh   (VM démarrées + deploy.sh fait)
#         VMS="cloudopt-vm1" KS="1 2" ./scripts/bench_concurrency.sh
#
# Sortie : results/concurrency.csv
#   machine,workload,size,k,copy,runtime_s
#   (k = nombre de tâches simultanées ; une ligne par copie)
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/config.sh

KS="${KS:-1 2 3 4}"
# Taille moyenne de chaque workload (mêmes valeurs que benchmark.sh).
CASES="${CASES:-cpu_task:10000000 memory_task:512 matrix_task:1600 sort_task:50000000}"

mkdir -p results/bench_logs
OUT=results/concurrency.csv
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# Script exécuté sur la VM : écrit les lignes CSV sur stdout.
remote_script='
set -eu
cd ~/'"$REMOTE_DIR"'
m=$(hostname -s)
for c in '"$CASES"'; do
  w=${c%%:*}; s=${c##*:}
  for k in '"$KS"'; do
    for i in $(seq 1 $k); do
      ( t0=$(date +%s.%N); ./bin/$w --size $s >/dev/null; t1=$(date +%s.%N)
        echo "$m,$w,$s,$k,$i,$(echo "$t1 - $t0" | bc)" ) &
    done
    wait
  done
done'

pids=()
for vm in $VMS; do
    echo "Concurrence lancée sur $vm (log : results/bench_logs/concurrency_$vm.log)"
    gssh "$vm" --command="bash -c '$remote_script'" \
        > "$TMP/$vm.csv" 2> "results/bench_logs/concurrency_$vm.log" &
    pids+=("$!")
done

failed=0
set -- $VMS
for pid in "${pids[@]}"; do
    if wait "$pid"; then echo "  ok     $1"; else echo "  ÉCHEC  $1"; failed=1; fi
    shift
done

{
    echo "machine,workload,size,k,copy,runtime_s"
    cat "$TMP"/*.csv | grep -E '^cloudopt-' | sort -t, -k1,1 -k2,2 -k4,4n -k5,5n
} > "$OUT"

echo
echo "$OUT — durée moyenne par copie (s) selon k :"
awk -F, 'NR > 1 { key = $1 "," $2 "," $4; s[key] += $6; n[key]++ }
         END { for (k in s) printf "%s,%.2f\n", k, s[k] / n[k] }' "$OUT" \
    | sort -t, -k1,1 -k2,2 -k3,3n \
    | awk -F, '{ printf "  %-17s %-12s k=%s  %6s s\n", $1, $2, $3, $4 }'
exit "$failed"
