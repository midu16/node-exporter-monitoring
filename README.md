# Node-exporter-zoneinfo (cluster Prometheus)

Supplementary node_exporter that exposes **zoneinfo**, **interrupts**, and **softirqs** (port 9101) when the cluster-monitoring-config does not support these collectors. Deploys into **node-exporter-zoneinfo** namespace. The kustomization includes a ClusterRoleBinding that grants the privileged SCC to the default service account so the DaemonSet can use hostPID/hostPath.

## Monitoring & Testing Modes

This repository includes a comprehensive monitoring tool that supports multiple test scenarios:

- **Single-phase**: Standard monitoring of deployed resources (30 min default)
- **Two-phase**: With/without node-exporter comparison (60 min)
- **Three-phase**: All collectors + baseline + single collector (90 min)
- **Six-phase**: Comprehensive test of all collector configurations (180 min)

**Documentation**:
- [COMPLETE_ENHANCEMENTS.md](COMPLETE_ENHANCEMENTS.md) - **⭐ START HERE** - Complete enhancement summary
- [SIX_PHASE_MONITORING.md](SIX_PHASE_MONITORING.md) - Detailed six-phase monitoring guide
- [OUTPUTS_GUIDE.md](OUTPUTS_GUIDE.md) - Complete output files reference
- [OUTPUT_STRUCTURE.md](OUTPUT_STRUCTURE.md) - Detailed output structure and data format
- [DYNAMIC_KUSTOMIZATION.md](DYNAMIC_KUSTOMIZATION.md) - Automatic kustomization.yaml management
- [KUBECONFIG_FIX.md](KUBECONFIG_FIX.md) - Kubeconfig authentication fix

**All monitoring runs automatically** (zero manual configuration):
- ✅ **Raw metrics JSON** - Real-time streaming of ALL samples (complete historical data)
- ✅ **Analysis reports** - User-selected format (text/html/json)
- ✅ **PNG charts** - Visual timelines for each monitored pod
- ✅ **Dynamic configuration** - Automatic kustomization.yaml updates per phase
- ✅ **Clean output** - No scary error messages, professional logging
- ✅ **Automatic restoration** - Original configuration preserved after completion

### Quick Start - Six-Phase Monitoring

```bash
# Build and run full 3-hour test
make build
./bin/monitor --six-phase

# Or use Makefile targets
make six-phase-monitor      # Full 180-minute test
make six-phase-quick        # Quick 12-minute test

# Quick test (5 min per phase = 30 min total)
./bin/monitor --six-phase --duration 5m --interval 15s
```

## Apply

```bash
oc apply -k abi-master-0/extra-manifests/node-exporter-zoneinfo/
```

If the DaemonSet stays at 0 pods after the first apply, bump the `node-exporter-zoneinfo/force-rollout` annotation in **daemonset.yaml** (e.g. from `"1"` to `"2"`) and re-apply to force a rollout.

## Verify metrics from the pods

```bash
oc port-forward -n node-exporter-zoneinfo daemonset/node-exporter-zoneinfo 9101:9101
# In another terminal:
curl -s http://localhost:9101/metrics | grep -E 'node_zoneinfo_|node_intr_|node_softirqs'
```

## Check if Prometheus is scraping node-exporter-zoneinfo

### 1. Prometheus UI (targets)

- Open **Observe → Targets** (or the Prometheus route and go to Status → Targets).
- Search for **node-exporter-zoneinfo**. If the job appears and targets are **Up**, Prometheus is scraping.

### 2. Prometheus UI (query)

- Open **Observe → Metrics** (or Prometheus → Graph).
- Run one of these (they only exist when the zoneinfo/interrupts/softirqs collectors are enabled):

  ```promql
  node_zoneinfo_managed_pages{job="node-exporter-zoneinfo"}
  node_intr_total{job="node-exporter-zoneinfo"}
  node_softirqs_total{job="node-exporter-zoneinfo"}
  node_softirqs_functions_total{job="node-exporter-zoneinfo"}
  ```

- If you get no data, try without the job label to see if the metric exists under another job:  
  `node_zoneinfo_managed_pages` or `node_intr_total`.

### 3. Check that `node_softirqs_functions_total` is being pulled constantly

This metric is exposed by the **softirqs** collector (enabled in this DaemonSet). To confirm Prometheus is scraping it:

**Prometheus UI:** Query and run a range query or look at the table view so you see timestamps updating every scrape interval (e.g. 30s):

```promql
node_softirqs_functions_total{job="node-exporter-zoneinfo"}
```

**CLI (port-forward Prometheus, then query the API):** Run this and check that `result` has entries and that the `"value"` array has a Unix timestamp (first element) that is recent (within the last minute). Re-run after 30s and the timestamp should increase.

```bash
# Start port-forward in background (or run in another terminal)
PROM=$(oc get pod -n openshift-monitoring -l app.kubernetes.io/name=prometheus -o jsonpath='{.items[0].metadata.name}')
oc port-forward -n openshift-monitoring "$PROM" 9090:9090 &
sleep 3
# Query the metric; jq shows sample count and a recent timestamp
curl -sG 'http://localhost:9090/api/v1/query' --data-urlencode 'query=node_softirqs_functions_total{job="node-exporter-zoneinfo"}' | jq '.data.result | length, (.[0].value[0] // empty)'
# Optional: kill port-forward when done
kill %1 2>/dev/null
```

- If the sample count is 0 or the query returns no data, the job is not scraping this metric (check targets and ServiceMonitor).
- If you get many samples and a recent timestamp, the metric is being pulled; run the same query again after 30s and confirm the timestamp increases.

### 4. CLI (port-forward to Prometheus)

```bash
# Get Prometheus pod name
PROM=$(oc get pod -n openshift-monitoring -l app.kubernetes.io/name=prometheus -o jsonpath='{.items[0].metadata.name}')
oc port-forward -n openshift-monitoring "$PROM" 9090:9090
```

Then open http://localhost:9090 → Status → Targets (find `node-exporter-zoneinfo`) or Graph and run the queries above.

### 5. Why "No datapoints" and how it's fixed

**Cause:** Cluster Prometheus only discovers ServiceMonitors in **openshift-monitoring**. A ServiceMonitor in **node-exporter-zoneinfo** is never seen, so no scrape target is created. **Fix:** The kustomization includes **servicemonitor-openshift-monitoring.yaml** (in openshift-monitoring with `namespaceSelector.matchNames: [node-exporter-zoneinfo]`) so Prometheus scrapes the Service there. Apply it; after a short delay, targets and metrics should appear.

- **ServiceMonitor in another namespace:** Cluster Prometheus often only watches `openshift-monitoring`. If the ServiceMonitor is in `node-exporter-zoneinfo`, add that namespace to Prometheus’s `serviceMonitorNamespaceSelector`, or create a copy of the ServiceMonitor in `openshift-monitoring` that selects the Service in `node-exporter-zoneinfo` (e.g. with `spec.namespaceSelector` / target namespace).
- **Targets down:** Check pod logs: `oc logs -n node-exporter-zoneinfo -l app.kubernetes.io/name=node-exporter-zoneinfo --tail=50`

---

## Resource Monitoring Tool

This repository includes a comprehensive Go-based monitoring tool that can:
1. Deploy node-exporter-zoneinfo automatically
2. Monitor resource consumption (CPU/Memory) for all pods
3. Generate detailed reports with pod-to-node mapping
4. Create PNG charts for visual analysis
5. Support two-phase monitoring (with and without node-exporter)

### Quick Start

```bash
# Build the binary
make build

# Run two-phase monitoring (recommended)
export KUBECONFIG=/path/to/your/kubeconfig
make two-phase-monitor
```

### What is Two-Phase Monitoring?

The monitoring tool can run in two-phase mode:

**Phase 1 (30 minutes)**: Monitors node-exporter-zoneinfo + prometheus-operator components
**Phase 2 (30 minutes)**: Removes node-exporter DaemonSet and monitors only prometheus-operator

This allows you to compare resource usage with and without node-exporter-zoneinfo.

### Usage Examples

```bash
# Single-phase monitoring (original behavior)
./bin/node-exporter-monitor --kubeconfig=/path/to/kubeconfig --deploy

# Two-phase monitoring (full 60-minute run)
./bin/node-exporter-monitor --kubeconfig=/path/to/kubeconfig --two-phase

# Quick two-phase test (2 min per phase)
./bin/node-exporter-monitor --kubeconfig=/path/to/kubeconfig --two-phase --duration=2m
```

### Output Structure

```
reports/
├── phase1-monitoring-report-TIMESTAMP.txt    # Phase 1 report (with node-exporter)
├── phase2-monitoring-report-TIMESTAMP.txt    # Phase 2 report (without node-exporter)
├── phase1/charts/                             # Phase 1 PNG charts
│   ├── node-exporter-zoneinfo_*.png          # 5 charts (one per node)
│   └── openshift-monitoring_*.png            # Operator charts
└── phase2/charts/                             # Phase 2 PNG charts
    └── openshift-monitoring_*.png            # Operator charts only
```

### Features

- ✅ Automatic deployment and cleanup
- ✅ 60 samples per pod at 30-second intervals
- ✅ Pod-to-node mapping for DaemonSet distribution
- ✅ Dual-axis PNG charts (CPU + Memory)
- ✅ Min/Max/Avg statistics
- ✅ Time-series data for trend analysis
- ✅ Separate reports for each phase
- ✅ Support for JSON and HTML output formats

### Makefile Targets

```bash
make help                 # Show all available targets
make build               # Build the monitoring binary
make test                # Run unit tests
make two-phase-monitor   # Run full two-phase monitoring (60 min)
make two-phase-quick     # Quick two-phase test (4 min)
make monitor             # Single-phase monitoring (30 min)
make quick-test          # Quick single-phase test (2 min)
make view-charts         # Open generated charts
make list-charts         # List all generated charts
make show-mapping        # Show pod-to-node mapping
```

### Requirements

- Go 1.22 or later
- kubectl or oc CLI tool
- Valid kubeconfig with cluster access
- Cluster with Metrics API enabled

### Building from Source

```bash
# Install dependencies
make deps

# Build binary
make build

# Run tests
make test

# Verify build
./bin/node-exporter-monitor -h
```

