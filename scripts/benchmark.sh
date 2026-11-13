#!/usr/bin/env bash
# Benchmark local des workloads (TD1), décliné en plusieurs tailles par workload.
#
# Usage :
#   ./scripts/benchmark.sh                      # toutes les déclinaisons, 3 runs
#   ./scripts/benchmark.sh -r 5                 # 5 runs par déclinaison
#   ./scripts/benchmark.sh -w matrix_task       # un seul workload
#   ./scripts/benchmark.sh -w cpu_task -s 300000 -r 3 -a   # une taille, ajout au CSV
#
# Options :
#   -w WORKLOAD   cpu_task | memory_task | matrix_task | sort_task (défaut : tous)
#   -s SIZE       une seule taille (nécessite -w), doit rester dans les bornes
#   -r RUNS       nombre d'exécutions par taille (défaut : 3)
#   -o FILE       CSV de sortie (défaut : data/benchmark_local.csv)
#   -a            ajoute au CSV existant au lieu de le recréer
#
# Variables d'environnement :
#   BIN_DIR        dossier des binaires (défaut : bin/local, créé par `make`)
#   MAX_RUNTIME_S  durée max acceptable d'une tâche en secondes (défaut : 2)
#
# Sortie : une ligne par exécution  workload,size,run,runtime_s,max_rss_kb
#   runtime_s  = temps réel (wall-clock) de l'exécution, en secondes
#   max_rss_kb = mémoire physique maximale utilisée, en Ko
set -euo pipefail
cd "$(dirname "$0")/.."

BIN_DIR="${BIN_DIR:-bin/local}"
MAX_RUNTIME_S="${MAX_RUNTIME_S:-2}"
OUT="data/benchmark_local.csv"
RUNS=3
ONLY_WORKLOAD=""
ONLY_SIZE=""
APPEND=0

# ---------------------------------------------------------------------------
# Déclinaisons : 5 tailles par workload, toutes inférieures aux tailles
# d'origine (cpu 1 000 000, mémoire 512 Mo, matrice 600, tri 5 000 000)
# pour qu'aucune tâche ne dure trop longtemps, y compris sur une VM e2-medium.
# Les bornes MIN/MAX encadrent les tailles acceptées par le script.
# ---------------------------------------------------------------------------
WORKLOADS="cpu_task memory_task matrix_task sort_task"

sizes_of() {
    case "$1" in
        cpu_task)    echo "100000 200000 300000 400000 500000" ;;  # entier max testé
        memory_task) echo "32 64 128 192 256" ;;                   # Mo alloués
        matrix_task) echo "200 250 300 350 400" ;;                 # dimension N (NxN)
        sort_task)   echo "250000 500000 1000000 1500000 2000000" ;; # nb d'entiers
        *) return 1 ;;
    esac
}

bounds_of() {   # affiche "MIN MAX"
    case "$1" in
        cpu_task)    echo "100000 500000" ;;
        memory_task) echo "32 256" ;;
        matrix_task) echo "200 400" ;;
        sort_task)   echo "250000 2000000" ;;
        *) return 1 ;;
    esac
}

usage() { sed -n '2,24p' "$0" | sed 's/^# \{0,1\}//'; exit "${1:-0}"; }
die()   { echo "Erreur : $*" >&2; exit 1; }

while getopts "w:s:r:o:ah" opt; do
    case "$opt" in
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
    echo "workload,size,run,runtime_s,max_rss_kb" > "$OUT"
fi

echo "Machine : $(uname -sm), $(getconf _NPROCESSORS_ONLN) CPU logiques | runs=$RUNS | binaires=$BIN_DIR -> $OUT"

too_long=0
for w in $WORKLOADS; do
    sizes="${ONLY_SIZE:-$(sizes_of "$w")}"
    for size in $sizes; do
        # 1 exécution de chauffe, non mesurée
        "$BIN_DIR/$w" --size "$size" >/dev/null || die "$w --size $size a échoué (exécution de chauffe)"
        for run in $(seq 1 "$RUNS"); do
            res="$(run_once "$BIN_DIR/$w" "$size")" || exit 1
            read -r wall rss <<< "$res"
            echo "$w,$size,$run,$wall,$rss" >> "$OUT"
            flag=""
            if awk -v t="$wall" -v m="$MAX_RUNTIME_S" 'BEGIN{exit !(t > m)}'; then
                flag="  <-- dépasse ${MAX_RUNTIME_S}s"; too_long=$((too_long + 1))
            fi
            printf "  %-12s size=%-8s run=%s  %8.4fs  %8d Ko%s\n" "$w" "$size" "$run" "$wall" "$rss" "$flag"
        done
    done
done

# ---------------------------------------------------------------- calculs
echo
echo "Résumé (calculé à partir des mesures brutes de $OUT) :"
awk -F, -v max="$MAX_RUNTIME_S" '
    NR == 1 { next }
    {
        k = $1 "," $2
        if (!(k in n)) { order[++nk] = k; mn[k] = $4; mx[k] = $4 }
        n[k]++; s[k] += $4; ss[k] += $4 * $4; rss[k] += $5
        if ($4 < mn[k]) mn[k] = $4
        if ($4 > mx[k]) mx[k] = $4
    }
    END {
        printf "%-12s %9s %4s %9s %9s %9s %9s %6s %9s\n", "workload", "size", "runs", "moy(s)", "écart(s)", "min(s)", "max(s)", "cv%", "RSS(Mo)"
        for (i = 1; i <= nk; i++) {
            k = order[i]; split(k, p, ",")
            m = s[k] / n[k]
            v = (n[k] > 1) ? (ss[k] - n[k] * m * m) / (n[k] - 1) : 0
            sd = (v > 0) ? sqrt(v) : 0
            printf "%-12s %9s %4d %9.4f %9.4f %9.4f %9.4f %6.1f %9.1f%s\n", p[1], p[2], n[k], m, sd, mn[k], mx[k], (m > 0 ? 100 * sd / m : 0), rss[k] / n[k] / 1024, (mx[k] > max ? "  TROP LONG" : "")
        }
    }' "$OUT"

if (( too_long > 0 )); then
    echo
    echo "Attention : $too_long exécution(s) au-delà de ${MAX_RUNTIME_S}s : réduire les tailles dans sizes_of()."
fi
