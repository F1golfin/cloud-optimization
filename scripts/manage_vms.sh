
#!/usr/bin/env bash
set -euo pipefail

PROJECT="cloud-opt-511106"
ZONE="europe-west1-b"
MACHINE_TYPE="e2-medium"
VMS=("cloudopt-vm1" "cloudopt-vm2" "cloudopt-vm3")

create_vms() {
    for vm in "${VMS[@]}"; do
        echo "Creating $vm..."

        if gcloud compute instances describe "$vm" \
            --project="$PROJECT" --zone="$ZONE" &>/dev/null; then
            echo "$vm already exists, skipping."
            continue
        fi

        gcloud compute instances create "$vm" \
            --project="$PROJECT" \
            --zone="$ZONE" \
            --machine-type="$MACHINE_TYPE" \
            --image-family=ubuntu-2404-lts-amd64 \
            --image-project=ubuntu-os-cloud \
            --boot-disk-size=20GB \
            --boot-disk-type=pd-balanced
    done
}

start_vms() {
    for vm in "${VMS[@]}"; do
        echo "Starting $vm..."
        gcloud compute instances start "$vm" \
            --project="$PROJECT" --zone="$ZONE"
    done
}

stop_vms() {
    for vm in "${VMS[@]}"; do
        echo "Stopping $vm..."
        gcloud compute instances stop "$vm" \
            --project="$PROJECT" --zone="$ZONE"
    done
}

status_vms() {
    gcloud compute instances list \
        --project="$PROJECT" \
        --filter="name~'^cloudopt-vm[123]$'" \
        --format="table(name,zone.basename(),machineType.basename(),status)"
}

delete_vms() {
    echo "This will permanently delete the three VMs and their auto-deleted disks."
    read -r -p "Type DELETE to confirm: " confirm
    [[ "$confirm" == "DELETE" ]] || exit 1

    for vm in "${VMS[@]}"; do
        gcloud compute instances delete "$vm" \
            --project="$PROJECT" --zone="$ZONE" --quiet
    done
}

case "${1:-}" in
    create) create_vms ;;
    start)  start_vms ;;
    stop)   stop_vms ;;
    status) status_vms ;;
    delete) delete_vms ;;
    *)
        echo "Usage: $0 {create|start|stop|status|delete}"
        exit 1
        ;;
esac
