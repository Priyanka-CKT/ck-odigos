package instrumentlang

import (
	"fmt"

	"github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/odiglet/pkg/env"
	"github.com/odigos-io/odigos/odiglet/pkg/instrumentation/consts"
	"k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

const (
	otelResourceAttributesEnvVar  = "OTEL_RESOURCE_ATTRIBUTES"
	otelResourceAttrPattern       = "service.name=%s,odigos.device=java"
	javaToolOptionsEnvVar         = "JAVA_TOOL_OPTIONS"
	javaOptsEnvVar                = "JAVA_OPTS"
	javaOtlpEndpointEnvVar        = "OTEL_EXPORTER_OTLP_ENDPOINT"
	javaOtlpProtocolEnvVar        = "OTEL_EXPORTER_OTLP_PROTOCOL"
	javaOtelLogsExporterEnvVar    = "OTEL_LOGS_EXPORTER"
	javaOtelMetricsExporterEnvVar = "OTEL_METRICS_EXPORTER"
	javaOtelTracesExporterEnvVar  = "OTEL_TRACES_EXPORTER"
	javaOtelTracesSamplerEnvVar   = "OTEL_TRACES_SAMPLER"
)

func Java(deviceId string, enabledSignals map[common.ObservabilitySignal]struct{}) *v1beta1.ContainerAllocateResponse {
	otlpEndpoint := fmt.Sprintf("http://%s:%d", env.Current.NodeIP, consts.OTLPPort)

	// Use the correct agent jar file name
	javaAgentPath := "/var/odigos/java/ck-agent-universal.jar"
	javaOptsVal := fmt.Sprintf("-javaagent:%s", javaAgentPath)
	javaToolOptionsVal := fmt.Sprintf("-javaagent:%s", javaAgentPath)

	logsExporter := "none"
	metricsExporter := "none"
	tracesExporter := "none"

	// Set the values based on the signals exists in the map
	if _, ok := enabledSignals[common.LogsObservabilitySignal]; ok {
		logsExporter = "otlp"
	}
	if _, ok := enabledSignals[common.MetricsObservabilitySignal]; ok {
		metricsExporter = "otlp"
	}
	if _, ok := enabledSignals[common.TracesObservabilitySignal]; ok {
		tracesExporter = "otlp"
	}

	// If no signals are enabled, enable traces by default
	if logsExporter == "none" && metricsExporter == "none" && tracesExporter == "none" {
		tracesExporter = "otlp"
	}

	return &v1beta1.ContainerAllocateResponse{
		Envs: map[string]string{
			otelResourceAttributesEnvVar:  fmt.Sprintf(otelResourceAttrPattern, deviceId),
			javaToolOptionsEnvVar:         javaToolOptionsVal,
			javaOptsEnvVar:                javaOptsVal,
			javaOtlpEndpointEnvVar:        otlpEndpoint,
			javaOtlpProtocolEnvVar:        "grpc",
			javaOtelLogsExporterEnvVar:    logsExporter,
			javaOtelMetricsExporterEnvVar: metricsExporter,
			javaOtelTracesExporterEnvVar:  tracesExporter,
			javaOtelTracesSamplerEnvVar:   "always_on",
		},
		Mounts: []*v1beta1.Mount{
			{
				ContainerPath: "/var/odigos/java",
				HostPath:      "/var/odigos/java",
				ReadOnly:      true,
			},
		},
	}
}
