#!/bin/bash
set -e

CLUSTER_NAME="${CLUSTER_NAME:-workload-schedule-operator}"

echo "=== Deleting Kind cluster: ${CLUSTER_NAME} ==="

# Check if cluster exists
if ! kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    echo "Cluster '${CLUSTER_NAME}' does not exist. Nothing to delete."
    exit 0
fi

# Delete the cluster
kind delete cluster --name "${CLUSTER_NAME}"

echo "=== Cluster '${CLUSTER_NAME}' deleted ==="

