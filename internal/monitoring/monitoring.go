package monitoring

import (
	"fmt"
	"os/exec"
	"runtime"

	"nebula/internal/metrics"
)

const (
	// PrometheusURL is the default local Prometheus UI.
	PrometheusURL = "http://localhost:9090"
)

// URLs returns the local observability entrypoints.
func URLs() map[string]string {
	return map[string]string{
		"metrics":    metrics.MetricsURL(),
		"prometheus": PrometheusURL,
	}
}

// OpenBrowser attempts to open a URL with the platform default browser.
func OpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	return nil
}
