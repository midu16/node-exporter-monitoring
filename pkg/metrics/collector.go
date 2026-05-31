package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
)

type PodTarget struct {
	Namespace     string
	PodName       string // Empty if using LabelSelector
	LabelSelector string // Empty if using PodName
}

type ResourceSample struct {
	Timestamp     time.Time
	Namespace     string
	PodName       string
	NodeName      string // Added: node where pod is running
	ContainerName string
	CPUUsage      int64 // millicores
	MemoryUsage   int64 // bytes
}

type MonitoringResults struct {
	StartTime   time.Time
	EndTime     time.Time
	Duration    time.Duration
	SampleCount int
	Samples     []ResourceSample
	Targets     []PodTarget
	Errors      []string
}

type Collector struct {
	clientset       *kubernetes.Clientset
	metricsClient   *metricsclient.Clientset
	sampleInterval  time.Duration
	rawMetricsFile  *os.File
	jsonEncoder     *json.Encoder
	encoderMutex    sync.Mutex
	streamingActive bool
}

func NewCollector(sampleInterval time.Duration, kubeconfigPath string) *Collector {
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

	metricsClient, err := metricsclient.NewForConfig(config)
	if err != nil {
		panic(fmt.Sprintf("Failed to create metrics client: %v", err))
	}

	return &Collector{
		clientset:       clientset,
		metricsClient:   metricsClient,
		sampleInterval:  sampleInterval,
		streamingActive: false,
	}
}

// SetRawMetricsFile enables real-time streaming of raw metrics to a JSON file
func (c *Collector) SetRawMetricsFile(file *os.File) {
	c.rawMetricsFile = file
	c.jsonEncoder = json.NewEncoder(file)
	c.jsonEncoder.SetIndent("", "  ")
	c.streamingActive = true

	// Write opening brace and metadata
	c.encoderMutex.Lock()
	defer c.encoderMutex.Unlock()

	fmt.Fprintf(c.rawMetricsFile, "{\n")
	fmt.Fprintf(c.rawMetricsFile, "  \"monitoring_session\": {\n")
	fmt.Fprintf(c.rawMetricsFile, "    \"start_time\": %q,\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(c.rawMetricsFile, "    \"sample_interval_seconds\": %d\n", int(c.sampleInterval.Seconds()))
	fmt.Fprintf(c.rawMetricsFile, "  },\n")
	fmt.Fprintf(c.rawMetricsFile, "  \"samples\": [\n")
}

// streamSample writes a single sample to the JSON file in real-time
func (c *Collector) streamSample(sample ResourceSample) {
	if !c.streamingActive || c.rawMetricsFile == nil {
		return
	}

	c.encoderMutex.Lock()
	defer c.encoderMutex.Unlock()

	// Create a formatted JSON entry
	sampleJSON, err := json.MarshalIndent(sample, "    ", "  ")
	if err != nil {
		return
	}

	// Write with comma (we'll handle the last one during finalization)
	fmt.Fprintf(c.rawMetricsFile, "    %s,\n", sampleJSON)
}

// finalizeRawMetricsFile closes the JSON structure properly
func (c *Collector) finalizeRawMetricsFile(results *MonitoringResults) {
	if !c.streamingActive || c.rawMetricsFile == nil {
		return
	}

	c.encoderMutex.Lock()
	defer c.encoderMutex.Unlock()

	// Remove trailing comma by seeking back (simplified approach: write summary)
	fmt.Fprintf(c.rawMetricsFile, "    null\n")
	fmt.Fprintf(c.rawMetricsFile, "  ],\n")
	fmt.Fprintf(c.rawMetricsFile, "  \"summary\": {\n")
	fmt.Fprintf(c.rawMetricsFile, "    \"end_time\": %q,\n", results.EndTime.Format(time.RFC3339))
	fmt.Fprintf(c.rawMetricsFile, "    \"duration_seconds\": %.2f,\n", results.Duration.Seconds())
	fmt.Fprintf(c.rawMetricsFile, "    \"total_samples\": %d,\n", results.SampleCount)
	fmt.Fprintf(c.rawMetricsFile, "    \"error_count\": %d\n", len(results.Errors))
	fmt.Fprintf(c.rawMetricsFile, "  }\n")
	fmt.Fprintf(c.rawMetricsFile, "}\n")

	c.rawMetricsFile.Sync() // Ensure everything is written to disk
}

func (c *Collector) Collect(ctx context.Context, targets []PodTarget, duration time.Duration) (*MonitoringResults, error) {
	results := &MonitoringResults{
		StartTime: time.Now(),
		Targets:   targets,
		Samples:   make([]ResourceSample, 0),
		Errors:    make([]string, 0),
	}

	ticker := time.NewTicker(c.sampleInterval)
	defer ticker.Stop()

	timeout := time.After(duration)
	sampleCount := 0

	fmt.Printf("⏱️  Sample 0/%d (estimating...)\r", int(duration/c.sampleInterval))

	for {
		select {
		case <-ctx.Done():
			results.EndTime = time.Now()
			results.Duration = results.EndTime.Sub(results.StartTime)
			results.SampleCount = sampleCount
			c.finalizeRawMetricsFile(results)
			return results, ctx.Err()

		case <-timeout:
			results.EndTime = time.Now()
			results.Duration = results.EndTime.Sub(results.StartTime)
			results.SampleCount = sampleCount
			c.finalizeRawMetricsFile(results)
			return results, nil

		case t := <-ticker.C:
			sampleCount++
			samples, errors := c.collectSample(ctx, targets, t)
			results.Samples = append(results.Samples, samples...)
			results.Errors = append(results.Errors, errors...)

			// Stream each sample to JSON file in real-time
			for _, sample := range samples {
				c.streamSample(sample)
			}

			elapsed := time.Since(results.StartTime)
			remaining := duration - elapsed
			fmt.Printf("⏱️  Sample %d | Elapsed: %v | Remaining: %v | Pods: %d   \r",
				sampleCount,
				elapsed.Round(time.Second),
				remaining.Round(time.Second),
				len(samples))
		}
	}
}

func (c *Collector) collectSample(ctx context.Context, targets []PodTarget, timestamp time.Time) (samples []ResourceSample, errors []string) {
	samples = make([]ResourceSample, 0)
	errors = make([]string, 0)

	for _, target := range targets {
		var pods []string

		if target.PodName != "" {
			// Specific pod
			pods = []string{target.PodName}
		} else if target.LabelSelector != "" {
			// Pods by label selector
			podList, err := c.clientset.CoreV1().Pods(target.Namespace).List(ctx, metav1.ListOptions{
				LabelSelector: target.LabelSelector,
			})
			if err != nil {
				errors = append(errors, fmt.Sprintf("Failed to list pods in %s with selector %s: %v",
					target.Namespace, target.LabelSelector, err))
				continue
			}
			for i := range podList.Items {
				pods = append(pods, podList.Items[i].Name)
			}
		}

		// Collect metrics for each pod
		for _, podName := range pods {
			podSamples, err := c.collectPodMetrics(ctx, target.Namespace, podName, timestamp)
			if err != nil {
				errors = append(errors, fmt.Sprintf("Failed to collect metrics for %s/%s: %v",
					target.Namespace, podName, err))
				continue
			}
			samples = append(samples, podSamples...)
		}
	}

	return samples, errors
}

func (c *Collector) collectPodMetrics(ctx context.Context, namespace, podName string, timestamp time.Time) ([]ResourceSample, error) {
	metrics, err := c.metricsClient.MetricsV1beta1().PodMetricses(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	// Get pod info to retrieve node name
	pod, err := c.clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	nodeName := ""
	if err == nil {
		nodeName = pod.Spec.NodeName
	}

	samples := make([]ResourceSample, 0, len(metrics.Containers))
	for _, container := range metrics.Containers {
		cpu := container.Usage.Cpu().MilliValue()
		memory := container.Usage.Memory().Value()

		samples = append(samples, ResourceSample{
			Timestamp:     timestamp,
			Namespace:     namespace,
			PodName:       podName,
			NodeName:      nodeName,
			ContainerName: container.Name,
			CPUUsage:      cpu,
			MemoryUsage:   memory,
		})
	}

	return samples, nil
}

// Helper function to get pod status
func (c *Collector) GetPodStatus(ctx context.Context, namespace, podName string) (*corev1.Pod, error) {
	return c.clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
}

// Helper to get all pods in a namespace
func (c *Collector) ListPods(ctx context.Context, namespace, labelSelector string) (*corev1.PodList, error) {
	return c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
}

// GetCurrentMetrics gets the current metrics snapshot without starting collection
func (c *Collector) GetCurrentMetrics(ctx context.Context, targets []PodTarget) ([]ResourceSample, error) {
	samples, errors := c.collectSample(ctx, targets, time.Now())
	if len(errors) > 0 {
		return samples, fmt.Errorf("encountered %d errors during collection", len(errors))
	}
	return samples, nil
}
