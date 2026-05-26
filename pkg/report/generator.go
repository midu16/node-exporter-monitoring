package report

import (
	"fmt"
	"os"
	"sort"
	"text/template"
	"time"

	"github.com/openshift/node-exporter-zoneinfo-test/pkg/metrics"
)

type PodStats struct {
	CPU      Stats
	Memory   Stats
	NodeName string
}

type Stats struct {
	Min   float64
	Max   float64
	Avg   float64
	Count int
}

type TextGenerator struct {
	TwoPhaseMode bool
}

func NewTextGenerator() *TextGenerator {
	return &TextGenerator{TwoPhaseMode: false}
}

func NewTextGeneratorWithMode(twoPhase bool) *TextGenerator {
	return &TextGenerator{TwoPhaseMode: twoPhase}
}

// writeHelper wraps fmt.Fprintf to accumulate errors
type writeHelper struct {
	w   *os.File
	err error
}

func (w *writeHelper) fprintf(format string, args ...interface{}) {
	if w.err != nil {
		return
	}
	_, w.err = fmt.Fprintf(w.w, format, args...)
}

func (g *TextGenerator) Generate(filename string, results *metrics.MonitoringResults) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	w := &writeHelper{w: f}

	// Use the TwoPhaseMode flag set during initialization
	isTwoPhase := g.TwoPhaseMode

	// Write header
	w.fprintf("╔═══════════════════════════════════════════════════════════════════════════════╗\n")
	w.fprintf("║         Node Exporter Zoneinfo - Resource Monitoring Report                  ║\n")
	w.fprintf("╚═══════════════════════════════════════════════════════════════════════════════╝\n\n")

	// Monitoring period
	w.fprintf("📅 Monitoring Period\n")
	w.fprintf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	w.fprintf("  Start Time:    %s\n", results.StartTime.Format("2006-01-02 15:04:05 MST"))
	w.fprintf("  End Time:      %s\n", results.EndTime.Format("2006-01-02 15:04:05 MST"))
	w.fprintf("  Duration:      %v\n", results.Duration.Round(time.Second))
	w.fprintf("  Samples:       %d\n", results.SampleCount)
	w.fprintf("  Data Points:   %d\n", len(results.Samples))
	if isTwoPhase {
		w.fprintf("  Mode:          Two-Phase (Phase 1 + Phase 2)\n")
	}
	w.fprintf("\n")

	if w.err != nil {
		return w.err
	}

	// Calculate summary for deployment section
	summary := CalculateSummary(results)

	// Deployment summary
	w.fprintf("📦 Deployment Summary\n")
	w.fprintf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	totalPods := make(map[string]int)
	for ns, pods := range summary {
		totalPods[ns] = len(pods)
	}

	w.fprintf("  Namespace: node-exporter-zoneinfo\n")
	w.fprintf("    Pods Monitored: %d", totalPods["node-exporter-zoneinfo"])
	if isTwoPhase {
		w.fprintf(" (Phase 1 only - deleted in Phase 2)")
	}
	w.fprintf("\n\n")

	w.fprintf("  Namespace: openshift-monitoring\n")
	w.fprintf("    Pods Monitored: %d", totalPods["openshift-monitoring"])
	if isTwoPhase {
		w.fprintf(" (Both phases)")
	}
	w.fprintf("\n")
	w.fprintf("    - Prometheus: 2 (prometheus-k8s-0, prometheus-k8s-1)\n")
	w.fprintf("    - Prometheus Operator: monitored via labels\n")
	w.fprintf("    - Thanos Querier: monitored via labels\n\n")

	if w.err != nil {
		return w.err
	}

	if isTwoPhase {
		// Generate two-phase report with separate sections
		if err := g.generateTwoPhaseReport(w, results); err != nil {
			return err
		}
	} else {
		// Generate standard single-phase report
		if err := g.generateResourceSection(w, summary, "📊 Resource Consumption Details"); err != nil {
			return err
		}
	}

	// Errors section
	if len(results.Errors) > 0 {
		w.fprintf("\n⚠️  Errors Encountered\n")
		w.fprintf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		uniqueErrors := make(map[string]int)
		for _, err := range results.Errors {
			uniqueErrors[err]++
		}
		for err, count := range uniqueErrors {
			w.fprintf("  [%d occurrences] %s\n", count, err)
		}
		w.fprintf("\n")
	}

	// Footer
	w.fprintf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	w.fprintf("Report generated: %s\n", time.Now().Format("2006-01-02 15:04:05 MST"))
	w.fprintf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	return w.err
}

func (g *TextGenerator) generateTwoPhaseReport(w *writeHelper, results *metrics.MonitoringResults) error {
	// Calculate midpoint time
	midpointTime := results.StartTime.Add(results.Duration / 2)

	// Split samples into Phase 1 and Phase 2
	phase1Samples := make([]metrics.ResourceSample, 0)
	phase2Samples := make([]metrics.ResourceSample, 0)

	for _, sample := range results.Samples {
		if sample.Timestamp.Before(midpointTime) || sample.Timestamp.Equal(midpointTime) {
			phase1Samples = append(phase1Samples, sample)
		} else {
			phase2Samples = append(phase2Samples, sample)
		}
	}

	// Create Phase 1 results
	phase1Results := &metrics.MonitoringResults{
		StartTime:   results.StartTime,
		EndTime:     midpointTime,
		Duration:    results.Duration / 2,
		SampleCount: len(phase1Samples),
		Samples:     phase1Samples,
		Errors:      results.Errors,
	}

	// Create Phase 2 results
	phase2Results := &metrics.MonitoringResults{
		StartTime:   midpointTime,
		EndTime:     results.EndTime,
		Duration:    results.Duration / 2,
		SampleCount: len(phase2Samples),
		Samples:     phase2Samples,
		Errors:      results.Errors,
	}

	// Generate Phase 1 section
	w.fprintf("╔═══════════════════════════════════════════════════════════════════════════════╗\n")
	w.fprintf("║  PHASE 1: WITH NODE-EXPORTER (First 30 minutes)                              ║\n")
	w.fprintf("╚═══════════════════════════════════════════════════════════════════════════════╝\n\n")
	w.fprintf("📅 Phase 1 Period\n")
	w.fprintf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	w.fprintf("  Start Time:    %s\n", phase1Results.StartTime.Format("2006-01-02 15:04:05 MST"))
	w.fprintf("  End Time:      %s\n", phase1Results.EndTime.Format("2006-01-02 15:04:05 MST"))
	w.fprintf("  Duration:      %v\n", phase1Results.Duration.Round(time.Second))
	w.fprintf("  Samples:       %d\n\n", len(phase1Results.Samples))

	phase1Summary := CalculateSummary(phase1Results)
	if err := g.generateResourceSection(w, phase1Summary, "📊 Phase 1 Resource Consumption"); err != nil {
		return err
	}

	// Generate Phase 2 section
	w.fprintf("\n")
	w.fprintf("╔═══════════════════════════════════════════════════════════════════════════════╗\n")
	w.fprintf("║  PHASE 2: WITHOUT NODE-EXPORTER (Second 30 minutes)                          ║\n")
	w.fprintf("╚═══════════════════════════════════════════════════════════════════════════════╝\n\n")
	w.fprintf("📅 Phase 2 Period\n")
	w.fprintf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	w.fprintf("  Start Time:    %s\n", phase2Results.StartTime.Format("2006-01-02 15:04:05 MST"))
	w.fprintf("  End Time:      %s\n", phase2Results.EndTime.Format("2006-01-02 15:04:05 MST"))
	w.fprintf("  Duration:      %v\n", phase2Results.Duration.Round(time.Second))
	w.fprintf("  Samples:       %d\n\n", len(phase2Results.Samples))

	phase2Summary := CalculateSummary(phase2Results)

	// Filter out node-exporter-zoneinfo from Phase 2
	filteredPhase2Summary := make(map[string]map[string]*PodStats)
	for ns, pods := range phase2Summary {
		if ns != "node-exporter-zoneinfo" {
			filteredPhase2Summary[ns] = pods
		}
	}

	if err := g.generateResourceSection(w, filteredPhase2Summary, "📊 Phase 2 Resource Consumption (Prometheus Operator Only)"); err != nil {
		return err
	}

	return w.err
}

func (g *TextGenerator) generateResourceSection(w *writeHelper, summary map[string]map[string]*PodStats, title string) error {
	w.fprintf("%s\n", title)
	w.fprintf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	// Sort namespaces for consistent output
	namespaces := make([]string, 0, len(summary))
	for ns := range summary {
		namespaces = append(namespaces, ns)
	}
	sort.Strings(namespaces)

	for _, ns := range namespaces {
		pods := summary[ns]
		w.fprintf("Namespace: %s\n", ns)
		w.fprintf("─────────────────────────────────────────────────────────────────────────────────\n")

		// Sort pod names
		podNames := make([]string, 0, len(pods))
		for podName := range pods {
			podNames = append(podNames, podName)
		}
		sort.Strings(podNames)

		for _, podName := range podNames {
			stats := pods[podName]
			if stats.NodeName != "" {
				w.fprintf("\n  Pod: %s (Node: %s)\n", podName, stats.NodeName)
			} else {
				w.fprintf("\n  Pod: %s\n", podName)
			}
			w.fprintf("  ┌─────────────────────────────────────────────────────────────────────────┐\n")
			w.fprintf("  │ CPU Usage (millicores)                                                  │\n")
			w.fprintf("  │   Minimum:  %10.2f m                                                 │\n", stats.CPU.Min)
			w.fprintf("  │   Maximum:  %10.2f m                                                 │\n", stats.CPU.Max)
			w.fprintf("  │   Average:  %10.2f m                                                 │\n", stats.CPU.Avg)
			w.fprintf("  │   Samples:  %10d                                                   │\n", stats.CPU.Count)
			w.fprintf("  ├─────────────────────────────────────────────────────────────────────────┤\n")
			w.fprintf("  │ Memory Usage (MiB)                                                      │\n")
			w.fprintf("  │   Minimum:  %10.2f MiB                                              │\n", stats.Memory.Min/1024/1024)
			w.fprintf("  │   Maximum:  %10.2f MiB                                              │\n", stats.Memory.Max/1024/1024)
			w.fprintf("  │   Average:  %10.2f MiB                                              │\n", stats.Memory.Avg/1024/1024)
			w.fprintf("  │   Samples:  %10d                                                   │\n", stats.Memory.Count)
			w.fprintf("  └─────────────────────────────────────────────────────────────────────────┘\n")
		}
		w.fprintf("\n")
	}

	return w.err
}

func CalculateSummary(results *metrics.MonitoringResults) map[string]map[string]*PodStats {
	summary := make(map[string]map[string]*PodStats)

	for _, sample := range results.Samples {
		if summary[sample.Namespace] == nil {
			summary[sample.Namespace] = make(map[string]*PodStats)
		}

		if summary[sample.Namespace][sample.PodName] == nil {
			summary[sample.Namespace][sample.PodName] = &PodStats{
				CPU:      Stats{Min: float64(sample.CPUUsage), Max: float64(sample.CPUUsage)},
				Memory:   Stats{Min: float64(sample.MemoryUsage), Max: float64(sample.MemoryUsage)},
				NodeName: sample.NodeName,
			}
		}

		stats := summary[sample.Namespace][sample.PodName]

		// Update CPU stats
		cpuUsage := float64(sample.CPUUsage)
		if cpuUsage < stats.CPU.Min {
			stats.CPU.Min = cpuUsage
		}
		if cpuUsage > stats.CPU.Max {
			stats.CPU.Max = cpuUsage
		}
		stats.CPU.Avg = (stats.CPU.Avg*float64(stats.CPU.Count) + cpuUsage) / float64(stats.CPU.Count+1)
		stats.CPU.Count++

		// Update Memory stats
		memUsage := float64(sample.MemoryUsage)
		if memUsage < stats.Memory.Min {
			stats.Memory.Min = memUsage
		}
		if memUsage > stats.Memory.Max {
			stats.Memory.Max = memUsage
		}
		stats.Memory.Avg = (stats.Memory.Avg*float64(stats.Memory.Count) + memUsage) / float64(stats.Memory.Count+1)
		stats.Memory.Count++
	}

	return summary
}

type HTMLGenerator struct{}

func NewHTMLGenerator() *HTMLGenerator {
	return &HTMLGenerator{}
}

func (g *HTMLGenerator) Generate(filename string, results *metrics.MonitoringResults) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	summary := CalculateSummary(results)

	tmpl := `<!DOCTYPE html>
<html>
<head>
    <title>Node Exporter Zoneinfo - Monitoring Report</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; background: #f5f5f5; }
        .container { max-width: 1200px; margin: 0 auto; background: white; padding: 30px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        h1 { color: #333; border-bottom: 3px solid #007bff; padding-bottom: 10px; }
        h2 { color: #555; margin-top: 30px; }
        .info-box { background: #e7f3ff; padding: 15px; border-radius: 5px; margin: 20px 0; border-left: 4px solid #007bff; }
        .pod-card { background: #f9f9f9; padding: 15px; margin: 15px 0; border-radius: 5px; border: 1px solid #ddd; }
        .pod-name { font-weight: bold; color: #007bff; font-size: 1.1em; }
        .stats-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; margin-top: 10px; }
        .stat-item { padding: 10px; background: white; border-radius: 3px; }
        .stat-label { font-size: 0.9em; color: #666; }
        .stat-value { font-size: 1.2em; font-weight: bold; color: #333; }
        .namespace { background: #28a745; color: white; padding: 10px; border-radius: 5px; margin-top: 20px; }
        table { width: 100%; border-collapse: collapse; margin: 20px 0; }
        th, td { padding: 12px; text-align: left; border-bottom: 1px solid #ddd; }
        th { background: #007bff; color: white; }
        tr:hover { background: #f5f5f5; }
        .footer { margin-top: 30px; padding-top: 20px; border-top: 2px solid #ddd; color: #666; text-align: center; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🔍 Node Exporter Zoneinfo - Resource Monitoring Report</h1>

        <div class="info-box">
            <strong>Monitoring Period:</strong> {{ .StartTime }} to {{ .EndTime }}<br>
            <strong>Duration:</strong> {{ .Duration }}<br>
            <strong>Samples Collected:</strong> {{ .SampleCount }}<br>
            <strong>Total Data Points:</strong> {{ .DataPoints }}
        </div>

        <h2>📊 Resource Consumption by Namespace</h2>
        {{ range $ns, $pods := .Summary }}
        <div class="namespace">📦 Namespace: {{ $ns }}</div>
        {{ range $podName, $stats := $pods }}
        <div class="pod-card">
            <div class="pod-name">{{ $podName }}</div>
            <div class="stats-grid">
                <div class="stat-item">
                    <div class="stat-label">CPU Min</div>
                    <div class="stat-value">{{ printf "%.2f" $stats.CPU.Min }} m</div>
                </div>
                <div class="stat-item">
                    <div class="stat-label">CPU Max</div>
                    <div class="stat-value">{{ printf "%.2f" $stats.CPU.Max }} m</div>
                </div>
                <div class="stat-item">
                    <div class="stat-label">CPU Avg</div>
                    <div class="stat-value">{{ printf "%.2f" $stats.CPU.Avg }} m</div>
                </div>
                <div class="stat-item">
                    <div class="stat-label">Memory Avg</div>
                    <div class="stat-value">{{ printf "%.2f" (div $stats.Memory.Avg 1048576) }} MiB</div>
                </div>
            </div>
        </div>
        {{ end }}
        {{ end }}

        <div class="footer">
            Report generated: {{ .Generated }}<br>
            Node Exporter Zoneinfo Monitoring Tool
        </div>
    </div>
</body>
</html>`

	data := struct {
		StartTime   string
		EndTime     string
		Duration    string
		SampleCount int
		DataPoints  int
		Summary     map[string]map[string]*PodStats
		Generated   string
	}{
		StartTime:   results.StartTime.Format("2006-01-02 15:04:05"),
		EndTime:     results.EndTime.Format("2006-01-02 15:04:05"),
		Duration:    results.Duration.Round(time.Second).String(),
		SampleCount: results.SampleCount,
		DataPoints:  len(results.Samples),
		Summary:     summary,
		Generated:   time.Now().Format("2006-01-02 15:04:05 MST"),
	}

	funcMap := template.FuncMap{
		"div": func(a, b float64) float64 { return a / b },
	}

	t := template.Must(template.New("report").Funcs(funcMap).Parse(tmpl))
	return t.Execute(f, data)
}
