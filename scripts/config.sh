#!/usr/bin/env bash
# Configuration GCP commune à tous les scripts (à « sourcer », pas à exécuter).
#
# Un seul endroit pour le projet, la zone et la liste des VM : les noms de VM
# ici sont les MÊMES que ceux écrits par le scheduler dans allocation.csv et
# lus par l'orchestrateur.

# Chaque membre du groupe peut utiliser son propre projet GCP : créer
# scripts/config.local.sh (non commité) contenant par exemple
#   PROJECT="mon-projet-123"
[[ -f "$(dirname "${BASH_SOURCE[0]}")/config.local.sh" ]] && source "$(dirname "${BASH_SOURCE[0]}")/config.local.sh"

PROJECT="${PROJECT:-cloud-opt-511106}"
ZONE="${ZONE:-europe-west1-b}"

# Les 4 VM du projet. On peut en cibler une partie : VMS="cloudopt-vm1" ./scripts/...
VMS="${VMS:-cloudopt-vm1 cloudopt-vm2 cloudopt-vm3 cloudopt-highcpu}"

# Type de machine de chaque VM (prix europe-west1 utilisés dans le modèle de coût).
machine_type_of() {
    case "$1" in
        cloudopt-vm1)     echo "e2-standard-2" ;;  # 2 vCPU, 8 Go   – 0,0737 $/h
        cloudopt-vm2)     echo "e2-medium" ;;      # 2 vCPU partagés, 4 Go – 0,0369 $/h
        cloudopt-vm3)     echo "e2-medium" ;;      # idem vm2, mesurée séparément
        cloudopt-highcpu) echo "e2-highcpu-4" ;;   # 4 vCPU, 4 Go   – 0,1088 $/h
        *) echo "VM inconnue : $1" >&2; return 1 ;;
    esac
}

# Dossier de travail sur chaque VM (binaires + script de benchmark).
REMOTE_DIR="cloudopt"

# Raccourcis gcloud avec projet et zone déjà renseignés.
gssh() { local vm="$1"; shift; gcloud compute ssh "$vm" --project="$PROJECT" --zone="$ZONE" --quiet "$@"; }
gscp() { gcloud compute scp --project="$PROJECT" --zone="$ZONE" --quiet "$@"; }

command -v gcloud >/dev/null || { echo "Erreur : gcloud introuvable dans le PATH" >&2; exit 1; }
