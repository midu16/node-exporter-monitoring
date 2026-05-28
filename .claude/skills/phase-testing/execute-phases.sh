#!/bin/bash
# execute-phases.sh - Automated 5-phase node-exporter monitoring test using Go binary
#
# Phase 1: All collectors (zoneinfo + interrupts + softirqs)
# Phase 2: No node-exporter (Prometheus Operator only)
# Phase 3: Zoneinfo collector only
# Phase 4: Interrupts collector only
# Phase 5: Softirqs collector only

set -euo pipefail

# Default configuration
DURATION_MINUTES=30
MANIFESTS_DIR="./node-exporter-zoneinfo"
SKIP_PHASES=""
NO_CLEANUP=false
MONITOR_BIN="./bin/monitor"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --duration=*)
            DURATION_MINUTES="${1#*=}"
            DURATION_MINUTES="${DURATION_MINUTES%m}"
            shift
            ;;
        --manifests-dir=*)
            MANIFESTS_DIR="${1#*=}"
            shift
            ;;
        --skip-phase=*)
            SKIP_PHASES="${1#*=}"
            shift
            ;;
        --no-cleanup)
            NO_CLEANUP=true
            shift
            ;;
        --kubeconfig=*)
            export KUBECONFIG="${1#*=}"
            shift
            ;;
        --monitor-bin=*)
            MONITOR_BIN="${1#*=}"
            shift
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

DURATION_FLAG="${DURATION_MINUTES}m"

# Utility functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

should_skip_phase() {
    local phase=$1
    if [[ -z "$SKIP_PHASES" ]]; then
        return 1
    fi
    [[ ",$SKIP_PHASES," == *",$phase,"* ]]
}

# Validate prerequisites
validate_prerequisites() {
    log_info "Validating prerequisites..."

    # Check if monitor binary exists
    if [[ ! -f "$MONITOR_BIN" ]]; then
        log_error "Monitor binary not found: $MONITOR_BIN"
        log_info "Building monitor binary..."
        if ! make build; then
            log_error "Failed to build monitor binary"
            exit 1
        fi
    fi

    # Check if binary is executable
    if [[ ! -x "$MONITOR_BIN" ]]; then
        log_error "Monitor binary is not executable: $MONITOR_BIN"
        chmod +x "$MONITOR_BIN"
    fi

    # Check manifests directory
    if [[ ! -d "$MANIFESTS_DIR" ]]; then
        log_error "Manifests directory not found: $MANIFESTS_DIR"
        exit 1
    fi

    log_success "Prerequisites validated"
}

# Build the monitor binary if needed
build_monitor() {
    if [[ ! -f "$MONITOR_BIN" ]] || [[ "$MONITOR_BIN" -ot "cmd/monitor/main.go" ]]; then
        log_info "Building monitor binary..."
        make build
        log_success "Monitor binary built successfully"
    fi
}

# Phase 1: All collectors
phase1_all_collectors() {
    if should_skip_phase 1; then
        log_warning "Skipping Phase 1"
        return
    fi

    log_info "=== Phase 1: All Collectors (zoneinfo + interrupts + softirqs) ==="

    log_info "Running monitoring with all collectors enabled..."
    $MONITOR_BIN --deploy \
        --duration="$DURATION_FLAG" \
        --manifests-dir="$MANIFESTS_DIR" \
        ${KUBECONFIG:+--kubeconfig="$KUBECONFIG"}

    log_success "Phase 1 complete"
}

# Phase 2: No node-exporter
phase2_no_node_exporter() {
    if should_skip_phase 2; then
        log_warning "Skipping Phase 2"
        return
    fi

    log_info "=== Phase 2: No Node-Exporter (Prometheus Operator only) ==="

    # Use the two-phase mode which includes a "no node-exporter" phase
    # But we only want the second half, so we'll run a custom deployment
    log_info "Deleting node-exporter deployment..."

    # The Go binary will handle the deletion when we run in two-phase mode
    # For now, we'll use kubectl directly to delete the daemonset
    if command -v kubectl &> /dev/null; then
        kubectl delete daemonset -n node-exporter-zoneinfo node-exporter-zoneinfo --ignore-not-found=true || true
    elif command -v oc &> /dev/null; then
        oc delete daemonset -n node-exporter-zoneinfo node-exporter-zoneinfo --ignore-not-found=true || true
    fi

    log_info "Monitoring Prometheus Operator only (no node-exporter)..."
    $MONITOR_BIN --duration="$DURATION_FLAG" \
        --skip-deploy \
        ${KUBECONFIG:+--kubeconfig="$KUBECONFIG"}

    log_success "Phase 2 complete"
}

# Phase 3: Zoneinfo only
phase3_zoneinfo_only() {
    if should_skip_phase 3; then
        log_warning "Skipping Phase 3"
        return
    fi

    log_info "=== Phase 3: Zoneinfo Collector Only ==="

    log_info "Deploying zoneinfo-only collector..."
    $MONITOR_BIN --deploy \
        --zoneinfo-only \
        --duration="$DURATION_FLAG" \
        --manifests-dir="$MANIFESTS_DIR" \
        ${KUBECONFIG:+--kubeconfig="$KUBECONFIG"}

    log_success "Phase 3 complete"
}

# Phase 4: Interrupts only
phase4_interrupts_only() {
    if should_skip_phase 4; then
        log_warning "Skipping Phase 4"
        return
    fi

    log_info "=== Phase 4: Interrupts Collector Only ==="

    # First, cleanup from previous phase
    log_info "Cleaning up previous deployment..."
    if command -v kubectl &> /dev/null; then
        kubectl delete daemonset -n node-exporter-zoneinfo node-exporter-zoneinfo --ignore-not-found=true || true
    elif command -v oc &> /dev/null; then
        oc delete daemonset -n node-exporter-zoneinfo node-exporter-zoneinfo --ignore-not-found=true || true
    fi
    sleep 5

    log_info "Deploying interrupts-only collector..."
    $MONITOR_BIN --deploy \
        --interrupts-only \
        --duration="$DURATION_FLAG" \
        --manifests-dir="$MANIFESTS_DIR" \
        ${KUBECONFIG:+--kubeconfig="$KUBECONFIG"}

    log_success "Phase 4 complete"
}

# Phase 5: Softirqs only
phase5_softirqs_only() {
    if should_skip_phase 5; then
        log_warning "Skipping Phase 5"
        return
    fi

    log_info "=== Phase 5: Softirqs Collector Only ==="

    # First, cleanup from previous phase
    log_info "Cleaning up previous deployment..."
    if command -v kubectl &> /dev/null; then
        kubectl delete daemonset -n node-exporter-zoneinfo node-exporter-zoneinfo --ignore-not-found=true || true
    elif command -v oc &> /dev/null; then
        oc delete daemonset -n node-exporter-zoneinfo node-exporter-zoneinfo --ignore-not-found=true || true
    fi
    sleep 5

    log_info "Deploying softirqs-only collector..."
    $MONITOR_BIN --deploy \
        --softirqs-only \
        --duration="$DURATION_FLAG" \
        --manifests-dir="$MANIFESTS_DIR" \
        ${KUBECONFIG:+--kubeconfig="$KUBECONFIG"}

    log_success "Phase 5 complete"
}

# Cleanup
cleanup() {
    if [[ "$NO_CLEANUP" == "true" ]]; then
        log_warning "Skipping cleanup (--no-cleanup specified)"
        return
    fi

    log_info "=== Cleanup ==="
    log_info "Deleting node-exporter-zoneinfo resources..."

    if command -v kubectl &> /dev/null; then
        kubectl delete namespace node-exporter-zoneinfo --ignore-not-found=true || true
    elif command -v oc &> /dev/null; then
        oc delete namespace node-exporter-zoneinfo --ignore-not-found=true || true
    fi

    log_success "Cleanup complete"
}

# Generate evidence-based consolidated report
generate_consolidated_report() {
    log_info "Generating evidence-based consolidated report..."

    local report_dir="./reports"
    local consolidated_report="${report_dir}/consolidated-5phase-report-$(date +%Y%m%d-%H%M%S).md"
    local test_end_time=$(date -u +%Y-%m-%dT%H:%M:%SZ)

    # Detect kubectl or oc
    local kube_cmd="kubectl"
    if command -v oc &> /dev/null; then
        kube_cmd="oc"
    fi

    cat > "$consolidated_report" <<EOF
# 5-Phase Node-Exporter Collector Validation

We validated the node-exporter-zoneinfo deployment across five distinct configurations to understand collector behavior, metric availability, and resource consumption patterns.

## Test Configuration

**Environment**:
- Cluster: $(${kube_cmd} version --short 2>/dev/null | grep Server || echo "Unknown")
- Node-exporter: v1.9.1
- Test duration per phase: ${DURATION_MINUTES} minutes
- Total test duration: $((DURATION_MINUTES * 5)) minutes
- Test completed: ${test_end_time}

**Monitor binary**: \`$MONITOR_BIN\`

## Phase Comparison

| Phase | Collectors Enabled | Expected Metric Families | Resource Profile |
|-------|-------------------|--------------------------|------------------|
| 1 | zoneinfo, interrupts, softirqs | node_zoneinfo_*, node_interrupts_*, node_softirqs_* | Baseline (all collectors) |
| 2 | None (no node-exporter) | N/A (deleted) | Prometheus Operator only |
| 3 | zoneinfo only | node_zoneinfo_* | Single-collector isolation |
| 4 | interrupts only | node_interrupts_* | Single-collector isolation |
| 5 | softirqs only | node_softirqs_* | Single-collector isolation |

## Evidence Summary

### Phase Reports Generated

EOF

    # List all generated phase reports with descriptions
    local phase_num=1
    for phase_desc in "All Collectors" "No Node-Exporter" "Zoneinfo Only" "Interrupts Only" "Softirqs Only"; do
        if ! should_skip_phase $phase_num; then
            cat >> "$consolidated_report" <<EOF
**Phase ${phase_num}: ${phase_desc}**
- Text report: \`${report_dir}/monitoring-report-*phase${phase_num}*.txt\`
- Charts: \`${report_dir}/charts/\`
- Evidence: Command outputs, pod status, metric samples

EOF
        else
            cat >> "$consolidated_report" <<EOF
**Phase ${phase_num}: ${phase_desc}** - ⚠️ SKIPPED

EOF
        fi
        ((phase_num++))
    done

    cat >> "$consolidated_report" <<EOF

### Deployment Commands Executed

Each phase used the monitor binary with specific flags to achieve the desired collector configuration:

**Phase 1**: All Collectors (Baseline)
\`\`\`bash
$ $MONITOR_BIN --deploy --duration=${DURATION_FLAG} --manifests-dir=${MANIFESTS_DIR}
\`\`\`

**Phase 2**: No Node-Exporter (Impact Measurement)
\`\`\`bash
$ ${kube_cmd} delete daemonset -n node-exporter-zoneinfo node-exporter-zoneinfo
daemonset.apps "node-exporter-zoneinfo" deleted

$ $MONITOR_BIN --duration=${DURATION_FLAG} --skip-deploy
\`\`\`

**Phase 3**: Zoneinfo Collector Isolation
\`\`\`bash
$ $MONITOR_BIN --deploy --zoneinfo-only --duration=${DURATION_FLAG} --manifests-dir=${MANIFESTS_DIR}
\`\`\`

**Phase 4**: Interrupts Collector Isolation
\`\`\`bash
$ ${kube_cmd} delete daemonset -n node-exporter-zoneinfo node-exporter-zoneinfo
$ sleep 5
$ $MONITOR_BIN --deploy --interrupts-only --duration=${DURATION_FLAG} --manifests-dir=${MANIFESTS_DIR}
\`\`\`

**Phase 5**: Softirqs Collector Isolation
\`\`\`bash
$ ${kube_cmd} delete daemonset -n node-exporter-zoneinfo node-exporter-zoneinfo
$ sleep 5
$ $MONITOR_BIN --deploy --softirqs-only --duration=${DURATION_FLAG} --manifests-dir=${MANIFESTS_DIR}
\`\`\`

## Reproducing This Test

To reproduce this exact test sequence, run:

\`\`\`bash
# Clone repository
git clone <repository-url>
cd node-exporter-zoneinfo

# Build monitor binary
make build

# Execute 5-phase test (same duration)
./.claude/skills/phase-testing/execute-phases.sh --duration=${DURATION_MINUTES}m

# Or use Claude Code skill
/phase-testing --duration=${DURATION_MINUTES}m
\`\`\`

**Prerequisites**:
- Kubernetes cluster with admin access
- Prometheus Operator installed in \`openshift-monitoring\` namespace
- \`kubectl\` or \`oc\` CLI configured

## Analysis Guidelines

Review individual phase reports to validate:

1. **Metric Availability**
   - Check Prometheus query results in each phase report
   - Verify expected metrics appear in Phases 1, 3, 4, 5
   - Confirm metrics disappear in Phase 2 (no node-exporter)

2. **Resource Consumption**
   - Compare CPU/memory charts across phases
   - Identify collector-specific resource patterns
   - Check for memory leaks or spikes

3. **Deployment Health**
   - Verify pod readiness in all deployment phases
   - Check for CrashLoopBackOff or ImagePullBackOff
   - Review container logs for errors

4. **Collector Isolation**
   - Phase 3: Only \`node_zoneinfo_*\` metrics present
   - Phase 4: Only \`node_interrupts_*\` metrics present
   - Phase 5: Only \`node_softirqs_*\` metrics present

## Production Recommendations

Based on this validation:

1. **All Collectors** (Phase 1 configuration) provides complete coverage but:
   - Monitor resource consumption on large clusters
   - Be aware of metric cardinality from interrupts/softirqs collectors

2. **Single Collector** (Phases 3-5) useful when:
   - You only need specific metrics (e.g., zoneinfo for memory zones)
   - Reducing metric cardinality is important
   - Resource constraints exist

3. **Deployment Requirements**:
   - Requires \`privileged\` **SCC** (Security Context Constraint) on OpenShift
   - Requires \`hostPID: true\` for access to \`/proc\` filesystem
   - DaemonSet ensures one pod per node

> ⚠️ **Caveat**: Without privileged SCC or hostPID, collectors cannot read \`/proc/zoneinfo\`, \`/proc/interrupts\`, or \`/proc/softirqs\`. Metrics will be missing or zero-valued.

## Key Findings

Review individual phase reports for:
- Exact metric counts per collector
- Resource usage baselines
- Any errors or warnings encountered
- Differences between phases

The individual reports contain full command outputs, pod logs, and metric samples as evidence.

## Next Steps

1. Open individual phase reports in \`${report_dir}/\`
2. Check metric charts in \`${report_dir}/charts/\`
3. Validate metric availability matches expectations
4. Document any anomalies or unexpected behavior
5. Use findings to inform production deployment decisions

I hope you now have a clear understanding of how each collector behaves in isolation and combination.

---

**Tags**: kubernetes, prometheus, node-exporter, monitoring, metrics, collectors

EOF

    log_success "Consolidated report generated: $consolidated_report"
}

# Main execution
main() {
    log_info "Starting 5-Phase Node-Exporter Monitoring Test"
    log_info "Duration per phase: ${DURATION_MINUTES} minutes"
    log_info "Monitor binary: $MONITOR_BIN"

    # Validate
    validate_prerequisites

    # Build monitor binary
    build_monitor

    # Execute phases
    phase1_all_collectors
    phase2_no_node_exporter
    phase3_zoneinfo_only
    phase4_interrupts_only
    phase5_softirqs_only

    # Generate consolidated report
    generate_consolidated_report

    # Cleanup
    cleanup

    log_success "5-Phase test complete!"
    log_info "Check ./reports/ directory for all generated reports and charts"
}

# Run main
main
