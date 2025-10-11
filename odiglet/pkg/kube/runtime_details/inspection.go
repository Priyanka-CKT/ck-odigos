package runtime_details

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/odigos-io/odigos/procdiscovery/pkg/libc"

	procdiscovery "github.com/odigos-io/odigos/procdiscovery/pkg/process"

	"github.com/odigos-io/odigos/odiglet/pkg/process"

	k8sutils "github.com/odigos-io/odigos/k8sutils/pkg/utils"

	"github.com/go-logr/logr"
	odigosv1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	"github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/common/utils"
	"github.com/odigos-io/odigos/k8sutils/pkg/workload"
	kubeutils "github.com/odigos-io/odigos/odiglet/pkg/kube/utils"
	"github.com/odigos-io/odigos/odiglet/pkg/log"
	"github.com/odigos-io/odigos/procdiscovery/pkg/inspectors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var errNoPodsFound = errors.New("no pods found")

func ignoreNoPodsFoundError(err error) error {
	if err.Error() == errNoPodsFound.Error() {
		return nil
	}
	return err
}

func inspectRuntimesOfRunningPods(ctx context.Context, logger *logr.Logger, labels map[string]string,
	kubeClient client.Client, scheme *runtime.Scheme, object client.Object) error {
	pods, err := kubeutils.GetRunningPods(ctx, labels, object.GetNamespace(), kubeClient)
	if err != nil {
		logger.Error(err, "error fetching running pods")
		return err
	}

	if len(pods) == 0 {
		return errNoPodsFound
	}

	codekarmaConfig, err := k8sutils.GetCurrentCodekarmaConfig(ctx, kubeClient)
	if err != nil {
		logger.Error(err, "failed to get odigos config")
		return err
	}

	runtimeResults, err := runtimeInspection(pods, codekarmaConfig.IgnoredContainers)
	if err != nil {
		logger.Error(err, "error inspecting pods")
		return err
	}

	err = persistRuntimeResults(ctx, runtimeResults, object, kubeClient, scheme)
	if err != nil {
		logger.Error(err, "error persisting runtime results")
		return err
	}

	return nil
}

func runtimeInspection(pods []corev1.Pod, ignoredContainers []string) ([]odigosv1.RuntimeDetailsByContainer, error) {
	resultsMap := make(map[string]odigosv1.RuntimeDetailsByContainer)
	for _, pod := range pods {
		for _, container := range pod.Spec.Containers {

			// Skip ignored containers, but label them as ignored
			if utils.IsItemIgnored(container.Name, ignoredContainers) {
				resultsMap[container.Name] = odigosv1.RuntimeDetailsByContainer{
					ContainerName: container.Name,
					Language:      common.IgnoredProgrammingLanguage,
				}
				continue
			}

		processes, err := process.FindAllInContainer(string(pod.UID), container.Name)
		if err != nil {
			log.Logger.Error(err, "failed to find processes in pod container", "pod", pod.Name, "container", container.Name, "namespace", pod.Namespace)
			return nil, err
		}
		
		log.Logger.V(0).Info("DEBUG INSPECTION: Found processes in pod container", "pod", pod.Name, "container", container.Name, "namespace", pod.Namespace, "totalProcesses", len(processes))
		for i, proc := range processes {
			cmdLinePreview := proc.CmdLine
			if len(cmdLinePreview) > 100 {
				cmdLinePreview = cmdLinePreview[:100] + "..."
			}
			log.Logger.V(0).Info("DEBUG INSPECTION: Process details", "pod", pod.Name, "index", i, "pid", proc.ProcessID, "exeName", proc.ExeName, "cmdLine", cmdLinePreview)
		}
		
		if len(processes) == 0 {
			log.Logger.V(0).Info("no processes found in pod container", "pod", pod.Name, "container", container.Name, "namespace", pod.Namespace)
			continue
		}

			programLanguageDetails := common.ProgramLanguageDetails{Language: common.UnknownProgrammingLanguage}
			var inspectProc *procdiscovery.Details

			// First, track any unsupported language as fallback (for UI display)
			var fallbackLanguageDetails *common.ProgramLanguageDetails
			var fallbackProc *procdiscovery.Details

			// Iterate through all processes and find supported languages (Java or Go)
			// If no supported language found, we'll use an unsupported one for UI display
			log.Logger.V(0).Info("DEBUG INSPECTION: Starting language detection loop", "pod", pod.Name, "container", container.Name, "processCount", len(processes))
			for i, proc := range processes {
				containerURL := kubeutils.GetPodExternalURL(pod.Status.PodIP, container.Ports)
				detectedLang, err := inspectors.DetectLanguage(proc, containerURL)
				
				if err != nil {
					log.Logger.V(0).Info("DEBUG INSPECTION: Language detection error", "pod", pod.Name, "index", i, "pid", proc.ProcessID, "error", err.Error())
					continue
				}
				
				log.Logger.V(0).Info("DEBUG INSPECTION: Process language detected", "pod", pod.Name, "index", i, "pid", proc.ProcessID, "language", detectedLang.Language)
				
				if err == nil && detectedLang.Language != common.UnknownProgrammingLanguage {
					// Check if this is a supported language (Java or Go)
					if isSupportedLanguage(detectedLang.Language) {
						log.Logger.V(0).Info("DEBUG INSPECTION: ✅ Found SUPPORTED language - will use this!", "pod", pod.Name, "container", container.Name, "language", detectedLang.Language, "pid", proc.ProcessID)
						// Found a supported language - use it immediately
						programLanguageDetails = detectedLang
						inspectProc = &proc
						break
					} else if fallbackLanguageDetails == nil {
						log.Logger.V(0).Info("DEBUG INSPECTION: ⚠️ Found UNSUPPORTED language - saving as fallback", "pod", pod.Name, "container", container.Name, "language", detectedLang.Language, "pid", proc.ProcessID)
						// Store first unsupported language as fallback (e.g., Python, Node.js)
						// This will be used if no Java/Go is found, so UI can show "detected but not supported"
						fallbackLanguageDetails = &detectedLang
						fallbackProc = &proc
					}
				}
			}
			
			log.Logger.V(0).Info("DEBUG INSPECTION: Language detection loop completed", "pod", pod.Name, "foundSupported", inspectProc != nil, "haveFallback", fallbackLanguageDetails != nil)

			// If no supported language was found but we have a fallback unsupported language,
			// use it so the UI can display "language not supported" instead of "unknown"
			if inspectProc == nil && fallbackLanguageDetails != nil {
				log.Logger.V(0).Info("DEBUG INSPECTION: Using fallback unsupported language", "pod", pod.Name, "container", container.Name, "language", fallbackLanguageDetails.Language)
				programLanguageDetails = *fallbackLanguageDetails
				inspectProc = fallbackProc
				log.Logger.V(0).Info("⚠️ no supported language found, using unsupported language for UI display", 
					"pod", pod.Name, 
					"container", container.Name, 
					"namespace", pod.Namespace, 
					"language", programLanguageDetails.Language)
			} else if inspectProc != nil {
				log.Logger.V(0).Info("DEBUG INSPECTION: ✅ Final result - using supported language", "pod", pod.Name, "container", container.Name, "language", programLanguageDetails.Language)
			} else {
				log.Logger.V(0).Info("DEBUG INSPECTION: ❌ Final result - no language detected at all", "pod", pod.Name, "container", container.Name)
			}

			envs := make([]odigosv1.EnvVar, 0)
			var detectedAgent *odigosv1.OtherAgent
			var libcType *common.LibCType

			if inspectProc == nil {
				log.Logger.V(0).Info("unable to detect supported language (Java or Go) for any process", "pod", pod.Name, "container", container.Name, "namespace", pod.Namespace, "totalProcesses", len(processes))
				programLanguageDetails.Language = common.UnknownProgrammingLanguage
			} else {
				if len(processes) > 1 {
					log.Logger.V(0).Info("multiple processes found in pod container, detected supported language", "pod", pod.Name, "container", container.Name, "namespace", pod.Namespace, "totalProcesses", len(processes), "detectedLanguage", programLanguageDetails.Language)
				}

				// Convert map to slice for k8s format
				envs = make([]odigosv1.EnvVar, 0, len(inspectProc.Environments.DetailedEnvs))

				for envName, envValue := range inspectProc.Environments.OverwriteEnvs {
					envs = append(envs, odigosv1.EnvVar{Name: envName, Value: envValue})
				}

				// Languages that can be detected using environment variables, e.g Python<>newrelic
				for envName := range inspectProc.Environments.DetailedEnvs {
					if otherAgentName, exists := procdiscovery.OtherAgentEnvs[envName]; exists {
						detectedAgent = &odigosv1.OtherAgent{Name: otherAgentName}
					}
				}
				// Languages that can be detected using command line Substrings, e.g. Java<>newrelic
				for otherAgentCmdSubstring, otherAgentName := range procdiscovery.OtherAgentCmdSubString {
					if strings.Contains(inspectProc.CmdLine, otherAgentCmdSubstring) {
						detectedAgent = &odigosv1.OtherAgent{Name: otherAgentName}
					}
				}

				// Inspecting libc type is expensive and not relevant for all languages
				if libc.ShouldInspectForLanguage(programLanguageDetails.Language) {
					typeFound, err := libc.InspectType(inspectProc)
					if err == nil {
						libcType = typeFound
					} else {
						log.Logger.Error(err, "error inspecting libc type", "pod", pod.Name, "container", container.Name, "namespace", pod.Namespace)
					}
				}
			}

			var runtimeVersion string
			if programLanguageDetails.RuntimeVersion != nil {
				runtimeVersion = programLanguageDetails.RuntimeVersion.String()
			}

			resultsMap[container.Name] = odigosv1.RuntimeDetailsByContainer{
				ContainerName:  container.Name,
				Language:       programLanguageDetails.Language,
				RuntimeVersion: runtimeVersion,
				EnvVars:        envs,
				OtherAgent:     detectedAgent,
				LibCType:       libcType,
			}
		}
	}

	results := make([]odigosv1.RuntimeDetailsByContainer, 0, len(resultsMap))
	for _, value := range resultsMap {
		results = append(results, value)
	}

	return results, nil
}

func persistRuntimeDetailsToInstrumentationConfig(ctx context.Context, kubeclient client.Client, instrumentationConfig *odigosv1.InstrumentationConfig, newStatus odigosv1.InstrumentationConfigStatus) error {

	// persist the runtime results into the status of the instrumentation config
	patchStatus := odigosv1.InstrumentationConfig{
		Status: newStatus,
	}
	patchData, err := json.Marshal(patchStatus)
	if err != nil {
		return err
	}
	err = kubeclient.Status().Patch(ctx, instrumentationConfig, client.RawPatch(types.MergePatchType, patchData), client.FieldOwner("odiglet-runtimedetails"))
	if err != nil {
		return err
	}

	return nil
}

func persistRuntimeResults(ctx context.Context, results []odigosv1.RuntimeDetailsByContainer, owner client.Object, kubeClient client.Client, scheme *runtime.Scheme) error {
	updatedIa := &odigosv1.InstrumentedApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workload.CalculateWorkloadRuntimeObjectName(owner.GetName(), owner.GetObjectKind().GroupVersionKind().Kind),
			Namespace: owner.GetNamespace(),
		},
	}

	// Check if all languages are supported before creating the InstrumentedApplication
	containsUnsupportedLanguage := false
	for _, result := range results {
		if !isSupportedLanguage(result.Language) {
			containsUnsupportedLanguage = true
			log.Logger.Info("Detected unsupported language, skipping instrumentation",
				"language", result.Language,
				"name", owner.GetName(),
				"namespace", owner.GetNamespace())
		}
	}

	// If only unsupported languages were detected, don't create the InstrumentedApplication
	if len(results) > 0 && containsUnsupportedLanguage {
		// We still want to create the InstrumentedApplication to track the detection,
		// but the UI will show that this application cannot be instrumented because
		// only Java and Go languages are supported
	}

	err := controllerutil.SetControllerReference(owner, updatedIa, scheme)
	if err != nil {
		log.Logger.Error(err, "Failed to set controller reference")
		return err
	}

	// Check if we should delay updating KarmaInstrumentedApplication Spec
	// If we only detected unsupported languages, we might be in a pod restart scenario
	// where Java hasn't started yet - delay the update to prevent wrong Spec
	shouldDelayUpdate := false
	for _, result := range results {
		if !isSupportedLanguage(result.Language) && result.Language != common.UnknownProgrammingLanguage {
			shouldDelayUpdate = true
			log.Logger.V(0).Info("DEBUG INSPECTION: Detected unsupported language, will delay KarmaInstrumentedApplication update to prevent wrong Spec", 
				"language", result.Language, "workload", owner.GetName(), "container", result.ContainerName)
			break
		}
	}
	
	var operationResult controllerutil.OperationResult
	if shouldDelayUpdate {
		log.Logger.V(0).Info("DEBUG INSPECTION: Skipping KarmaInstrumentedApplication Spec update - will retry later", 
			"workload", owner.GetName(), "reason", "unsupported language detected, likely pod restart")
		operationResult = controllerutil.OperationResultNone
	} else {
		operationResult, err = controllerutil.CreateOrPatch(ctx, kubeClient, updatedIa, func() error {
			updatedIa.Spec.RuntimeDetails = results
			return nil
		})
	}

	if err != nil {
		log.Logger.Error(err, "Failed to update runtime info", "name", owner.GetName(), "kind",
			owner.GetObjectKind().GroupVersionKind().Kind, "namespace", owner.GetNamespace())
	}

	if operationResult != controllerutil.OperationResultNone {
		log.Logger.V(0).Info("updated runtime info", "result", operationResult, "name", owner.GetName(), "kind",
			owner.GetObjectKind().GroupVersionKind().Kind, "namespace", owner.GetNamespace())
	}
	return nil
}

// isSupportedLanguage returns true if the language is supported for instrumentation
func isSupportedLanguage(language common.ProgrammingLanguage) bool {
	switch language {
	case common.JavaProgrammingLanguage, common.GoProgrammingLanguage:
		return true
	default:
		return false
	}
}

func GetRuntimeDetails(ctx context.Context, kubeClient client.Client, podWorkload *workload.PodWorkload) (*odigosv1.InstrumentedApplication, error) {
	instrumentedApplicationName := workload.CalculateWorkloadRuntimeObjectName(podWorkload.Name, podWorkload.Kind)

	var runtimeDetails odigosv1.InstrumentedApplication
	err := kubeClient.Get(ctx, client.ObjectKey{
		Namespace: podWorkload.Namespace,
		Name:      instrumentedApplicationName,
	}, &runtimeDetails)
	if err != nil {
		return nil, err
	}

	return &runtimeDetails, nil
}
