package metrics

import (
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultAddress      = "127.0.0.1:2112"
	defaultMetricsPath  = "/metrics"
	snapshotFileName    = "observability.json"
	maxLatencySamples   = 512
	maxRecentActions    = 16
	defaultServerStatus = "stopped"
)

var histogramBuckets = []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

type persistentState struct {
	TxSuccessTotal uint64            `json:"tx_success_total"`
	TxFailureTotal uint64            `json:"tx_failure_total"`
	TxLatencyCount uint64            `json:"tx_latency_count"`
	TxLatencySum   float64           `json:"tx_latency_sum"`
	BucketCounts   []uint64          `json:"bucket_counts"`
	LatencySamples []float64         `json:"latency_samples"`
	WalletActions  map[string]uint64 `json:"wallet_actions"`
	IndexedTxTotal int               `json:"indexed_tx_total"`
	RecentActions  []ActionRecord    `json:"recent_actions"`
}

// ActionRecord is one recent operator-visible wallet action.
type ActionRecord struct {
	Action    string    `json:"action"`
	Timestamp time.Time `json:"timestamp"`
	Note      string    `json:"note,omitempty"`
}

// Snapshot is the shared summary used by CLI and TUI surfaces.
type Snapshot struct {
	TxSuccessTotal     uint64
	TxFailureTotal     uint64
	TxLatencyCount     uint64
	TxLatencyP95MS     float64
	TxLatencyAverageMS float64
	WalletActions      map[string]uint64
	IndexedTxTotal     int
	RecentActions      []ActionRecord
	ServerAddress      string
	ServerStatus       string
	ServerError        string
	MetricsURL         string
	RuntimeGoroutines  int
	RuntimeHeapAlloc   uint64
	PersistedStatsPath string
}

type manager struct {
	mu              sync.Mutex
	state           persistentState
	rootDir         string
	statsPath       string
	serverAddress   string
	serverStatus    string
	serverError     string
	serverStarted   bool
	serverAttempted bool
}

var (
	once sync.Once
	mgr  *manager
)

func getManager() *manager {
	once.Do(func() {
		root := defaultRootDir()
		mgr = &manager{
			rootDir:       root,
			statsPath:     filepath.Join(root, snapshotFileName),
			serverAddress: defaultAddress,
			serverStatus:  defaultServerStatus,
			state: persistentState{
				BucketCounts:  make([]uint64, len(histogramBuckets)),
				WalletActions: map[string]uint64{},
			},
		}
		_ = os.MkdirAll(root, 0o700)
		_ = mgr.load()
	})
	return mgr
}

// EnsureServer starts the local metrics server in a background goroutine.
func EnsureServer() {
	m := getManager()
	m.mu.Lock()
	if m.serverAttempted {
		m.mu.Unlock()
		return
	}
	m.serverAttempted = true
	m.serverStatus = "starting"
	addr := m.serverAddress
	m.mu.Unlock()

	go func() {
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			m.mu.Lock()
			if strings.Contains(strings.ToLower(err.Error()), "address already in use") {
				m.serverStatus = "already_running"
				m.serverError = ""
			} else {
				m.serverStatus = "error"
				m.serverError = err.Error()
			}
			m.mu.Unlock()
			return
		}
		m.mu.Lock()
		m.serverStarted = true
		m.serverStatus = "running"
		m.serverError = ""
		m.mu.Unlock()

		handler := http.NewServeMux()
		handler.HandleFunc(defaultMetricsPath, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
			_, _ = w.Write([]byte(RenderPrometheus()))
		})
		server := &http.Server{Handler: handler}
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			m.mu.Lock()
			m.serverStatus = "error"
			m.serverError = err.Error()
			m.mu.Unlock()
		}
	}()
}

// MetricsURL returns the local Prometheus scrape URL.
func MetricsURL() string {
	return "http://localhost:2112/metrics"
}

// StatsPath returns the persisted local stats file.
func StatsPath() string {
	return getManager().statsPath
}

// RecordWalletAction increments a wallet action counter.
func RecordWalletAction(action, note string) {
	m := getManager()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.WalletActions[action]++
	m.state.RecentActions = append([]ActionRecord{{
		Action:    action,
		Timestamp: time.Now().UTC(),
		Note:      strings.TrimSpace(note),
	}}, m.state.RecentActions...)
	if len(m.state.RecentActions) > maxRecentActions {
		m.state.RecentActions = m.state.RecentActions[:maxRecentActions]
	}
	_ = m.saveLocked()
}

// ObserveTx records one submitted transaction result and latency.
func ObserveTx(success bool, latency time.Duration) {
	m := getManager()
	m.mu.Lock()
	defer m.mu.Unlock()
	seconds := latency.Seconds()
	if success {
		m.state.TxSuccessTotal++
	} else {
		m.state.TxFailureTotal++
	}
	m.state.TxLatencyCount++
	m.state.TxLatencySum += seconds
	for i, bucket := range histogramBuckets {
		if seconds <= bucket {
			m.state.BucketCounts[i]++
		}
	}
	m.state.LatencySamples = append(m.state.LatencySamples, seconds)
	if len(m.state.LatencySamples) > maxLatencySamples {
		m.state.LatencySamples = m.state.LatencySamples[len(m.state.LatencySamples)-maxLatencySamples:]
	}
	_ = m.saveLocked()
}

// SetIndexedTxTotal updates the indexed transaction gauge.
func SetIndexedTxTotal(total int) {
	m := getManager()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.IndexedTxTotal = total
	_ = m.saveLocked()
}

// SnapshotNow returns the current shared summary.
func SnapshotNow() Snapshot {
	m := getManager()
	m.mu.Lock()
	defer m.mu.Unlock()
	s := snapshotFromLocked(m)
	return s
}

// RenderPrometheus renders the Prometheus text exposition format.
func RenderPrometheus() string {
	m := getManager()
	m.mu.Lock()
	defer m.mu.Unlock()
	s := snapshotFromLocked(m)

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	var b strings.Builder
	writeMetric := func(format string, args ...any) {
		b.WriteString(fmt.Sprintf(format, args...))
		b.WriteByte('\n')
	}

	writeMetric("# HELP nebula_tx_success_total Successful submitted transactions")
	writeMetric("# TYPE nebula_tx_success_total counter")
	writeMetric("nebula_tx_success_total %d", s.TxSuccessTotal)

	writeMetric("# HELP nebula_tx_failure_total Failed submitted transactions")
	writeMetric("# TYPE nebula_tx_failure_total counter")
	writeMetric("nebula_tx_failure_total %d", s.TxFailureTotal)

	writeMetric("# HELP nebula_tx_latency_seconds End-to-end submitted transaction latency")
	writeMetric("# TYPE nebula_tx_latency_seconds histogram")
	var cumulative uint64
	for i, bucket := range histogramBuckets {
		cumulative += m.state.BucketCounts[i]
		writeMetric(`nebula_tx_latency_seconds_bucket{le="%g"} %d`, bucket, cumulative)
	}
	writeMetric(`nebula_tx_latency_seconds_bucket{le="+Inf"} %d`, s.TxLatencyCount)
	writeMetric("nebula_tx_latency_seconds_sum %.6f", m.state.TxLatencySum)
	writeMetric("nebula_tx_latency_seconds_count %d", s.TxLatencyCount)

	writeMetric("# HELP nebula_wallet_actions_total Wallet actions by type")
	writeMetric("# TYPE nebula_wallet_actions_total counter")
	for _, action := range sortedActionKeys(m.state.WalletActions) {
		writeMetric(`nebula_wallet_actions_total{action="%s"} %d`, action, m.state.WalletActions[action])
	}

	writeMetric("# HELP nebula_indexed_tx_total Indexed transaction count")
	writeMetric("# TYPE nebula_indexed_tx_total gauge")
	writeMetric("nebula_indexed_tx_total %d", s.IndexedTxTotal)

	writeMetric("# HELP nebula_runtime_goroutines Current goroutine count")
	writeMetric("# TYPE nebula_runtime_goroutines gauge")
	writeMetric("nebula_runtime_goroutines %d", runtime.NumGoroutine())

	writeMetric("# HELP nebula_runtime_heap_bytes Current Go heap allocation")
	writeMetric("# TYPE nebula_runtime_heap_bytes gauge")
	writeMetric("nebula_runtime_heap_bytes %d", mem.HeapAlloc)
	return b.String()
}

func snapshotFromLocked(m *manager) Snapshot {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	actions := make(map[string]uint64, len(m.state.WalletActions))
	for k, v := range m.state.WalletActions {
		actions[k] = v
	}
	recent := append([]ActionRecord(nil), m.state.RecentActions...)
	avg := 0.0
	if m.state.TxLatencyCount > 0 {
		avg = (m.state.TxLatencySum / float64(m.state.TxLatencyCount)) * 1000
	}
	return Snapshot{
		TxSuccessTotal:     m.state.TxSuccessTotal,
		TxFailureTotal:     m.state.TxFailureTotal,
		TxLatencyCount:     m.state.TxLatencyCount,
		TxLatencyP95MS:     percentile95MS(m.state.LatencySamples),
		TxLatencyAverageMS: avg,
		WalletActions:      actions,
		IndexedTxTotal:     m.state.IndexedTxTotal,
		RecentActions:      recent,
		ServerAddress:      m.serverAddress,
		ServerStatus:       m.serverStatus,
		ServerError:        m.serverError,
		MetricsURL:         MetricsURL(),
		RuntimeGoroutines:  runtime.NumGoroutine(),
		RuntimeHeapAlloc:   mem.HeapAlloc,
		PersistedStatsPath: m.statsPath,
	}
}

func percentile95MS(samples []float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	cp := append([]float64(nil), samples...)
	sort.Float64s(cp)
	idx := int(math.Ceil(0.95*float64(len(cp)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(cp) {
		idx = len(cp) - 1
	}
	return cp[idx] * 1000
}

func sortedActionKeys(values map[string]uint64) []string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (m *manager) load() error {
	data, err := os.ReadFile(m.statsPath)
	if err != nil {
		return nil
	}
	var state persistentState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	if len(state.BucketCounts) != len(histogramBuckets) {
		state.BucketCounts = make([]uint64, len(histogramBuckets))
	}
	if state.WalletActions == nil {
		state.WalletActions = map[string]uint64{}
	}
	m.state = state
	return nil
}

func (m *manager) saveLocked() error {
	payload, err := json.MarshalIndent(m.state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.statsPath, payload, 0o600)
}

func defaultRootDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".nebula")
	}
	return filepath.Join(home, ".nebula")
}
