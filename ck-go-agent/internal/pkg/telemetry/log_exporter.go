package telemetry

import (
	"fmt"
	"log/slog"
)

type LogExporter struct {
	logger *slog.Logger
}

func NewLogExporter(logger *slog.Logger) (*LogExporter, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}
	return &LogExporter{logger: logger}, nil
}

func (e *LogExporter) Export(pathKey *PathKey) (bool, error) {
	// jsonBytes, err := json.MarshalIndent(pathKey, "", "    ")
	// if err != nil {
	// 	return fmt.Errorf("failed to marshal span: %w", err)
	// }

	// e.logger.Info("span data",
	// 	"data", string(jsonBytes))
	return false, nil
}

func (e *LogExporter) Shutdown() error {
	return nil
}
