#!/usr/bin/env bash
# Étape 6 — expérience RÉELLE : Round Robin contre select-maxmin sur GCP.
#
# Pour chaque lot :
#   1. le scheduler produit l'allocation de chaque algorithme et les valeurs
#      PRÉDITES par le modèle (makespan, coût) ;
#   2. l'orchestrateur exécute chaque allocation sur les VM, REPS fois, en
#      alternant les algorithmes (RR, select, RR, select...) pour qu'une
#      variation des VM dans le temps ne favorise pas l'un des deux ;
#   3. synthèse : makespan et coût MESURÉS (moyenne ± écart-type), score S
#      mesuré (référence = RR mesuré) et erreur réel / prédit.
#
# Usage : ./scripts/exp_real.sh               (les 4 VM démarrées : RR les utilise toutes)
#         LOTS="data/tasks.csv" REPS=1 ./scripts/exp_real.sh
#         ORCH_FLAGS=-local REPS=1 ./scripts/exp_real.sh   (test sans GCP)
#
# Sorties : results/exp_real/<lot>/  allocations, prédictions, une exécution par
#           fichier (<algo>_r<k>.csv + _vms.csv), runs.csv et summary.csv
set -euo pipefail
cd "$(dirname "$0")/.."

LOTS="${LOTS:-data/tasks.csv data/tasks_500.csv}"
ALGOS="${ALGOS:-rr select-maxmin}"
REPS="${REPS:-3}"
OUT="results/exp_real"

[[ -x bin/scheduler && -x bin/orchestrator ]] || make tools
# Rappel affiché quoi qu'il arrive, même si une exécution échoue en cours de route.
trap 'echo; echo "Pensez à arrêter les VM : ./scripts/manage_vms.sh stop"' EXIT

for lot in $LOTS; do
    name="$(basename "$lot" .csv)"
    dir="$OUT/$name"
    mkdir -p "$dir"
    echo "################ Lot $lot ################"

    # 1. Allocations + prédictions du modèle (aucune VM n'est contactée ici).
    for algo in $ALGOS; do
        ./bin/scheduler -algo "$algo" -tasks "$lot" -out "$dir/alloc_$algo.csv" > "$dir/scheduler_$algo.txt"
        grep "VM à démarrer" "$dir/scheduler_$algo.txt" | sed "s/^/  $algo : /"
    done
    ./bin/scheduler -compare -tasks "$lot" -csv "$dir/predicted.csv" > /dev/null

    # 2. Exécutions réelles, algorithmes alternés.
    for r in $(seq 1 "$REPS"); do
        for algo in $ALGOS; do
            echo
            echo "===== $name | $algo | répétition $r/$REPS ====="
            ./bin/orchestrator ${ORCH_FLAGS:-} -alloc "$dir/alloc_$algo.csv" -out "$dir/${algo}_r$r.csv" | tail -9
        done
    done

    # 3a. Une ligne par exécution : makespan = fin de la dernière tâche,
    #     coût = somme des coûts par VM (même règle que le modèle : min. 60 s).
    {
        echo "algo,rep,makespan_s,cost_usd"
        for algo in $ALGOS; do
            for r in $(seq 1 "$REPS"); do
                awk -F, -v algo="$algo" -v r="$r" 'NR > 1 { if ($4 > d) d = $4; c += $10 }
                    END { printf "%s,%d,%.6f,%.8f\n", algo, r, d, c }' "$dir/${algo}_r${r}_vms.csv"
            done
        done
    } > "$dir/runs.csv"

    # 3b. Synthèse : moyenne ± écart-type, S mesuré (réf. = RR mesuré), réel / prédit.
    awk -F, '
        FNR == 1 { next }
        FILENAME ~ /predicted/ { pd[$1] = $4; pc[$1] = $5; ps[$1] = $6; next }
        { n[$1]++; sd[$1] += $3; sd2[$1] += $3 * $3; sc[$1] += $4; sc2[$1] += $4 * $4
          if (!($1 in seen)) { seen[$1] = 1; order[++k] = $1 } }
        function sdev(s, s2, m) { return m > 1 ? sqrt((s2 - s * s / m) / (m - 1)) : 0 }
        END {
            refd = sd["rr"] / n["rr"]; refc = sc["rr"] / n["rr"]
            print "algo,runs,makespan_mean_s,makespan_std_s,cost_mean_usd,cost_std_usd,score_measured,makespan_predicted_s,cost_predicted_usd,score_predicted,makespan_real_over_pred"
            for (i = 1; i <= k; i++) {
                a = order[i]; m = n[a]; d = sd[a] / m; c = sc[a] / m
                s = 0.6 * c / refc + 0.4 * d / refd
                printf "%s,%d,%.3f,%.3f,%.6f,%.6f,%.4f,%.3f,%.6f,%.4f,%.3f\n", a, m, d, sdev(sd[a], sd2[a], m),
                       c, sdev(sc[a], sc2[a], m), s, pd[a], pc[a], ps[a], (pd[a] > 0 ? d / pd[a] : 0)
            }
        }' "$dir/predicted.csv" "$dir/runs.csv" > "$dir/summary.csv"

    echo
    echo "Synthèse $name ($dir/summary.csv) :"
    awk -F, 'NR == 1 { printf "%-14s %4s %18s %20s %9s | %9s %10s %8s | %9s\n", "algo", "runs", "makespan mesuré", "coût mesuré ($)", "S mesuré", "D prédit", "C prédit", "S prédit", "réel/préd"; next }
        { printf "%-14s %4s %10.3f ± %5.3f %11.6f ± %7.6f %9.4f | %9.3f %10.6f %8.4f | %9.3f\n", $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11 }' "$dir/summary.csv"
    echo
done
