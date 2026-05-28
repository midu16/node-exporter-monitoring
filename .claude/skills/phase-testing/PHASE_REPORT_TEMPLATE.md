# Phase Report Template

This template shows the expected structure and evidence requirements for each phase report generated during the 5-phase test.

---

# Phase [N]: [Configuration Name]

We test the node-exporter deployment with [specific collector configuration] to validate [specific goal]. This phase measures [key metric or behavior].

## Foundation

The **[collector name]** collector scrapes metrics from `/proc/[source]` and exposes them as `node_[family]_*` metric families. In this configuration:

- **Enabled collectors**: [list]
- **Disabled collectors**: [list]
- **Expected metric families**: `node_[family]_*`
- **Unexpected metric families**: None (or list what should NOT appear)

The deployment uses the `daemonset-[variant].yaml` manifest with `--collector.[name]` flags.

## Prerequisites Verified

Before starting the phase, we confirmed:

- ✅ Cluster connectivity: `kubectl cluster-info` succeeded
- ✅ Prometheus Operator running: Checked pods in `openshift-monitoring` namespace
- ✅ Manifests present: Verified `./node-exporter-zoneinfo/daemonset-[variant].yaml` exists
- ✅ Previous cleanup complete: No conflicting DaemonSets exist

## Execution Evidence

### Deployment

**Command executed**:
```bash
$ ./bin/monitor --deploy --[variant-flag] --duration=30m --manifests-dir=./node-exporter-zoneinfo/
```

**Deployment output**:
```
╔═══════════════════════════════════════════════════════════════╗
║   Node Exporter Zoneinfo - Resource Monitoring Tool          ║
╚═══════════════════════════════════════════════════════════════╝

📦 Deploying node-exporter-zoneinfo...
namespace/node-exporter-zoneinfo created
clusterrolebinding.rbac.authorization.k8s.io/node-exporter-zoneinfo-privileged-scc created
daemonset.apps/node-exporter-zoneinfo created
service/node-exporter-zoneinfo created
servicemonitor.monitoring.coreos.com/node-exporter-zoneinfo created
✅ Deployment completed successfully

⏳ Waiting for pods to be ready...
```

**Pod status at T+0**:
```bash
$ kubectl get pods -n node-exporter-zoneinfo -o wide
NAME                          READY   STATUS    RESTARTS   AGE   NODE
node-exporter-zoneinfo-abc12  1/1     Running   0          45s   worker-0
node-exporter-zoneinfo-def34  1/1     Running   0          45s   worker-1
node-exporter-zoneinfo-ghi56  1/1     Running   0          45s   worker-2
```

**DaemonSet status**:
```bash
$ kubectl get daemonset -n node-exporter-zoneinfo
NAME                     DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   AGE
node-exporter-zoneinfo   3         3         3       3            3           1m
```

### Container Logs (First 20 Lines)

```bash
$ kubectl logs -n node-exporter-zoneinfo node-exporter-zoneinfo-abc12 --tail=20
ts=2026-05-28T10:15:30.123Z caller=node_exporter.go:199 level=info msg="Starting node_exporter" version=(version=1.9.1)
ts=2026-05-28T10:15:30.124Z caller=node_exporter.go:200 level=info msg="Build context" build_context="(go=go1.24.3, platform=linux/amd64)"
ts=2026-05-28T10:15:30.125Z caller=node_exporter.go:201 level=info msg="Enabled collectors"
ts=2026-05-28T10:15:30.126Z caller=node_exporter.go:208 level=info collector=zoneinfo
ts=2026-05-28T10:15:30.127Z caller=node_exporter.go:218 level=info msg="Listening on" address=0.0.0.0:9101
ts=2026-05-28T10:15:30.128Z caller=tls_config.go:313 level=info msg="Listening on" address=[::]:9101
ts=2026-05-28T10:15:30.129Z caller=tls_config.go:316 level=info msg="TLS is disabled." http2=false address=[::]:9101
```

**No errors** present in logs.

### Metric Availability

**Checking for expected metrics** (zoneinfo in this example):

```bash
$ kubectl exec -n openshift-monitoring prometheus-k8s-0 -- promtool query instant \
  'http://localhost:9090' \
  'count(node_zoneinfo_nr_active_anon{job="node-exporter-zoneinfo"})'

150
```

**Evidence**: We observe **150 time series** for the `node_zoneinfo_nr_active_anon` metric, confirming the zoneinfo collector is active.

**Checking for unexpected metrics** (interrupts should NOT be present):

```bash
$ kubectl exec -n openshift-monitoring prometheus-k8s-0 -- promtool query instant \
  'http://localhost:9090' \
  'count(node_interrupts_total{job="node-exporter-zoneinfo"})'

(no data)
```

**Evidence**: Query returns no data, confirming the interrupts collector is correctly disabled.

### Sample Metrics

**Snapshot at T+5 minutes** (2026-05-28T10:20:30Z):

```promql
node_zoneinfo_nr_active_anon{instance="10.0.1.5:9101", job="node-exporter-zoneinfo", node="Normal", zone="worker-0"} 12543
node_zoneinfo_nr_inactive_anon{instance="10.0.1.5:9101", job="node-exporter-zoneinfo", node="Normal", zone="worker-0"} 8721
node_zoneinfo_nr_active_file{instance="10.0.1.5:9101", job="node-exporter-zoneinfo", node="Normal", zone="worker-0"} 45832
```

### Resource Consumption

**Pod resource usage at T+15 minutes**:

```bash
$ kubectl top pod -n node-exporter-zoneinfo
NAME                          CPU(cores)   MEMORY(bytes)
node-exporter-zoneinfo-abc12  8m           42Mi
node-exporter-zoneinfo-def34  7m           41Mi
node-exporter-zoneinfo-ghi56  9m           43Mi
```

**Average resource usage**: ~8m CPU, ~42Mi memory per pod.

### Monitoring Timeline

| Event | Timestamp (UTC) | Duration from Start |
|-------|-----------------|---------------------|
| Deployment started | 2026-05-28T10:15:00Z | T+0 |
| Pods ready | 2026-05-28T10:15:45Z | T+45s |
| First scrape | 2026-05-28T10:16:00Z | T+1m |
| Monitoring started | 2026-05-28T10:16:00Z | T+1m |
| Monitoring ended | 2026-05-28T10:46:00Z | T+31m |

**Actual monitoring duration**: 30m 0s (planned: 30m)

### Charts Generated

The monitor binary generated the following charts in `reports/charts/`:

- **CPU Usage**: `cpu_usage.html` - Shows 8-9m average CPU across 30 minutes
- **Memory Usage**: `memory_usage.html` - Shows stable 41-43Mi memory usage

## Observations

The zoneinfo collector functioned correctly with **150 time series** scraped successfully. Resource consumption remained stable at ~8m CPU and ~42Mi memory per pod throughout the 30-minute monitoring period. No errors appeared in container logs.

---

## Notes for Different Phase Types

### Phase 1 (All Collectors)

- Show metrics for ALL three families: `node_zoneinfo_*`, `node_interrupts_*`, `node_softirqs_*`
- Higher time series count (expect 400-500+)
- Higher resource consumption baseline

### Phase 2 (No Node-Exporter)

**Deployment section becomes**:

```bash
$ kubectl delete daemonset -n node-exporter-zoneinfo node-exporter-zoneinfo
daemonset.apps "node-exporter-zoneinfo" deleted

$ kubectl get pods -n node-exporter-zoneinfo
No resources found in node-exporter-zoneinfo namespace.
```

**Metric availability becomes**:

```bash
$ kubectl exec -n openshift-monitoring prometheus-k8s-0 -- promtool query instant \
  'http://localhost:9090' \
  'count(node_zoneinfo_nr_active_anon{job="node-exporter-zoneinfo"})'

(no data)
```

**Evidence**: All `job="node-exporter-zoneinfo"` metrics disappeared after DaemonSet deletion.

**Resource consumption section**:
- Show Prometheus Operator pods only
- Compare to Phase 1 to measure impact

### Phase 3, 4, 5 (Single Collectors)

- Show only ONE metric family is present
- Show the other two families are NOT present (with query evidence)
- Compare resource usage to Phase 1 (baseline)

## Validation Checklist

A complete phase report includes:

- ✅ Concept framing (2-4 sentences, direct)
- ✅ Foundation (collector explanation, expected metrics)
- ✅ Prerequisites checklist
- ✅ Command + output pairs (not summaries)
- ✅ Pod status at multiple time points
- ✅ Container logs (first 20 lines + errors)
- ✅ Prometheus queries proving metrics present/absent
- ✅ Sample metric values with timestamps
- ✅ Resource consumption data
- ✅ Timeline with RFC3339 timestamps
- ✅ Charts referenced
- ✅ Observations (1-3 sentences, data-driven)
- ✅ All technical terms bolded on first use
- ✅ Caveats/warnings in callout blocks
- ✅ Copy-paste ready commands
- ✅ No undefined jargon

## Anti-Patterns to Avoid

❌ **Don't write**:
> "The monitoring completed successfully and we collected many samples."

✅ **Do write**:
> "We collected 3,600 samples over 30 minutes (one sample per target every 30 seconds × 3 pods × 2 targets)."

---

❌ **Don't write**:
> "As we can see from the logs, the collector is working."

✅ **Do write**:
> "Container logs show `level=info collector=zoneinfo` confirming the collector initialized."

---

❌ **Don't write**:
> "The phase ran for approximately 30 minutes."

✅ **Do write**:
> "Start: 2026-05-28T10:16:00Z, End: 2026-05-28T10:46:00Z, Duration: 30m 0s"

---

❌ **Don't write**:
> "Metrics were available in Prometheus."

✅ **Do write**:
```bash
$ promtool query instant 'http://localhost:9090' 'count(node_zoneinfo_nr_active_anon{job="node-exporter-zoneinfo"})'
150
```
