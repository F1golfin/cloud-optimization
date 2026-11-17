#!/usr/bin/env bash
# Configuration GCP commune à tous les scripts (à « sourcer », pas à exécuter).
#
# Les VM sont décrites à UN SEUL endroit : data/machines.csv (nom, type GCP,
# vCPU, RAM, prix, contention). Ce fichier est lu à la fois par le scheduler
# (modèle de coût et de durée) et par ces scripts (création, démarrage, arrêt,
# déploiement) : ajouter, retirer ou changer le type d'une VM, c'est modifier
# une ligne de ce CSV, rien d'autre.

PROJECT="${PROJECT:-cloud-opt-511106}"
ZONE="${ZONE:-europe-west1-b}"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MACHINES_CSV="${MACHINES_CSV:-$ROOT_DIR/data/machines.csv}"
[[ -r "$MACHINES_CSV" ]] || { echo "Erreur : $MACHINES_CSV introuvable" >&2; exit 1; }

# csv_column NOM : valeurs d'une colonne de machines.csv (repérée par son en-tête).
csv_column() {
    awk -F, -v col="$1" '
        NR == 1 { for (i = 1; i <= NF; i++) if ($i == col) c = i; next }
        c && $1 != "" { print $c }' "$MACHINES_CSV"
}

# Toutes les VM du CSV par défaut. On peut en cibler une partie :
#   VMS="cloudopt-vm1" ./scripts/...
VMS="${VMS:-$(csv_column vm | xargs)}"

# Type de machine GCP d'une VM, lu dans machines.csv.
machine_type_of() {
    local t
    t="$(awk -F, -v vm="$1" '
        NR == 1 { for (i = 1; i <= NF; i++) if ($i == "machine_type") c = i; next }
        $1 == vm { print $c }' "$MACHINES_CSV")"
    [[ -n "$t" ]] || { echo "VM inconnue : $1 (absente de $MACHINES_CSV)" >&2; return 1; }
    echo "$t"
}

# Une VM demandée dans VMS mais absente du CSV est sûrement une faute de frappe.
for _vm in $VMS; do
    machine_type_of "$_vm" > /dev/null || exit 1
done
unset _vm

# Dossier de travail sur chaque VM (binaires + script de benchmark).
REMOTE_DIR="cloudopt"

# Raccourcis gcloud avec projet et zone déjà renseignés.
gssh() { local vm="$1"; shift; gcloud compute ssh "$vm" --project="$PROJECT" --zone="$ZONE" --quiet "$@"; }
gscp() { gcloud compute scp --project="$PROJECT" --zone="$ZONE" --quiet "$@"; }

command -v gcloud >/dev/null || { echo "Erreur : gcloud introuvable dans le PATH" >&2; exit 1; }
