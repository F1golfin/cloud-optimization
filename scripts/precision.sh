#!/usr/bin/env bash
# Précision du modèle : makespan PRÉDIT par le scheduler contre makespan MESURÉ
# sur GCP, exécution par exécution, pour un algorithme (select-maxmin par défaut).
#
# Lit les dossiers produits par scripts/exp_real.sh (predicted.csv + <algo>_r<k>_vms.csv).
# Signale aussi les exécutions où une e2-medium a été bridée (une des deux
# e2-medium plus de 1,3 fois plus lente que l'autre ; seulement si chacune a
# reçu au moins 100 tâches, sinon « - »).
#
# Usage : ./scripts/precision.sh results/exp_precision/*/
#         ALGO=rr ./scripts/precision.sh results/exp_real/tasks_500
#         LATEX=1 ./scripts/precision.sh results/exp_precision/*/   (lignes pour le rapport)
set -euo pipefail
cd "$(dirname "$0")/.."

ALGO="${ALGO:-select-maxmin}"
[[ $# -gt 0 ]] || { echo "usage : $0 <dossier de lot> [...]" >&2; exit 2; }

if [[ -z "${LATEX:-}" ]]; then
    printf "%-34s %10s %10s %12s %7s\n" "lot / exécution" "prédit (s)" "mesuré (s)" "mesuré/préd" "bridée"
fi

for dir in "$@"; do
    dir="${dir%/}"
    pred=$(awk -F, -v a="$ALGO" '$1 == a { print $4 }' "$dir/predicted.csv")
    [[ -n "$pred" ]] || { echo "$dir : pas de prédiction pour $ALGO" >&2; continue; }
    sum=0; n=0
    for f in $(ls "$dir"/"${ALGO}"_r*_vms.csv 2>/dev/null | sort -V); do
        run=$(basename "$f" _vms.csv); run="${run##*_}"
        # makespan = fin de la VM la plus tardive ; bridage = rapport entre les deux e2-medium
        read -r mes throttled < <(awk -F, '
            NR == 1 { next }
            { if ($4 > d) d = $4 }
            $1 ~ /vm2$|vm3$/ && $2 >= 100 { t[++k] = $5 }   # trop peu de tâches = pas significatif
            END {
                b = "-"
                if (k == 2) { r = t[1] > t[2] ? t[1] / t[2] : t[2] / t[1]; b = (r > 1.3 ? "oui" : "non") }
                print d, b
            }' "$f")
        ratio=$(awk -v m="$mes" -v p="$pred" 'BEGIN { printf "%.2f", m / p }')
        sum=$(awk -v s="$sum" -v m="$mes" 'BEGIN { print s + m }'); n=$((n + 1))
        if [[ -n "${LATEX:-}" ]]; then
            mark=""; [[ "$throttled" == oui ]] && mark='$^{b}$'
            printf "%s & %s & %s & %s%s & %s \\\\\\\\\n" "$(basename "$dir" | sed 's/_/\\_/g')" "$run" \
                "$(printf %.2f "$pred" | tr . ,)" "$(printf %.2f "$mes" | tr . ,)" "$mark" "$(echo "$ratio" | tr . ,)"
        else
            printf "%-34s %10.3f %10.3f %12s %7s\n" "$(basename "$dir") $run" "$pred" "$mes" "× $ratio" "$throttled"
        fi
    done
    if [[ -z "${LATEX:-}" && $n -gt 0 ]]; then
        awk -v s="$sum" -v n="$n" -v p="$pred" -v name="$(basename "$dir") moyenne" \
            'BEGIN { printf "%-34s %10.3f %10.3f %12s\n\n", name, p, s / n, sprintf("× %.2f", s / n / p) }'
    fi
done
echo "Prédit = modèle du scheduler ; mesuré = orchestrateur sur GCP. Bridée : une e2-medium > 1,3 × plus lente que l'autre." >&2
