// Package metrics provides local-first observability for Nebula.
//
// It persists a small summary snapshot under ~/.nebula, exposes a
// Prometheus-compatible text endpoint on localhost:2112, and gives the CLI
// and TUI a shared in-process API for counters, latency summaries, and recent
// wallet actions.
package metrics
