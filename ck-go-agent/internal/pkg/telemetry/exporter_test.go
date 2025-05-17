package telemetry

import (
	"os"
	"testing"
)

func TestFileExporter(t *testing.T) {
	// Create a temporary file for testing
	tmpFile := "test_spans.json"
	defer os.Remove(tmpFile) // Clean up after test

	// Create exporter
	exporter, err := NewFileExporter(tmpFile)
	if err != nil {
		t.Fatalf("Failed to create exporter: %v", err)
	}
	defer exporter.Shutdown()

	// Create a test PathKey
	parentTraceId := "test_parent_trace_id"
	testPathKey := &PathKey{
		PathKey: "test_trace_id",
		GraphPathNode: GraphPathNode{
			IncomingPath: &parentTraceId,
			GraphPathElement: GrpcServiceElement{
				Type:               "grpc",
				GrpcFullMethodName: "/test.Service/TestMethod",
			},
		},
	}

	// Export the PathKey
	if err := exporter.Export(testPathKey); err != nil {
		t.Errorf("Failed to export PathKey: %v", err)
	}

	// Read and verify the file contents
	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read test file: %v", err)
	}

	t.Logf("Written content: %s", string(content))
}
