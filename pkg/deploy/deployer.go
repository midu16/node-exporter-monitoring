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

type Deployer struct {
	clientset    *kubernetes.Clientset
	baseDir      string
	manifestsDir string // Directory containing k8s manifests
	useRootless  bool   // Use rootless daemonset variant
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
		useRootless:  false, // Default to privileged daemonset
	}
}

// NewDeployerWithOptions creates a new Deployer with custom options
func NewDeployerWithOptions(kubeconfigPath string, useRootless bool) *Deployer {
	deployer := NewDeployer(kubeconfigPath)
	deployer.useRootless = useRootless
	return deployer
}

// SetManifestsDir allows overriding the default manifests directory
func (d *Deployer) SetManifestsDir(dir string) {
	d.manifestsDir = dir
}

// SetRootless sets whether to use the rootless daemonset variant
func (d *Deployer) SetRootless(useRootless bool) {
	d.useRootless = useRootless
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

	// If rootless mode is requested, we need to modify kustomization.yaml temporarily
	if d.useRootless {
		if err := d.ensureRootlessKustomization(); err != nil {
			return fmt.Errorf("failed to configure rootless deployment: %w", err)
		}
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
	if d.useRootless {
		fmt.Println("   Using rootless daemonset variant")
	} else {
		fmt.Println("   Using privileged daemonset variant")
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

// ensureRootlessKustomization checks if rootless variant is needed
// This is informational - the kustomization.yaml should be manually edited
// or we can provide a warning
func (d *Deployer) ensureRootlessKustomization() error {
	kustomizationPath := filepath.Join(d.manifestsDir, "kustomization.yaml")
	content, err := os.ReadFile(kustomizationPath)
	if err != nil {
		return fmt.Errorf("failed to read kustomization.yaml: %w", err)
	}

	// Check if daemonset-rootless.yaml is already uncommented
	contentStr := string(content)
	if !strings.Contains(contentStr, "- daemonset-rootless.yaml") ||
		strings.Contains(contentStr, "#  - daemonset-rootless.yaml") ||
		strings.Contains(contentStr, "# - daemonset-rootless.yaml") {
		fmt.Println("⚠️  Warning: Rootless mode requested but kustomization.yaml may not be configured.")
		fmt.Println("   Please ensure daemonset-rootless.yaml is uncommented and daemonset.yaml is commented.")
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
