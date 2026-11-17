#!/usr/bin/env bash
# Analyse du test sur des VM jamais mesurées (après scripts/exp_newvms.sh).
# Aucun appel à GCP : tout vient des fichiers de résultats.
#
# Pour chaque VM de test, trois façons de prédire la fin de la VM, comparées à
# la fin MESURÉE dans chaque exécution réelle (results/exp_newvms/) :
#   A « fiche seule »     : benchmark de la référence + fiche (vcpu), speed = 1
#   B « fiche + 1 tâche » : idem, speed estimé sur UNE seule tâche
#                           (cpu_task 500000, ~0,15 s) mesurée sur la VM
#   C « benchmark complet » : les 12 combinaisons mesurées sur la VM (modèle V1)
# Les trois utilisent la contention de la référence (1,40).
# Colonnes en plus : vitesse réelle (moyenne sur les 12 combinaisons) et
# ralentissement réel en parallèle (temps de calcul pendant le lot / tâche seule).
#
# Usage : ./scripts/newvms_report.sh        -> tableau + results/exp_newvms/report.md
set -euo pipefail
cd "$(dirname "$0")/.."

MACHINES="${MACHINES_CSV:-data/machines_newvms.csv}"
PERF_NEW="${PERF_NEW:-data/vm_performance_newvms.csv}"
REF="${REF:-cloudopt-vm2}"
RUNS="${RUNS:-results/exp_newvms}"
CAL_W="cpu_task"; CAL_S="500000"   # la tâche de calibration du prédicteur B
OUT="$RUNS/report.md"
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT

[[ -f "$PERF_NEW" ]] || { echo "Erreur : $PERF_NEW absent (lancer d'abord scripts/exp_newvms.sh)" >&2; exit 1; }
[[ -x bin/modelcheck ]] || make tools >/dev/null

# Vitesse de chaque VM relative à la référence (> 1 = plus rapide).
#   calib : sur la seule tâche de calibration ; reelle : moyenne géométrique des 12.
awk -F, -v ref="$REF" -v cw="$CAL_W" -v cs="$CAL_S" '
    FNR == 1 { next }
    { m[$1 "," $2 "," $3] = $5; vms[$1] = 1; combos[$2 "," $3] = 1 }
    END {
        for (v in vms) {
            n = 0; s = 0
            for (c in combos) if ((ref "," c) in m && (v "," c) in m) { s += log(m[ref "," c] / m[v "," c]); n++ }
            printf "%s,%.4f,%.4f\n", v, m[ref "," cw "," cs] / m[v "," cw "," cs], exp(s / n)
        }
    }' "$PERF_NEW" > "$TMP/speed.csv"

# Prédicteur B : même fiche, colonne speed remplacée par la vitesse « 1 tâche ».
awk -F, 'NR == FNR { cal[$1] = $2; next }
    FNR == 1 { for (i = 1; i <= NF; i++) if ($i == "speed") c = i; print; next }
    { if ($1 in cal) $c = cal[$1]; print }' OFS=, "$TMP/speed.csv" "$MACHINES" > "$TMP/machines_calib.csv"

dirs=("$RUNS"/tasks*/)
./bin/modelcheck -machines "$MACHINES"           -ref "$REF" "${dirs[@]}" > "$TMP/A.csv"
./bin/modelcheck -machines "$TMP/machines_calib.csv" -ref "$REF" "${dirs[@]}" > "$TMP/B.csv"
./bin/modelcheck -machines "$MACHINES" -perf "$PERF_NEW"     "${dirs[@]}" > "$TMP/C.csv"

# Ralentissement réel en parallèle : temps de calcul de chaque tâche pendant le
# lot / sa médiane seule (benchmark de la VM), moyenné par VM.
cat "$RUNS"/tasks*/rr_r[0-9].csv | awk -F, 'NR == FNR { if (FNR > 1) m[$1 "," $2 "," $3] = $5; next }
    $1 != "task_id" && $9 != "" && ($4 "," $2 "," $3) in m { s[$4] += $9 / m[$4 "," $2 "," $3]; n[$4]++ }
    END { for (v in s) printf "%s,%.3f\n", v, s[v] / n[v] }' "$PERF_NEW" - > "$TMP/cont.csv"

# Synthèse par VM : médiane mesuré/prédit et erreur moyenne |mesuré/prédit - 1|.
paste -d, "$TMP/A.csv" <(cut -d, -f7 "$TMP/B.csv") <(cut -d, -f7 "$TMP/C.csv") | awk -F, '
    NR == FNR && FILENAME ~ /speed/ { sc[$1] = $2; sr[$1] = $3; next }
    FILENAME ~ /cont/ { ct[$1] = $2; next }
    FNR == 1 { next }
    { vm = $4; type[vm] = $5; k = ++n[vm]; meas = $8
      ra[vm, k] = meas / $7; rb[vm, k] = meas / $11; rc[vm, k] = meas / $12 }
    function med(arr, vm, cnt,   i, j, t, a) {
        for (i = 1; i <= cnt; i++) a[i] = arr[vm, i]
        for (i = 1; i <= cnt; i++) for (j = i + 1; j <= cnt; j++) if (a[j] < a[i]) { t = a[i]; a[i] = a[j]; a[j] = t }
        return (cnt % 2) ? a[(cnt + 1) / 2] : (a[cnt / 2] + a[cnt / 2 + 1]) / 2
    }
    function err(arr, vm, cnt,   i, s) { for (i = 1; i <= cnt; i++) s += (arr[vm, i] > 1 ? arr[vm, i] - 1 : 1 - arr[vm, i]); return 100 * s / cnt }
    END {
        print "| VM | Type | Exéc. | Vitesse réelle | Vitesse 1 tâche | Contention réelle | A fiche seule | B fiche + 1 tâche | C benchmark complet |"
        print "|---|---|---|---|---|---|---|---|---|"
        for (vm in n) printf "| %s | %s | %d | %.2f | %.2f | %.2f | %.2f (±%.0f %%) | %.2f (±%.0f %%) | %.2f (±%.0f %%) |\n",
            vm, type[vm], n[vm], sr[vm], sc[vm], ct[vm],
            med(ra, vm, n[vm]), err(ra, vm, n[vm]), med(rb, vm, n[vm]), err(rb, vm, n[vm]), med(rc, vm, n[vm]), err(rc, vm, n[vm])
    }' "$TMP/speed.csv" "$TMP/cont.csv" - > "$TMP/table.md"

{
    echo "# Test sur des VM jamais mesurées"
    echo
    echo "Référence : $REF (seule VM benchmarkée pour A et B). Exécutions : $RUNS."
    echo "Colonnes A/B/C : médiane de (fin mesurée / fin prédite) de la VM, et entre parenthèses l'erreur moyenne."
    echo "Vitesse : par tâche, relative à la référence (> 1 = plus rapide). Contention réelle : ralentissement"
    echo "d'une tâche quand tous les workers tournent (le modèle suppose 1,40)."
    echo
    cat "$TMP/table.md"
} > "$OUT"
cat "$OUT"
echo
echo "Écrit : $OUT"
