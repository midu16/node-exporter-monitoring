package charts

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/openshift/node-exporter-zoneinfo-test/pkg/metrics"
	"github.com/wcharczuk/go-chart/v2"
	"github.com/wcharczuk/go-chart/v2/drawing"
)

type ChartGenerator struct {
	outputDir string
}

type PodTimeSeries struct {
	PodName   string
	NodeName  string
	Namespace string
	Times     []time.Time
	CPUValues []float64
	MemValues []float64
}

func NewChartGenerator(outputDir string) *ChartGenerator {
	return &ChartGenerator{
		outputDir: outputDir,
	}
}

func (g *ChartGenerator) GenerateCharts(results *metrics.MonitoringResults) error {
	// Create charts directory
	chartsDir := filepath.Join(g.outputDir, "charts")
	if err := os.MkdirAll(chartsDir, 0o755); err != nil {
		return fmt.Errorf("failed to create charts directory: %w", err)
	}

	// Group samples by pod
	podData := g.groupSamplesByPod(results.Samples)

	// Generate chart for each pod
	for _, data := range podData {
		if err := g.generatePodChart(chartsDir, data); err != nil {
			return fmt.Errorf("failed to generate chart for pod %s: %w", data.PodName, err)
		}
	}

	return nil
}

func (g *ChartGenerator) groupSamplesByPod(samples []metrics.ResourceSample) map[string]*PodTimeSeries {
	podData := make(map[string]*PodTimeSeries)

	for _, sample := range samples {
		key := fmt.Sprintf("%s/%s", sample.Namespace, sample.PodName)

		if podData[key] == nil {
			podData[key] = &PodTimeSeries{
				PodName:   sample.PodName,
				NodeName:  sample.NodeName,
				Namespace: sample.Namespace,
				Times:     []time.Time{},
				CPUValues: []float64{},
				MemValues: []float64{},
			}
		}

		// Aggregate containers per pod (sum CPU and Memory)
		// Find if we already have this timestamp
		found := false
		for i, t := range podData[key].Times {
			if t.Equal(sample.Timestamp) {
				podData[key].CPUValues[i] += float64(sample.CPUUsage)
				podData[key].MemValues[i] += float64(sample.MemoryUsage) / 1024 / 1024 // Convert to MiB
				found = true
				break
			}
		}

		if !found {
			podData[key].Times = append(podData[key].Times, sample.Timestamp)
			podData[key].CPUValues = append(podData[key].CPUValues, float64(sample.CPUUsage))
			podData[key].MemValues = append(podData[key].MemValues, float64(sample.MemoryUsage)/1024/1024)
		}
	}

	// Sort by timestamp
	for _, data := range podData {
		g.sortTimeSeries(data)
	}

	return podData
}

func (g *ChartGenerator) sortTimeSeries(data *PodTimeSeries) {
	// Create index array
	indices := make([]int, len(data.Times))
	for i := range indices {
		indices[i] = i
	}

	// Sort indices by time
	sort.Slice(indices, func(i, j int) bool {
		return data.Times[indices[i]].Before(data.Times[indices[j]])
	})

	// Reorder arrays
	sortedTimes := make([]time.Time, len(data.Times))
	sortedCPU := make([]float64, len(data.CPUValues))
	sortedMem := make([]float64, len(data.MemValues))

	for i, idx := range indices {
		sortedTimes[i] = data.Times[idx]
		sortedCPU[i] = data.CPUValues[idx]
		sortedMem[i] = data.MemValues[idx]
	}

	data.Times = sortedTimes
	data.CPUValues = sortedCPU
	data.MemValues = sortedMem
}

func (g *ChartGenerator) generatePodChart(chartsDir string, data *PodTimeSeries) error {
	if len(data.Times) == 0 {
		return nil // Skip pods with no data
	}

	// Create filename
	filename := filepath.Join(chartsDir, fmt.Sprintf("%s_%s.png", data.Namespace, data.PodName))

	// Create time series for chart
	cpuSeries := chart.TimeSeries{
		Name: "CPU (millicores)",
		Style: chart.Style{
			StrokeColor: chart.ColorBlue,
			StrokeWidth: 2,
		},
		XValues: data.Times,
		YValues: data.CPUValues,
	}

	memSeries := chart.TimeSeries{
		Name: "Memory (MiB)",
		Style: chart.Style{
			StrokeColor: chart.ColorRed,
			StrokeWidth: 2,
		},
		XValues: data.Times,
		YValues: data.MemValues,
		YAxis:   chart.YAxisSecondary,
	}

	// Create chart title with node info
	title := fmt.Sprintf("%s (Node: %s)", data.PodName, data.NodeName)
	if data.NodeName == "" {
		title = data.PodName
	}

	// Create the chart
	graph := chart.Chart{
		Title:      title,
		TitleStyle: chart.Style{FontSize: 12},
		Width:      1200,
		Height:     600,
		Background: chart.Style{
			Padding: chart.Box{
				Top:    40,
				Left:   20,
				Right:  20,
				Bottom: 20,
			},
		},
		XAxis: chart.XAxis{
			Name:           "Time",
			NameStyle:      chart.Style{FontSize: 10},
			Style:          chart.Style{FontSize: 8},
			ValueFormatter: chart.TimeMinuteValueFormatter,
		},
		YAxis: chart.YAxis{
			Name:      "CPU (millicores)",
			NameStyle: chart.Style{FontSize: 10},
			Style:     chart.Style{FontSize: 8},
		},
		YAxisSecondary: chart.YAxis{
			Name:      "Memory (MiB)",
			NameStyle: chart.Style{FontSize: 10},
			Style:     chart.Style{FontSize: 8, FontColor: drawing.ColorRed},
		},
		Series: []chart.Series{
			cpuSeries,
			memSeries,
		},
	}

	// Add legend
	graph.Elements = []chart.Renderable{
		chart.Legend(&graph),
	}

	// Render to file
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	return graph.Render(chart.PNG, f)
}
