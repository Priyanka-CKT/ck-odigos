package telemetry

import (
	"fmt"
	"log/slog"
)

// Factory creates and manages exporters
type Factory struct {
	exporters []Exporter
	logger    *slog.Logger
}

// Status codes for Export operation
const (
	InstrumentationDisabled = 0
	InstrumentationEnabled  = 1
)

// NewFactory creates a new exporter factory
func NewFactory(logger *slog.Logger, configs []ExporterConfig) (*Factory, error) {
	factory := &Factory{
		logger: logger,
	}

	for _, cfg := range configs {
		if !cfg.Enabled {
			continue
		}

		exporter, err := factory.createExporter(cfg)
		if err != nil {
			return nil, err
		}
		factory.exporters = append(factory.exporters, exporter)
	}

	return factory, nil
}

func (f *Factory) createExporter(cfg ExporterConfig) (Exporter, error) {
	switch cfg.Type {
	case ExporterTypeLog:
		f.logger.Info("====== In the createExporter fucntion for log exporter")
		return NewLogExporter(f.logger)
	case ExporterTypeHTTP:
		f.logger.Info("====== In the createExporter fucntion for http exporter")
		return NewHTTPExporter(f.logger, cfg)
	default:
		return nil, fmt.Errorf("unknown exporter type: %s", cfg.Type)
	}
}

// Export sends span to all configured exporters
// Returns a status code and an error if any
func (f *Factory) Export(pathKey *PathKey) (bool, error) {
	var errs []error
	instrumentationEnabled := false //Get the default value
	for _, exp := range f.exporters {
		enabled, err := exp.Export(pathKey)
		if err != nil {
			errs = append(errs, err)
		}
		// If any exporter indicates instrumentation is enabled, set the flag
		if enabled {
			instrumentationEnabled = true
		}
	}
	if len(errs) > 0 {
		return instrumentationEnabled, fmt.Errorf("export errors: %v", errs)
	}
	return instrumentationEnabled, nil
}

// Shutdown propagates shutdown to all exporters
func (f *Factory) Shutdown() error {
	var errs []error
	for _, exp := range f.exporters {
		if err := exp.Shutdown(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}
	return nil
}
