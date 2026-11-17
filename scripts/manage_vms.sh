#!/usr/bin/env bash
# Gestion du cycle de vie des VM du projet : celles de data/machines.csv
# (nom et type GCP), lues par scripts/config.sh.
#
# Usage : ./scripts/manage_vms.sh {create|start|stop|status|delete}
#
#   create  crée les VM manquantes avec le bon type ; si une VM existe déjà
#           ARRÊTÉE avec un mauvais type, corrige son type (set-machine-type)
#   start   démarre les VM (facturation à partir de là)
#   stop    arrête les VM  (À FAIRE APRÈS CHAQUE SESSION DE TESTS)
#   status  affiche nom, type et état
#   delete  supprime définitivement les VM (demande confirmation)
#
# Cibler une seule VM : VMS="cloudopt-highcpu" ./scripts/manage_vms.sh start
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/config.sh

# Renvoie "TYPE STATUT" d'une VM, ou rien si elle n'existe pas.
describe() {
    gcloud compute instances describe "$1" --project="$PROJECT" --zone="$ZONE" \
        --format="value(machineType.basename(),status)" 2>/dev/null || true
}

create_vms() {
    for vm in $VMS; do
        want="$(machine_type_of "$vm")"
        read -r have state <<< "$(describe "$vm")"
        if [[ -z "${have:-}" ]]; then
            echo "Création de $vm ($want)..."
            gcloud compute instances create "$vm" \
                --project="$PROJECT" --zone="$ZONE" \
                --machine-type="$want" \
                --image-family=ubuntu-2404-lts-amd64 --image-project=ubuntu-os-cloud \
                --boot-disk-size=20GB --boot-disk-type=pd-balanced
        elif [[ "$have" == "$want" ]]; then
            echo "$vm existe déjà ($have, $state)."
        elif [[ "$state" == "TERMINATED" ]]; then
            echo "$vm : type $have -> $want"
            gcloud compute instances set-machine-type "$vm" \
                --project="$PROJECT" --zone="$ZONE" --machine-type="$want"
        else
            echo "Attention : $vm est $have (attendu $want) mais $state ; l'arrêter puis relancer create." >&2
        fi
    done
}

start_vms() {
    # shellcheck disable=SC2086
    gcloud compute instances start $VMS --project="$PROJECT" --zone="$ZONE"
}

stop_vms() {
    # shellcheck disable=SC2086
    gcloud compute instances stop $VMS --project="$PROJECT" --zone="$ZONE"
}

status_vms() {
    gcloud compute instances list --project="$PROJECT" \
        --filter="name=( $VMS )" \
        --format="table(name,zone.basename(),machineType.basename(),status)"
}

delete_vms() {
    echo "Suppression DÉFINITIVE de : $VMS (disques compris)."
    read -r -p "Taper DELETE pour confirmer : " confirm
    [[ "$confirm" == "DELETE" ]] || { echo "Annulé."; exit 1; }
    # shellcheck disable=SC2086
    gcloud compute instances delete $VMS --project="$PROJECT" --zone="$ZONE" --quiet
}

case "${1:-}" in
    create) create_vms ;;
    start)  start_vms ;;
    stop)   stop_vms ;;
    status) status_vms ;;
    delete) delete_vms ;;
    *) sed -n '2,14p' "$0" | sed 's/^# \{0,1\}//'; exit 1 ;;
esac
