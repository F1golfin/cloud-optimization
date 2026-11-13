
#!/usr/bin/env bash
set -euo pipefail

PROJECT="cloud-opt-511106"
ZONE="europe-west1-b"
VMS=("cloudopt-vm1" "cloudopt-vm2" "cloudopt-vm3")

cd "$(dirname "$0")/.."

for vm in "${VMS[@]}"; do
    echo "===== Deploying to $vm ====="

    # Create remote project directory
    gcloud compute ssh "$vm" \
        --project="$PROJECT" \
        --zone="$ZONE" \
        --command="mkdir -p ~/cloud-optimization/workloads ~/cloud-optimization/bin"

    # Copy Go source files
    gcloud compute scp --recurse workloads \
        "$vm:~/cloud-optimization/" \
        --project="$PROJECT" \
        --zone="$ZONE"

    # Install Go if needed, then compile workloads
    gcloud compute ssh "$vm" \
        --project="$PROJECT" \
        --zone="$ZONE" \
        --command='
            set -e
            if ! command -v go >/dev/null 2>&1; then
                sudo apt-get update
                sudo apt-get install -y golang-go
            fi

            cd ~/cloud-optimization
            mkdir -p bin

            go build -o bin/cpu_task workloads/cpu_task.go
            go build -o bin/memory_task workloads/memory_task.go
            go build -o bin/matrix_task workloads/matrix_task.go

            echo "Compilation successful"
            ls -lh bin/
        '

    echo "===== $vm ready ====="
done

echo "All VMs deployed successfully!"
