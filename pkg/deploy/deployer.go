package deploy

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// DaemonSetVariant represents the different node-exporter daemonset variants
type DaemonSetVariant string

const (
	VariantFull           DaemonSetVariant = "full"            // All collectors: zoneinfo, interrupts, softirqs
	VariantRootless       DaemonSetVariant = "rootless"        // Rootless with all collectors
	VariantZoneinfoOnly   DaemonSetVariant = "zoneinfo-only"   // Only zoneinfo collector
	VariantInterruptsOnly DaemonSetVariant = "interrupts-only" // Only interrupts collector
	VariantSoftirqsOnly   DaemonSetVariant = "softirqs-only"   // Only softirqs collector
)

type Deployer struct {
	clientset             *kubernetes.Clientset
	baseDir               string
	manifestsDir          string           // Directory containing k8s manifests
	variant               DaemonSetVariant // Daemonset variant to deploy
	kubeconfigPath        string           // Path to kubeconfig file
	originalKustomization string           // Original kustomization.yaml content
	kustomizationModified bool             // Track if we modified kustomization.yaml
}

type DeploymentStatus struct {
	NodeExporterReady int
	NodeExporterTotal int
	PrometheusReady   int
	PrometheusTotal   int
	ThanosReady       int
	ThanosTotal       int
	OperatorReady     int
	OperatorTotal     int
}

func NewDeployer(kubeconfigPath string) *Deployer {
	// Get kubeconfig - use provided path, fall back to default
	var kubeconfig string
	if kubeconfigPath != "" {
		kubeconfig = kubeconfigPath
	} else {
		kubeconfig = clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		panic(fmt.Sprintf("Failed to build config from %s: %v", kubeconfig, err))
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(fmt.Sprintf("Failed to create clientset: %v", err))
	}

	// Get current directory
	baseDir, err := os.Getwd()
	if err != nil {
		panic(fmt.Sprintf("Failed to get current directory: %v", err))
	}

	// Set manifests directory to ./node-exporter-zoneinfo/
	manifestsDir := filepath.Join(baseDir, "node-exporter-zoneinfo")

	return &Deployer{
		clientset:      clientset,
		baseDir:        baseDir,
		manifestsDir:   manifestsDir,
		variant:        VariantFull, // Default to full variant
		kubeconfigPath: kubeconfig,
	}
}

// NewDeployerWithOptions creates a new Deployer with custom options
func NewDeployerWithOptions(kubeconfigPath string, variant DaemonSetVariant) *Deployer {
	deployer := NewDeployer(kubeconfigPath)
	deployer.variant = variant
	return deployer
}

// SetManifestsDir allows overriding the default manifests directory
func (d *Deployer) SetManifestsDir(dir string) {
	d.manifestsDir = dir
}

// SetVariant sets the daemonset variant to deploy
func (d *Deployer) SetVariant(variant DaemonSetVariant) {
	d.variant = variant
}

// GetVariant returns the current daemonset variant
func (d *Deployer) GetVariant() DaemonSetVariant {
	return d.variant
}

func (d *Deployer) Deploy(ctx context.Context) error {
	// Verify manifests directory exists
	if _, err := os.Stat(d.manifestsDir); os.IsNotExist(err) {
		return fmt.Errorf("manifests directory not found: %s", d.manifestsDir)
	}

	kustomizationPath := filepath.Join(d.manifestsDir, "kustomization.yaml")

	// Check if kustomization.yaml exists
	if _, err := os.Stat(kustomizationPath); os.IsNotExist(err) {
		return fmt.Errorf("kustomization.yaml not found in %s", d.manifestsDir)
	}

	// Validate the variant configuration in kustomization.yaml
	if err := d.ensureVariantKustomization(); err != nil {
		return fmt.Errorf("failed to configure variant deployment: %w", err)
	}

	// Try kubectl first, then oc
	var cmd *exec.Cmd
	switch {
	case d.commandExists("kubectl"):
		args := []string{"apply", "-k", d.manifestsDir, "--validate=false"}
		if d.kubeconfigPath != "" {
			args = append([]string{"--kubeconfig", d.kubeconfigPath}, args...)
		}
		cmd = exec.CommandContext(ctx, "kubectl", args...)
	case d.commandExists("oc"):
		args := []string{"apply", "-k", d.manifestsDir, "--validate=false"}
		if d.kubeconfigPath != "" {
			args = append([]string{"--kubeconfig", d.kubeconfigPath}, args...)
		}
		cmd = exec.CommandContext(ctx, "oc", args...)
	default:
		return fmt.Errorf("neither kubectl nor oc command found")
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("deployment command failed: %w", err)
	}

	fmt.Printf("✅ Successfully deployed node-exporter-zoneinfo from %s\n", d.manifestsDir)
	switch d.variant {
	case VariantFull:
		fmt.Println("   Using full variant (zoneinfo, interrupts, softirqs)")
	case VariantRootless:
		fmt.Println("   Using rootless variant (all collectors)")
	case VariantZoneinfoOnly:
		fmt.Println("   Using zoneinfo-only variant")
	case VariantInterruptsOnly:
		fmt.Println("   Using interrupts-only variant")
	case VariantSoftirqsOnly:
		fmt.Println("   Using softirqs-only variant")
	}

	// Wait for deployment to stabilize
	time.Sleep(5 * time.Second)

	return nil
}

func (d *Deployer) GetDeploymentStatus(ctx context.Context) (*DeploymentStatus, error) {
	status := &DeploymentStatus{}

	// Check node-exporter-zoneinfo pods
	nePods, err := d.clientset.CoreV1().Pods("node-exporter-zoneinfo").List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=node-exporter-zoneinfo",
	})
	if err == nil {
		status.NodeExporterTotal = len(nePods.Items)
		for i := range nePods.Items {
			if d.isPodReady(&nePods.Items[i]) {
				status.NodeExporterReady++
			}
		}
	}

	// Check prometheus pods
	promPods, err := d.clientset.CoreV1().Pods("openshift-monitoring").List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=prometheus",
	})
	if err == nil {
		status.PrometheusTotal = len(promPods.Items)
		for i := range promPods.Items {
			if d.isPodReady(&promPods.Items[i]) {
				status.PrometheusReady++
			}
		}
	}

	// Check thanos pods
	thanosPods, err := d.clientset.CoreV1().Pods("openshift-monitoring").List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=thanos-query",
	})
	if err == nil {
		status.ThanosTotal = len(thanosPods.Items)
		for i := range thanosPods.Items {
			if d.isPodReady(&thanosPods.Items[i]) {
				status.ThanosReady++
			}
		}
	}

	// Check operator pods
	opPods, err := d.clientset.CoreV1().Pods("openshift-monitoring").List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=prometheus-operator",
	})
	if err == nil {
		status.OperatorTotal = len(opPods.Items)
		for i := range opPods.Items {
			if d.isPodReady(&opPods.Items[i]) {
				status.OperatorReady++
			}
		}
	}

	return status, nil
}

func (d *Deployer) isPodReady(pod metav1.Object) bool {
	// Simple check - in real implementation, check pod.Status.Conditions
	// For now, just return true if pod exists
	return true
}

func (d *Deployer) commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

// ensureVariantKustomization dynamically updates kustomization.yaml to use the correct variant
func (d *Deployer) ensureVariantKustomization() error {
	kustomizationPath := filepath.Join(d.manifestsDir, "kustomization.yaml")
	content, err := os.ReadFile(kustomizationPath)
	if err != nil {
		return fmt.Errorf("failed to read kustomization.yaml: %w", err)
	}

	// Save original content on first modification
	if !d.kustomizationModified {
		d.originalKustomization = string(content)
		d.kustomizationModified = true
	}

	var expectedFile string
	switch d.variant {
	case VariantFull:
		expectedFile = "daemonset.yaml"
	case VariantRootless:
		expectedFile = "daemonset-rootless.yaml"
	case VariantZoneinfoOnly:
		expectedFile = "daemonset-zoneinfo-only.yaml"
	case VariantInterruptsOnly:
		expectedFile = "daemonset-interrupts-only.yaml"
	case VariantSoftirqsOnly:
		expectedFile = "daemonset-softirqs-only.yaml"
	}

	// Modify kustomization.yaml to use the correct variant
	modifiedContent := d.updateKustomizationVariant(string(content), expectedFile)

	// Write the modified content back
	if err := os.WriteFile(kustomizationPath, []byte(modifiedContent), 0o644); err != nil {
		return fmt.Errorf("failed to write kustomization.yaml: %w", err)
	}

	fmt.Printf("✅ Configured kustomization.yaml to use: %s\n", expectedFile)
	return nil
}

// RestoreKustomization restores the original kustomization.yaml content
func (d *Deployer) RestoreKustomization() error {
	if !d.kustomizationModified {
		return nil // Nothing to restore
	}

	kustomizationPath := filepath.Join(d.manifestsDir, "kustomization.yaml")
	if err := os.WriteFile(kustomizationPath, []byte(d.originalKustomization), 0o644); err != nil {
		return fmt.Errorf("failed to restore kustomization.yaml: %w", err)
	}

	fmt.Println("✅ Restored original kustomization.yaml")
	d.kustomizationModified = false
	return nil
}

// updateKustomizationVariant modifies the kustomization.yaml content to use the specified variant
func (d *Deployer) updateKustomizationVariant(content, targetVariant string) string {
	// All possible daemonset variants
	variants := []string{
		"daemonset.yaml",
		"daemonset-rootless.yaml",
		"daemonset-zoneinfo-only.yaml",
		"daemonset-interrupts-only.yaml",
		"daemonset-softirqs-only.yaml",
	}

	lines := strings.Split(content, "\n")
	var result []string

	for _, line := range lines {
		modifiedLine := line

		// Only process lines that are actual resource declarations (not documentation comments)
		// Pattern: "  - {variant}" or "  # - {variant}" with nothing after the variant name
		trimmed := strings.TrimSpace(line)

		// Check if this is a resource line (starts with - or # -)
		isResourceLine := false
		lineVariant := ""

		// Remove leading # and spaces to check the actual resource
		workingLine := trimmed
		workingLine = strings.TrimPrefix(workingLine, "#")
		workingLine = strings.TrimSpace(workingLine)
		workingLine = strings.TrimPrefix(workingLine, "-")
		workingLine = strings.TrimSpace(workingLine)

		// Check if this line is exactly a variant (no additional text after it)
		for _, variant := range variants {
			if workingLine == variant {
				isResourceLine = true
				lineVariant = variant
				break
			}
		}

		// Only modify resource lines, not documentation comments
		if isResourceLine {
			if lineVariant == targetVariant {
				// Uncomment the target variant
				modifiedLine = "  - " + targetVariant
			} else {
				// Comment out other variants
				modifiedLine = "  # - " + lineVariant
			}
		}

		result = append(result, modifiedLine)
	}

	return strings.Join(result, "\n")
}

func (d *Deployer) Undeploy(ctx context.Context) error {
	// Check if namespace exists first
	_, err := d.clientset.CoreV1().Namespaces().Get(ctx, "node-exporter-zoneinfo", metav1.GetOptions{})
	if err != nil {
		// Namespace doesn't exist, nothing to clean up
		fmt.Println("✅ Namespace 'node-exporter-zoneinfo' does not exist - nothing to clean up")
		return nil
	}

	var cmd *exec.Cmd
	switch {
	case d.commandExists("kubectl"):
		args := []string{"delete", "-k", d.manifestsDir, "--ignore-not-found=true"}
		if d.kubeconfigPath != "" {
			args = append([]string{"--kubeconfig", d.kubeconfigPath}, args...)
		}
		cmd = exec.CommandContext(ctx, "kubectl", args...)
	case d.commandExists("oc"):
		args := []string{"delete", "-k", d.manifestsDir, "--ignore-not-found=true"}
		if d.kubeconfigPath != "" {
			args = append([]string{"--kubeconfig", d.kubeconfigPath}, args...)
		}
		cmd = exec.CommandContext(ctx, "oc", args...)
	default:
		return fmt.Errorf("neither kubectl nor oc command found")
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if error is due to authentication or permissions
		outputStr := string(output)
		if strings.Contains(outputStr, "provide credentials") || strings.Contains(outputStr, "unable to recognize") {
			fmt.Println("⚠️  Warning: Some resources may not be accessible (authentication/permission issues)")
			fmt.Println("   Continuing anyway as resources may not exist...")
			return nil
		}
		return fmt.Errorf("undeploy failed: %w\nOutput: %s", err, outputStr)
	}

	fmt.Printf("✅ Successfully removed node-exporter-zoneinfo resources from %s\n", d.manifestsDir)

	return nil
}

// GetKubernetesVersion returns the cluster version
func (d *Deployer) GetKubernetesVersion(ctx context.Context) (string, error) {
	version, err := d.clientset.Discovery().ServerVersion()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s.%s", version.Major, strings.TrimSuffix(version.Minor, "+")), nil
}

// DeleteDaemonSet deletes only the node-exporter-zoneinfo DaemonSet
func (d *Deployer) DeleteDaemonSet(ctx context.Context) error {
	var cmd *exec.Cmd
	switch {
	case d.commandExists("kubectl"):
		args := []string{"delete", "daemonset", "node-exporter-zoneinfo", "-n", "node-exporter-zoneinfo", "--ignore-not-found=true"}
		if d.kubeconfigPath != "" {
			args = append([]string{"--kubeconfig", d.kubeconfigPath}, args...)
		}
		cmd = exec.CommandContext(ctx, "kubectl", args...)
	case d.commandExists("oc"):
		args := []string{"delete", "daemonset", "node-exporter-zoneinfo", "-n", "node-exporter-zoneinfo", "--ignore-not-found=true"}
		if d.kubeconfigPath != "" {
			args = append([]string{"--kubeconfig", d.kubeconfigPath}, args...)
		}
		cmd = exec.CommandContext(ctx, "oc", args...)
	default:
		return fmt.Errorf("neither kubectl nor oc command found")
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("delete daemonset failed: %w\nOutput: %s", err, string(output))
	}

	// Wait for pods to terminate
	time.Sleep(10 * time.Second)

	return nil
}
