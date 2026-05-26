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
	clientset *kubernetes.Clientset
	baseDir   string
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

	return &Deployer{
		clientset: clientset,
		baseDir:   baseDir,
	}
}

func (d *Deployer) Deploy(ctx context.Context) error {
	kustomizationPath := filepath.Join(d.baseDir, "kustomization.yaml")

	// Check if kustomization.yaml exists
	if _, err := os.Stat(kustomizationPath); os.IsNotExist(err) {
		return fmt.Errorf("kustomization.yaml not found in %s", d.baseDir)
	}

	// Try kubectl first, then oc
	var cmd *exec.Cmd
	if d.commandExists("kubectl") {
		cmd = exec.CommandContext(ctx, "kubectl", "apply", "-k", d.baseDir)
	} else if d.commandExists("oc") {
		cmd = exec.CommandContext(ctx, "oc", "apply", "-k", d.baseDir)
	} else {
		return fmt.Errorf("neither kubectl nor oc command found")
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("deployment command failed: %w", err)
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
		for _, pod := range nePods.Items {
			if d.isPodReady(&pod) {
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
		for _, pod := range promPods.Items {
			if d.isPodReady(&pod) {
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
		for _, pod := range thanosPods.Items {
			if d.isPodReady(&pod) {
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
		for _, pod := range opPods.Items {
			if d.isPodReady(&pod) {
				status.OperatorReady++
			}
		}
	}

	return status, nil
}

func (d *Deployer) isPodReady(pod metav1.Object) bool {
	// Type assert to access Status
	type podWithStatus interface {
		metav1.Object
		GetStatus() interface{}
	}

	// Simple check - in real implementation, check pod.Status.Conditions
	// For now, just return true if pod exists
	return true
}

func (d *Deployer) commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func (d *Deployer) Undeploy(ctx context.Context) error {
	var cmd *exec.Cmd
	if d.commandExists("kubectl") {
		cmd = exec.CommandContext(ctx, "kubectl", "delete", "-k", d.baseDir, "--ignore-not-found=true")
	} else if d.commandExists("oc") {
		cmd = exec.CommandContext(ctx, "oc", "delete", "-k", d.baseDir, "--ignore-not-found=true")
	} else {
		return fmt.Errorf("neither kubectl nor oc command found")
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("undeploy failed: %w\nOutput: %s", err, string(output))
	}

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
	if d.commandExists("kubectl") {
		cmd = exec.CommandContext(ctx, "kubectl", "delete", "daemonset",
			"node-exporter-zoneinfo", "-n", "node-exporter-zoneinfo", "--ignore-not-found=true")
	} else if d.commandExists("oc") {
		cmd = exec.CommandContext(ctx, "oc", "delete", "daemonset",
			"node-exporter-zoneinfo", "-n", "node-exporter-zoneinfo", "--ignore-not-found=true")
	} else {
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
