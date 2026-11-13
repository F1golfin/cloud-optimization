#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

CSV="${1:-data/allocation_manual.csv}"
ZONE="europe-west1-b"

mkdir -p results

pids=()
task_ids=()

echo "Starting distributed execution..."

# Read CSV, skipping header
while IFS=, read -r task_id workload size vm; do
    [[ "$task_id" == "task_id" || -z "$task_id" ]] && continue

    ./scripts/run_task.sh \
        "$task_id" "$workload" "$size" "$vm" "$ZONE" &

    pids+=("$!")
    task_ids+=("$task_id")

done < "$CSV"

# Wait for every launched task and check its result
failed=0

for i in "${!pids[@]}"; do
    if wait "${pids[$i]}"; then
        echo "[${task_ids[$i]}] SUCCESS"
    else
        echo "[${task_ids[$i]}] FAILED"
        failed=1
    fi
done

if [[ "$failed" -eq 0 ]]; then
    echo "All tasks completed successfully!"
else
    echo "Some tasks failed." >&2
    exit 1
fi
