#!/usr/bin/env bash
# Benchmark local des workloads (TD1), décliné en plusieurs tailles par workload.
#
# Le MÊME script sert en local (TD1) et sur chaque VM GCP (étape 3) : seules
# la machine (-m) et la provenance des binaires (BIN_DIR) changent.
#
# Usage :
#   ./scripts/benchmark.sh -m local             # local : 4 workloads x 3 tailles x 5 runs
#   ./scripts/benchmark.sh                      # sur une VM : machine = nom d'hôte (ex. cloudopt-vm1)
#   ./scripts/benchmark.sh -w matrix_task       # un seul workload
#   ./scripts/benchmark.sh -w cpu_task -s 500000 -r 3 -a   # une taille, ajout au CSV
#
# Options :
#   -m MACHINE    nom de la machine écrit dans le CSV (défaut : nom d'hôte court)
#   -w WORKLOAD   cpu_task | memory_task | matrix_task | sort_task (défaut : tous)
#   -s SIZE       une seule taille (nécessite -w), doit rester dans les bornes
#   -r RUNS       nombre d'exécutions par taille (défaut : 5)
#   -o FILE       CSV de sortie (défaut : results/benchmark_<MACHINE>.csv)
#   -a            ajoute au CSV existant au lieu de le recréer
#
# Variables d'environnement :
#   BIN_DIR        dossier des binaires (défaut : bin/local, créé par `make`)
#   MIN_RUNTIME_S  durée en dessous de laquelle une mesure est signalée « courte » (défaut : 0.5)
#   MAX_RUNTIME_S  durée au-delà de laquelle une mesure est signalée « longue » (défaut : 10)
#
# Sortie : une ligne par exécution  machine,workload,size,run,runtime_s,max_rss_kb
#   runtime_s  = temps réel (wall-clock) de l'exécution, en secondes
#   max_rss_kb = mémoire physique maximale utilisée, en Ko
set -euo pipefail
cd "$(dirname "$0")/.."

BIN_DIR="${BIN_DIR:-bin/local}"
MIN_RUNTIME_S="${MIN_RUNTIME_S:-0.5}"
MAX_RUNTIME_S="${MAX_RUNTIME_S:-10}"
MACHINE="$(hostname -s)"
OUT=""
RUNS=5
ONLY_WORKLOAD=""
ONLY_SIZE=""
APPEND=0

# ---------------------------------------------------------------------------
# Tailles : 3 niveaux (petit / moyen / grand) par workload = les tailles de
# référence du prof (workloads/README.md). Ce sont AUSSI les tailles utilisées
# dans data/tasks.csv : le scheduler retrouve ainsi directement la durée mesurée
# de chaque tâche sur chaque VM, sans extrapolation.
# Les durées qui comptent sont celles mesurées sur les VM GCP (étape 3) ; si
# elles sortent de [MIN_RUNTIME_S, MAX_RUNTIME_S], on ajustera ces tailles.
# Les bornes MIN/MAX encadrent les tailles acceptées par l'option -s.
# ---------------------------------------------------------------------------
WORKLOADS="cpu_task memory_task matrix_task sort_task"

sizes_of() {
    case "$1" in
        cpu_task)    echo "200000 500000 1000000" ;;     # entier max testé
        memory_task) echo "128 256 512" ;;               # Mo alloués
        matrix_task) echo "200 400 600" ;;               # dimension N (NxN)
        sort_task)   echo "1000000 5000000 10000000" ;;  # nb d'entiers
        *) return 1 ;;
    esac
}

bounds_of() {   # affiche "MIN MAX"
    case "$1" in
        cpu_task)    echo "100000 2000000" ;;
        memory_task) echo "32 1024" ;;      # e2-medium = 4 Go de RAM
        matrix_task) echo "100 1000" ;;
        sort_task)   echo "250000 20000000" ;;
        *) return 1 ;;
    esac
}

usage() { sed -n '2,28p' "$0" | sed 's/^# \{0,1\}//'; exit "${1:-0}"; }
die()   { echo "Erreur : $*" >&2; exit 1; }

while getopts "m:w:s:r:o:ah" opt; do
    case "$opt" in
        m) MACHINE="$OPTARG" ;;
        w) ONLY_WORKLOAD="$OPTARG" ;;
        s) ONLY_SIZE="$OPTARG" ;;
        r) RUNS="$OPTARG" ;;
        o) OUT="$OPTARG" ;;
        a) APPEND=1 ;;
        h) usage 0 ;;
        *) usage 1 ;;
    esac
done

# ---------------------------------------------------------------- vérifications
[[ "$RUNS" =~ ^[1-9][0-9]*$ ]] || die "-r doit être un entier >= 1"
[[ "$MACHINE" =~ ^[A-Za-z0-9_.-]+$ ]] || die "-m : nom de machine invalide ($MACHINE)"
OUT="${OUT:-results/benchmark_${MACHINE}.csv}"
if [[ -n "$ONLY_WORKLOAD" ]]; then
    sizes_of "$ONLY_WORKLOAD" >/dev/null || die "workload inconnu : $ONLY_WORKLOAD (choix : $WORKLOADS)"
    WORKLOADS="$ONLY_WORKLOAD"
fi
if [[ -n "$ONLY_SIZE" ]]; then
    [[ -n "$ONLY_WORKLOAD" ]] || die "-s nécessite -w"
    [[ "$ONLY_SIZE" =~ ^[0-9]+$ ]] || die "-s doit être un entier"
    read -r lo hi <<< "$(bounds_of "$ONLY_WORKLOAD")"
    (( ONLY_SIZE >= lo && ONLY_SIZE <= hi )) \
        || die "taille $ONLY_SIZE hors bornes pour $ONLY_WORKLOAD : [$lo, $hi]"
fi
for w in $WORKLOADS; do
    [[ -x "$BIN_DIR/$w" ]] || die "exécutable $BIN_DIR/$w introuvable (lancer : make)"
done
command -v perl >/dev/null || die "perl est nécessaire pour mesurer le temps"
[[ -x /usr/bin/time ]]     || die "/usr/bin/time introuvable (Ubuntu : sudo apt install time)"

# GNU time (Linux) et BSD time (macOS) n'ont pas les mêmes options.
if /usr/bin/time -f %M true >/dev/null 2>&1; then
    TIME_MODE=gnu      # %M = RSS max en Ko
else
    TIME_MODE=bsd      # -l : "maximum resident set size" en octets
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# Exécute une fois le workload. Affiche "runtime_s max_rss_kb".
# Le temps est mesuré par perl (précision microseconde) autour de l'exécution.
run_once() {
    local bin="$1" size="$2" wall rss
    if [[ "$TIME_MODE" == gnu ]]; then
        set -- /usr/bin/time -f %M -o "$TMP/time" "$bin" --size "$size"
    else
        set -- /usr/bin/time -l -o "$TMP/time" "$bin" --size "$size"
    fi
    if ! perl -MTime::HiRes=time -e '
            my $t0 = time; my $rc = system(@ARGV); my $dt = time - $t0;
            open(my $f, ">", $ENV{WALL_FILE}) or die; printf $f "%.6f\n", $dt;
            exit($rc == 0 ? 0 : 1);' "$@" >"$TMP/out" 2>"$TMP/err"; then
        echo "Erreur : $bin --size $size a échoué" >&2
        cat "$TMP/err" >&2
        return 1
    fi
    wall="$(cat "$TMP/wall")"
    if [[ "$TIME_MODE" == gnu ]]; then
        rss="$(tail -n 1 "$TMP/time")"
    else
        rss="$(awk '/maximum resident set size/ {print int($1/1024)}' "$TMP/time")"
    fi
    [[ "$rss" =~ ^[0-9]+$ ]] || { echo "Erreur : RSS illisible pour $bin" >&2; return 1; }
    echo "$wall $rss"
}
export WALL_FILE="$TMP/wall"

# ---------------------------------------------------------------- campagne
mkdir -p "$(dirname "$OUT")"
if [[ "$APPEND" -eq 0 || ! -s "$OUT" ]]; then
    echo "machine,workload,size,run,runtime_s,max_rss_kb" > "$OUT"
fi

# Modèle de processeur : une VM e2 peut changer de famille de CPU à chaque démarrage.
CPU_MODEL="$(grep -m1 'model name' /proc/cpuinfo 2>/dev/null | cut -d: -f2- | sed 's/^ *//' || true)"
echo "Machine : $MACHINE ($(uname -sm), $(getconf _NPROCESSORS_ONLN) CPU logiques, ${CPU_MODEL:-CPU inconnu}) | runs=$RUNS | binaires=$BIN_DIR -> $OUT"

too_long=0
too_short=0
for w in $WORKLOADS; do
    sizes="${ONLY_SIZE:-$(sizes_of "$w")}"
    for size in $sizes; do
        # 1 exécution de chauffe, non mesurée
        "$BIN_DIR/$w" --size "$size" >/dev/null || die "$w --size $size a échoué (exécution de chauffe)"
        for run in $(seq 1 "$RUNS"); do
            res="$(run_once "$BIN_DIR/$w" "$size")" || exit 1
            read -r wall rss <<< "$res"
            echo "$MACHINE,$w,$size,$run,$wall,$rss" >> "$OUT"
            flag=""
            if awk -v t="$wall" -v m="$MAX_RUNTIME_S" 'BEGIN{exit !(t > m)}'; then
                flag="  <-- dépasse ${MAX_RUNTIME_S}s"; too_long=$((too_long + 1))
            elif awk -v t="$wall" -v m="$MIN_RUNTIME_S" 'BEGIN{exit !(t < m)}'; then
                flag="  (court : < ${MIN_RUNTIME_S}s)"; too_short=$((too_short + 1))
            fi
            printf "  %-12s size=%-8s run=%s  %8.4fs  %8d Ko%s\n" "$w" "$size" "$run" "$wall" "$rss" "$flag"
        done
    done
done

# ---------------------------------------------------------------- calculs
echo
echo "Résumé (calculé à partir des mesures brutes de $OUT) :"
# Colonnes : 1 machine, 2 workload, 3 size, 4 run, 5 runtime_s, 6 max_rss_kb
# On n'affiche que les lignes de la machine courante (utile avec -a).
awk -F, -v machine="$MACHINE" -v max="$MAX_RUNTIME_S" -v min="$MIN_RUNTIME_S" '
    NR == 1 || $1 != machine { next }
    {
        k = $2 "," $3
        if (!(k in n)) { order[++nk] = k; mn[k] = $5; mx[k] = $5 }
        n[k]++; s[k] += $5; ss[k] += $5 * $5; rss[k] += $6
        if ($5 < mn[k]) mn[k] = $5
        if ($5 > mx[k]) mx[k] = $5
    }
    END {
        printf "%-12s %9s %4s %9s %9s %9s %9s %6s %9s\n", "workload", "size", "runs", "moy(s)", "écart(s)", "min(s)", "max(s)", "cv%", "RSS(Mo)"
        for (i = 1; i <= nk; i++) {
            k = order[i]; split(k, p, ",")
            m = s[k] / n[k]
            v = (n[k] > 1) ? (ss[k] - n[k] * m * m) / (n[k] - 1) : 0
            sd = (v > 0) ? sqrt(v) : 0
            note = (mx[k] > max) ? "  TROP LONG" : ((m < min) ? "  court" : "")
            printf "%-12s %9s %4d %9.4f %9.4f %9.4f %9.4f %6.1f %9.1f%s\n", p[1], p[2], n[k], m, sd, mn[k], mx[k], (m > 0 ? 100 * sd / m : 0), rss[k] / n[k] / 1024, note
        }
    }' "$OUT"

if (( too_long > 0 )); then
    echo
    echo "Attention : $too_long exécution(s) au-delà de ${MAX_RUNTIME_S}s : réduire les tailles dans sizes_of()."
fi
if (( too_short > 0 )); then
    echo
    echo "Note : $too_short exécution(s) sous ${MIN_RUNTIME_S}s sur $MACHINE. Normal sur une machine rapide ;"
    echo "       ce sont les durées sur les VM GCP qui servent à calibrer les tailles."
fi
