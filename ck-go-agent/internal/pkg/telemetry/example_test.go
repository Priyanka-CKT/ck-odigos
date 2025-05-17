package telemetry_test

import (
	"fmt"
	"io/ioutil"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"

	"go.opentelemetry.io/auto/internal/pkg/telemetry"
)

func ExampleFactory() {
	// Create a no-op logger to suppress log output
	logger := slog.New(slog.NewTextHandler(ioutil.Discard, nil))

	// Setup test HTTP server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("Received HTTP span")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// Create configs for all exporters
	configs := []telemetry.ExporterConfig{
		{
			Type:     telemetry.ExporterTypeLog,
			Enabled:  true,
			LogLevel: slog.LevelInfo,
		},
		{
			Type:     telemetry.ExporterTypeHTTP,
			Enabled:  true,
			Endpoint: ts.URL,
		},
	}

	// Create factory
	factory, err := telemetry.NewFactory(logger, configs)
	if err != nil {
		log.Fatalf("Failed to create factory: %v", err)
	}

	// Create test PathKey
	parentTraceId := "test_parent_trace_id"
	testPathKey := &telemetry.PathKey{
		PathKey: "test_trace_id",
		GraphPathNode: telemetry.GraphPathNode{
			IncomingPath: &parentTraceId,
			GraphPathElement: telemetry.GrpcServiceElement{
				Type:               "grpc",
				GrpcFullMethodName: "/test.Service/TestMethod",
			},
		},
	}

	// Export span
	status, err := factory.Export(testPathKey)
	if err != nil {
		log.Fatalf("Failed to export span: %v", err)
	}
	fmt.Printf("Export status: %d\n", status)

	// Cleanup
	//os.Remove("test_spans.json")

	// Output:
	// Received HTTP span
	// Export status: 200
}
