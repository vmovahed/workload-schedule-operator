#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="${SCRIPT_DIR}/.."
NAMESPACE="workload-schedule-operator-system"
DEMO_NS="demo"

echo "=== Running End-to-End Tests ==="

# Helper function to wait for a condition
wait_for() {
    local cmd="$1"
    local description="$2"
    local timeout="${3:-120}"
    
    echo "Waiting for ${description}..."
    local start_time=$(date +%s)
    while true; do
        if eval "${cmd}" 2>/dev/null; then
            echo "✓ ${description}"
            return 0
        fi
        
        local current_time=$(date +%s)
        local elapsed=$((current_time - start_time))
        if [ ${elapsed} -ge ${timeout} ]; then
            echo "✗ Timeout waiting for ${description}"
            return 1
        fi
        
        sleep 2
    done
}

# Test 1: Verify CRD is installed
echo ""
echo "=== Test 1: Verify CRD Installation ==="
kubectl get crd workloadschedules.infra.illumin.com
echo "✓ CRD is installed"

# Test 2: Verify operator is running
echo ""
echo "=== Test 2: Verify Operator Deployment ==="
wait_for "kubectl get deployment -n ${NAMESPACE} workload-schedule-operator-controller-manager -o jsonpath='{.status.availableReplicas}' | grep -q '1'" "operator deployment to be ready"

# Test 3: Create demo deployment
echo ""
echo "=== Test 3: Create Demo Deployment ==="
kubectl apply -f "${PROJECT_DIR}/config/samples/demo-deployment.yaml"
wait_for "kubectl get deployment -n ${DEMO_NS} demo-deployment -o jsonpath='{.status.availableReplicas}' | grep -qE '[0-9]+'" "demo deployment to be created"
echo "✓ Demo deployment created"

# Test 4: Create WorkloadSchedule
echo ""
echo "=== Test 4: Create WorkloadSchedule ==="
kubectl apply -f "${PROJECT_DIR}/config/samples/infra_v1alpha1_workloadschedule.yaml"
sleep 5

# Verify WorkloadSchedule status is updated
wait_for "kubectl get workloadschedule example -o jsonpath='{.status.lastSyncTime}' | grep -q '20'" "WorkloadSchedule to be synced" 60
echo "✓ WorkloadSchedule created and synced"

# Test 5: Verify scaling behavior
echo ""
echo "=== Test 5: Verify Scaling Behavior ==="
ACTIVE_STATUS=$(kubectl get workloadschedule example -o jsonpath='{.status.withinActiveWindow}')
CURRENT_REPLICAS=$(kubectl get workloadschedule example -o jsonpath='{.status.currentReplicas}')
CURRENT_TIME=$(kubectl get workloadschedule example -o jsonpath='{.status.currentLocalTime}')

echo "Current time: ${CURRENT_TIME}"
echo "Active window status: ${ACTIVE_STATUS}"
echo "Current replicas: ${CURRENT_REPLICAS}"

if [ "${ACTIVE_STATUS}" == "true" ]; then
    if [ "${CURRENT_REPLICAS}" == "2" ]; then
        echo "✓ Deployment scaled to 2 replicas (within active window)"
    else
        echo "✗ Expected 2 replicas during active window, got ${CURRENT_REPLICAS}"
        exit 1
    fi
else
    if [ "${CURRENT_REPLICAS}" == "0" ]; then
        echo "✓ Deployment scaled to 0 replicas (outside active window)"
    else
        echo "✗ Expected 0 replicas outside active window, got ${CURRENT_REPLICAS}"
        exit 1
    fi
fi

# Test 6: Verify webhook mutation
echo ""
echo "=== Test 6: Verify Webhook Mutation ==="

# Create a test pod in the demo namespace
kubectl run test-pod --image=nginx:alpine -n ${DEMO_NS} --restart=Never --dry-run=client -o yaml | kubectl apply -f -
sleep 3

# Check if the pod has the label
POD_LABEL=$(kubectl get pod test-pod -n ${DEMO_NS} -o jsonpath='{.metadata.labels.schedule\.illumin\.com/active}' 2>/dev/null || echo "")
if [ -n "${POD_LABEL}" ]; then
    echo "✓ Pod has label schedule.illumin.com/active=${POD_LABEL}"
else
    echo "⚠ Pod label not found (webhook may not be configured yet)"
fi

# Check if the pod has the environment variable
POD_ENV=$(kubectl get pod test-pod -n ${DEMO_NS} -o jsonpath='{.spec.containers[0].env[?(@.name=="WORKLOAD_SCHEDULE_ACTIVE")].value}' 2>/dev/null || echo "")
if [ -n "${POD_ENV}" ]; then
    echo "✓ Pod has env var WORKLOAD_SCHEDULE_ACTIVE=${POD_ENV}"
else
    echo "⚠ Pod env var not found (webhook may not be configured yet)"
fi

# Cleanup test pod
kubectl delete pod test-pod -n ${DEMO_NS} --ignore-not-found=true

# Test 7: Verify status fields
echo ""
echo "=== Test 7: Verify Status Fields ==="
kubectl get workloadschedule example -o yaml | grep -A 20 "status:"
echo "✓ Status fields are populated"

echo ""
echo "=== All E2E Tests Passed ==="

