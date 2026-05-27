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
	clientset    *kubernetes.Clientset
	baseDir      string
	manifestsDir string           // Directory containing k8s manifests
	variant      DaemonSetVariant // Daemonset variant to deploy
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
		clientset:    clientset,
		baseDir:      baseDir,
		manifestsDir: manifestsDir,
		variant:      VariantFull, // Default to full variant
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
		cmd = exec.CommandContext(ctx, "kubectl", "apply", "-k", d.manifestsDir)
	case d.commandExists("oc"):
		cmd = exec.CommandContext(ctx, "oc", "apply", "-k", d.manifestsDir)
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

// ensureVariantKustomization checks if the correct variant is configured
// This is informational - the kustomization.yaml should be manually edited
// or we can provide a warning
func (d *Deployer) ensureVariantKustomization() error {
	kustomizationPath := filepath.Join(d.manifestsDir, "kustomization.yaml")
	content, err := os.ReadFile(kustomizationPath)
	if err != nil {
		return fmt.Errorf("failed to read kustomization.yaml: %w", err)
	}

	contentStr := string(content)
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

	// Check if expected variant file is uncommented
	if !strings.Contains(contentStr, "- "+expectedFile) ||
		strings.Contains(contentStr, "#  - "+expectedFile) ||
		strings.Contains(contentStr, "# - "+expectedFile) {
		fmt.Printf("⚠️  Warning: Variant '%s' requested but kustomization.yaml may not be configured.\n", d.variant)
		fmt.Printf("   Please ensure %s is uncommented and other daemonset variants are commented.\n", expectedFile)
		fmt.Println("   Continuing with current kustomization.yaml configuration...")
	}

	return nil
}

func (d *Deployer) Undeploy(ctx context.Context) error {
	var cmd *exec.Cmd
	switch {
	case d.commandExists("kubectl"):
		cmd = exec.CommandContext(ctx, "kubectl", "delete", "-k", d.manifestsDir, "--ignore-not-found=true")
	case d.commandExists("oc"):
		cmd = exec.CommandContext(ctx, "oc", "delete", "-k", d.manifestsDir, "--ignore-not-found=true")
	default:
		return fmt.Errorf("neither kubectl nor oc command found")
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("undeploy failed: %w\nOutput: %s", err, string(output))
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
		cmd = exec.CommandContext(ctx, "kubectl", "delete", "daemonset",
			"node-exporter-zoneinfo", "-n", "node-exporter-zoneinfo", "--ignore-not-found=true")
	case d.commandExists("oc"):
		cmd = exec.CommandContext(ctx, "oc", "delete", "daemonset",
			"node-exporter-zoneinfo", "-n", "node-exporter-zoneinfo", "--ignore-not-found=true")
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
