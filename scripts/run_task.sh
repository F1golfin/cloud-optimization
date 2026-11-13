#!/usr/bin/env bash
set -euo pipefail

# Check arguments
if [ "$#" -ne 5 ]; then
    echo "Usage: $0 TASK_ID WORKLOAD SIZE VM ZONE" >&2
    exit 1
fi

TASK_ID="$1"
WORKLOAD="$2"
SIZE="$3"
VM="$4"
ZONE="$5"

PROJECT="cloud-opt-511106"

# Go to project root
cd "$(dirname "$0")/.."

# Create results directory
mkdir -p results

echo "[$TASK_ID] Starting $WORKLOAD on $VM..."

# Execute the workload remotely and collect outputs
if gcloud compute ssh "$VM" \
    --project="$PROJECT" \
    --zone="$ZONE" \
    --command="cd ~/cloud-optimization && ./bin/$WORKLOAD --size $SIZE" \
    > "results/$TASK_ID.out" \
    2> "results/$TASK_ID.err"; then

    echo "[$TASK_ID] Completed successfully."
else
    status=$?
    echo "[$TASK_ID] Failed (exit code: $status)." >&2
    exit "$status"
fi
