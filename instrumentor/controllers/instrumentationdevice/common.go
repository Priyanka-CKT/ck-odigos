package instrumentationdevice

import (
	"context"
	"errors"
	"fmt"

	odigosv1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	"github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/instrumentor/controllers/utils"
	"github.com/odigos-io/odigos/instrumentor/controllers/utils/versionsupport"
	"github.com/odigos-io/odigos/instrumentor/instrumentation"
	"github.com/odigos-io/odigos/instrumentor/sdks"
	"github.com/odigos-io/odigos/k8sutils/pkg/conditions"
	k8sprofiles "github.com/odigos-io/odigos/k8sutils/pkg/profiles"
	k8sutils "github.com/odigos-io/odigos/k8sutils/pkg/utils"
	"github.com/odigos-io/odigos/k8sutils/pkg/workload"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type ApplyInstrumentationDeviceReason string

const (
	ApplyInstrumentationDeviceReasonDataCollectionNotReady     ApplyInstrumentationDeviceReason = "DataCollectionNotReady"
	ApplyInstrumentationDeviceReasonNoRuntimeDetails           ApplyInstrumentationDeviceReason = "NoRuntimeDetails"
	ApplyInstrumentationDeviceReasonErrApplying                ApplyInstrumentationDeviceReason = "ErrorApplying"
	ApplyInstrumentationDeviceReasonErrRemoving                ApplyInstrumentationDeviceReason = "ErrorRemoving"
	ApplyInstrumentationDeviceReasonRuntimeVersionNotSupported ApplyInstrumentationDeviceReason = "RuntimeVersionNotSupported"
	ApplyInstrumentationDeviceReasonUnsupportedLanguage        ApplyInstrumentationDeviceReason = "UnsupportedLanguage"
)

const (
	appliedInstrumentationDeviceType = "AppliedInstrumentationDevice"
)

var (
	// can be overridden in tests
	GetDefaultSDKs = sdks.GetDefaultSDKs
)

func isDataCollectionReady(ctx context.Context, c client.Client) bool {
	// logger := log.FromContext(ctx)

	// nodeCollectorsGroup := odigosv1.CollectorsGroup{}
	// err := c.Get(ctx, client.ObjectKey{
	// 	Namespace: env.GetCurrentNamespace(),
	// 	Name:      odigosk8sconsts.OdigosNodeCollectorCollectorGroupName,
	// }, &nodeCollectorsGroup)

	// if err != nil {
	// 	if apierrors.IsNotFound(err) {
	// 		// if node collector is not yet created, then it is not ready
	// 		return false
	// 	} else {
	// 		logger.Error(err, "error getting node collector group, skipping instrumentation")
	// 		return false
	// 	}
	// }

	return true
}

func addInstrumentationDeviceToWorkload(ctx context.Context, kubeClient client.Client, runtimeDetails *odigosv1.KarmaInstrumentedApplication) (error, bool) {
	// devicePartiallyApplied is used to indicate that the instrumentation device was partially applied for some of the containers.
	devicePartiallyApplied := false
	deviceNotAppliedDueToPresenceOfAnotherAgent := false

	logger := log.FromContext(ctx)
	obj, err := getWorkloadObject(ctx, kubeClient, runtimeDetails)
	if err != nil {
		return err, false
	}

	podWorkload := workload.PodWorkload{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
		Kind:      workload.WorkloadKind(obj.GetObjectKind().GroupVersionKind().Kind),
	}

	// build an otel sdk map from instrumentation rules first, and merge it with the default otel sdk map
	// this way, we can override the default otel sdk with the instrumentation rules
	instrumentationRules := odigosv1.KarmaInstrumentationRuleList{}
	err = kubeClient.List(ctx, &instrumentationRules)
	if err != nil {
		return err, false
	}

	// default otel sdk map according to Odigos tier
	otelSdkToUse := GetDefaultSDKs()

	for i := range instrumentationRules.Items {
		instrumentationRule := &instrumentationRules.Items[i]
		if instrumentationRule.Spec.Disabled || instrumentationRule.Spec.OtelSdks == nil {
			// we only care about rules that have otel sdks configuration
			continue
		}

		participating := utils.IsWorkloadParticipatingInRule(podWorkload, instrumentationRule)
		if !participating {
			// filter rules that do not apply to the workload
			continue
		}

		for lang, otelSdk := range instrumentationRule.Spec.OtelSdks.OtelSdkByLanguage {
			// languages can override the default otel sdk or another rule.
			// there is not check or warning if a language is defined in multiple rules at the moment.
			otelSdkToUse[lang] = otelSdk
		}
	}

	result, err := controllerutil.CreateOrPatch(ctx, kubeClient, obj, func() error {
		podSpec, err := getPodSpecFromObject(obj)
		if err != nil {
			return err
		}

		// get the odigos configuration to check if agents can run concurrently
		// if the configuration is not found, we assume that agents can't run concurrently [default behavior]
		codekarmaConfiguration, err := k8sutils.GetCurrentCodekarmaConfig(ctx, kubeClient)
		if err != nil {
			return err
		}

		// User input <codekarmaConfiguration.AllowConcurrentAgents> prefered over the profile configuration
		agentsCanRunConcurrently := k8sprofiles.AgentsCanRunConcurrently(codekarmaConfiguration.Profiles)
		if codekarmaConfiguration.AllowConcurrentAgents != nil {
			agentsCanRunConcurrently = *codekarmaConfiguration.AllowConcurrentAgents
		}

		err, deviceApplied, deviceSkippedDueToOtherAgent := instrumentation.ApplyInstrumentationDevicesToPodTemplate(podSpec, runtimeDetails, otelSdkToUse, obj, logger, agentsCanRunConcurrently)
		if err != nil {
			return err
		}
		// if non of the devices were applied due to the presence of another agent, return an error.
		if !deviceApplied && deviceSkippedDueToOtherAgent {
			deviceNotAppliedDueToPresenceOfAnotherAgent = true
		}

		devicePartiallyApplied = deviceSkippedDueToOtherAgent && deviceApplied
		// If instrumentation device is applied successfully, add codekarma.tech/inject-instrumentation label to enable the webhook
		if deviceApplied {
			instrumentation.SetInjectInstrumentationLabel(podSpec)
		}

		return nil
	})

	// if non of the devices were applied due to the presence of another agent, return an error.
	if deviceNotAppliedDueToPresenceOfAnotherAgent {
		return fmt.Errorf("device not added to any container due to the presence of another agent"), false
	}

	if err != nil {
		return err, false
	}

	modified := result != controllerutil.OperationResultNone
	if modified {
		logger.V(0).Info("added instrumentation device to workload", "name", obj.GetName(), "namespace", obj.GetNamespace())
	}

	return nil, devicePartiallyApplied
}

// validateAndCleanupJavaToolOptionsWhenEmpty validates JAVA_TOOL_OPTIONS even when RuntimeDetails are empty
func validateAndCleanupJavaToolOptionsWhenEmpty(ctx context.Context, client client.Client, instrumentedApplication *odigosv1.KarmaInstrumentedApplication) error {
	// Get the workload object
	workloadName, workloadKind, err := workload.ExtractWorkloadInfoFromRuntimeObjectName(instrumentedApplication.Name)
	if err != nil {
		return err
	}

	workloadObj := workload.ClientObjectFromWorkloadKind(workloadKind)
	if workloadObj == nil {
		return fmt.Errorf("unknown workload kind: %s", workloadKind)
	}

	err = client.Get(ctx, types.NamespacedName{
		Namespace: instrumentedApplication.Namespace,
		Name:      workloadName,
	}, workloadObj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	// Get pod template spec
	podSpec, err := getPodSpecFromObject(workloadObj)
	if err != nil {
		return err
	}

	// Get pod labels for proper validation
	podLabels := make(map[string]string)
	if podSpec.Labels != nil {
		podLabels = podSpec.Labels
	}

	// Validate JAVA_TOOL_OPTIONS for each container
	changed := false
	for i, container := range podSpec.Spec.Containers {
		for j, envVar := range container.Env {
			if envVar.Name == "JAVA_TOOL_OPTIONS" {
				// Use label-aware validation for proper cleanup
				validatedValue := validateAllJavaAgents(envVar.Value, podLabels)
				if validatedValue != envVar.Value {
					podSpec.Spec.Containers[i].Env[j].Value = validatedValue
					changed = true
					log.FromContext(ctx).Info(
						"Cleaned up JAVA_TOOL_OPTIONS when RuntimeDetails are empty",
						"container", container.Name,
						"original", envVar.Value,
						"validated", validatedValue,
						"podLabels", podLabels,
					)
				}
				break
			}
		}
	}

	// Update the workload if changes were made
	if changed {
		return client.Update(ctx, workloadObj)
	}

	return nil
}

func removeInstrumentationDeviceFromWorkload(ctx context.Context, kubeClient client.Client, namespace string, workloadKind workload.WorkloadKind, workloadName string, uninstrumentReason ApplyInstrumentationDeviceReason) error {

	workloadObj := workload.ClientObjectFromWorkloadKind(workloadKind)
	if workloadObj == nil {
		return errors.New("unknown kind")
	}

	err := kubeClient.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      workloadName,
	}, workloadObj)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	podSpec, err := getPodSpecFromObject(workloadObj)
	if err != nil {
		return err
	}
	// If instrumentation device is removed successfully, remove codekarma.tech/inject-instrumentation label to disable the webhook
	webhookLabelRemoved := instrumentation.RemoveInjectInstrumentationLabel(podSpec)
	deviceRemoved := instrumentation.RevertInstrumentationDevices(podSpec)
	envChanged, err := instrumentation.RevertEnvOverwrites(workloadObj, podSpec)
	if err != nil {
		return err
	}

	// if we didn't change anything, we don't need to update the object
	// skip the api-server call, return no-op and skip the log message
	if !webhookLabelRemoved && !deviceRemoved && !envChanged {
		return nil
	}

	err = kubeClient.Update(ctx, workloadObj)
	if err != nil {
		// if the update fails due to a conflict, the controller will retry the operation
		return err
	}

	logger := log.FromContext(ctx)
	logger.V(0).Info("removed instrumentation device from workload", "namespace", workloadObj.GetNamespace(), "kind", workloadObj.GetObjectKind(), "name", workloadObj.GetName(), "reason", uninstrumentReason)

	return nil
}

func getWorkloadObject(ctx context.Context, kubeClient client.Client, runtimeDetails *odigosv1.KarmaInstrumentedApplication) (client.Object, error) {
	name, kind, err := workload.ExtractWorkloadInfoFromRuntimeObjectName(runtimeDetails.Name)
	if err != nil {
		return nil, err
	}

	workloadObject := workload.ClientObjectFromWorkloadKind(kind)
	if workloadObject == nil {
		return nil, errors.New("unknown kind")
	}

	err = kubeClient.Get(ctx, client.ObjectKey{
		Namespace: runtimeDetails.Namespace,
		Name:      name,
	}, workloadObject)
	if err != nil {
		return nil, err
	}

	return workloadObject, nil
}

func getPodSpecFromObject(obj client.Object) (*corev1.PodTemplateSpec, error) {
	switch o := obj.(type) {
	case *appsv1.Deployment:
		return &o.Spec.Template, nil
	case *appsv1.StatefulSet:
		return &o.Spec.Template, nil
	case *appsv1.DaemonSet:
		return &o.Spec.Template, nil
	default:
		return nil, errors.New("unknown kind")
	}
}

// Add a new function to check if a language is supported
func isSupportedLanguage(language common.ProgrammingLanguage) bool {
	switch language {
	case common.JavaProgrammingLanguage, common.GoProgrammingLanguage:
		return true
	default:
		return false
	}
}

// validateAllJavaAgentsFileOnly keeps all agents (no file existence check)
// Used when pod labels are not available (e.g., in reconciliation logic)
// File existence check is removed because files are only available inside containers at runtime
func validateAllJavaAgentsFileOnly(javaToolOptions string) string {
	if javaToolOptions == "" {
		return ""
	}

	// Keep all agents - let the JVM handle missing files at runtime
	return javaToolOptions
}

// reconciles a single workload, which might be triggered by a change in multiple resources.
// each time a relevant resource changes, this function is called to reconcile the workload
// and always writes the status into the KarmaInstrumentedApplication CR
func reconcileSingleWorkload(ctx context.Context, kubeClient client.Client, instrumentedApplication *odigosv1.KarmaInstrumentedApplication, isNodeCollectorReady bool) error {

	workloadName, workloadKind, err := workload.ExtractWorkloadInfoFromRuntimeObjectName(instrumentedApplication.Name)
	if err != nil {
		conditions.UpdateStatusConditions(ctx, kubeClient, instrumentedApplication, &instrumentedApplication.Status.Conditions, metav1.ConditionFalse, appliedInstrumentationDeviceType, string(ApplyInstrumentationDeviceReasonErrRemoving), err.Error())
		return err
	}

	if !isNodeCollectorReady {
		err := removeInstrumentationDeviceFromWorkload(ctx, kubeClient, instrumentedApplication.Namespace, workloadKind, workloadName, ApplyInstrumentationDeviceReasonDataCollectionNotReady)
		if err == nil {
			conditions.UpdateStatusConditions(ctx, kubeClient, instrumentedApplication, &instrumentedApplication.Status.Conditions, metav1.ConditionFalse, appliedInstrumentationDeviceType, string(ApplyInstrumentationDeviceReasonDataCollectionNotReady), "OpenTelemetry pipeline not yet ready to receive data")
		} else {
			conditions.UpdateStatusConditions(ctx, kubeClient, instrumentedApplication, &instrumentedApplication.Status.Conditions, metav1.ConditionFalse, appliedInstrumentationDeviceType, string(ApplyInstrumentationDeviceReasonErrRemoving), err.Error())
		}
		return err
	}

	if len(instrumentedApplication.Spec.RuntimeDetails) == 0 {
		// Before removing instrumentation, try to validate and cleanup JAVA_TOOL_OPTIONS
		// This handles the case where RuntimeDetails are empty but we still need to clean up
		err := validateAndCleanupJavaToolOptionsWhenEmpty(ctx, kubeClient, instrumentedApplication)
		if err != nil {
			log.FromContext(ctx).Error(err, "Failed to validate JAVA_TOOL_OPTIONS when RuntimeDetails are empty")
		}

		err = removeInstrumentationDeviceFromWorkload(ctx, kubeClient, instrumentedApplication.Namespace, workloadKind, workloadName, ApplyInstrumentationDeviceReasonNoRuntimeDetails)
		if err == nil {
			conditions.UpdateStatusConditions(ctx, kubeClient, instrumentedApplication, &instrumentedApplication.Status.Conditions, metav1.ConditionFalse, appliedInstrumentationDeviceType, string(ApplyInstrumentationDeviceReasonNoRuntimeDetails), "No runtime details found")
		} else {
			conditions.UpdateStatusConditions(ctx, kubeClient, instrumentedApplication, &instrumentedApplication.Status.Conditions, metav1.ConditionFalse, appliedInstrumentationDeviceType, string(ApplyInstrumentationDeviceReasonErrRemoving), err.Error())
		}
		return err
	}

	// Check if there are any unsupported languages
	for _, containerDetails := range instrumentedApplication.Spec.RuntimeDetails {
		if !isSupportedLanguage(containerDetails.Language) {
			// Unsupported language detected
			errRemove := removeInstrumentationDeviceFromWorkload(ctx, kubeClient, instrumentedApplication.Namespace, workloadKind, workloadName, ApplyInstrumentationDeviceReasonUnsupportedLanguage)
			if errRemove == nil {
				conditions.UpdateStatusConditions(ctx, kubeClient, instrumentedApplication, &instrumentedApplication.Status.Conditions, metav1.ConditionFalse, appliedInstrumentationDeviceType, string(ApplyInstrumentationDeviceReasonUnsupportedLanguage),
					"Unsupported language detected. Only Java and Go are currently supported for instrumentation.")
			} else {
				conditions.UpdateStatusConditions(ctx, kubeClient, instrumentedApplication, &instrumentedApplication.Status.Conditions, metav1.ConditionFalse, appliedInstrumentationDeviceType, string(ApplyInstrumentationDeviceReasonErrRemoving), errRemove.Error())
			}
			return nil
		}
	}

	runtimeVersionSupport, err := versionsupport.IsRuntimeVersionSupported(ctx, instrumentedApplication.Spec.RuntimeDetails)
	if !runtimeVersionSupport {
		errRemove := removeInstrumentationDeviceFromWorkload(ctx, kubeClient, instrumentedApplication.Namespace, workloadKind, workloadName, ApplyInstrumentationDeviceReasonRuntimeVersionNotSupported)
		if errRemove == nil {
			conditions.UpdateStatusConditions(ctx, kubeClient, instrumentedApplication, &instrumentedApplication.Status.Conditions, metav1.ConditionFalse, appliedInstrumentationDeviceType, string(ApplyInstrumentationDeviceReasonRuntimeVersionNotSupported), err.Error())
		} else {
			conditions.UpdateStatusConditions(ctx, kubeClient, instrumentedApplication, &instrumentedApplication.Status.Conditions, metav1.ConditionFalse, appliedInstrumentationDeviceType, string(ApplyInstrumentationDeviceReasonErrRemoving), errRemove.Error())
		}
		return nil
	}

	err, devicePartiallyApplied := addInstrumentationDeviceToWorkload(ctx, kubeClient, instrumentedApplication)
	if err == nil {
		var successMessage string
		if devicePartiallyApplied {
			successMessage = "Instrumentation device partially applied"
		} else {
			successMessage = "Instrumentation device applied successfully"
		}
		conditions.UpdateStatusConditions(ctx, kubeClient, instrumentedApplication, &instrumentedApplication.Status.Conditions, metav1.ConditionTrue, appliedInstrumentationDeviceType, "InstrumentationDeviceApplied", successMessage)
	} else {
		conditions.UpdateStatusConditions(ctx, kubeClient, instrumentedApplication, &instrumentedApplication.Status.Conditions, metav1.ConditionFalse, appliedInstrumentationDeviceType, string(ApplyInstrumentationDeviceReasonErrApplying), err.Error())
	}
	return err
}
