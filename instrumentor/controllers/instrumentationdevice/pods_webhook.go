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
const ckPgEndpointEnvVarName = "CK_PG_ENDPOINT"
const ckMetricsEndpointEnvVarName = "CK_METRICS_ENDPOINT"
const ckAppNameEnvVarName = "CK_APP_NAME"
const appNameEnvVarName = "APP_NAME"

// Default values if environment variables are not set
const defaultNexusEndpoint = "https://api.codekarma.tech/nexus/test"
const defaultPgEndpoint = "https://api.codekarma.tech/prometheus"
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
	log.FromContext(ctx).Info("Defaulting pod")
	log.FromContext(ctx).Info("Injecting environment variables for pod",
		"pod", obj.GetObjectKind().GroupVersionKind().Kind)
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return fmt.Errorf("expected a Pod but got a %T", obj)
	}

	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}

	serviceName, podWorkload := p.getServiceNameForEnv(ctx, pod)

	// Inject ODIGOS environment variables into all containers
	injectOdigosEnvVars(pod, podWorkload, serviceName)

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

	workloadObj, err := workload.GetWorkloadObject(ctx, client.ObjectKey{Namespace: podWorkload.Namespace, Name: podWorkload.Name}, podWorkload.Kind, p.Client)
	if err != nil {
		logger.Error(err, "failed to get workload object from cache. cannot check for workload annotation. using workload name as OTEL_SERVICE_NAME")
		return &podWorkload.Name, podWorkload
	}
	resolvedServiceName := workload.ExtractServiceNameFromAnnotations(workloadObj.GetAnnotations(), podWorkload.Name)
	return &resolvedServiceName, podWorkload
}

func injectOdigosEnvVars(pod *corev1.Pod, podWorkload *workload.PodWorkload, serviceName *string) {

	logger := log.FromContext(context.Background())
	logger.Info("Injecting environment variables for pod",
		"pod", pod.Name,
		"namespace", pod.Namespace,
		"serviceName", serviceName,
		"workload", podWorkload)
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
			Name:  ckPgEndpointEnvVarName,
			Value: getEnvWithDefault(ckPgEndpointEnvVarName, defaultPgEndpoint),
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

	var serviceNameEnv *corev1.EnvVar
	if serviceName != nil {
		serviceNameEnv = &corev1.EnvVar{
			Name:  otelServiceNameEnvVarName,
			Value: *serviceName,
		}
	}

	for i := range pod.Spec.Containers {
		container := &pod.Spec.Containers[i]

		pl, otelsdk, found := containerutils.GetLanguageAndOtelSdk(container)
		if !found {
			continue
		}

		// Check if the environment variables are already present, if so skip inject them again.
		if envVarsExist(container.Env, commonEnvVars) {
			continue
		}

		containerNameEnv := corev1.EnvVar{
			Name:  k8sconsts.OdigosEnvVarContainerName,
			Value: container.Name,
		}

		// Set CK_APP_NAME based on APP_NAME or deployment name
		appNameEnv := getAppNameEnv(container.Env, podWorkload)
		if appNameEnv != nil {
			container.Env = append(container.Env, *appNameEnv)
		}
		// Log APP_NAME environment variable details
		log.FromContext(context.Background()).Info(
			"APP_NAME environment variable details",
			"container", container.Name,
			"appNameEnv", appNameEnv,
			"existingEnv", container.Env,
		)

		resourceAttributes := getResourceAttributes(podWorkload, container.Name)
		resourceAttributesEnvValue := getResourceAttributesEnvVarValue(resourceAttributes)

		container.Env = append(container.Env, append(commonEnvVars, containerNameEnv)...)

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

	for _, commonEnvVar := range commonEnvVars {
		if _, exists := envMap[commonEnvVar.Name]; exists { // Checking if key exists
			return true
		}
	}
	return false
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
