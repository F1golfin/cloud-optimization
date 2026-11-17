#!/usr/bin/env bash
# Analyse du test sur des VM jamais mesurées (après scripts/exp_newvms.sh).
# Aucun appel à GCP : tout vient des fichiers de résultats.
#
# Pour chaque VM de test, cinq façons de prédire sa fin, comparées à la fin
# MESURÉE dans chaque exécution réelle (results/exp_newvms/) :
#   A « fiche seule »          benchmark de la référence + fiche (vcpu), vitesse 1
#   B « fiche + 1 tâche »      vitesse estimée sur UNE tâche (cpu_task 500000)
#   D « fiche + 4 tâches »     une vitesse PAR TYPE de tâche, estimée sur la
#                              taille moyenne de chaque type (4 tâches, ~2 s)
#   E « D + hyperthreading »   comme D, contention 1,00 (au lieu de 1,40) pour
#                              les VM sans hyperthreading (threads_per_core = 1)
#   C « benchmark complet »    les 12 combinaisons mesurées sur la VM
# A, B, C et D utilisent la contention de la référence (1,40).
# Colonnes en plus : vitesse réelle (moyenne géométrique sur les 12
# combinaisons) et ralentissement réel en parallèle (calcul pendant le lot /
# tâche seule).
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

# Garde les 2 lignes d'en-tête d'un tableau markdown, trie les suivantes.
sort_rows() { local h1 h2; IFS= read -r h1; IFS= read -r h2; printf '%s\n%s\n' "$h1" "$h2"; sort; }

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

# B : même fiche, colonne speed remplacée par la vitesse « 1 tâche ».
awk -F, 'NR == FNR { cal[$1] = $2; next }
    FNR == 1 { for (i = 1; i <= NF; i++) if ($i == "speed") c = i; print; next }
    { if ($1 in cal) $c = cal[$1]; print }' OFS=, "$TMP/speed.csv" "$MACHINES" > "$TMP/machines_calib.csv"

# D : benchmark reconstruit = médiane de la référence / vitesse du TYPE de tâche,
#     cette vitesse étant mesurée sur la taille moyenne du type (4 tâches au total).
awk -F, -v ref="$REF" 'BEGIN { OFS = ","; mid["cpu_task"] = 500000; mid["matrix_task"] = 400; mid["memory_task"] = 256; mid["sort_task"] = 5000000 }
    FNR == 1 { print; next }
    { m[$1 "," $2 "," $3] = $5; line[$1 "," $2 "," $3] = $0; vms[$1] = 1; combos[$2 "," $3] = 1 }
    END {
        for (v in vms) for (c in combos) {
            split(c, a, ","); w = a[1]
            f = m[ref "," w "," mid[w]] / m[v "," w "," mid[w]]
            split(line[ref "," c], r, ","); r[1] = v; r[5] = r[5] / f
            print r[1], r[2], r[3], r[4], r[5], r[6], r[7], r[8]
        }
    }' "$PERF_NEW" > "$TMP/perf_4taches.csv"

# E : contention 1,00 pour les VM sans hyperthreading (colonne threads_per_core).
awk -F, 'NR == 1 { for (i = 1; i <= NF; i++) { if ($i == "contention") c = i; if ($i == "threads_per_core") t = i }; print; next }
    { if (t && $t == 1) $c = "1.00"; print }' OFS=, "$MACHINES" > "$TMP/machines_smt.csv"

dirs=("$RUNS"/tasks*/)
./bin/modelcheck -machines "$MACHINES"               -ref "$REF"                 "${dirs[@]}" > "$TMP/A.csv"
./bin/modelcheck -machines "$TMP/machines_calib.csv" -ref "$REF"                 "${dirs[@]}" > "$TMP/B.csv"
./bin/modelcheck -machines "$MACHINES"               -perf "$TMP/perf_4taches.csv" "${dirs[@]}" > "$TMP/D.csv"
./bin/modelcheck -machines "$TMP/machines_smt.csv"   -perf "$TMP/perf_4taches.csv" "${dirs[@]}" > "$TMP/E.csv"
./bin/modelcheck -machines "$MACHINES"               -perf "$PERF_NEW"           "${dirs[@]}" > "$TMP/C.csv"

# Ralentissement réel en parallèle : temps de calcul de chaque tâche pendant le
# lot / sa médiane seule (benchmark de la VM), moyenné par VM.
cat "$RUNS"/tasks*/rr_r[0-9].csv | awk -F, 'NR == FNR { if (FNR > 1) m[$1 "," $2 "," $3] = $5; next }
    $1 != "task_id" && $9 != "" && ($4 "," $2 "," $3) in m { s[$4] += $9 / m[$4 "," $2 "," $3]; n[$4]++ }
    END { for (v in s) printf "%s,%.3f\n", v, s[v] / n[v] }' "$PERF_NEW" - > "$TMP/cont.csv"

# Synthèse par VM : médiane mesuré/prédit et erreur moyenne |mesuré/prédit - 1|.
paste -d, "$TMP/A.csv" <(cut -d, -f7 "$TMP/B.csv") <(cut -d, -f7 "$TMP/D.csv") \
          <(cut -d, -f7 "$TMP/E.csv") <(cut -d, -f7 "$TMP/C.csv") | awk -F, '
    NR == FNR && FILENAME ~ /speed/ { sc[$1] = $2; sr[$1] = $3; next }
    FILENAME ~ /cont/ { ct[$1] = $2; next }
    FNR == 1 { next }
    { vm = $4; type[vm] = $5; k = ++n[vm]; meas = $8
      r[1, vm, k] = meas / $7; r[2, vm, k] = meas / $11; r[3, vm, k] = meas / $12
      r[4, vm, k] = meas / $13; r[5, vm, k] = meas / $14 }
    function med(p, vm, cnt,   i, j, t, a) {
        for (i = 1; i <= cnt; i++) a[i] = r[p, vm, i]
        for (i = 1; i <= cnt; i++) for (j = i + 1; j <= cnt; j++) if (a[j] < a[i]) { t = a[i]; a[i] = a[j]; a[j] = t }
        return (cnt % 2) ? a[(cnt + 1) / 2] : (a[cnt / 2] + a[cnt / 2 + 1]) / 2
    }
    function err(p, vm, cnt,   i, s, x) { for (i = 1; i <= cnt; i++) { x = r[p, vm, i]; s += (x > 1 ? x - 1 : 1 - x) }; return 100 * s / cnt }
    function cell(p, vm) { return sprintf("%.2f (±%.0f %%)", med(p, vm, n[vm]), err(p, vm, n[vm])) }
    END {
        print "| VM | Type | Exéc. | Vitesse réelle | Vitesse 1 tâche | Contention réelle | A fiche seule | B fiche + 1 tâche | D fiche + 4 tâches | E D + hyperthreading | C benchmark complet |"
        print "|---|---|---|---|---|---|---|---|---|---|---|"
        for (vm in n) printf "| %s | %s | %d | %.2f | %.2f | %.2f | %s | %s | %s | %s | %s |\n",
            vm, type[vm], n[vm], sr[vm], sc[vm], ct[vm], cell(1, vm), cell(2, vm), cell(3, vm), cell(4, vm), cell(5, vm)
    }' "$TMP/speed.csv" "$TMP/cont.csv" - | sort_rows > "$TMP/table.md"

# Vitesse par type de tâche (taille moyenne) : montre pourquoi une seule tâche ne suffit pas.
awk -F, -v ref="$REF" 'BEGIN { mid["cpu_task"] = 500000; mid["matrix_task"] = 400; mid["memory_task"] = 256; mid["sort_task"] = 5000000 }
    FNR == 1 { next }
    { m[$1 "," $2 "," $3] = $5; if ($1 != ref) vms[$1] = 1 }
    END {
        print "| VM | cpu_task | matrix_task | memory_task | sort_task |"
        print "|---|---|---|---|---|"
        for (v in vms) printf "| %s | %.2f | %.2f | %.2f | %.2f |\n", v,
            m[ref ",cpu_task," mid["cpu_task"]] / m[v ",cpu_task," mid["cpu_task"]],
            m[ref ",matrix_task," mid["matrix_task"]] / m[v ",matrix_task," mid["matrix_task"]],
            m[ref ",memory_task," mid["memory_task"]] / m[v ",memory_task," mid["memory_task"]],
            m[ref ",sort_task," mid["sort_task"]] / m[v ",sort_task," mid["sort_task"]]
    }' "$PERF_NEW" | sort_rows > "$TMP/speed_by_type.md"

{
    echo "# Test sur des VM jamais mesurées"
    echo
    echo "Référence : $REF (seule VM benchmarkée pour A et B). Exécutions : $RUNS."
    echo "Colonnes A à E : médiane de (fin mesurée / fin prédite) de la VM, et entre parenthèses l'erreur moyenne."
    echo "Vitesse : par tâche, relative à la référence (> 1 = plus rapide). Contention réelle : ralentissement"
    echo "d'une tâche quand tous les workers tournent (le modèle suppose 1,40)."
    echo
    cat "$TMP/table.md"
    echo
    echo "## Vitesse par type de tâche (taille moyenne), relative à la référence"
    echo
    cat "$TMP/speed_by_type.md"
} > "$OUT"
cat "$OUT"
echo
echo "Écrit : $OUT"
