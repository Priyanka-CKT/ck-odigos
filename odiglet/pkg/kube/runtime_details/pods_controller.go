package runtime_details

import (
	"context"
	"time"

	odigosv1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	"github.com/odigos-io/odigos/common"
	k8sutils "github.com/odigos-io/odigos/k8sutils/pkg/utils"
	"github.com/odigos-io/odigos/k8sutils/pkg/workload"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type PodsReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// the clientset is used to interact with the k8s API directly,
	// without pulling in specific objects into the controller runtime cache
	// which can be expensive (memory and CPU)
	Clientset *kubernetes.Clientset
}

// We need to apply runtime details detection for a new running pod in the following cases:
// 1. When a new workload generation is applied, the runtime details might be changed (different env, versions, etc).
// 2. When a source is added, but there are no running pods yet. When the first pod starts running, this is chance to apply runtime details detection.
func (p *PodsReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	var pod corev1.Pod
	err := p.Client.Get(ctx, request.NamespacedName, &pod)
	if err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	podWorkload, err := p.getPodWorkloadObject(ctx, &pod)
	if err != nil {
		logger.Error(err, "error getting pod workload object")
		return reconcile.Result{}, err
	}
	if podWorkload == nil {
		// pod is not managed by a workload, no runtime details detection needed
		return reconcile.Result{}, nil
	}

	// get instrumentation config for the pod to check if it is instrumented or not
	instrumentationConfigName := workload.CalculateWorkloadRuntimeObjectName(podWorkload.Name, podWorkload.Kind)
	instrumentationConfig := odigosv1.KarmaInstrumentationConfig{}
	err = p.Client.Get(ctx, client.ObjectKey{Name: instrumentationConfigName, Namespace: podWorkload.Namespace}, &instrumentationConfig)
	if err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}
	podGeneration, err := GetPodGeneration(ctx, p.Clientset, &pod)
	if err != nil {
		return reconcile.Result{}, err
	}

	// prevent runtime inspection on pods for which we already have the runtime details for this generation
	// Exception: if instrumentation config contains unknown language (not unsupported like Python)
	failedToGetPodGeneration := podGeneration == 0
	isNewPodGeneration := podGeneration > instrumentationConfig.Status.ObservedWorkloadGeneration
	instrumentedConfigContainUnknown := InstrumentationConfigContainsUnknownLanguage(instrumentationConfig)

	shouldSkipDetection := failedToGetPodGeneration || (!isNewPodGeneration && !instrumentedConfigContainUnknown)

	if shouldSkipDetection {
		logger.V(3).Info("skipping redundant runtime details detection since generation is not newer", "name", request.Name, "namespace", request.Namespace, "currentPodGeneration", podGeneration, "observedWorkloadGeneration", instrumentationConfig.Status.ObservedWorkloadGeneration)
		return reconcile.Result{}, nil
	}

	codekarmaConfig, err := k8sutils.GetCurrentCodekarmaConfig(ctx, p.Client)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Perform runtime inspection once we know the pod is newer that the latest runtime inspection performed and saved.
	logger.V(0).Info("DEBUG: Starting runtime inspection for pod", "name", request.Name, "namespace", request.Namespace, "generation", podGeneration)
	
	runtimeResults, err := runtimeInspection([]corev1.Pod{pod}, codekarmaConfig.IgnoredContainers)
	if err != nil {
		logger.Error(err, "DEBUG: Runtime inspection failed")
		return reconcile.Result{}, err
	}

	logger.V(0).Info("DEBUG: Runtime inspection completed", "resultCount", len(runtimeResults))
	for i, result := range runtimeResults {
		logger.V(0).Info("DEBUG: Detected language in runtime result", "index", i, "container", result.ContainerName, "language", result.Language)
	}

	// Check if current status already has a supported language - if so, don't overwrite with unsupported
	// This prevents race conditions where pods_controller overwrites Java (from instrumentationconfigs_controller) with Python
	logger.V(0).Info("DEBUG: Checking current InstrumentationConfig status", "statusCount", len(instrumentationConfig.Status.RuntimeDetailsByContainer))
	for i, containerDetails := range instrumentationConfig.Status.RuntimeDetailsByContainer {
		logger.V(0).Info("DEBUG: Current status container", "index", i, "container", containerDetails.ContainerName, "language", containerDetails.Language)
	}

	currentHasSupportedLanguage := false
	for _, containerDetails := range instrumentationConfig.Status.RuntimeDetailsByContainer {
		if isSupportedLanguage(containerDetails.Language) {
			currentHasSupportedLanguage = true
			logger.V(0).Info("DEBUG: Current status has supported language", "language", containerDetails.Language)
			break
		}
	}

	newHasSupportedLanguage := false
	for _, containerDetails := range runtimeResults {
		if isSupportedLanguage(containerDetails.Language) {
			newHasSupportedLanguage = true
			logger.V(0).Info("DEBUG: New detection has supported language", "language", containerDetails.Language)
			break
		}
	}

	logger.V(0).Info("DEBUG: Language check results", "currentHasSupported", currentHasSupportedLanguage, "newHasSupported", newHasSupportedLanguage)

		// Only update if:
		// 1. New detection has supported language (Java/Go), OR
		// 2. Current doesn't have supported language (can update with anything)
		// This prevents overwriting Java with Python
		if newHasSupportedLanguage || !currentHasSupportedLanguage {
			logger.V(0).Info("DEBUG: Decision is to UPDATE", "reason", func() string {
				if newHasSupportedLanguage {
					return "new has supported language"
				}
				return "current doesn't have supported language"
			}())

			err = persistRuntimeDetailsToInstrumentationConfig(ctx, p.Client, &instrumentationConfig, odigosv1.KarmaInstrumentationConfigStatus{
				RuntimeDetailsByContainer:  runtimeResults,
				ObservedWorkloadGeneration: podGeneration,
			})
			if err != nil {
				logger.Error(err, "DEBUG: Failed to persist runtime details to InstrumentationConfig")
				return reconcile.Result{}, err
			}

			detectedLang := "unknown"
			if len(runtimeResults) > 0 {
				detectedLang = string(runtimeResults[0].Language)
			}
			logger.V(0).Info("✅ Completed runtime details detection for a new running pod - UPDATED", "name", request.Name, "namespace", request.Namespace, "generation", podGeneration, "language", detectedLang)
		} else {
			currentLang := "unknown"
			if len(instrumentationConfig.Status.RuntimeDetailsByContainer) > 0 {
				currentLang = string(instrumentationConfig.Status.RuntimeDetailsByContainer[0].Language)
			}
			newLang := "unknown"
			if len(runtimeResults) > 0 {
				newLang = string(runtimeResults[0].Language)
			}
			
			// Special case: If current has supported language but new detection only found unsupported,
			// and this is a pod restart (generation increased), retry after a delay to catch Java startup
			// But only retry if we haven't already updated the observed generation (prevents infinite retries)
			if currentHasSupportedLanguage && !newHasSupportedLanguage && podGeneration > instrumentationConfig.Status.ObservedWorkloadGeneration {
				logger.V(0).Info("🔄 Pod restart detected - retrying language detection after delay to catch Java startup", 
					"name", request.Name, "namespace", request.Namespace, 
					"currentLanguage", currentLang, "newLanguage", newLang,
					"podGeneration", podGeneration, "observedGeneration", instrumentationConfig.Status.ObservedWorkloadGeneration)
				return reconcile.Result{RequeueAfter: 10 * time.Second}, nil
			}
			
			logger.V(0).Info("⏭️  Skipping runtime details update - current has supported language, new detection has unsupported - SKIPPED", "name", request.Name, "namespace", request.Namespace, "currentLanguage", currentLang, "newLanguage", newLang)
		}

	return reconcile.Result{}, nil
}

func (p *PodsReconciler) getPodWorkloadObject(ctx context.Context, pod *corev1.Pod) (*workload.PodWorkload, error) {
	for _, owner := range pod.OwnerReferences {
		workloadName, workloadKind, err := workload.GetWorkloadFromOwnerReference(owner)
		if err != nil {
			return nil, workload.IgnoreErrorKindNotSupported(err)
		}

		return &workload.PodWorkload{
			Name:      workloadName,
			Kind:      workloadKind,
			Namespace: pod.Namespace,
		}, nil
	}

	// Pod does not necessarily have to be managed by a controller
	return nil, nil
}

func InstrumentationConfigContainsUnknownLanguage(config odigosv1.KarmaInstrumentationConfig) bool {
	for _, containerDetails := range config.Status.RuntimeDetailsByContainer {
		if containerDetails.Language == common.UnknownProgrammingLanguage {
			return true
		}
	}
	return false
}
