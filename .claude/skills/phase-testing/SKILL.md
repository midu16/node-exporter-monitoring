---
name: phase-testing
description: Execute a 5-phase node-exporter monitoring test scenario (all collectors → no node-exporter → zoneinfo-only → interrupts-only → softirqs-only) and generate evidence-based reports following technical writing framework
---

Execute a comprehensive 5-phase monitoring test scenario for the node-exporter-zoneinfo project.

## Report Writing Framework

Apply the following structure, voice, and evidence requirements to **all reports generated** from the phase-testing execution.

### Voice and Tone

- Use collaborative "we" pronoun throughout
- Be honest about limitations and gotchas observed
- Prioritize pragmatic insights over theoretical descriptions
- Write short, declarative sentences

### Report Structure

Each phase report must follow this five-part structure:

1. **Concept Framing** (2-4 sentences)
   - What this phase tests and why it matters
   - No preamble phrases like "In this phase we will..."
   - Direct problem/goal statement

2. **Foundation** (1-2 paragraphs)
   - Brief explanation of collector configuration
   - What metrics are expected vs. not expected
   - Deployment topology

3. **Prerequisites Verified** (checklist)
   - Cluster connectivity confirmed
   - Prometheus Operator status
   - Node-exporter deployment status

4. **Execution Evidence** (detailed walkthrough with outputs)
   - Exact commands executed
   - Complete command outputs (not summaries)
   - Pod status, logs, metrics scraped
   - Timestamps and durations
   - Screenshots or metric samples where applicable

5. **Observations** (1-3 sentences)
   - What worked, what didn't
   - Notable differences from previous phases
   - Brief summary, no unnecessary closing phrases

### Evidence Requirements

Every report must include:

**1. Command Outputs** - Show command-then-output pairs:
```bash
$ kubectl get pods -n node-exporter-zoneinfo
NAME                          READY   STATUS    RESTARTS   AGE
node-exporter-zoneinfo-abc    1/1     Running   0          2m15s
```

**2. Deployment Evidence**:
- Pod readiness timestamps
- Container logs (first 20 lines and any errors)
- Resource usage snapshots

**3. Metric Evidence**:
- Prometheus query results showing metrics ARE present
- Prometheus query results showing metrics are NOT present (for validation)
- Sample metric values with timestamps

**4. Comparison Tables**:

| Metric Family | Phase 1 | Phase 2 | Phase 3 | Phase 4 | Phase 5 |
|--------------|---------|---------|---------|---------|---------|
| node_zoneinfo_* | ✓ (150 series) | ✗ (0 series) | ✓ (150 series) | ✗ (0 series) | ✗ (0 series) |

**5. Timeline Evidence**:
- Phase start time (RFC3339)
- Phase end time (RFC3339)
- Actual duration vs. planned duration
- Key events with timestamps

### Formatting Conventions

- **Bold new terms** on first use with immediate definition
- Use H2 for major sections (## Phase 1: All Collectors)
- Use H3 for subsections (### Deployment Evidence)
- Inline backticks for commands, flags, paths, metrics
- Fenced code blocks with language hints
- Tables for multi-option comparisons
- Callout blocks for warnings/caveats

**Example**:
> ⚠️ **Caveat**: The zoneinfo collector requires read access to `/proc/zoneinfo`. Without `hostPID: true`, all metrics will be zero.

### Caveats and Warnings

Surface risks early in each phase:
- Known issues with collector (e.g., high cardinality)
- Deployment requirements (privileged SCC, hostPID)
- Expected vs. unexpected behavior
- Production considerations

### Consolidated Report Framework

The final consolidated report must:

1. **Open with Impact** (2-3 sentences)
   - What we learned across all 5 phases
   - Key finding or surprising result

2. **Phase Comparison** (table)
   - Side-by-side metrics availability
   - Resource consumption (CPU, memory)
   - Collector-specific observations

3. **Evidence Summary**
   - Link to each phase report with one-line description
   - Total samples collected
   - Errors encountered

4. **Reproducibility Section**
   - Exact commands to reproduce entire test
   - Environment details (cluster version, node-exporter version)
   - Copy-paste ready command sequence

5. **Production Recommendations** (if applicable)
   - Which collectors to enable
   - Resource requirements observed
   - Known limitations

6. **Close with Outcome** (1-2 sentences)
   - Summary of validation results
   - Next steps if any

### What NOT to Do

- ❌ Don't summarize outputs — show them verbatim
- ❌ Don't say "monitoring completed successfully" without evidence
- ❌ Don't use phrases like "As we can see..." — the reader can see
- ❌ Don't write "the phase ran for 30 minutes" — show timestamps
- ❌ Don't claim metrics are present without query results
- ❌ Don't use emojis in body text (only in callout headers)
- ❌ Don't pad with unnecessary context or backstory
- ❌ Don't leave undefined jargon (e.g., "SCC" without expansion)

### Validation Criteria

A good phase report is one where:
- A colleague can reproduce the exact test by copying commands
- Evidence proves every claim (metrics present/absent, pods ready, etc.)
- Comparison to other phases is data-driven, not subjective
- Gotchas are surfaced early with workarounds

## Test Scenario Overview

This skill automates the following 5-phase test sequence, with each phase running for 30 minutes:

1. **Phase 1 (30min)**: Deploy all collectors (zoneinfo, interrupts, softirqs)
2. **Phase 2 (30min)**: Delete node-exporter DaemonSet (Prometheus Operator only)
3. **Phase 3 (30min)**: Deploy zoneinfo collector only
4. **Phase 4 (30min)**: Deploy interrupts collector only
5. **Phase 5 (30min)**: Deploy softirqs collector only

## Implementation Details

### Phase 1: All Collectors
- Deploy using `daemonset.yaml`
- Enables: `--collector.zoneinfo`, `--collector.interrupts`, `--collector.softirqs`
- Monitor for 30 minutes
- Generate phase report

### Phase 2: No Node-Exporter
- Delete the node-exporter-zoneinfo DaemonSet
- Monitor Prometheus Operator metrics only for 30 minutes
- Generate phase report showing impact of removal

### Phase 3: Zoneinfo Only
- Deploy using `daemonset-zoneinfo-only.yaml`
- Enables only: `--collector.zoneinfo`
- Monitor for 30 minutes
- Generate phase report

### Phase 4: Interrupts Only
- Delete previous DaemonSet
- Deploy using `daemonset-interrupts-only.yaml`
- Enables only: `--collector.interrupts`
- Monitor for 30 minutes
- Generate phase report

### Phase 5: Softirqs Only
- Delete previous DaemonSet
- Deploy using `daemonset-softirqs-only.yaml`
- Enables only: `--collector.softirqs`
- Monitor for 30 minutes
- Generate phase report

## Execution Steps

When invoked, this skill will:

1. **Validate Prerequisites**
   - Check kubectl/oc connectivity
   - Verify manifests directory exists
   - Ensure Prometheus Operator is running

2. **Phase 1: All Collectors**
   ```bash
   kubectl apply -k ./node-exporter-zoneinfo/
   # Monitor for 30 minutes
   # Generate report: reports/phase1-all-collectors-$(date +%Y%m%d-%H%M%S).md
   ```

3. **Phase 2: No Node-Exporter**
   ```bash
   kubectl delete daemonset -n node-exporter-zoneinfo node-exporter-zoneinfo
   # Monitor Prometheus only for 30 minutes
   # Generate report: reports/phase2-no-node-exporter-$(date +%Y%m%d-%H%M%S).md
   ```

4. **Phase 3: Zoneinfo Only**
   ```bash
   kubectl apply -f ./node-exporter-zoneinfo/daemonset-zoneinfo-only.yaml
   # Monitor for 30 minutes
   # Generate report: reports/phase3-zoneinfo-only-$(date +%Y%m%d-%H%M%S).md
   ```

5. **Phase 4: Interrupts Only**
   ```bash
   kubectl delete daemonset -n node-exporter-zoneinfo node-exporter-zoneinfo
   kubectl apply -f ./node-exporter-zoneinfo/daemonset-interrupts-only.yaml
   # Monitor for 30 minutes
   # Generate report: reports/phase4-interrupts-only-$(date +%Y%m%d-%H%M%S).md
   ```

6. **Phase 5: Softirqs Only**
   ```bash
   kubectl delete daemonset -n node-exporter-zoneinfo node-exporter-zoneinfo
   kubectl apply -f ./node-exporter-zoneinfo/daemonset-softirqs-only.yaml
   # Monitor for 30 minutes
   # Generate report: reports/phase5-softirqs-only-$(date +%Y%m%d-%H%M%S).md
   ```

7. **Generate Consolidated Report**
   - Combine all phase reports
   - Compare metric availability across phases
   - Output: `reports/consolidated-5phase-report-$(date +%Y%m%d-%H%M%S).md`

## Configuration Options

Arguments can be passed to customize the test:

- `--duration=<minutes>`: Duration per phase (default: 30m)
- `--skip-phase=<N>`: Skip specific phase(s), comma-separated
- `--manifests-dir=<path>`: Custom path to manifests (default: ./node-exporter-zoneinfo/)
- `--kubeconfig=<path>`: Custom kubeconfig file
- `--no-cleanup`: Don't delete DaemonSet after final phase

## Example Usage

```bash
# Run full 5-phase test with default 30min per phase
/phase-testing

# Run with custom 15min per phase
/phase-testing --duration=15m

# Skip phase 2 (no node-exporter)
/phase-testing --skip-phase=2

# Custom duration and skip multiple phases
/phase-testing --duration=20m --skip-phase=2,4
```

## Monitoring During Phases

For each phase, the skill will:
- Query Prometheus for metric availability
- Track which collectors are active
- Monitor pod status and readiness
- Capture sample metrics from each collector
- Record any errors or warnings

## Report Structure

Each phase report includes:
- Phase summary (duration, collectors enabled)
- Deployment status
- Pod health and logs
- Metric availability
- Sample metric values
- Comparison to previous phase
- Issues encountered

## Success Criteria

A phase is considered successful if:
- DaemonSet (when applicable) reaches ready state
- ServiceMonitor is discovered by Prometheus
- Expected metrics are scraped successfully
- No pod crashes or errors

## Notes

- Total test duration: 2.5 hours (5 phases × 30 minutes)
- Reports are saved to `./reports/` directory
- Each phase waits for pod readiness before starting monitoring
- Namespace `node-exporter-zoneinfo` is created in Phase 1
- Final cleanup removes all resources unless `--no-cleanup` is specified

## Automated Execution

This skill provides an executable script that automates the entire 5-phase test workflow using the Go monitor binary.

**Script**: `.claude/skills/phase-testing/execute-phases.sh`  
**Binary**: `./bin/monitor` (Go binary)

### How It Works

The script orchestrates 5 phases by:
1. Building the Go monitor binary (if needed)
2. Executing each phase using the monitor binary with appropriate flags
3. Collecting reports generated by the monitor binary
4. Generating a consolidated summary

### Running the Script

Execute the phases using the provided shell script:

```bash
# Run with defaults (30 minutes per phase)
./.claude/skills/phase-testing/execute-phases.sh

# Custom duration (15 minutes per phase)
./.claude/skills/phase-testing/execute-phases.sh --duration=15m

# Skip specific phases
./.claude/skills/phase-testing/execute-phases.sh --skip-phase=2,4

# Custom manifests directory
./.claude/skills/phase-testing/execute-phases.sh --manifests-dir=/path/to/manifests

# Don't cleanup after test
./.claude/skills/phase-testing/execute-phases.sh --no-cleanup

# Custom kubeconfig
./.claude/skills/phase-testing/execute-phases.sh --kubeconfig=/path/to/kubeconfig

# Custom monitor binary location
./.claude/skills/phase-testing/execute-phases.sh --monitor-bin=/path/to/monitor
```

### Script Features

The script automatically:
- Builds the Go monitor binary if not present or outdated
- Validates prerequisites (monitor binary, manifests)
- Executes each phase using the monitor binary
- Uses Go binary flags for collector variants:
  - Phase 1: `--deploy` (all collectors)
  - Phase 2: `--skip-deploy` (no node-exporter)
  - Phase 3: `--deploy --zoneinfo-only`
  - Phase 4: `--deploy --interrupts-only`
  - Phase 5: `--deploy --softirqs-only`
- Generates individual phase reports via monitor binary
- Creates a consolidated summary report
- Cleans up resources (unless `--no-cleanup`)

### Monitor Binary Flags

The script uses the following monitor binary flags for each phase:

**Phase 1**: All Collectors
```bash
./bin/monitor --deploy --duration=30m --manifests-dir=./node-exporter-zoneinfo
```

**Phase 2**: No Node-Exporter
```bash
kubectl delete daemonset -n node-exporter-zoneinfo node-exporter-zoneinfo
./bin/monitor --duration=30m --skip-deploy
```

**Phase 3**: Zoneinfo Only
```bash
./bin/monitor --deploy --zoneinfo-only --duration=30m --manifests-dir=./node-exporter-zoneinfo
```

**Phase 4**: Interrupts Only
```bash
./bin/monitor --deploy --interrupts-only --duration=30m --manifests-dir=./node-exporter-zoneinfo
```

**Phase 5**: Softirqs Only
```bash
./bin/monitor --deploy --softirqs-only --duration=30m --manifests-dir=./node-exporter-zoneinfo
```

## Manual Execution Steps

If you prefer to run phases manually using the Go binary, follow these steps:

### Prerequisites
1. Build the monitor binary: `make build`
2. Verify Prometheus Operator is running
3. Reports directory is created automatically

### Phase-by-Phase Commands

**Phase 1: All Collectors**
```bash
./bin/monitor --deploy --duration=30m --manifests-dir=./node-exporter-zoneinfo/
```
Deploys node-exporter with all collectors (zoneinfo, interrupts, softirqs) and monitors for 30 minutes.

**Phase 2: No Node-Exporter**
```bash
kubectl delete daemonset -n node-exporter-zoneinfo node-exporter-zoneinfo
./bin/monitor --duration=30m --skip-deploy
```
Deletes the node-exporter DaemonSet and monitors Prometheus Operator only.

**Phase 3: Zoneinfo Only**
```bash
./bin/monitor --deploy --zoneinfo-only --duration=30m --manifests-dir=./node-exporter-zoneinfo/
```
Deploys node-exporter with only the zoneinfo collector enabled.

**Phase 4: Interrupts Only**
```bash
kubectl delete daemonset -n node-exporter-zoneinfo node-exporter-zoneinfo
sleep 5
./bin/monitor --deploy --interrupts-only --duration=30m --manifests-dir=./node-exporter-zoneinfo/
```
Cleans up previous deployment and deploys node-exporter with only the interrupts collector.

**Phase 5: Softirqs Only**
```bash
kubectl delete daemonset -n node-exporter-zoneinfo node-exporter-zoneinfo
sleep 5
./bin/monitor --deploy --softirqs-only --duration=30m --manifests-dir=./node-exporter-zoneinfo/
```
Cleans up previous deployment and deploys node-exporter with only the softirqs collector.

**Cleanup**
```bash
kubectl delete namespace node-exporter-zoneinfo
```

### Understanding the Monitor Binary Flags

- `--deploy`: Deploy node-exporter-zoneinfo before monitoring
- `--skip-deploy`: Skip deployment, monitor existing resources
- `--duration=<time>`: Monitoring duration (e.g., 30m, 1h)
- `--manifests-dir=<path>`: Path to Kubernetes manifests
- `--zoneinfo-only`: Deploy with only zoneinfo collector
- `--interrupts-only`: Deploy with only interrupts collector
- `--softirqs-only`: Deploy with only softirqs collector
- `--format=<format>`: Report format (text, json, html)
- `--kubeconfig=<path>`: Custom kubeconfig file

## Report Generation Workflow

After executing the phases via the script or monitor binary, generate evidence-based reports:

### Step 1: Collect Evidence

For each phase, gather:

**Deployment Evidence**:
```bash
kubectl get pods -n node-exporter-zoneinfo -o wide
kubectl get daemonset -n node-exporter-zoneinfo
kubectl logs -n node-exporter-zoneinfo <pod-name> --tail=20
```

**Metric Evidence** (via Prometheus queries):
```bash
# Check if expected metrics are present
kubectl exec -n openshift-monitoring prometheus-k8s-0 -- \
  promtool query instant 'http://localhost:9090' \
  'count(node_zoneinfo_*{job="node-exporter-zoneinfo"})'

# Check if unexpected metrics are absent
kubectl exec -n openshift-monitoring prometheus-k8s-0 -- \
  promtool query instant 'http://localhost:9090' \
  'count(node_interrupts_*{job="node-exporter-zoneinfo"})'

# Get sample values
kubectl exec -n openshift-monitoring prometheus-k8s-0 -- \
  promtool query instant 'http://localhost:9090' \
  'node_zoneinfo_nr_active_anon{job="node-exporter-zoneinfo"}'
```

**Resource Evidence**:
```bash
kubectl top pod -n node-exporter-zoneinfo
```

**Timeline Evidence**:
- Capture timestamps at: deployment start, pods ready, monitoring start, monitoring end
- Use RFC3339 format: `date -u +%Y-%m-%dT%H:%M:%SZ`

### Step 2: Create Phase Report

Using the evidence collected, create a phase report following the template at:
`.claude/skills/phase-testing/PHASE_REPORT_TEMPLATE.md`

**Critical requirements**:
1. Show command outputs verbatim (not summaries)
2. Include Prometheus query results proving metrics present/absent
3. Use RFC3339 timestamps for all events
4. Provide sample metric values
5. Include resource consumption data
6. Surface caveats early with workarounds

### Step 3: Generate Consolidated Report

The script automatically generates the consolidated report. Review it to ensure:
- Phase comparison table is accurate
- All evidence links are correct
- Reproducibility section has exact commands
- Production recommendations are data-driven

### Step 4: Validate Reports

Check that every report passes the validation criteria:
- A colleague can reproduce by copying commands
- Every claim has evidence (not assertions)
- Comparisons are data-driven, not subjective
- Gotchas are surfaced with workarounds

## Task Execution

When invoked via `/phase-testing $ARGUMENTS`:

1. **Execute the test**: Run the automation script with parsed arguments
2. **Collect evidence**: Gather deployment, metric, and resource data for each phase
3. **Generate phase reports**: Create evidence-based reports following the framework
4. **Create consolidated report**: Summarize findings with comparison tables
5. **Validate completeness**: Ensure all reports meet validation criteria

The monitor binary generates initial reports in text/html/json format. Transform these into evidence-based markdown reports following the writing framework defined above.

**Arguments**: $ARGUMENTS

## Example Execution

```bash
# User invokes
/phase-testing --duration=15m

# Claude executes
./.claude/skills/phase-testing/execute-phases.sh --duration=15m

# For each phase, Claude:
# 1. Runs monitor binary: ./bin/monitor --deploy --[variant] --duration=15m
# 2. Collects evidence: pod status, logs, Prometheus queries
# 3. Generates phase report with command outputs
# 4. Validates evidence completeness

# Finally, Claude:
# 5. Reviews consolidated report
# 6. Ensures all evidence is present
# 7. Reports completion with links to reports
```

## Evidence Checklist

Every phase report must include:

- [ ] Command-then-output pairs (verbatim)
- [ ] Pod status at T+0, T+5min, T+end
- [ ] Container logs (first 20 lines)
- [ ] Prometheus queries showing metrics present
- [ ] Prometheus queries showing metrics absent (for validation)
- [ ] Sample metric values with timestamps
- [ ] Resource consumption (CPU, memory)
- [ ] Timeline with RFC3339 timestamps
- [ ] Comparison to previous phase (if applicable)
- [ ] Caveats in callout blocks
- [ ] Bold technical terms on first use
- [ ] Copy-paste ready commands

$ARGUMENTS
