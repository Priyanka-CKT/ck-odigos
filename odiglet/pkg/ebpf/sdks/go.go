package sdks

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	odigosv1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	"github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/instrumentation"
	"github.com/odigos-io/odigos/odiglet/pkg/ebpf"

	"github.com/odigos-io/odigos/odiglet/pkg/env"
	"github.com/odigos-io/odigos/odiglet/pkg/instrumentation/consts"
	"github.com/odigos-io/odigos/odiglet/pkg/log"
	"go.opentelemetry.io/auto"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
)

type GoOtelEbpfSdk struct {
	inst *auto.Instrumentation
	cp   *ebpf.ConfigProvider[auto.InstrumentationConfig]
}

// compile-time check that configProvider[auto.InstrumentationConfig] implements auto.Provider
var _ auto.ConfigProvider = (*ebpf.ConfigProvider[auto.InstrumentationConfig])(nil)

type GoInstrumentationFactory struct {
}

func NewGoInstrumentationFactory() instrumentation.Factory {
	return &GoInstrumentationFactory{}
}

func (g *GoInstrumentationFactory) CreateInstrumentation(ctx context.Context, pid int, settings instrumentation.Settings) (instrumentation.Instrumentation, error) {

	log.Logger.Info("Creating Go instrumentation with context", "context", ctx)
	log.Logger.Info("Process ID", "pid", pid)
	log.Logger.Info("Instrumentation settings", "settings", settings)

	defaultExporter, err := otlptracegrpc.New(
		ctx,
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint(fmt.Sprintf("%s:%d", env.Current.NodeIP, consts.OTLPPort)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create exporter: %w", err)
	}

	initialConfig, err := convertToGoInstrumentationConfig(settings.InitialConfig)
	if err != nil {
		return nil, fmt.Errorf("invalid initial config type, expected *odigosv1.SdkConfig, got %T", settings.InitialConfig)
	}

	cp := ebpf.NewConfigProvider(initialConfig)

	// Try to read APP_NAME directly from the process environment
	serviceName := settings.ServiceName
	appName, err := readAppNameFromProcess(pid)
	if err == nil && appName != "" {
		log.Logger.Info("Found APP_NAME environment variable in process", "APP_NAME", appName, "pid", pid)
		serviceName = appName

		// Add APP_NAME as a resource attribute
		settings.ResourceAttributes = append(settings.ResourceAttributes,
			attribute.String("app.name", appName))
	} else if serviceName == "" || serviceName == "unknown_service" {
		// Only use workload name as fallback if service name is empty or default
		// If APP_NAME is not found, use the workload name as a fallback
		log.Logger.Info("APP_NAME not found in process environment, using workload name as fallback", "pid", pid)

		// Extract workload name from resource attributes
		workloadName := extractWorkloadName(settings.ResourceAttributes)
		if workloadName != "" {
			log.Logger.Info("Using workload name as service name", "workloadName", workloadName)
			serviceName = workloadName

			// Add workload name as app.name attribute
			settings.ResourceAttributes = append(settings.ResourceAttributes,
				attribute.String("app.name", workloadName))
		} else {
			// Final fallback: use a combination of namespace and pod name if available
			log.Logger.Info("Attempting to extract pod name and namespace as final fallback")
			podName := extractPodName(settings.ResourceAttributes)
			log.Logger.Info("Extracted pod name", "podName", podName)
			namespace := extractNamespace(settings.ResourceAttributes)
			log.Logger.Info("Extracted namespace", "namespace", namespace)

			if podName != "" && namespace != "" {
				fallbackName := fmt.Sprintf("%s-%s", namespace, podName)
				log.Logger.Info("Using namespace-podname as service name", "fallbackName", fallbackName)
				serviceName = fallbackName

				// Add fallback name as app.name attribute
				settings.ResourceAttributes = append(settings.ResourceAttributes,
					attribute.String("app.name", fallbackName))
			}
		}
	} else {
		log.Logger.Info("Using existing service name", "serviceName", serviceName)
	}

	log.Logger.Info("Creating Go instrumentation",
		"pid", pid,
		"serviceName", serviceName,
		"resourceAttributes", settings.ResourceAttributes)

	// For complex objects, you might want to use JSON formatting
	configJSON, _ := json.Marshal(initialConfig)
	log.Logger.Info("Initial configuration",
		"config", string(configJSON))

	// Log resource attributes properly
	for i, attr := range settings.ResourceAttributes {
		log.Logger.Info("Resource attribute",
			"index", i,
			"key", attr.Key,
			"value", attr.Value)
	}

	log.Logger.Info("creating new instrumentation with settings")

	inst, err := auto.NewInstrumentation(
		ctx,
		auto.WithEnv(), // for OTEL_LOG_LEVEL
		auto.WithPID(pid),
		auto.WithResourceAttributes(settings.ResourceAttributes...),
		auto.WithServiceName(serviceName),
		auto.WithTraceExporter(defaultExporter),
		auto.WithGlobal(),
		auto.WithConfigProvider(cp),
	)
	if err != nil {
		log.Logger.Error(err, "instrumentation setup failed")
		return nil, err
	}

	return &GoOtelEbpfSdk{inst: inst, cp: cp}, nil
}

// readAppNameFromProcess attempts to read the APP_NAME environment variable from a process
func readAppNameFromProcess(pid int) (string, error) {
	log.Logger.Info("Reading APP_NAME from process environment", "pid", pid)
	// Read environment variables from /proc/{pid}/environ
	environPath := fmt.Sprintf("/proc/%d/environ", pid)
	environBytes, err := os.ReadFile(environPath)
	if err != nil {
		return "", fmt.Errorf("failed to read process environment: %w", err)
	}

	// Environment variables are null-separated
	environ := strings.Split(string(environBytes), "\x00")
	log.Logger.Info("readAppNameFromProcess environ", "environ", environ)
	for _, env := range environ {
		if strings.HasPrefix(env, "APP_NAME=") {
			return strings.TrimPrefix(env, "APP_NAME="), nil
		}
	}

	return "", fmt.Errorf("APP_NAME environment variable not found")
}

func (g *GoOtelEbpfSdk) Run(ctx context.Context) error {
	return g.inst.Run(ctx)
}

func (g *GoOtelEbpfSdk) Load(ctx context.Context) error {
	return g.inst.Load(ctx)
}

func (g *GoOtelEbpfSdk) Close(_ context.Context) error {
	return g.inst.Close()
}

func (g *GoOtelEbpfSdk) ApplyConfig(ctx context.Context, sdkConfig instrumentation.Config) error {
	updatedConfig, err := convertToGoInstrumentationConfig(sdkConfig)
	if err != nil {
		return err
	}

	return g.cp.SendConfig(ctx, updatedConfig)
}

func convertToGoInstrumentationConfig(sdkConfig instrumentation.Config) (auto.InstrumentationConfig, error) {
	initialConfig, ok := sdkConfig.(*odigosv1.SdkConfig)
	if !ok {
		return auto.InstrumentationConfig{}, fmt.Errorf("invalid initial config type, expected *odigosv1.SdkConfig, got %T", sdkConfig)
	}
	ic := auto.InstrumentationConfig{}
	if sdkConfig == nil {
		log.Logger.V(0).Info("No SDK config provided for Go instrumentation, using default")
		return ic, nil
	}
	ic.InstrumentationLibraryConfigs = make(map[auto.InstrumentationLibraryID]auto.InstrumentationLibrary)
	for _, ilc := range initialConfig.InstrumentationLibraryConfigs {
		libID := auto.InstrumentationLibraryID{
			InstrumentedPkg: ilc.InstrumentationLibraryId.InstrumentationLibraryName,
			SpanKind:        common.SpanKindOdigosToOtel(ilc.InstrumentationLibraryId.SpanKind),
		}
		var tracesEnabled *bool
		if ilc.TraceConfig != nil {
			tracesEnabled = ilc.TraceConfig.Enabled
		}
		ic.InstrumentationLibraryConfigs[libID] = auto.InstrumentationLibrary{
			TracesEnabled: tracesEnabled,
		}
	}

	// TODO: take sampling config from the CR
	ic.Sampler = auto.DefaultSampler()
	return ic, nil
}

// extractWorkloadName extracts the workload name from resource attributes
// It checks for deployment, statefulset, or daemonset names in order
func extractWorkloadName(attrs []attribute.KeyValue) string {
	// Check for deployment name first
	for _, attr := range attrs {
		if attr.Key == attribute.Key("k8s.deployment.name") {
			return attr.Value.AsString()
		}
	}

	// Check for statefulset name
	for _, attr := range attrs {
		if attr.Key == attribute.Key("k8s.statefulset.name") {
			return attr.Value.AsString()
		}
	}

	// Check for daemonset name
	for _, attr := range attrs {
		if attr.Key == attribute.Key("k8s.daemonset.name") {
			return attr.Value.AsString()
		}
	}

	return ""
}

// extractPodName extracts the pod name from resource attributes
func extractPodName(attrs []attribute.KeyValue) string {
	for _, attr := range attrs {
		if attr.Key == attribute.Key("k8s.pod.name") {
			return attr.Value.AsString()
		}
	}
	return ""
}

// extractNamespace extracts the namespace from resource attributes
func extractNamespace(attrs []attribute.KeyValue) string {
	for _, attr := range attrs {
		if attr.Key == attribute.Key("k8s.namespace.name") {
			return attr.Value.AsString()
		}
	}
	return ""
}
