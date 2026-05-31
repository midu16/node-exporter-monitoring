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
	threePhase     = flag.Bool("three-phase", false, "Run three-phase monitoring: Phase 1 (all collectors), Phase 2 (no node-exporter), Phase 3 (zoneinfo only)")
	sixPhase       = flag.Bool("six-phase", false, "Run six-phase monitoring: Phase 1 (no node-exporter), Phase 2 (all collectors), Phase 3 (no node-exporter), Phase 4 (zoneinfo only), Phase 5 (interrupts only), Phase 6 (softirqs only)")
	useRootless    = flag.Bool("rootless", false, "Deploy rootless variant of node-exporter daemonset")
	zoneinfoOnly   = flag.Bool("zoneinfo-only", false, "Deploy with only zoneinfo collector enabled")
	interruptsOnly = flag.Bool("interrupts-only", false, "Deploy with only interrupts collector enabled")
	softirqsOnly   = flag.Bool("softirqs-only", false, "Deploy with only softirqs collector enabled")
	manifestsDir   = flag.String("manifests-dir", "", "Custom path to manifests directory (default: ./node-exporter-zoneinfo/)")
)

func main() {
	flag.Parse()

	// Validate flags first (before any setup)
	phaseFlags := 0
	if *twoPhase {
		phaseFlags++
	}
	if *threePhase {
		phaseFlags++
	}
	if *sixPhase {
		phaseFlags++
	}
	if phaseFlags > 1 {
		fmt.Println("Error: Cannot specify multiple phase flags (--two-phase, --three-phase, --six-phase)")
		os.Exit(1)
	}

	// Create reports directory first (before setting up context)
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		log.Fatalf("Failed to create reports directory: %v", err)
	}

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

	fmt.Println("╔═══════════════════════════════════════════════════════════════╗")
	fmt.Println("║   Node Exporter Zoneinfo - Resource Monitoring Tool          ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	switch {
	case *sixPhase:
		runSixPhaseMonitoring(ctx)
	case *threePhase:
		runThreePhaseMonitoring(ctx)
	case *twoPhase:
		runTwoPhaseMonitoring(ctx)
	default:
		runSinglePhaseMonitoring(ctx)
	}
}

func createDeployer() *deploy.Deployer {
	variant := determineVariant()
	deployer := deploy.NewDeployerWithOptions(*kubeconfig, variant)
	if *manifestsDir != "" {
		deployer.SetManifestsDir(*manifestsDir)
	}
	return deployer
}

func determineVariant() deploy.DaemonSetVariant {
	// Check for variant flags (mutually exclusive)
	variantCount := 0
	selectedVariant := deploy.VariantFull

	if *useRootless {
		variantCount++
		selectedVariant = deploy.VariantRootless
	}
	if *zoneinfoOnly {
		variantCount++
		selectedVariant = deploy.VariantZoneinfoOnly
	}
	if *interruptsOnly {
		variantCount++
		selectedVariant = deploy.VariantInterruptsOnly
	}
	if *softirqsOnly {
		variantCount++
		selectedVariant = deploy.VariantSoftirqsOnly
	}

	if variantCount > 1 {
		fmt.Println("Error: Only one variant flag can be specified at a time")
		fmt.Println("  Flags: --rootless, --zoneinfo-only, --interrupts-only, --softirqs-only")
		os.Exit(1)
	}

	return selectedVariant
}

func createDeployerWithVariant(variant deploy.DaemonSetVariant) *deploy.Deployer {
	deployer := deploy.NewDeployerWithOptions(*kubeconfig, variant)
	if *manifestsDir != "" {
		deployer.SetManifestsDir(*manifestsDir)
	}
	return deployer
}

func runTwoPhaseMonitoring(ctx context.Context) {
	deployer := createDeployer()

	// CRITICAL: Ensure kustomization.yaml is ALWAYS restored
	defer func() {
		fmt.Println()
		fmt.Println("🔄 Restoring original configuration...")
		if err := deployer.RestoreKustomization(); err != nil {
			log.Printf("Warning: Failed to restore kustomization.yaml: %v", err)
			log.Println("   Please manually restore: git checkout node-exporter-zoneinfo/kustomization.yaml")
		}
	}()

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

	// Create timestamped raw metrics JSON file for streaming (entire two-phase run)
	timestamp := time.Now().Format("20060102-150405")
	rawMetricsFile := fmt.Sprintf("%s/two-phase-raw-metrics-%s.json", reportDir, timestamp)
	jsonFile, err := os.Create(rawMetricsFile)
	if err != nil {
		log.Fatalf("Failed to create raw metrics JSON file: %v", err)
	}
	defer func() {
		if closeErr := jsonFile.Close(); closeErr != nil {
			log.Printf("Warning: Failed to close raw metrics file: %v", closeErr)
		}
	}()

	// Set the streaming file in collector (will capture ALL samples across both phases)
	collector.SetRawMetricsFile(jsonFile)

	// ==================== PHASE 1: First 30 minutes ====================
	fmt.Printf("📊 Starting Phase 1 monitoring for %v (interval: %v)\n", *duration, *sampleInterval)
	fmt.Printf("   Monitoring: node-exporter-zoneinfo + prometheus-operator*\n")
	fmt.Println()

	phase1Start := time.Now()
	phase1Results, err := collector.Collect(ctx, allTargets, *duration)
	if err != nil && err != context.Canceled {
		log.Printf("Error: Phase 1 monitoring failed: %v", err)
		return
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
		log.Printf("Error: Failed to delete DaemonSet: %v", err)
		return
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
		log.Printf("Error: Phase 2 monitoring failed: %v", err)
		return
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

	// Note: Restoration happens in defer() at function start

	// ==================== GENERATE COMBINED REPORT ====================
	fmt.Println()
	fmt.Println("📝 Generating combined reports (60 minutes)...")

	// Raw metrics already saved during collection via streaming
	fmt.Printf("   ✅ Raw metrics saved to: %s\n", rawMetricsFile)

	// Generate user-selected format report
	switch *outputFormat {
	case "json":
		filename := fmt.Sprintf("%s/two-phase-monitoring-report-%s.json", reportDir, timestamp)
		if err := saveJSONReport(filename, combinedResults); err != nil {
			log.Fatalf("Failed to save JSON report: %v", err)
		}
		fmt.Printf("   ✅ JSON report saved to: %s\n", filename)

	case "html":
		filename := fmt.Sprintf("%s/two-phase-monitoring-report-%s.html", reportDir, timestamp)
		generator := report.NewHTMLGenerator()
		if err := generator.Generate(filename, combinedResults); err != nil {
			log.Fatalf("Failed to generate HTML report: %v", err)
		}
		fmt.Printf("   ✅ HTML report saved to: %s\n", filename)

	default: // "text" or any other format
		filename := fmt.Sprintf("%s/two-phase-monitoring-report-%s.txt", reportDir, timestamp)
		generator := report.NewTextGeneratorWithMode(true) // Two-phase mode enabled
		if err := generator.Generate(filename, combinedResults); err != nil {
			log.Fatalf("Failed to generate text report: %v", err)
		}
		fmt.Printf("   ✅ Text report saved to: %s\n", filename)
	}

	// ALWAYS generate charts (regardless of format)
	fmt.Println()
	fmt.Println("📈 Generating combined metric charts (60 minutes)...")
	chartGen := charts.NewChartGenerator(reportDir)
	if err := chartGen.GenerateCharts(combinedResults); err != nil {
		log.Printf("Warning: Failed to generate charts: %v", err)
	} else {
		fmt.Printf("   ✅ Charts saved to: %s/charts/\n", reportDir)
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
	fmt.Printf("   Raw Metrics (JSON):  reports/two-phase-raw-metrics-%s.json\n", timestamp)
	fmt.Printf("   Analysis Report:     reports/two-phase-monitoring-report-%s.txt\n", timestamp)
	fmt.Printf("   Charts (PNG):        reports/charts/ (full 60-minute timeline)\n")
	fmt.Println()
}

func deployAndVerify(ctx context.Context, deployer *deploy.Deployer, description string) error {
	fmt.Printf("📦 Deploying node-exporter-zoneinfo (%s)...\n", description)
	if err := deployer.Deploy(ctx); err != nil {
		return fmt.Errorf("deployment failed: %w", err)
	}
	fmt.Println("✅ Deployment completed successfully")
	fmt.Println()

	fmt.Println("⏳ Waiting for pods to be ready...")
	time.Sleep(15 * time.Second)

	fmt.Println("🔍 Verifying deployment status...")
	status, err := deployer.GetDeploymentStatus(ctx)
	if err != nil {
		log.Printf("Warning: Could not verify deployment: %v", err)
	} else {
		fmt.Printf("   Node Exporter Zoneinfo: %d/%d pods ready\n",
			status.NodeExporterReady, status.NodeExporterTotal)
		fmt.Printf("   Prometheus: %d/%d pods ready\n",
			status.PrometheusReady, status.PrometheusTotal)
		fmt.Println()
	}
	return nil
}

func cleanupResources(ctx context.Context, deployer *deploy.Deployer) {
	fmt.Println("🧹 Checking for existing resources...")
	if err := deployer.Undeploy(ctx); err != nil {
		log.Printf("⚠️  Warning: Cleanup encountered an issue: %v", err)
		log.Println("   This may be expected if resources don't exist yet")
	}
	time.Sleep(5 * time.Second)
}

func runMonitoringPhase(ctx context.Context, collector *metrics.Collector, targets []metrics.PodTarget, phaseName string) (*metrics.MonitoringResults, time.Duration, error) {
	fmt.Printf("📊 Starting %s monitoring for %v (interval: %v)\n", phaseName, *duration, *sampleInterval)
	fmt.Println()

	phaseStart := time.Now()
	results, err := collector.Collect(ctx, targets, *duration)
	if err != nil && err != context.Canceled {
		return nil, 0, fmt.Errorf("%s monitoring failed: %w", phaseName, err)
	}
	phaseDuration := time.Since(phaseStart)

	fmt.Println()
	fmt.Printf("✅ %s monitoring completed!\n", phaseName)
	fmt.Printf("   Duration: %v, Samples: %d\n", phaseDuration.Round(time.Second), len(results.Samples))
	fmt.Println()

	return results, phaseDuration, nil
}

func runThreePhaseMonitoring(ctx context.Context) {
	deployer := createDeployerWithVariant(deploy.VariantFull)

	// CRITICAL: Ensure kustomization.yaml is ALWAYS restored
	defer func() {
		fmt.Println()
		fmt.Println("🔄 Restoring original configuration...")
		if err := deployer.RestoreKustomization(); err != nil {
			log.Printf("Warning: Failed to restore kustomization.yaml: %v", err)
			log.Println("   Please manually restore: git checkout node-exporter-zoneinfo/kustomization.yaml")
		}
	}()

	fmt.Println("╔═══════════════════════════════════════════════════════════════╗")
	fmt.Println("║            THREE-PHASE MONITORING MODE                        ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// ==================== PHASE 1 ====================
	fmt.Println("┌───────────────────────────────────────────────────────────────┐")
	fmt.Println("│  PHASE 1: All Collectors (30 minutes)                        │")
	fmt.Println("│  node-exporter with: zoneinfo, interrupts, softirqs          │")
	fmt.Println("└───────────────────────────────────────────────────────────────┘")
	fmt.Println()

	cleanupResources(ctx, deployer)
	if err := deployAndVerify(ctx, deployer, "all collectors"); err != nil {
		log.Printf("Error: Phase 1 deployment failed: %v", err)
		return
	}

	// Define monitoring targets
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

	collector := metrics.NewCollector(*sampleInterval, *kubeconfig)

	// Create timestamped raw metrics JSON file for streaming (entire three-phase run)
	timestamp := time.Now().Format("20060102-150405")
	rawMetricsFile := fmt.Sprintf("%s/three-phase-raw-metrics-%s.json", reportDir, timestamp)
	jsonFile, err := os.Create(rawMetricsFile)
	if err != nil {
		log.Fatalf("Failed to create raw metrics JSON file: %v", err)
	}
	defer func() {
		if closeErr := jsonFile.Close(); closeErr != nil {
			log.Printf("Warning: Failed to close raw metrics file: %v", closeErr)
		}
	}()

	// Set the streaming file in collector (will capture ALL samples across all phases)
	collector.SetRawMetricsFile(jsonFile)

	// ==================== PHASE 1: Monitoring ====================
	phase1Results, phase1Duration, err := runMonitoringPhase(ctx, collector, allTargets, "Phase 1")
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}

	combinedResults.Samples = append(combinedResults.Samples, phase1Results.Samples...)
	combinedResults.Errors = append(combinedResults.Errors, phase1Results.Errors...)

	// ==================== PHASE 2 ====================
	fmt.Println()
	fmt.Println("┌───────────────────────────────────────────────────────────────┐")
	fmt.Println("│  PHASE 2: No Node-Exporter (30 minutes)                      │")
	fmt.Println("│  Prometheus Operator only                                    │")
	fmt.Println("└───────────────────────────────────────────────────────────────┘")
	fmt.Println()

	fmt.Println("🗑️  Removing all node-exporter-zoneinfo resources...")
	if err := deployer.Undeploy(ctx); err != nil {
		log.Printf("Error: Failed to undeploy: %v", err)
		return
	}
	fmt.Println("✅ Resources removed successfully")
	time.Sleep(10 * time.Second)
	fmt.Println()

	phase2Results, phase2Duration, err := runMonitoringPhase(ctx, collector, allTargets, "Phase 2")
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}

	combinedResults.Samples = append(combinedResults.Samples, phase2Results.Samples...)
	combinedResults.Errors = append(combinedResults.Errors, phase2Results.Errors...)

	// ==================== PHASE 3 ====================
	fmt.Println()
	fmt.Println("┌───────────────────────────────────────────────────────────────┐")
	fmt.Println("│  PHASE 3: Zoneinfo Only (30 minutes)                         │")
	fmt.Println("│  node-exporter with: --collector.zoneinfo ONLY               │")
	fmt.Println("└───────────────────────────────────────────────────────────────┘")
	fmt.Println()

	zoneinfoDeployer := createDeployerWithVariant(deploy.VariantZoneinfoOnly)
	if err := deployAndVerify(ctx, zoneinfoDeployer, "zoneinfo collector only"); err != nil {
		log.Printf("Error: Phase 3 deployment failed: %v", err)
		return
	}

	phase3Results, phase3Duration, err := runMonitoringPhase(ctx, collector, allTargets, "Phase 3")
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}

	combinedResults.Samples = append(combinedResults.Samples, phase3Results.Samples...)
	combinedResults.Errors = append(combinedResults.Errors, phase3Results.Errors...)

	// Note: Restoration happens in defer() at function start

	// ==================== GENERATE COMBINED REPORT ====================
	combinedResults.EndTime = time.Now()
	combinedResults.Duration = combinedResults.EndTime.Sub(combinedResults.StartTime)
	combinedResults.SampleCount = len(combinedResults.Samples)

	fmt.Println()
	fmt.Println("📝 Generating combined three-phase reports (90 minutes)...")

	// Raw metrics already saved during collection via streaming
	fmt.Printf("   ✅ Raw metrics saved to: %s\n", rawMetricsFile)

	// Generate user-selected format report
	switch *outputFormat {
	case "json":
		filename := fmt.Sprintf("%s/three-phase-monitoring-report-%s.json", reportDir, timestamp)
		if err := saveJSONReport(filename, combinedResults); err != nil {
			log.Fatalf("Failed to save JSON report: %v", err)
		}
		fmt.Printf("   ✅ JSON report saved to: %s\n", filename)

	case "html":
		filename := fmt.Sprintf("%s/three-phase-monitoring-report-%s.html", reportDir, timestamp)
		generator := report.NewHTMLGenerator()
		if err := generator.Generate(filename, combinedResults); err != nil {
			log.Fatalf("Failed to generate HTML report: %v", err)
		}
		fmt.Printf("   ✅ HTML report saved to: %s\n", filename)

	default: // "text" or any other format
		filename := fmt.Sprintf("%s/three-phase-monitoring-report-%s.txt", reportDir, timestamp)
		generator := report.NewTextGenerator()
		if err := generator.Generate(filename, combinedResults); err != nil {
			log.Fatalf("Failed to generate text report: %v", err)
		}
		fmt.Printf("   ✅ Text report saved to: %s\n", filename)
	}

	// ALWAYS generate charts (regardless of format)
	fmt.Println()
	fmt.Println("📈 Generating combined metric charts (90 minutes)...")
	chartGen := charts.NewChartGenerator(reportDir)
	if err := chartGen.GenerateCharts(combinedResults); err != nil {
		log.Printf("Warning: Failed to generate charts: %v", err)
	} else {
		fmt.Printf("   ✅ Charts saved to: %s/charts/\n", reportDir)
	}

	// Print summary
	fmt.Println()
	printSummary(combinedResults)

	// Final summary
	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════════════════════════╗")
	fmt.Println("║        THREE-PHASE MONITORING COMPLETE                        ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("📋 Results Summary:")
	fmt.Printf("   Total monitoring duration: %v\n", combinedResults.Duration.Round(time.Second))
	fmt.Printf("   Phase 1 duration: %v (all collectors)\n", phase1Duration.Round(time.Second))
	fmt.Printf("   Phase 2 duration: %v (no node-exporter)\n", phase2Duration.Round(time.Second))
	fmt.Printf("   Phase 3 duration: %v (zoneinfo only)\n", phase3Duration.Round(time.Second))
	fmt.Printf("   Total samples collected: %d\n", len(combinedResults.Samples))
	fmt.Printf("   Phase 1 samples: %d\n", len(phase1Results.Samples))
	fmt.Printf("   Phase 2 samples: %d\n", len(phase2Results.Samples))
	fmt.Printf("   Phase 3 samples: %d\n", len(phase3Results.Samples))
	fmt.Println()
	fmt.Println("📁 Output Files:")
	fmt.Printf("   Raw Metrics (JSON):  reports/three-phase-raw-metrics-%s.json\n", timestamp)
	fmt.Printf("   Analysis Report:     reports/three-phase-monitoring-report-%s.txt\n", timestamp)
	fmt.Printf("   Charts (PNG):        reports/charts/ (full 90-minute timeline)\n")
	fmt.Println()
}

func runSixPhaseMonitoring(ctx context.Context) {
	fmt.Println("╔═══════════════════════════════════════════════════════════════╗")
	fmt.Println("║              SIX-PHASE MONITORING MODE                        ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Create a deployer for restoration - ensure it's available even if phases fail
	restorationDeployer := createDeployerWithVariant(deploy.VariantFull)

	// CRITICAL: Ensure kustomization.yaml is ALWAYS restored, even on panic or interruption
	defer func() {
		fmt.Println()
		fmt.Println("🔄 Restoring original configuration...")
		if err := restorationDeployer.RestoreKustomization(); err != nil {
			log.Printf("Warning: Failed to restore kustomization.yaml: %v", err)
			log.Println("   Please manually restore: git checkout node-exporter-zoneinfo/kustomization.yaml")
		}
	}()

	// Define monitoring targets - same for all phases
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

	collector := metrics.NewCollector(*sampleInterval, *kubeconfig)

	// Create timestamped raw metrics JSON file for streaming (entire six-phase run)
	timestamp := time.Now().Format("20060102-150405")
	rawMetricsFile := fmt.Sprintf("%s/six-phase-raw-metrics-%s.json", reportDir, timestamp)
	jsonFile, err := os.Create(rawMetricsFile)
	if err != nil {
		log.Fatalf("Failed to create raw metrics JSON file: %v", err)
	}
	defer func() {
		if closeErr := jsonFile.Close(); closeErr != nil {
			log.Printf("Warning: Failed to close raw metrics file: %v", closeErr)
		}
	}()

	// Set the streaming file in collector (will capture ALL samples across all six phases)
	collector.SetRawMetricsFile(jsonFile)

	// Track all phase durations and sample counts
	phaseDurations := make([]time.Duration, 6)
	phaseSampleCounts := make([]int, 6)

	// ==================== PHASE 1: No Node-Exporter (30 min) ====================
	fmt.Println("┌───────────────────────────────────────────────────────────────┐")
	fmt.Println("│  PHASE 1: Prometheus Operator Only (30 minutes)              │")
	fmt.Println("│  No node-exporter deployed                                   │")
	fmt.Println("└───────────────────────────────────────────────────────────────┘")
	fmt.Println()

	// Ensure clean state - undeploy any existing resources
	deployer := createDeployerWithVariant(deploy.VariantFull)
	cleanupResources(ctx, deployer)

	phase1Results, phase1Duration, err := runMonitoringPhase(ctx, collector, allTargets, "Phase 1")
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}
	phaseDurations[0] = phase1Duration
	phaseSampleCounts[0] = len(phase1Results.Samples)

	combinedResults.Samples = append(combinedResults.Samples, phase1Results.Samples...)
	combinedResults.Errors = append(combinedResults.Errors, phase1Results.Errors...)

	// ==================== PHASE 2: All Collectors (30 min) ====================
	fmt.Println()
	fmt.Println("┌───────────────────────────────────────────────────────────────┐")
	fmt.Println("│  PHASE 2: All Collectors (30 minutes)                        │")
	fmt.Println("│  node-exporter with: zoneinfo, interrupts, softirqs          │")
	fmt.Println("└───────────────────────────────────────────────────────────────┘")
	fmt.Println()

	fullDeployer := createDeployerWithVariant(deploy.VariantFull)
	if err := deployAndVerify(ctx, fullDeployer, "all collectors"); err != nil {
		log.Printf("Error: Phase 2 deployment failed: %v", err)
		return
	}

	phase2Results, phase2Duration, err := runMonitoringPhase(ctx, collector, allTargets, "Phase 2")
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}
	phaseDurations[1] = phase2Duration
	phaseSampleCounts[1] = len(phase2Results.Samples)

	combinedResults.Samples = append(combinedResults.Samples, phase2Results.Samples...)
	combinedResults.Errors = append(combinedResults.Errors, phase2Results.Errors...)

	// ==================== PHASE 3: No Node-Exporter (30 min) ====================
	fmt.Println()
	fmt.Println("┌───────────────────────────────────────────────────────────────┐")
	fmt.Println("│  PHASE 3: Prometheus Operator Only (30 minutes)              │")
	fmt.Println("│  No node-exporter deployed                                   │")
	fmt.Println("└───────────────────────────────────────────────────────────────┘")
	fmt.Println()

	fmt.Println("🗑️  Removing all node-exporter-zoneinfo resources...")
	if err := fullDeployer.Undeploy(ctx); err != nil {
		log.Printf("Error: Failed to undeploy: %v", err)
		return
	}
	fmt.Println("✅ Resources removed successfully")
	time.Sleep(10 * time.Second)
	fmt.Println()

	phase3Results, phase3Duration, err := runMonitoringPhase(ctx, collector, allTargets, "Phase 3")
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}
	phaseDurations[2] = phase3Duration
	phaseSampleCounts[2] = len(phase3Results.Samples)

	combinedResults.Samples = append(combinedResults.Samples, phase3Results.Samples...)
	combinedResults.Errors = append(combinedResults.Errors, phase3Results.Errors...)

	// ==================== PHASE 4: Zoneinfo Only (30 min) ====================
	fmt.Println()
	fmt.Println("┌───────────────────────────────────────────────────────────────┐")
	fmt.Println("│  PHASE 4: Zoneinfo Collector Only (30 minutes)               │")
	fmt.Println("│  node-exporter with: --collector.zoneinfo ONLY               │")
	fmt.Println("└───────────────────────────────────────────────────────────────┘")
	fmt.Println()

	zoneinfoDeployer := createDeployerWithVariant(deploy.VariantZoneinfoOnly)
	if err := deployAndVerify(ctx, zoneinfoDeployer, "zoneinfo collector only"); err != nil {
		log.Printf("Error: Phase 4 deployment failed: %v", err)
		return
	}

	phase4Results, phase4Duration, err := runMonitoringPhase(ctx, collector, allTargets, "Phase 4")
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}
	phaseDurations[3] = phase4Duration
	phaseSampleCounts[3] = len(phase4Results.Samples)

	combinedResults.Samples = append(combinedResults.Samples, phase4Results.Samples...)
	combinedResults.Errors = append(combinedResults.Errors, phase4Results.Errors...)

	// ==================== PHASE 5: Interrupts Only (30 min) ====================
	fmt.Println()
	fmt.Println("┌───────────────────────────────────────────────────────────────┐")
	fmt.Println("│  PHASE 5: Interrupts Collector Only (30 minutes)             │")
	fmt.Println("│  node-exporter with: --collector.interrupts ONLY             │")
	fmt.Println("└───────────────────────────────────────────────────────────────┘")
	fmt.Println()

	// Clean up zoneinfo deployment first
	fmt.Println("🗑️  Removing zoneinfo-only deployment...")
	if err := zoneinfoDeployer.Undeploy(ctx); err != nil {
		log.Printf("Error: Failed to undeploy zoneinfo variant: %v", err)
		return
	}
	fmt.Println("✅ Resources removed successfully")
	time.Sleep(10 * time.Second)

	interruptsDeployer := createDeployerWithVariant(deploy.VariantInterruptsOnly)
	if err := deployAndVerify(ctx, interruptsDeployer, "interrupts collector only"); err != nil {
		log.Printf("Error: Phase 5 deployment failed: %v", err)
		return
	}

	phase5Results, phase5Duration, err := runMonitoringPhase(ctx, collector, allTargets, "Phase 5")
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}
	phaseDurations[4] = phase5Duration
	phaseSampleCounts[4] = len(phase5Results.Samples)

	combinedResults.Samples = append(combinedResults.Samples, phase5Results.Samples...)
	combinedResults.Errors = append(combinedResults.Errors, phase5Results.Errors...)

	// ==================== PHASE 6: Softirqs Only (30 min) ====================
	fmt.Println()
	fmt.Println("┌───────────────────────────────────────────────────────────────┐")
	fmt.Println("│  PHASE 6: Softirqs Collector Only (30 minutes)               │")
	fmt.Println("│  node-exporter with: --collector.softirqs ONLY               │")
	fmt.Println("└───────────────────────────────────────────────────────────────┘")
	fmt.Println()

	// Clean up interrupts deployment first
	fmt.Println("🗑️  Removing interrupts-only deployment...")
	if err := interruptsDeployer.Undeploy(ctx); err != nil {
		log.Printf("Error: Failed to undeploy interrupts variant: %v", err)
		return
	}
	fmt.Println("✅ Resources removed successfully")
	time.Sleep(10 * time.Second)

	softirqsDeployer := createDeployerWithVariant(deploy.VariantSoftirqsOnly)
	if err := deployAndVerify(ctx, softirqsDeployer, "softirqs collector only"); err != nil {
		log.Printf("Error: Phase 6 deployment failed: %v", err)
		return
	}

	phase6Results, phase6Duration, err := runMonitoringPhase(ctx, collector, allTargets, "Phase 6")
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}
	phaseDurations[5] = phase6Duration
	phaseSampleCounts[5] = len(phase6Results.Samples)

	combinedResults.Samples = append(combinedResults.Samples, phase6Results.Samples...)
	combinedResults.Errors = append(combinedResults.Errors, phase6Results.Errors...)

	// Note: Restoration happens in defer() at function start - no explicit call needed here

	// ==================== GENERATE COMBINED REPORT ====================
	combinedResults.EndTime = time.Now()
	combinedResults.Duration = combinedResults.EndTime.Sub(combinedResults.StartTime)
	combinedResults.SampleCount = len(combinedResults.Samples)

	fmt.Println()
	fmt.Println("📝 Generating combined six-phase reports (180 minutes)...")

	// Raw metrics already saved during collection via streaming
	fmt.Printf("   ✅ Raw metrics saved to: %s\n", rawMetricsFile)

	// Generate user-selected format report
	switch *outputFormat {
	case "json":
		filename := fmt.Sprintf("%s/six-phase-monitoring-report-%s.json", reportDir, timestamp)
		if err := saveJSONReport(filename, combinedResults); err != nil {
			log.Fatalf("Failed to save JSON report: %v", err)
		}
		fmt.Printf("   ✅ JSON report saved to: %s\n", filename)

	case "html":
		filename := fmt.Sprintf("%s/six-phase-monitoring-report-%s.html", reportDir, timestamp)
		generator := report.NewHTMLGenerator()
		if err := generator.Generate(filename, combinedResults); err != nil {
			log.Fatalf("Failed to generate HTML report: %v", err)
		}
		fmt.Printf("   ✅ HTML report saved to: %s\n", filename)

	default: // "text" or any other format
		filename := fmt.Sprintf("%s/six-phase-monitoring-report-%s.txt", reportDir, timestamp)
		generator := report.NewTextGenerator()
		if err := generator.Generate(filename, combinedResults); err != nil {
			log.Fatalf("Failed to generate text report: %v", err)
		}
		fmt.Printf("   ✅ Text report saved to: %s\n", filename)
	}

	// ALWAYS generate charts (regardless of format)
	fmt.Println()
	fmt.Println("📈 Generating combined metric charts (180 minutes)...")
	chartGen := charts.NewChartGenerator(reportDir)
	if err := chartGen.GenerateCharts(combinedResults); err != nil {
		log.Printf("Warning: Failed to generate charts: %v", err)
	} else {
		fmt.Printf("   ✅ Charts saved to: %s/charts/\n", reportDir)
	}

	// Print summary
	fmt.Println()
	printSummary(combinedResults)

	// Final summary
	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════════════════════════╗")
	fmt.Println("║           SIX-PHASE MONITORING COMPLETE                       ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("📋 Results Summary:")
	fmt.Printf("   Total monitoring duration: %v\n", combinedResults.Duration.Round(time.Second))
	fmt.Printf("   Phase 1 duration: %v (no node-exporter)\n", phaseDurations[0].Round(time.Second))
	fmt.Printf("   Phase 2 duration: %v (all collectors)\n", phaseDurations[1].Round(time.Second))
	fmt.Printf("   Phase 3 duration: %v (no node-exporter)\n", phaseDurations[2].Round(time.Second))
	fmt.Printf("   Phase 4 duration: %v (zoneinfo only)\n", phaseDurations[3].Round(time.Second))
	fmt.Printf("   Phase 5 duration: %v (interrupts only)\n", phaseDurations[4].Round(time.Second))
	fmt.Printf("   Phase 6 duration: %v (softirqs only)\n", phaseDurations[5].Round(time.Second))
	fmt.Printf("   Total samples collected: %d\n", len(combinedResults.Samples))
	fmt.Printf("   Phase 1 samples: %d\n", phaseSampleCounts[0])
	fmt.Printf("   Phase 2 samples: %d\n", phaseSampleCounts[1])
	fmt.Printf("   Phase 3 samples: %d\n", phaseSampleCounts[2])
	fmt.Printf("   Phase 4 samples: %d\n", phaseSampleCounts[3])
	fmt.Printf("   Phase 5 samples: %d\n", phaseSampleCounts[4])
	fmt.Printf("   Phase 6 samples: %d\n", phaseSampleCounts[5])
	fmt.Println()
	fmt.Println("📁 Output Files:")
	fmt.Printf("   Raw Metrics (JSON):  reports/six-phase-raw-metrics-%s.json\n", timestamp)
	fmt.Printf("   Analysis Report:     reports/six-phase-monitoring-report-%s.txt\n", timestamp)
	fmt.Printf("   Charts (PNG):        reports/charts/ (full 180-minute timeline)\n")
	fmt.Println()
}

func runSinglePhaseMonitoring(ctx context.Context) {
	// Deploy if requested
	if *doDeploy {
		fmt.Println("📦 Deploying node-exporter-zoneinfo...")
		deployer := createDeployer()
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
		deployer := createDeployer()
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
	// Initialize metrics collector with real-time JSON streaming
	collector := metrics.NewCollector(*sampleInterval, *kubeconfig)

	// Start monitoring
	startTime := time.Now()

	// Create timestamped raw metrics JSON file for streaming
	timestamp := time.Now().Format("20060102-150405")
	prefix := ""
	if phasePrefix != "" {
		prefix = phasePrefix + "-"
	}

	// Initialize real-time raw metrics JSON file
	rawMetricsFile := fmt.Sprintf("%s/%sraw-metrics-%s.json", reportDir, prefix, timestamp)
	jsonFile, err := os.Create(rawMetricsFile)
	if err != nil {
		log.Fatalf("Failed to create raw metrics JSON file: %v", err)
	}
	defer func() {
		if closeErr := jsonFile.Close(); closeErr != nil {
			log.Printf("Warning: Failed to close raw metrics file: %v", closeErr)
		}
	}()

	// Set the streaming file in collector
	collector.SetRawMetricsFile(jsonFile)

	results, err := collector.Collect(ctx, targets, duration)
	if err != nil && err != context.Canceled {
		log.Printf("Error: Monitoring failed: %v", err)
		return nil
	}
	actualDuration := time.Since(startTime)

	fmt.Println()
	fmt.Println("✅ Monitoring completed!")
	fmt.Printf("   Duration: %v (planned: %v)\n", actualDuration.Round(time.Second), duration)
	fmt.Printf("   Samples collected: %d\n", len(results.Samples))
	fmt.Println()

	// Generate report
	fmt.Println("📝 Generating reports...")

	// Raw metrics already saved during collection via streaming
	fmt.Printf("   ✅ Raw metrics saved to: %s\n", rawMetricsFile)

	// Generate user-selected format report
	switch *outputFormat {
	case "json":
		filename := fmt.Sprintf("%s/%smonitoring-report-%s.json", reportDir, prefix, timestamp)
		if err := saveJSONReport(filename, results); err != nil {
			log.Fatalf("Failed to save JSON report: %v", err)
		}
		fmt.Printf("   ✅ JSON report saved to: %s\n", filename)

	case "html":
		filename := fmt.Sprintf("%s/%smonitoring-report-%s.html", reportDir, prefix, timestamp)
		generator := report.NewHTMLGenerator()
		if err := generator.Generate(filename, results); err != nil {
			log.Fatalf("Failed to generate HTML report: %v", err)
		}
		fmt.Printf("   ✅ HTML report saved to: %s\n", filename)

	default: // "text" or any other format
		filename := fmt.Sprintf("%s/%smonitoring-report-%s.txt", reportDir, prefix, timestamp)
		generator := report.NewTextGenerator()
		if err := generator.Generate(filename, results); err != nil {
			log.Fatalf("Failed to generate text report: %v", err)
		}
		fmt.Printf("   ✅ Text report saved to: %s\n", filename)
	}

	// ALWAYS generate charts (regardless of format)
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
		fmt.Printf("   ✅ Charts saved to: %s/charts/\n", chartDir)
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
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

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
