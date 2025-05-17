package telemetry

import (
	"log/slog"
)

// ExporterType represents the type of exporter
type ExporterType string

const (
	ExporterTypeLog  ExporterType = "log"
	ExporterTypeHTTP ExporterType = "http"
)

// ExporterConfig holds configuration for an exporter
type ExporterConfig struct {
	Type          ExporterType `json:"type"`
	Enabled       bool         `json:"enabled"`
	Endpoint      string       `json:"endpoint,omitempty"`
	PushGateway   string       `json:"push_gateway,omitempty"`
	ServiceName   string       `json:"service_name,omitempty"`
	ClusterName   string       `json:"cluster_name,omitempty"`
	Namespace     string       `json:"namespace,omitempty"`
	PodName       string       `json:"pod_name,omitempty"`
	PushInterval  string       `json:"push_interval,omitempty"`
	BatchInterval string       `json:"batch_interval,omitempty"`
	LogLevel      slog.Level   `json:"log_level,omitempty"`
	Version       string       `json:"version,omitempty"`
}

// Exporter interface that all exporters must implement
type Exporter interface {
	Export(pathKey *PathKey) (bool, error)
	Shutdown() error
}
