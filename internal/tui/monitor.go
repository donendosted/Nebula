package tui

import (
	"fmt"
	"strings"

	"nebula/internal/metrics"

	"github.com/charmbracelet/lipgloss"
)

// RenderMonitoringPanel renders the observability view in the TUI.
func RenderMonitoringPanel(snapshot metrics.Snapshot, prometheusURL string) string {
	box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(1)
	lines := []string{
		"Monitoring",
		"",
		fmt.Sprintf("Tx success: %d", snapshot.TxSuccessTotal),
		fmt.Sprintf("Tx failure: %d", snapshot.TxFailureTotal),
		fmt.Sprintf("Latency p95: %.2f ms", snapshot.TxLatencyP95MS),
		fmt.Sprintf("Latency avg: %.2f ms", snapshot.TxLatencyAverageMS),
		fmt.Sprintf("Indexed tx: %d", snapshot.IndexedTxTotal),
		fmt.Sprintf("Metrics server: %s", snapshot.ServerStatus),
		fmt.Sprintf("Metrics URL: %s", snapshot.MetricsURL),
		fmt.Sprintf("Prometheus: %s", prometheusURL),
		"",
		"[r] Refresh  [o] Open Prometheus  [esc] Back",
	}
	if snapshot.ServerError != "" {
		lines = append(lines, "", "Server error: "+snapshot.ServerError)
	}
	return box.Render(strings.Join(lines, "\n"))
}
