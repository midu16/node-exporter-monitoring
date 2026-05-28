# Claude Skills for node-exporter-zoneinfo

This directory contains Claude Code skills and commands for automating testing and monitoring of the node-exporter-zoneinfo project.

## Available Skills

### phase-testing

**Command**: `/phase-testing`

Execute a comprehensive 5-phase monitoring test scenario that validates node-exporter behavior with different collector configurations. Generates **evidence-based reports** with command outputs, Prometheus queries, and metric samples.

**Test Phases**:
1. Phase 1 (30min): All collectors (zoneinfo, interrupts, softirqs)
2. Phase 2 (30min): No node-exporter (Prometheus Operator only)
3. Phase 3 (30min): Zoneinfo collector only
4. Phase 4 (30min): Interrupts collector only
5. Phase 5 (30min): Softirqs collector only

**Evidence Provided**:
- Exact command outputs (deployment, pod status, logs)
- Prometheus query results (metrics present/absent)
- Sample metric values with timestamps
- Resource consumption data (CPU, memory)
- Timeline with RFC3339 timestamps
- Comparison tables across phases

**Usage**:
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

**Options**:
- `--duration=<minutes>`: Duration per phase (default: 30m)
- `--skip-phase=<N>`: Skip specific phase(s), comma-separated
- `--manifests-dir=<path>`: Custom path to manifests (default: ./node-exporter-zoneinfo/)
- `--kubeconfig=<path>`: Custom kubeconfig file
- `--no-cleanup`: Don't delete DaemonSet after final phase

**Output**:
- Individual phase reports in `./reports/` (evidence-based, following technical writing framework)
- Consolidated report in `./reports/consolidated-5phase-report-*.md`
- Charts in `./reports/charts/` (CPU usage, memory usage)

**Report Framework**:
Reports follow a structured evidence-based framework:
- Command-then-output pairs (verbatim, not summaries)
- Prometheus queries proving metrics present/absent
- Sample metric values with timestamps
- Resource consumption snapshots
- Timeline with RFC3339 timestamps
- Caveats and warnings in callout blocks
- Copy-paste ready commands for reproducibility

## Directory Structure

```
.claude/
├── AGENTS.md                           # This file
├── commands/
│   └── phase-testing.md               # /phase-testing command definition
└── skills/
    └── phase-testing/
        └── SKILL.md                   # Phase testing skill implementation
```

## How It Works

When you invoke `/phase-testing`, Claude Code will:

1. Load the phase-testing skill from `skills/phase-testing/SKILL.md`
2. Parse any arguments you provided (e.g., `--duration=15m`)
3. Execute the 5-phase test scenario:
   - Deploy appropriate DaemonSet configurations
   - Monitor metrics for specified duration
   - Generate phase-specific reports
   - Transition between phases
4. Generate a consolidated report comparing all phases

## Prerequisites

Before running `/phase-testing`, ensure:

- kubectl or oc CLI is available and configured
- Access to an OpenShift/Kubernetes cluster
- Prometheus Operator is installed and running
- Manifests directory exists at `./node-exporter-zoneinfo/`

## Monitoring Approach

Each phase:
- Waits for DaemonSet (if applicable) to reach ready state
- Queries Prometheus for metric availability
- Tracks collector activity
- Captures sample metrics
- Records pod status and logs
- Generates detailed phase report

## Report Contents

Phase reports include:
- Phase configuration summary
- Deployment status and pod health
- Metric availability matrix
- Sample metric values
- Comparison to previous phase
- Issues and errors encountered

The consolidated report provides:
- Side-by-side phase comparison
- Metric availability across all phases
- Performance observations
- Recommendations

## Example Workflow

```bash
# Start the 5-phase test
/phase-testing

# Claude will:
# 1. Deploy all collectors (Phase 1)
# 2. Monitor for 30 minutes
# 3. Delete DaemonSet (Phase 2)
# 4. Monitor Prometheus only for 30 minutes
# 5. Deploy zoneinfo-only (Phase 3)
# 6. Monitor for 30 minutes
# 7. Switch to interrupts-only (Phase 4)
# 8. Monitor for 30 minutes
# 9. Switch to softirqs-only (Phase 5)
# 10. Monitor for 30 minutes
# 11. Generate consolidated report
# 12. Clean up resources

# Total duration: ~2.5 hours
```

## Customization

To modify the skill behavior, edit:
- `skills/phase-testing/SKILL.md` - Core logic and phase definitions
- `commands/phase-testing.md` - Command description and invocation

## Related Documentation

- [DEPLOYMENT.md](../DEPLOYMENT.md) - Manual deployment guide
- [TWO_PHASE_MONITORING_V2.md](../TWO_PHASE_MONITORING_V2.md) - Two-phase monitoring approach
- [README.md](../README.md) - Project overview

## Notes

- The skill uses existing DaemonSet manifests in `./node-exporter-zoneinfo/`
- Each phase transition includes a cleanup step
- Reports are timestamped and saved to `./reports/`
- The skill validates prerequisites before starting
- Progress is displayed in real-time during execution
