#!/usr/bin/env bash
# Lance le benchmark sur chaque VM (en parallèle, une VM = une machine
# indépendante), rapatrie les mesures brutes et construit le tableau de
# performance utilisé par le scheduler.
#
# Usage : ./scripts/bench_vms.sh                (après manage_vms.sh start + deploy.sh)
#         VMS="cloudopt-vm3" ./scripts/bench_vms.sh
#
# Sorties :
#   results/benchmark_<vm>.csv   mesures brutes (machine,workload,size,run,runtime_s,max_rss_kb)
#   results/bench_logs/<vm>.log  sortie console de chaque VM
#   data/vm_performance.csv      1 ligne par (machine, workload, taille) :
#                                médiane, moyenne, écart-type, RSS max
#                                (reconstruit à partir de TOUS les results/benchmark_cloudopt-*.csv)
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/config.sh

mkdir -p results/bench_logs

pids=()
for vm in $VMS; do
    echo "Benchmark lancé sur $vm (log : results/bench_logs/$vm.log)"
    (
        gssh "$vm" --command="cd ~/$REMOTE_DIR && BIN_DIR=bin ./scripts/benchmark.sh -m $vm" \
            > "results/bench_logs/$vm.log" 2>&1
        gscp "$vm:~/$REMOTE_DIR/results/benchmark_$vm.csv" "results/benchmark_$vm.csv" >/dev/null
    ) &
    pids+=("$!")
done

failed=0
set -- $VMS
for pid in "${pids[@]}"; do
    if wait "$pid"; then echo "  ok     $1"; else echo "  ÉCHEC  $1 (voir results/bench_logs/$1.log)"; failed=1; fi
    shift
done

# ---------------------------------------------------------------- agrégation
# La médiane résiste mieux qu'une moyenne à une exécution perturbée
# (voisin bruyant, vCPU partagé des e2-medium) : c'est elle que le scheduler utilise.
shopt -s nullglob
files=(results/benchmark_cloudopt-*.csv)
if (( ${#files[@]} > 0 )); then
    {
        echo "machine,workload,size,runs,median_s,mean_s,std_s,max_rss_kb"
        tail -q -n +2 "${files[@]}" \
        | sort -t, -k1,1 -k2,2 -k3,3n -k5,5g \
        | awk -F, '
            function flush(   m, mean, v, sd) {
                if (n == 0) return
                m = (n % 2) ? t[(n + 1) / 2] : (t[n / 2] + t[n / 2 + 1]) / 2
                mean = s / n
                v = (n > 1) ? (ss - n * mean * mean) / (n - 1) : 0
                sd = (v > 0) ? sqrt(v) : 0
                printf "%s,%d,%.6f,%.6f,%.6f,%d\n", key, n, m, mean, sd, rss
            }
            {
                k = $1 "," $2 "," $3
                if (k != key) { flush(); key = k; n = 0; s = 0; ss = 0; rss = 0 }
                t[++n] = $5; s += $5; ss += $5 * $5
                if ($6 > rss) rss = $6
            }
            END { flush() }'
    } > data/vm_performance.csv
    echo
    echo "data/vm_performance.csv :"
    column -s, -t data/vm_performance.csv
fi

echo
echo "Pensez à arrêter les VM : ./scripts/manage_vms.sh stop"
exit "$failed"
