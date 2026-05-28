# Claude Code Skills for node-exporter-zoneinfo

This directory contains Claude Code skills and commands for automating testing, deployment, and monitoring workflows for the node-exporter-zoneinfo project.

## Quick Start

To use the available skills, simply invoke them as slash commands in Claude Code:

```bash
/phase-testing                    # Run full 5-phase test
/phase-testing --duration=15m     # Run with 15min per phase
/phase-testing --skip-phase=2     # Skip phase 2
```

## Structure

This directory follows the Claude Code skill module pattern:

```
.claude/
├── README.md                      # This file
├── AGENTS.md                      # Skill documentation
├── settings.local.json            # Local Claude settings
├── commands/                      # User-invocable commands
│   └── phase-testing.md          # /phase-testing command
└── skills/                        # Skill implementations
    └── phase-testing/
        └── SKILL.md              # Phase testing skill logic
```

## Available Skills

### `/phase-testing` - 5-Phase Monitoring Test with Evidence-Based Reports

Automates a comprehensive testing scenario for node-exporter-zoneinfo with **evidence-based reporting framework**.

**Phases**:
1. **All collectors** (30min) - zoneinfo + interrupts + softirqs
2. **No node-exporter** (30min) - Prometheus Operator only
3. **Zoneinfo only** (30min) - Single collector test
4. **Interrupts only** (30min) - Single collector test
5. **Softirqs only** (30min) - Single collector test

**Key Features**:
- Automated deployment using Go monitor binary
- Evidence-based reports with command outputs
- Prometheus queries proving metrics present/absent
- Sample metric values with timestamps
- Resource consumption data (CPU, memory)
- Timeline with RFC3339 timestamps
- Per-phase reports following technical writing framework
- Consolidated analysis with comparison tables
- Copy-paste ready commands for reproducibility

**Evidence Framework**:
Reports follow structured framework inspired by technical writing best practices:
- Command-then-output pairs (verbatim, not summaries)
- Prometheus queries as proof of metric availability
- Sample metric values at specific timestamps
- Resource usage snapshots at T+5min, T+15min, T+30min
- Timeline with RFC3339 timestamps for reproducibility
- Caveats and warnings in callout blocks
- Production recommendations based on observed data

See [AGENTS.md](AGENTS.md) for detailed usage and [FINAL_SUMMARY.md](FINAL_SUMMARY.md) for complete implementation details.

## How Skills Work

### Commands vs Skills

- **Commands** (`commands/*.md`): User-facing slash commands (e.g., `/phase-testing`)
- **Skills** (`skills/*/SKILL.md`): Implementation logic loaded by commands

### Command File Format

Commands use YAML frontmatter + instruction:

```markdown
---
description: Brief description shown in /help
---
Load the <skill-name> skill and execute <task>. $ARGUMENTS
```

### Skill File Format

Skills use YAML frontmatter + detailed instructions:

```markdown
---
name: skill-name
description: Detailed description of capabilities
---

[Detailed implementation instructions]

## Task Execution

$ARGUMENTS
```

## Adding New Skills

To add a new skill:

1. **Create the skill implementation**:
   ```bash
   mkdir -p .claude/skills/my-skill
   # Create .claude/skills/my-skill/SKILL.md
   ```

2. **Create the command**:
   ```bash
   # Create .claude/commands/my-skill.md
   ```

3. **Document in AGENTS.md**:
   - Add skill description
   - Document usage examples
   - List options and output

4. **Test the skill**:
   ```bash
   /my-skill
   ```

## Example: Creating a Simple Skill

**File**: `.claude/skills/hello/SKILL.md`
```markdown
---
name: hello
description: Say hello to the user
---

Print a friendly greeting message.

Arguments: $ARGUMENTS

Output: "Hello, World!"
```

**File**: `.claude/commands/hello.md`
```markdown
---
description: Print a hello message
---
Load the hello skill and greet the user. $ARGUMENTS
```

**Usage**:
```bash
/hello
```

## Integration with Existing Tools

The phase-testing skill integrates with:

- **kubectl/oc**: Kubernetes deployment and management
- **Prometheus**: Metric querying and validation
- **Existing manifests**: Uses DaemonSet files in `./node-exporter-zoneinfo/`
- **Reports**: Generates markdown reports in `./reports/`

## Development Workflow

1. **Edit skill**: Modify `.claude/skills/phase-testing/SKILL.md`
2. **Test**: Invoke `/phase-testing` in Claude Code
3. **Iterate**: Refine based on results
4. **Document**: Update AGENTS.md with changes

## Best Practices

- **Use descriptive names**: Skills should have clear, action-oriented names
- **Include examples**: Show usage patterns in documentation
- **Handle errors**: Validate prerequisites before execution
- **Generate reports**: Output results to files for review
- **Support options**: Allow customization via arguments

## Related Files

- **Project root**:
  - `node-exporter-zoneinfo/` - Kubernetes manifests
  - `reports/` - Generated test reports
  - `DEPLOYMENT.md` - Manual deployment guide
  - `README.md` - Project overview

## Troubleshooting

### Skill not found

If `/phase-testing` shows "skill not found":

1. Verify files exist:
   ```bash
   ls .claude/commands/phase-testing.md
   ls .claude/skills/phase-testing/SKILL.md
   ```

2. Check file format (YAML frontmatter required)

3. Restart Claude Code if needed

### Skill errors during execution

- Check kubectl/oc connectivity
- Verify manifests directory exists
- Ensure Prometheus Operator is running
- Review error messages in Claude output

## Additional Resources

- [Claude Code Documentation](https://claude.ai/claude-code)
- [Example Skills Repository](https://github.com/mvazquezc/coding-agents-resources)
- [AGENTS.md](AGENTS.md) - Detailed skill documentation

## Contributing

To contribute new skills:

1. Follow the directory structure above
2. Use YAML frontmatter format
3. Document in AGENTS.md
4. Test thoroughly
5. Submit with examples
