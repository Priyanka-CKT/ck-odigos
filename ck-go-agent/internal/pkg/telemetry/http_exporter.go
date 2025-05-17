package telemetry

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
)

// Instrumentation status constants
const (
	InstrumentationStatusEnabled     = "enabled"
	InstrumentationStatusDisabled    = "disabled"
	InstrumentationStatusRateLimited = "rate-limited"

	// Maximum number of path keys to send in a single Nexus request
	MaxNexusBatchSize = 50
)

type HTTPExporter struct {
	endpoint                          string
	client                            *http.Client
	seenPathKeys                      sync.Map
	sentPathKeys                      sync.Map
	serviceName                       string
	pushGateway                       string
	graphCounter                      *prometheus.CounterVec
	graphErrorCounter                 *prometheus.CounterVec
	graphPathLatencySummary           *prometheus.SummaryVec
	batchInterval                     time.Duration
	logger                            *slog.Logger
	instrumentationEnabled            bool
	instrumentationStatusLastSyncTime time.Time
	clusterName                       string
	namespace                         string
	podName                           string
	version                           string
	done                              chan struct{}
}

func NewHTTPExporter(logger *slog.Logger, cfg ExporterConfig) (*HTTPExporter, error) {
	if cfg.Endpoint == "" || cfg.PushGateway == "" {
		return nil, fmt.Errorf("endpoint and pushGateway cannot be empty")
	}

	if cfg.PodName == "" {
		logger.Info("Pod name is empty, generating new UUID")
		cfg.PodName = uuid.New().String()
	}

	// Create a new counter vector with pathKey as label
	pgServiceName := strings.ReplaceAll(strings.ReplaceAll(cfg.ServiceName, "-", "_"), " ", "_")
	counter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: pgServiceName + ":ck_graph_throughput_total",
			Help: "Total number of times each path key is seen",
		},
		[]string{"pathKey", "ck_pod_name"},
	)

	// Try to register the counter, handle case where it might already be registered
	if err := prometheus.Register(counter); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			// If the error is due to already registered collector, use the existing one
			counter = are.ExistingCollector.(*prometheus.CounterVec)
			logger.Info("Using existing counter vector", "name", pgServiceName+":ck_graph_throughput_total")
		} else {
			// If it's a different error, return it
			return nil, fmt.Errorf("failed to register prometheus counter: %w", err)
		}
	}

	graphErrorCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: pgServiceName + ":ck_graph_error_total",
			Help: "Total number of times each path key is seen",
		},
		[]string{"pathKey", "ck_pod_name", "error_code"},
	)

	graphPathLatencySummary := prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name: pgServiceName + ":ck_graph_latency_summary",
			Help: "Latency summary for each path key",
			Objectives: map[float64]float64{
				0.50: 0.05,  // p50 with 5% error
				0.90: 0.01,  // p90 with 1% error
				0.95: 0.005, // p95 with 0.5% error
				0.99: 0.001, // p99 with 0.1% error
			},
			MaxAge:     5 * time.Minute, // Keep samples for 5 minutes
			AgeBuckets: 10,              // Number of buckets used to calculate age
			BufCap:     1000,            // Capacity of the buffer for streaming quantiles
		},
		[]string{"pathKey", "ck_pod_name"},
	)
	exporter := &HTTPExporter{
		endpoint:                          cfg.Endpoint,
		client:                            &http.Client{},
		serviceName:                       cfg.ServiceName,
		pushGateway:                       cfg.PushGateway,
		clusterName:                       cfg.ClusterName,
		namespace:                         cfg.Namespace,
		podName:                           cfg.PodName,
		graphCounter:                      counter,
		graphErrorCounter:                 graphErrorCounter,
		graphPathLatencySummary:           graphPathLatencySummary,
		batchInterval:                     30 * time.Second,
		logger:                            logger,
		instrumentationEnabled:            true,
		instrumentationStatusLastSyncTime: time.Time{}, // Initialize with zero time to force first check
		version:                           cfg.Version,
		done:                              make(chan struct{}),
	}

	// Perform initial check of instrumentation status
	status, err := exporter.checkInstrumentationStatus()
	if err != nil {
		logger.Error("Failed to check initial instrumentation status", "error", err)
		// Continue anyway, using the default value (true)
	} else {
		logger.Info("Initial instrumentation status received", "status", status)
		exporter.updateInstrumentationStatus(status)
	}

	instanceName := uuid.New().String()
	exporter.startScheduler(pgServiceName, instanceName)
	logger.Info("Starting HTTP exporter with params ", "instanceName", instanceName,
		"endpoint", cfg.Endpoint, "pushGateway", cfg.PushGateway, "serviceName", cfg.ServiceName,
		"pgServiceName", pgServiceName)
	return exporter, nil
}

func (e *HTTPExporter) initializeFromServer() error {
	e.logger.Info("Initializing sent pathkeys from server...")

	req, err := http.NewRequest("GET", e.endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create init request: %w", err)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to get existing pathkeys: %w", err)
	}

	func() {
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			e.logger.Error("Unexpected status code during init", "status", resp.StatusCode)
			return
		}

		var existingPathKeys []*PathKey
		if err := json.NewDecoder(resp.Body).Decode(&existingPathKeys); err != nil {
			e.logger.Error("Failed to decode existing pathkeys", "error", err)
			return
		}

		// Initialize sentPathKeys
		for _, pk := range existingPathKeys {
			e.sentPathKeys.Store(pk.PathKey, true)
		}
		e.logger.Info("Initialized existing pathkeys", "count", len(existingPathKeys))
	}()

	return nil
}

// Helper function to check if a sync.Map is empty
func isSyncMapEmpty(m *sync.Map) bool {
	empty := true
	m.Range(func(_, _ interface{}) bool {
		empty = false
		return false // stop after first item
	})
	return empty
}

func (e *HTTPExporter) needsInitialization() bool {
	seenEmpty := isSyncMapEmpty(&e.seenPathKeys)
	sentEmpty := isSyncMapEmpty(&e.sentPathKeys)
	return !seenEmpty && sentEmpty
}

func (e *HTTPExporter) startScheduler(pgServiceName string, instanceName string) {
	go func() {
		e.logger.Info("Starting scheduler", "podName", e.podName, "namespace", e.namespace, "clusterName", e.clusterName, "version", e.version)
		ticker := time.NewTicker(e.batchInterval)
		defer ticker.Stop()

		for {
			select {
			case <-e.done:
				e.logger.Info("Scheduler received shutdown signal, stopping...")
				return
			case <-ticker.C:
				// Check if we need to update instrumentation status (every 30 minutes)
				if time.Since(e.instrumentationStatusLastSyncTime) > 30*time.Minute {
					status, err := e.checkInstrumentationStatus()
					if err != nil {
						e.logger.Error("Failed to check instrumentation status", "error", err)
					} else {
						e.logger.Debug("Received instrumentation status", "status", status)
						e.updateInstrumentationStatus(status)
					}
				}

				if !e.instrumentationEnabled {
					e.logger.Debug("Instrumentation is disabled, skipping")
					continue
				}

				// Initialize if needed
				if e.needsInitialization() {
					if err := e.initializeFromServer(); err != nil {
						e.logger.Error("Failed to initialize from server", "error", err)
					}
				}

				// Handle Nexus updates
				newPathKeys := []*PathKey{}
				e.seenPathKeys.Range(func(key, value interface{}) bool {
					pathKey := key.(string)
					if _, sent := e.sentPathKeys.Load(pathKey); !sent {
						newPathKeys = append(newPathKeys, value.(*PathKey))
					}
					return true
				})

				if len(newPathKeys) > 0 {
					e.logger.Info("Found new pathkeys to export", "count", len(newPathKeys))
					// Handle Nexus export - this will send in batches and return successfully sent path keys
					sentPathKeys, err := e.sendToNexus(newPathKeys)
					// Mark the successfully sent path keys
					if len(sentPathKeys) > 0 {
						e.logger.Info("Marking successfully sent pathkeys", "count", len(sentPathKeys))
						for _, pk := range sentPathKeys {
							e.sentPathKeys.Store(pk.PathKey, true)
						}
					}
					// Log any errors that occurred during sending
					if err != nil {
						e.logger.Error("Failed to send some batches to Nexus", "error", err,
							"sentCount", len(sentPathKeys),
							"totalCount", len(newPathKeys))
					} else if len(sentPathKeys) == len(newPathKeys) {
						e.logger.Info("Successfully sent all pathkeys to Nexus", "count", len(sentPathKeys))
					}
				}

				if e.instrumentationEnabled && !isSyncMapEmpty(&e.seenPathKeys) {
					pusher := push.New(e.pushGateway, pgServiceName).
						Collector(e.graphCounter).
						Collector(e.graphErrorCounter).
						Collector(e.graphPathLatencySummary)

					// Add groupings with fallback values where needed
					//pusher = addGroupingIfNotEmpty(pusher, "ck_pod_name", e.podName, instanceName)
					pusher = addGroupingIfNotEmpty(pusher, "ck_namespace", e.namespace, "")
					pusher = addGroupingIfNotEmpty(pusher, "ck_cluster_name", e.clusterName, "")
					pusher = addGroupingIfNotEmpty(pusher, "agent_version", e.version, "")
					if err := pusher.Push(); err != nil {
						e.logger.Error("Failed to push metrics to Pushgateway",
							"error", err,
							"pushGateway", e.pushGateway,
							"serviceName", pgServiceName)
					}
				}
			}
		}
	}()
}

// Helper function to handle Nexus export in batches
// Returns the list of successfully sent path keys across all batches
// If any batch fails, it still returns the path keys that were successfully sent in other batches
// along with an error indicating which batch failed
func (e *HTTPExporter) sendToNexus(pathKeys []*PathKey) ([]*PathKey, error) {
	successfulPathKeys := []*PathKey{}
	totalBatches := (len(pathKeys) + MaxNexusBatchSize - 1) / MaxNexusBatchSize
	var lastError error

	// Process path keys in batches
	for i := 0; i < len(pathKeys); i += MaxNexusBatchSize {
		if !e.instrumentationEnabled {
			e.logger.Debug("Skipping sending batch to Nexus due to instrumentation being disabled")
			lastError = fmt.Errorf("instrumentation is disabled")
			break
		}

		// Calculate end index for current batch
		end := i + MaxNexusBatchSize
		if end > len(pathKeys) {
			end = len(pathKeys)
		}

		// Get current batch
		batch := pathKeys[i:end]
		batchNumber := (i / MaxNexusBatchSize) + 1
		e.logger.Info("Sending batch to Nexus",
			"batchSize", len(batch),
			"batchNumber", batchNumber,
			"totalBatches", totalBatches)

		// Send the batch
		status, err := e.sendBatchToNexus(batch)
		if err != nil {
			// Record the error but continue processing other batches
			lastError = fmt.Errorf("failed to send batch %d of %d: %w",
				batchNumber, totalBatches, err)
			e.logger.Error("Failed to send batch to Nexus",
				"batchNumber", batchNumber,
				"totalBatches", totalBatches,
				"error", err)
			continue
		}

		// Add successfully sent path keys to the tracking list
		successfulPathKeys = append(successfulPathKeys, batch...)
		e.logger.Info("Successfully sent batch to Nexus",
			"batchNumber", batchNumber,
			"totalBatches", totalBatches,
			"batchSize", len(batch),
			"status", status)
		e.updateInstrumentationStatus(status)
	}

	// Return all successfully sent path keys and the last error (if any)
	return successfulPathKeys, lastError
}

// Helper function to send a single batch to Nexus
// This function only handles the HTTP request for a single batch
// It does not update any application-level tracking of sent path keys
func (e *HTTPExporter) sendBatchToNexus(batch []*PathKey) (string, error) {

	jsonBytes, err := json.Marshal(batch)
	if err != nil {
		return "", fmt.Errorf("failed to marshal pathkeys: %w", err)
	}
	endpoint := fmt.Sprintf("%s/api/graph-paths/%s", e.endpoint, e.serviceName)
	e.logger.Info("Sending batch to Nexus", "endpoint", endpoint)
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send batch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Parse the response to extract the status
	var response struct {
		Status string `json:"status"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	// Default to enabled if status is empty
	if response.Status == "" {
		return "", nil
	}
	return response.Status, nil
}

func (e *HTTPExporter) Export(pathKey *PathKey) (bool, error) {
	key := pathKey.PathKey

	e.logger.Info("Received pathKey for export", "pathKey", key)

	// Update seenPathKeys if new
	_, exists := e.seenPathKeys.LoadOrStore(key, pathKey)
	if !exists {
		e.logger.Info("New pathKey found, adding to seenPathKeys", "pathKey", key)
	}

	//Important note of EventCount.
	//EventCount is used if the operation was a batch operation and the number of events in the batch
	//need to be recorded for throughput calculation. Eg. Batch Kafka Producer.
	//For non-batch operations, EventCount is 1.
	//Remember that the latency is recorded for the entire batch operation once. However, the throughput is recorded for each event in the batch.
	//This might lead to some weirdness in the overall latency calculation, but there is nothing we can do about it.

	if pathKey.GraphPathNode.EventCount > 1 {
		e.graphCounter.WithLabelValues(key, e.podName).Add(float64(pathKey.GraphPathNode.EventCount))
	} else {
		e.graphCounter.WithLabelValues(key, e.podName).Inc()
	}

	if pathKey.GraphPathNode.IsError {
		errorCode := pathKey.GraphPathNode.ErrorCode
		if errorCode == "" {
			errorCode = "CK001"
		}
		if pathKey.GraphPathNode.EventCount > 1 {
			e.graphErrorCounter.WithLabelValues(key, e.podName, errorCode).Add(float64(pathKey.GraphPathNode.EventCount))
		} else {
			e.graphErrorCounter.WithLabelValues(key, e.podName, errorCode).Inc()
		}
	}

	//This records the latency in nanoseconds
	latency := float64(pathKey.GraphPathNode.EndTime - pathKey.GraphPathNode.StartTime)
	if latency > 0 {
		e.graphPathLatencySummary.WithLabelValues(key, e.podName).Observe(latency)
	}

	return e.instrumentationEnabled, nil
}

func (e *HTTPExporter) Shutdown() error {
	if e.done != nil {
		close(e.done)
	}
	e.client.CloseIdleConnections()
	return nil
}

// updateInstrumentationStatus updates the instrumentation enabled flag based on the status
// and cleans up tracking maps and counters if instrumentation is disabled
func (e *HTTPExporter) updateInstrumentationStatus(rawStatus string) {
	if rawStatus == "" {
		e.logger.Info("Received empty instrumentation status, making no changes")
		return
	}

	// Convert status to lowercase for case-insensitive comparison
	status := strings.ToLower(rawStatus)

	// Update instrumentation enabled flag based on status
	wasEnabled := e.instrumentationEnabled
	e.instrumentationEnabled = status == strings.ToLower(InstrumentationStatusEnabled)

	if wasEnabled != e.instrumentationEnabled {
		e.logger.Info("Instrumentation status changed", "previous", wasEnabled, "current", e.instrumentationEnabled, "status", status)
		// If instrumentation was previously enabled and is now disabled,
		// clear the tracking maps to reset state
		if wasEnabled && !e.instrumentationEnabled {
			e.logger.Info("Clearing pathkey tracking maps due to instrumentation being disabled")
			e.seenPathKeys.Range(func(key, value interface{}) bool {
				e.seenPathKeys.Delete(key)
				return true
			})
			e.sentPathKeys.Range(func(key, value interface{}) bool {
				e.sentPathKeys.Delete(key)
				return true
			})
			e.graphCounter.Reset()
			e.logger.Info("Cleared pathkey tracking maps & counter due to instrumentation being disabled")
		}
	} else {
		e.logger.Debug("Instrumentation status unchanged", "enabled", e.instrumentationEnabled, "status", status)
	}
}

// checkInstrumentationStatus checks with the server if instrumentation should be enabled or disabled
func (e *HTTPExporter) checkInstrumentationStatus() (string, error) {
	// Update the last sync time before checking the status so dont check again too soon
	e.instrumentationStatusLastSyncTime = time.Now()

	url := fmt.Sprintf("%s/api/agent-status/%s", e.endpoint, e.serviceName)
	e.logger.Info("Checking instrumentation status", "url", url)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create status request: %w", err)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get instrumentation status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("received non-OK status code: %d", resp.StatusCode)
	}

	var statusResponse struct {
		Status string `json:"status"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&statusResponse); err != nil {
		return "", fmt.Errorf("failed to decode status response: %w", err)
	}

	// Update instrumentation status based on the response
	e.updateInstrumentationStatus(statusResponse.Status)

	return statusResponse.Status, nil
}

// addGroupingIfNotEmpty adds a grouping key-value pair if either value or defaultValue is not empty
// preferring the primary value over the default if both are present
func addGroupingIfNotEmpty(pusher *push.Pusher, key, value, defaultValue string) *push.Pusher {
	if value != "" {
		return pusher.Grouping(key, value)
	}
	if defaultValue != "" {
		return pusher.Grouping(key, defaultValue)
	}
	return pusher
}
