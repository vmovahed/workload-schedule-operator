#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLUSTER_NAME="${CLUSTER_NAME:-workload-schedule-operator}"
KIND_CONFIG="${SCRIPT_DIR}/kind-config.yaml"

echo "=== Creating Kind cluster: ${CLUSTER_NAME} ==="

# Check if cluster already exists
if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    echo "Cluster '${CLUSTER_NAME}' already exists. Skipping creation."
    kubectl cluster-info --context "kind-${CLUSTER_NAME}" || true
    exit 0
fi

# Create the cluster
echo "Creating Kind cluster with config: ${KIND_CONFIG}"
kind create cluster --name "${CLUSTER_NAME}" --config "${KIND_CONFIG}"

# Wait for cluster to be ready
echo "Waiting for cluster to be ready..."
kubectl wait --for=condition=Ready nodes --all --timeout=120s

echo "=== Cluster '${CLUSTER_NAME}' is ready ==="
kubectl cluster-info --context "kind-${CLUSTER_NAME}"

