package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/openshift/node-exporter-zoneinfo-test/pkg/charts"
	"github.com/openshift/node-exporter-zoneinfo-test/pkg/deploy"
	"github.com/openshift/node-exporter-zoneinfo-test/pkg/metrics"
	"github.com/openshift/node-exporter-zoneinfo-test/pkg/report"
)

const (
	defaultDuration       = 30 * time.Minute
	defaultSampleInterval = 30 * time.Second
	reportDir             = "reports"
)

var (
	doDeploy       = flag.Bool("deploy", false, "Deploy node-exporter-zoneinfo before monitoring")
	duration       = flag.Duration("duration", defaultDuration, "Monitoring duration (e.g., 30m, 2m)")
	sampleInterval = flag.Duration("interval", defaultSampleInterval, "Sample collection interval")
	outputFormat   = flag.String("format", "text", "Output format: text, json, html")
	skipDeploy     = flag.Bool("skip-deploy", false, "Skip deployment verification")
	kubeconfig     = flag.String("kubeconfig", "", "Path to kubeconfig file (optional, defaults to $KUBECONFIG or ~/.kube/config)")
	twoPhase       = flag.Bool("two-phase", false, "Run two-phase monitoring: Phase 1 with node-exporter, Phase 2 without")
)

func main() {
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n\nReceived interrupt signal, shutting down gracefully...")
		cancel()
	}()

	// Create reports directory
	if err := os.MkdirAll(reportDir, 0755); err != nil {
		log.Fatalf("Failed to create reports directory: %v", err)
	}

	fmt.Println("╔═══════════════════════════════════════════════════════════════╗")
	fmt.Println("║   Node Exporter Zoneinfo - Resource Monitoring Tool          ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	if *twoPhase {
		runTwoPhaseMonitoring(ctx)
	} else {
		runSinglePhaseMonitoring(ctx)
	}
}

func runTwoPhaseMonitoring(ctx context.Context) {
	deployer := deploy.NewDeployer(*kubeconfig)

	fmt.Println("╔═══════════════════════════════════════════════════════════════╗")
	fmt.Println("║              TWO-PHASE MONITORING MODE                        ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// ==================== PHASE 1 ====================
	fmt.Println("┌───────────────────────────────────────────────────────────────┐")
	fmt.Println("│  PHASE 1: Node-Exporter + Prometheus Operator (30 minutes)   │")
	fmt.Println("└───────────────────────────────────────────────────────────────┘")
	fmt.Println()

	// Step 1: Ensure clean state - undeploy first
	fmt.Println("🧹 Cleaning up existing resources...")
	if err := deployer.Undeploy(ctx); err != nil {
		log.Printf("Warning: Cleanup failed (may not exist): %v", err)
	}
	time.Sleep(5 * time.Second)

	// Step 2: Deploy fresh resources
	fmt.Println("📦 Deploying node-exporter-zoneinfo...")
	if err := deployer.Deploy(ctx); err != nil {
		log.Fatalf("Deployment failed: %v", err)
	}
	fmt.Println("✅ Deployment completed successfully")
	fmt.Println()

	// Wait for pods to be ready
	fmt.Println("⏳ Waiting for pods to be ready...")
	time.Sleep(15 * time.Second)

	// Verify deployment
	fmt.Println("🔍 Verifying deployment status...")
	status, err := deployer.GetDeploymentStatus(ctx)
	if err != nil {
		log.Printf("Warning: Could not verify deployment: %v", err)
	} else {
		fmt.Printf("   Node Exporter Zoneinfo: %d/%d pods ready\n",
			status.NodeExporterReady, status.NodeExporterTotal)
		fmt.Printf("   Prometheus: %d/%d pods ready\n",
			status.PrometheusReady, status.PrometheusTotal)
		fmt.Printf("   Thanos: %d/%d pods ready\n",
			status.ThanosReady, status.ThanosTotal)
		fmt.Println()
	}

	// Define monitoring targets - we'll monitor all of them for full 60 minutes
	// Phase 1: both node-exporter and operators
	// Phase 2: only operators (node-exporter will fail to collect, which is fine)
	allTargets := []metrics.PodTarget{
		{Namespace: "node-exporter-zoneinfo", LabelSelector: "app.kubernetes.io/name=node-exporter-zoneinfo"},
		{Namespace: "openshift-monitoring", LabelSelector: "app.kubernetes.io/name=prometheus-operator"},
		{Namespace: "openshift-monitoring", LabelSelector: "app.kubernetes.io/name=prometheus-operator-admission-webhook"},
	}

	// Initialize combined results
	combinedResults := &metrics.MonitoringResults{
		StartTime: time.Now(),
		Samples:   make([]metrics.ResourceSample, 0),
		Errors:    make([]string, 0),
	}

	// Initialize metrics collector
	collector := metrics.NewCollector(*sampleInterval, *kubeconfig)

	// ==================== PHASE 1: First 30 minutes ====================
	fmt.Printf("📊 Starting Phase 1 monitoring for %v (interval: %v)\n", *duration, *sampleInterval)
	fmt.Printf("   Monitoring: node-exporter-zoneinfo + prometheus-operator*\n")
	fmt.Println()

	phase1Start := time.Now()
	phase1Results, err := collector.Collect(ctx, allTargets, *duration)
	if err != nil && err != context.Canceled {
		log.Fatalf("Phase 1 monitoring failed: %v", err)
	}
	phase1Duration := time.Since(phase1Start)

	fmt.Println()
	fmt.Println("✅ Phase 1 monitoring completed!")
	fmt.Printf("   Duration: %v (planned: %v)\n", phase1Duration.Round(time.Second), *duration)
	fmt.Printf("   Samples collected: %d\n", len(phase1Results.Samples))
	fmt.Println()

	// Add Phase 1 samples to combined results
	combinedResults.Samples = append(combinedResults.Samples, phase1Results.Samples...)
	combinedResults.Errors = append(combinedResults.Errors, phase1Results.Errors...)

	// ==================== PHASE 2: Second 30 minutes ====================
	fmt.Println()
	fmt.Println("┌───────────────────────────────────────────────────────────────┐")
	fmt.Println("│  PHASE 2: Prometheus Operator Only (30 minutes)              │")
	fmt.Println("└───────────────────────────────────────────────────────────────┘")
	fmt.Println()

	// Delete node-exporter-zoneinfo DaemonSet
	fmt.Println("🗑️  Removing node-exporter-zoneinfo DaemonSet...")
	if err := deployer.DeleteDaemonSet(ctx); err != nil {
		log.Fatalf("Failed to delete DaemonSet: %v", err)
	}
	fmt.Println("✅ DaemonSet removed successfully")
	fmt.Println()

	// Continue monitoring same targets - node-exporter will fail (expected)
	fmt.Printf("📊 Starting Phase 2 monitoring for %v (interval: %v)\n", *duration, *sampleInterval)
	fmt.Printf("   Monitoring: prometheus-operator* (node-exporter pods deleted)\n")
	fmt.Println()

	phase2Start := time.Now()
	phase2Results, err := collector.Collect(ctx, allTargets, *duration)
	if err != nil && err != context.Canceled {
		log.Fatalf("Phase 2 monitoring failed: %v", err)
	}
	phase2Duration := time.Since(phase2Start)

	fmt.Println()
	fmt.Println("✅ Phase 2 monitoring completed!")
	fmt.Printf("   Duration: %v (planned: %v)\n", phase2Duration.Round(time.Second), *duration)
	fmt.Printf("   Samples collected: %d\n", len(phase2Results.Samples))
	fmt.Println()

	// Add Phase 2 samples to combined results
	combinedResults.Samples = append(combinedResults.Samples, phase2Results.Samples...)
	combinedResults.Errors = append(combinedResults.Errors, phase2Results.Errors...)

	// Set combined results metadata
	combinedResults.EndTime = time.Now()
	combinedResults.Duration = combinedResults.EndTime.Sub(combinedResults.StartTime)
	combinedResults.SampleCount = len(combinedResults.Samples)

	// ==================== GENERATE COMBINED REPORT ====================
	fmt.Println()
	fmt.Println("📝 Generating combined report (60 minutes)...")

	timestamp := time.Now().Format("20060102-150405")

	switch *outputFormat {
	case "json":
		filename := fmt.Sprintf("%s/two-phase-monitoring-report-%s.json", reportDir, timestamp)
		if err := saveJSONReport(filename, combinedResults); err != nil {
			log.Fatalf("Failed to save JSON report: %v", err)
		}
		fmt.Printf("   JSON report saved to: %s\n", filename)

	case "html":
		filename := fmt.Sprintf("%s/two-phase-monitoring-report-%s.html", reportDir, timestamp)
		generator := report.NewHTMLGenerator()
		if err := generator.Generate(filename, combinedResults); err != nil {
			log.Fatalf("Failed to generate HTML report: %v", err)
		}
		fmt.Printf("   HTML report saved to: %s\n", filename)

	default:
		filename := fmt.Sprintf("%s/two-phase-monitoring-report-%s.txt", reportDir, timestamp)
		generator := report.NewTextGeneratorWithMode(true) // Two-phase mode enabled
		if err := generator.Generate(filename, combinedResults); err != nil {
			log.Fatalf("Failed to generate text report: %v", err)
		}
		fmt.Printf("   Text report saved to: %s\n", filename)
	}

	// Generate combined charts
	fmt.Println()
	fmt.Println("📈 Generating combined metric charts (60 minutes)...")
	chartGen := charts.NewChartGenerator(reportDir)
	if err := chartGen.GenerateCharts(combinedResults); err != nil {
		log.Printf("Warning: Failed to generate charts: %v", err)
	} else {
		fmt.Printf("   Charts saved to: %s/charts/\n", reportDir)
	}

	// Print summary
	fmt.Println()
	printSummary(combinedResults)

	// Final summary
	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════════════════════════╗")
	fmt.Println("║           TWO-PHASE MONITORING COMPLETE                       ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("📋 Results Summary:")
	fmt.Printf("   Total monitoring duration: %v\n", combinedResults.Duration.Round(time.Second))
	fmt.Printf("   Phase 1 duration: %v (with node-exporter)\n", phase1Duration.Round(time.Second))
	fmt.Printf("   Phase 2 duration: %v (without node-exporter)\n", phase2Duration.Round(time.Second))
	fmt.Printf("   Total samples collected: %d\n", len(combinedResults.Samples))
	fmt.Printf("   Phase 1 samples: %d\n", len(phase1Results.Samples))
	fmt.Printf("   Phase 2 samples: %d\n", len(phase2Results.Samples))
	fmt.Println()
	fmt.Println("📁 Output Files:")
	fmt.Printf("   Report: reports/two-phase-monitoring-report-%s.txt\n", timestamp)
	fmt.Printf("   Charts: reports/charts/ (showing full 60-minute timeline)\n")
	fmt.Println()
}

func runSinglePhaseMonitoring(ctx context.Context) {
	// Deploy if requested
	if *doDeploy {
		fmt.Println("📦 Deploying node-exporter-zoneinfo...")
		deployer := deploy.NewDeployer(*kubeconfig)
		if err := deployer.Deploy(ctx); err != nil {
			log.Fatalf("Deployment failed: %v", err)
		}
		fmt.Println("✅ Deployment completed successfully")
		fmt.Println()

		// Wait for pods to be ready
		fmt.Println("⏳ Waiting for pods to be ready...")
		time.Sleep(10 * time.Second)
	}

	// Verify deployment status
	if !*skipDeploy {
		fmt.Println("🔍 Verifying deployment status...")
		deployer := deploy.NewDeployer(*kubeconfig)
		status, err := deployer.GetDeploymentStatus(ctx)
		if err != nil {
			log.Printf("Warning: Could not verify deployment: %v", err)
		} else {
			fmt.Printf("   Node Exporter Zoneinfo: %d/%d pods ready\n",
				status.NodeExporterReady, status.NodeExporterTotal)
			fmt.Printf("   Prometheus: %d/%d pods ready\n",
				status.PrometheusReady, status.PrometheusTotal)
			fmt.Printf("   Thanos: %d/%d pods ready\n",
				status.ThanosReady, status.ThanosTotal)
			fmt.Println()
		}
	}

	// Define target pods
	targets := []metrics.PodTarget{
		{Namespace: "node-exporter-zoneinfo", LabelSelector: "app.kubernetes.io/name=node-exporter-zoneinfo"},
		{Namespace: "openshift-monitoring", PodName: "prometheus-k8s-0"},
		{Namespace: "openshift-monitoring", PodName: "prometheus-k8s-1"},
		{Namespace: "openshift-monitoring", LabelSelector: "app.kubernetes.io/name=prometheus-operator"},
		{Namespace: "openshift-monitoring", LabelSelector: "app.kubernetes.io/name=prometheus-operator-admission-webhook"},
		{Namespace: "openshift-monitoring", LabelSelector: "app.kubernetes.io/name=thanos-query"},
	}

	fmt.Printf("📊 Starting resource monitoring for %v (interval: %v)\n", *duration, *sampleInterval)
	fmt.Printf("   Monitoring %d target groups\n", len(targets))
	fmt.Println()

	runMonitoring(ctx, targets, *duration, "")

	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    Monitoring Complete                        ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════╝")
}

func runMonitoring(ctx context.Context, targets []metrics.PodTarget, duration time.Duration, phasePrefix string) *metrics.MonitoringResults {
	// Initialize metrics collector
	collector := metrics.NewCollector(*sampleInterval, *kubeconfig)

	// Start monitoring
	startTime := time.Now()
	results, err := collector.Collect(ctx, targets, duration)
	if err != nil && err != context.Canceled {
		log.Fatalf("Monitoring failed: %v", err)
	}
	actualDuration := time.Since(startTime)

	fmt.Println()
	fmt.Println("✅ Monitoring completed!")
	fmt.Printf("   Duration: %v (planned: %v)\n", actualDuration.Round(time.Second), duration)
	fmt.Printf("   Samples collected: %d\n", len(results.Samples))
	fmt.Println()

	// Generate report
	fmt.Println("📝 Generating report...")

	timestamp := time.Now().Format("20060102-150405")
	prefix := ""
	if phasePrefix != "" {
		prefix = phasePrefix + "-"
	}

	switch *outputFormat {
	case "json":
		filename := fmt.Sprintf("%s/%smonitoring-report-%s.json", reportDir, prefix, timestamp)
		if err := saveJSONReport(filename, results); err != nil {
			log.Fatalf("Failed to save JSON report: %v", err)
		}
		fmt.Printf("   JSON report saved to: %s\n", filename)

	case "html":
		filename := fmt.Sprintf("%s/%smonitoring-report-%s.html", reportDir, prefix, timestamp)
		generator := report.NewHTMLGenerator()
		if err := generator.Generate(filename, results); err != nil {
			log.Fatalf("Failed to generate HTML report: %v", err)
		}
		fmt.Printf("   HTML report saved to: %s\n", filename)

	default:
		filename := fmt.Sprintf("%s/%smonitoring-report-%s.txt", reportDir, prefix, timestamp)
		generator := report.NewTextGenerator()
		if err := generator.Generate(filename, results); err != nil {
			log.Fatalf("Failed to generate text report: %v", err)
		}
		fmt.Printf("   Text report saved to: %s\n", filename)
	}

	// Generate charts
	fmt.Println()
	fmt.Println("📈 Generating metric charts...")
	chartDir := reportDir
	if phasePrefix != "" {
		chartDir = fmt.Sprintf("%s/%s", reportDir, phasePrefix)
	}
	chartGen := charts.NewChartGenerator(chartDir)
	if err := chartGen.GenerateCharts(results); err != nil {
		log.Printf("Warning: Failed to generate charts: %v", err)
	} else {
		fmt.Printf("   Charts saved to: %s/charts/\n", chartDir)
	}

	// Print summary to console
	fmt.Println()
	printSummary(results)

	return results
}

func saveJSONReport(filename string, results *metrics.MonitoringResults) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	return encoder.Encode(results)
}

func printSummary(results *metrics.MonitoringResults) {
	fmt.Println("📋 Summary")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	summary := report.CalculateSummary(results)

	for ns, pods := range summary {
		fmt.Printf("\n📦 Namespace: %s\n", ns)
		fmt.Println("   ────────────────────────────────────────────────────────────")

		for podName, stats := range pods {
			if stats.NodeName != "" {
				fmt.Printf("   Pod: %s (Node: %s)\n", podName, stats.NodeName)
			} else {
				fmt.Printf("   Pod: %s\n", podName)
			}
			fmt.Printf("      CPU:    avg=%.2fm, min=%.2fm, max=%.2fm\n",
				stats.CPU.Avg, stats.CPU.Min, stats.CPU.Max)
			fmt.Printf("      Memory: avg=%.2fMi, min=%.2fMi, max=%.2fMi\n",
				stats.Memory.Avg/1024/1024,
				stats.Memory.Min/1024/1024,
				stats.Memory.Max/1024/1024)
		}
	}

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}
