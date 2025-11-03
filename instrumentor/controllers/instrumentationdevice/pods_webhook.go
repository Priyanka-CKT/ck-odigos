package instrumentationdevice

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/odigos-io/odigos/common"
	k8sconsts "github.com/odigos-io/odigos/k8sutils/pkg/consts"
	containerutils "github.com/odigos-io/odigos/k8sutils/pkg/container"
	"github.com/odigos-io/odigos/k8sutils/pkg/workload"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const otelServiceNameEnvVarName = "OTEL_SERVICE_NAME"
const otelResourceAttributesEnvVarName = "OTEL_RESOURCE_ATTRIBUTES"
const ckClusterNameEnvVarName = "CK_CLUSTER_NAME"
const ckNexusEndpointEnvVarName = "CK_NEXUS_ENDPOINT"
const ckMetricsEndpointEnvVarName = "CK_METRICS_ENDPOINT"
const ckEndpointEnvVarName = "CK_ENDPOINT"
const ckApiKeyEnvVarName = "CK_API_KEY"
const ckAppNameEnvVarName = "CK_APP_NAME"
const ckInstPackagesEnvVarName = "CK_INST_PACKAGES"
const appNameEnvVarName = "APP_NAME"

// Default values if environment variables are not set
const defaultNexusEndpoint = "https://api.codekarma.tech/nexus/test"
const defaultMetricsEndpoint = "https://api.codekarma.tech/metrics"

type resourceAttribute struct {
	Key   attribute.Key
	Value string
}

type PodsWebhook struct {
	client.Client
}

var _ webhook.CustomDefaulter = &PodsWebhook{}

func (p *PodsWebhook) Default(ctx context.Context, obj runtime.Object) error {
	logger := log.FromContext(ctx)

	logger.Info("🚀 WEBHOOK TRIGGERED: Starting webhook processing")

	pod, ok := obj.(*corev1.Pod)
	if !ok {
		logger.Error(fmt.Errorf("expected a Pod but got a %T", obj), "Invalid object type")
		return fmt.Errorf("expected a Pod but got a %T", obj)
	}

	logger.Info("🔍 WEBHOOK DEBUG: Pod details",
		"pod", pod.Name,
		"namespace", pod.Namespace,
		"hasLabels", pod.Labels != nil,
		"labelCount", len(pod.Labels),
		"instrumentationLabel", pod.Labels["codekarma.tech/inject-instrumentation"],
		"containerCount", len(pod.Spec.Containers),
	)

	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}

	serviceName, podWorkload := p.getServiceNameForEnv(ctx, pod)

	logger.Info("🔍 WEBHOOK DEBUG: Service name and workload",
		"pod", pod.Name,
		"namespace", pod.Namespace,
		"serviceName", serviceName,
		"podWorkload", podWorkload,
	)

	// Inject ODIGOS environment variables into all containers
	injectOdigosEnvVars(pod, podWorkload, serviceName)

	logger.Info("✅ WEBHOOK COMPLETED: Finished processing pod",
		"pod", pod.Name,
		"namespace", pod.Namespace,
	)

	return nil
}

// checks for the service name on the annotation, or fallback to the workload name
func (p *PodsWebhook) getServiceNameForEnv(ctx context.Context, pod *corev1.Pod) (*string, *workload.PodWorkload) {

	logger := log.FromContext(ctx)

	podWorkload, err := workload.PodWorkloadObject(ctx, pod)
	if err != nil {
		logger.Error(err, "failed to extract pod workload details from pod. skipping OTEL_SERVICE_NAME injection")
		return nil, nil
	}

	// CRITICAL FIX: Check if podWorkload is nil before using it
	if podWorkload == nil {
		logger.Error(fmt.Errorf("podWorkload is nil"), "podWorkload is nil, cannot proceed")
		return nil, nil
	}

	workloadObj, err := workload.GetWorkloadObject(ctx, client.ObjectKey{Namespace: podWorkload.Namespace, Name: podWorkload.Name}, podWorkload.Kind, p.Client)
	if err != nil {
		logger.Error(err, "failed to get workload object from cache. cannot check for workload annotation. using workload name as OTEL_SERVICE_NAME")
		return &podWorkload.Name, podWorkload
	}
	resolvedServiceName := workload.ExtractServiceNameFromAnnotations(workloadObj.GetAnnotations(), podWorkload.Name)
	return &resolvedServiceName, podWorkload
}

// validateAllJavaAgents checks if ALL agents in JAVA_TOOL_OPTIONS exist and removes invalid ones
// Uses both label presence and file existence for more accurate validation
func validateAllJavaAgents(javaToolOptions string, podLabels map[string]string) string {
	if javaToolOptions == "" {
		return ""
	}

	parts := strings.Split(javaToolOptions, " ")
	var validParts []string

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if strings.HasPrefix(part, "-javaagent:") {
			agentPath := strings.TrimPrefix(part, "-javaagent:")

			// Enhanced validation: Check both file existence AND label presence
			if shouldKeepJavaAgent(agentPath, podLabels) {
				validParts = append(validParts, part)
			}
			// Skip invalid agents - they will be removed
		} else {
			// Keep non-javaagent parts (like -Xmx512m, -XX:+UseG1GC, etc.)
			validParts = append(validParts, part)
		}
	}

	return strings.Join(validParts, " ")
}

// shouldKeepJavaAgent determines if a Java agent should be kept based on label presence only
// File existence check is removed to avoid race conditions with device plugin mounting
// JVM will handle missing files gracefully at startup
func shouldKeepJavaAgent(agentPath string, podLabels map[string]string) bool {
	// Enhanced logic: Check label presence for specific agents
	if strings.Contains(agentPath, "/var/odigos/") {
		// For Odigos agents:
		// - If Odigos label is present, keep it
		// - If Odigos label is NOT present, remove it
		if odigosLabel, exists := podLabels["odigos.io/inject-instrumentation"]; exists && odigosLabel == "true" {
			return true
		}
		// If Odigos label is not present, remove the Odigos agent
		return false
	}

	if strings.Contains(agentPath, "/var/codekarma/") {
		// For CodeKarma agents:
		// - If CodeKarma label is present, keep it
		// - If CodeKarma label is NOT present, remove it
		if codekarmaLabel, exists := podLabels["codekarma.tech/inject-instrumentation"]; exists && codekarmaLabel == "true" {
			return true
		}
		// If CodeKarma label is not present, remove the CodeKarma agent
		return false
	}

	return true
}

// agentExists checks if the agent file exists on the filesystem
func agentExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func injectOdigosEnvVars(pod *corev1.Pod, podWorkload *workload.PodWorkload, serviceName *string) {
	logger := log.FromContext(context.Background())

	logger.Info("🚀 DEBUG WEBHOOK: Starting env var injection",
		"pod", pod.Name,
		"namespace", pod.Namespace,
		"hasLabels", pod.Labels != nil,
		"instrumentationLabel", pod.Labels["codekarma.tech/inject-instrumentation"],
		"podWorkload", podWorkload,
		"serviceName", serviceName,
	)

	// CRITICAL FIX: Handle nil podWorkload
	if podWorkload == nil {
		logger.Info("⚠️ DEBUG WEBHOOK: podWorkload is nil, skipping env var injection",
			"pod", pod.Name,
			"namespace", pod.Namespace,
		)
		return
	}

	// Common environment variables that do not change across containers
	commonEnvVars := []corev1.EnvVar{
		{
			Name: k8sconsts.OdigosEnvVarNamespace,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		},
		{
			Name: k8sconsts.OdigosEnvVarPodName,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		{
			Name: "CK_NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		},
		{
			Name: "CK_POD_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		{
			Name:  ckNexusEndpointEnvVarName,
			Value: getEnvWithDefault(ckNexusEndpointEnvVarName, defaultNexusEndpoint),
		},
		{
			Name:  ckMetricsEndpointEnvVarName,
			Value: getEnvWithDefault(ckMetricsEndpointEnvVarName, defaultMetricsEndpoint),
		},
	}

	// Add CK_CLUSTER_NAME if it's available
	clusterName := os.Getenv(ckClusterNameEnvVarName)
	if clusterName != "" {
		commonEnvVars = append(commonEnvVars, corev1.EnvVar{
			Name:  ckClusterNameEnvVarName,
			Value: clusterName,
		})
	}

	// Add CK_ENDPOINT if it's available and not empty
	ckEndpoint := os.Getenv(ckEndpointEnvVarName)
	if ckEndpoint != "" {
		commonEnvVars = append(commonEnvVars, corev1.EnvVar{
			Name:  ckEndpointEnvVarName,
			Value: ckEndpoint,
		})
	}

	// Add CK_INST_PACKAGES if it's available and not empty
	ckInstPackages := os.Getenv(ckInstPackagesEnvVarName)
	if ckInstPackages != "" {
		commonEnvVars = append(commonEnvVars, corev1.EnvVar{
			Name:  ckInstPackagesEnvVarName,
			Value: ckInstPackages,
		})
	}
	// Add CK_API_KEY if it's available and not empty
	ckApiKey := os.Getenv(ckApiKeyEnvVarName)
	if ckApiKey != "" {
		commonEnvVars = append(commonEnvVars, corev1.EnvVar{
			Name:  ckApiKeyEnvVarName,
			Value: ckApiKey,
		})
	}

	// Log the final commonEnvVars that will be checked/injected
	commonEnvVarNames := []string{}
	for _, env := range commonEnvVars {
		commonEnvVarNames = append(commonEnvVarNames, env.Name)
	}
	logger.Info("🔍 DEBUG WEBHOOK: Built commonEnvVars",
		"pod", pod.Name,
		"namespace", pod.Namespace,
		"count", len(commonEnvVars),
		"varNames", commonEnvVarNames,
	)

	var serviceNameEnv *corev1.EnvVar
	if serviceName != nil {
		serviceNameEnv = &corev1.EnvVar{
			Name:  otelServiceNameEnvVarName,
			Value: *serviceName,
		}
	}

	logger.Info("🔍 DEBUG WEBHOOK: Processing containers",
		"pod", pod.Name,
		"namespace", pod.Namespace,
		"containerCount", len(pod.Spec.Containers),
	)

	for i := range pod.Spec.Containers {
		container := &pod.Spec.Containers[i]

		logger.Info("🔍 DEBUG WEBHOOK: Processing container",
			"pod", pod.Name,
			"namespace", pod.Namespace,
			"container", container.Name,
			"containerIndex", i,
			"hasResources", container.Resources.Limits != nil,
			"resourceCount", len(container.Resources.Limits),
		)

		pl, otelsdk, found := containerutils.GetLanguageAndOtelSdk(container)
		if !found {
			logger.Info("⚠️ DEBUG WEBHOOK: Device not found, skipping container",
				"pod", pod.Name,
				"namespace", pod.Namespace,
				"container", container.Name,
				"reason", "No device resources found",
			)
			continue
		}

		logger.Info("✅ DEBUG WEBHOOK: Device found, checking env vars",
			"pod", pod.Name,
			"namespace", pod.Namespace,
			"container", container.Name,
			"language", pl,
			"sdk", otelsdk,
			"commonEnvVarsCount", len(commonEnvVars),
		)

		// Check if the environment variables are already present, if so skip inject them again.
		if envVarsExist(container.Env, commonEnvVars) {
			logger.Info("⏭️ DEBUG WEBHOOK: All common env vars already exist, skipping injection",
				"pod", pod.Name,
				"namespace", pod.Namespace,
				"container", container.Name,
				"existingEnvCount", len(container.Env),
			)
			continue
		}

		logger.Info("✅ DEBUG WEBHOOK: Will inject env vars",
			"pod", pod.Name,
			"namespace", pod.Namespace,
			"container", container.Name,
			"commonEnvVarsCount", len(commonEnvVars),
			"existingEnvCount", len(container.Env),
		)

		containerNameEnv := corev1.EnvVar{
			Name:  k8sconsts.OdigosEnvVarContainerName,
			Value: container.Name,
		}

		resourceAttributes := getResourceAttributes(podWorkload, container.Name)
		resourceAttributesEnvValue := getResourceAttributesEnvVarValue(resourceAttributes)

		// Inject common env vars first
		container.Env = append(container.Env, append(commonEnvVars, containerNameEnv)...)

		logger.Info("✅ DEBUG WEBHOOK: Injected common env vars",
			"pod", pod.Name,
			"namespace", pod.Namespace,
			"container", container.Name,
			"injectedCount", len(commonEnvVars)+1,
			"totalEnvCount", len(container.Env),
		)

		// Set CK_APP_NAME based on APP_NAME or deployment name (after common vars)
		appNameEnv := getAppNameEnv(container.Env, podWorkload)
		if appNameEnv != nil {
			container.Env = append(container.Env, *appNameEnv)
		}
		// Log APP_NAME environment variable details
		logger.Info("🔍 DEBUG WEBHOOK: APP_NAME environment variable details",
			"pod", pod.Name,
			"namespace", pod.Namespace,
			"container", container.Name,
			"appNameEnv", appNameEnv,
			"totalEnvCount", len(container.Env),
		)

		// Validate and clean up JAVA_TOOL_OPTIONS for Java containers
		if pl == common.JavaProgrammingLanguage {
			for i, envVar := range container.Env {
				if envVar.Name == "JAVA_TOOL_OPTIONS" {
					// Get pod labels for enhanced validation
					podLabels := make(map[string]string)
					if pod.Labels != nil {
						podLabels = pod.Labels
					}

					validatedValue := validateAllJavaAgents(envVar.Value, podLabels)
					if validatedValue != envVar.Value {
						container.Env[i].Value = validatedValue
						logger.Info("🔧 DEBUG WEBHOOK: Cleaned up JAVA_TOOL_OPTIONS",
							"pod", pod.Name,
							"namespace", pod.Namespace,
							"container", container.Name,
							"original", envVar.Value,
							"validated", validatedValue,
						)
					}
					break
				}
			}
		}

		if serviceNameEnv != nil && shouldInjectServiceName(pl, otelsdk) {
			if !otelNameExists(container.Env) {
				container.Env = append(container.Env, *serviceNameEnv)
			}
			container.Env = append(container.Env, corev1.EnvVar{
				Name:  otelResourceAttributesEnvVarName,
				Value: resourceAttributesEnvValue,
			})
		}
	}
}

func envVarsExist(containerEnv []corev1.EnvVar, commonEnvVars []corev1.EnvVar) bool {
	envMap := make(map[string]struct{})
	for _, envVar := range containerEnv {
		envMap[envVar.Name] = struct{}{} // Inserting empty struct as value
	}

	// Check if ALL common env vars exist, not just ANY one
	// This ensures we don't skip injection if only some vars are present
	for _, commonEnvVar := range commonEnvVars {
		if _, exists := envMap[commonEnvVar.Name]; !exists {
			return false // At least one var is missing, need to inject
		}
	}
	return true // All common vars already exist, skip injection
}

func getWorkloadKindAttributeKey(podWorkload *workload.PodWorkload) attribute.Key {
	switch podWorkload.Kind {
	case workload.WorkloadKindDeployment:
		return semconv.K8SDeploymentNameKey
	case workload.WorkloadKindStatefulSet:
		return semconv.K8SStatefulSetNameKey
	case workload.WorkloadKindDaemonSet:
		return semconv.K8SDaemonSetNameKey
	}
	return attribute.Key("")
}

func getResourceAttributes(podWorkload *workload.PodWorkload, containerName string) []resourceAttribute {
	if podWorkload == nil {
		return []resourceAttribute{}
	}

	workloadKindKey := getWorkloadKindAttributeKey(podWorkload)
	return []resourceAttribute{
		{
			Key:   semconv.K8SContainerNameKey,
			Value: containerName,
		},
		{
			Key:   semconv.K8SNamespaceNameKey,
			Value: podWorkload.Namespace,
		},
		{
			Key:   workloadKindKey,
			Value: podWorkload.Name,
		},
	}
}

func getResourceAttributesEnvVarValue(ra []resourceAttribute) string {
	var attrs []string
	for _, a := range ra {
		attrs = append(attrs, fmt.Sprintf("%s=%s", a.Key, a.Value))
	}
	return strings.Join(attrs, ",")
}

func otelNameExists(containerEnv []corev1.EnvVar) bool {
	for _, envVar := range containerEnv {
		if envVar.Name == otelServiceNameEnvVarName {
			return true
		}
	}
	return false
}

// this is used to set the OTEL_SERVICE_NAME for programming languages and otel sdks that requires it.
// eBPF instrumentations sets the service name in code, thus it's not needed here.
// OpAMP sends the service name in the protocol, thus it's not needed here.
// We are only left with OSS Java and Dotnet that requires the OTEL_SERVICE_NAME to be set.
func shouldInjectServiceName(pl common.ProgrammingLanguage, otelsdk common.OtelSdk) bool {
	if pl == common.DotNetProgrammingLanguage {
		return true
	}
	if pl == common.JavaProgrammingLanguage && otelsdk.SdkTier == common.CommunityOtelSdkTier {
		return true
	}
	return false
}

// getEnvWithDefault returns the value of the environment variable or the default value if not set
func getEnvWithDefault(envVarName, defaultValue string) string {
	value := os.Getenv(envVarName)
	if value == "" {
		return defaultValue
	}
	return value
}

// getAppNameEnv returns a CK_APP_NAME environment variable based on APP_NAME or deployment name
func getAppNameEnv(containerEnv []corev1.EnvVar, podWorkload *workload.PodWorkload) *corev1.EnvVar {
	// First check if APP_NAME is already set in the container
	for _, envVar := range containerEnv {
		if envVar.Name == appNameEnvVarName {
			return &corev1.EnvVar{
				Name:  ckAppNameEnvVarName,
				Value: envVar.Value,
			}
		}
	}

	// If APP_NAME is not set and we have a deployment, use the deployment name
	if podWorkload != nil && podWorkload.Kind == workload.WorkloadKindDeployment {
		return &corev1.EnvVar{
			Name:  ckAppNameEnvVarName,
			Value: podWorkload.Name,
		}
	}

	return nil
}
