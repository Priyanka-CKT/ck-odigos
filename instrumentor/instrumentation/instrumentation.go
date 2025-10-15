package instrumentation

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	odigosv1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	"github.com/odigos-io/odigos/common/envOverwrite"
	"github.com/odigos-io/odigos/k8sutils/pkg/consts"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/k8sutils/pkg/envoverwrite"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

var (
	ErrNoDefaultSDK = errors.New("no default sdks found")
	ErrPatchEnvVars = errors.New("failed to patch env vars")
)

func ApplyInstrumentationDevicesToPodTemplate(original *corev1.PodTemplateSpec, runtimeDetails *odigosv1.KarmaInstrumentedApplication, defaultSdks map[common.ProgrammingLanguage]common.OtelSdk, targetObj client.Object,
	logger logr.Logger, agentsCanRunConcurrently bool) (error, bool, bool) {
	// delete any existing instrumentation devices.
	// this is necessary for example when migrating from community to enterprise,
	// and we need to cleanup the community device before adding the enterprise one.
	RevertInstrumentationDevices(original)

	deviceApplied := false
	deviceSkippedDueToOtherAgent := false
	var modifiedContainers []corev1.Container

	manifestEnvOriginal, err := envoverwrite.NewOrigWorkloadEnvValues(targetObj.GetAnnotations())
	if err != nil {
		return err, deviceApplied, deviceSkippedDueToOtherAgent
	}

	for _, container := range original.Spec.Containers {
		containerLanguage := getLanguageOfContainer(runtimeDetails, container.Name)
		containerHaveOtherAgent := getContainerOtherAgents(runtimeDetails, container.Name)
		libcType := getLibCTypeOfContainer(runtimeDetails, container.Name)

		// By default, Odigos does not run alongside other agents.
		// However, if configured in the odigos-config, it can be allowed to run in parallel.
		if containerHaveOtherAgent != nil && !agentsCanRunConcurrently {
			logger.Info("Container is running other agent, skip applying instrumentation device", "agent", containerHaveOtherAgent.Name, "container", container.Name)

			// Not actually modifying the container, but we need to append it to the list.
			modifiedContainers = append(modifiedContainers, container)
			deviceSkippedDueToOtherAgent = true
			continue
		}
		// handle containers with unknown language or ignored language
		if containerLanguage == common.UnknownProgrammingLanguage || containerLanguage == common.IgnoredProgrammingLanguage || containerLanguage == common.NginxProgrammingLanguage {
			// always patch the env vars, even if the language is unknown or ignored.
			// this is necessary to sync the existing envs with the missing language if changed for any reason.
			err = patchEnvVarsForContainer(runtimeDetails, &container, nil, containerLanguage, manifestEnvOriginal, logger)
			if err != nil {
				return fmt.Errorf("%w: %v", ErrPatchEnvVars, err), deviceApplied, deviceSkippedDueToOtherAgent
			}
			modifiedContainers = append(modifiedContainers, container)
			continue
		}

		// Find and apply the appropriate SDK for the container language.
		otelSdk, found := defaultSdks[containerLanguage]
		if !found {
			return fmt.Errorf("%w for language: %s, container:%s", ErrNoDefaultSDK, containerLanguage, container.Name), deviceApplied, deviceSkippedDueToOtherAgent
		}

		instrumentationDeviceName := common.InstrumentationDeviceName(containerLanguage, otelSdk, libcType)
		if container.Resources.Limits == nil {
			container.Resources.Limits = make(map[corev1.ResourceName]resource.Quantity)
		}
		container.Resources.Limits[corev1.ResourceName(instrumentationDeviceName)] = resource.MustParse("1")
		deviceApplied = true

		err = patchEnvVarsForContainer(runtimeDetails, &container, &otelSdk, containerLanguage, manifestEnvOriginal, logger)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrPatchEnvVars, err), deviceApplied, deviceSkippedDueToOtherAgent
		}

		modifiedContainers = append(modifiedContainers, container)
	}

	if modifiedContainers != nil {
		original.Spec.Containers = modifiedContainers
	}

	// persist the original values if changed
	manifestEnvOriginal.SerializeToAnnotation(targetObj)

	return nil, deviceApplied, deviceSkippedDueToOtherAgent
}

// this function restores a workload manifest env vars to their original values.
// it is used when the instrumentation is removed from the workload.
// the original values are read from the annotation which was saved when the instrumentation was applied.
func RevertEnvOverwrites(obj client.Object, podSpec *corev1.PodTemplateSpec) (bool, error) {
	manifestEnvOriginal, err := envoverwrite.NewOrigWorkloadEnvValues(obj.GetAnnotations())
	if err != nil {
		return false, err
	}

	changed := false
	for iContainer, c := range podSpec.Spec.Containers {
		containerOriginalEnv := manifestEnvOriginal.GetContainerStoredEnvs(c.Name)
		newContainerEnvs := make([]corev1.EnvVar, 0, len(c.Env))
		for _, envVar := range c.Env {
			if origValue, found := containerOriginalEnv[envVar.Name]; found {
				// revert the env var to its original value
				if origValue != nil {
					newContainerEnvs = append(newContainerEnvs, corev1.EnvVar{
						Name:  envVar.Name,
						Value: *containerOriginalEnv[envVar.Name],
					})
				} else {
					// if the value is nil, the env var was not set by the user to begin with.
					// we will simply not append it to the new envs to achieve the same effect.
				}
				changed = true
			} else {
				newContainerEnvs = append(newContainerEnvs, envVar)
			}
		}
		podSpec.Spec.Containers[iContainer].Env = newContainerEnvs
	}

	annotationRemoved := manifestEnvOriginal.DeleteFromObj(obj)

	return changed || annotationRemoved, nil
}

func RevertInstrumentationDevices(original *corev1.PodTemplateSpec) bool {
	changed := false
	for _, container := range original.Spec.Containers {
		for resourceName := range container.Resources.Limits {
			if strings.HasPrefix(string(resourceName), common.OdigosResourceNamespace) {
				delete(container.Resources.Limits, resourceName)
				changed = true
			}
		}
		// Is it needed?
		for resourceName := range container.Resources.Requests {
			if strings.HasPrefix(string(resourceName), common.OdigosResourceNamespace) {
				delete(container.Resources.Requests, resourceName)
				changed = true
			}
		}
	}
	return changed
}

func getLanguageOfContainer(instrumentation *odigosv1.KarmaInstrumentedApplication, containerName string) common.ProgrammingLanguage {
	for _, l := range instrumentation.Spec.RuntimeDetails {
		if l.ContainerName == containerName {
			return l.Language
		}
	}

	return common.UnknownProgrammingLanguage
}

func getContainerOtherAgents(instrumentation *odigosv1.KarmaInstrumentedApplication, containerName string) *odigosv1.OtherAgent {
	for _, l := range instrumentation.Spec.RuntimeDetails {
		if l.ContainerName == containerName {
			if l.OtherAgent != nil && *l.OtherAgent != (odigosv1.OtherAgent{}) {
				return l.OtherAgent
			}
		}
	}
	return nil
}

func getLibCTypeOfContainer(instrumentation *odigosv1.KarmaInstrumentedApplication, containerName string) *common.LibCType {
	for _, l := range instrumentation.Spec.RuntimeDetails {
		if l.ContainerName == containerName {
			return l.LibCType
		}
	}

	return nil
}

// getEnvVarsOfContainer returns the env vars which are defined for the given container and are used for instrumentation purposes.
// This function also returns env vars which are declared in the container build.
// NOTE: This reads from KarmaInstrumentedApplication.Spec which has the ORIGINAL values (before any patching).
func getEnvVarsOfContainer(instrumentation *odigosv1.KarmaInstrumentedApplication, containerName string) map[string]string {
	envVars := make(map[string]string)

	for _, l := range instrumentation.Spec.RuntimeDetails {
		if l.ContainerName == containerName {
			for _, env := range l.EnvVars {
				envVars[env.Name] = env.Value
			}
			return envVars
		}
	}

	return envVars
}

// when otelsdk is nil, it means that the container is not instrumented.
// this will trigger reverting of any existing env vars which were set by odigos before.
func patchEnvVarsForContainer(runtimeDetails *odigosv1.KarmaInstrumentedApplication, container *corev1.Container, sdk *common.OtelSdk, programmingLanguage common.ProgrammingLanguage, manifestEnvOriginal *envoverwrite.OrigWorkloadEnvValues, logger logr.Logger) error {

	// Get observed env vars from KarmaInstrumentedApplication.Spec (original values)
	observedEnvs := getEnvVarsOfContainer(runtimeDetails, container.Name)

	logger.V(0).Info("DEBUG ENV PATCH: Using env vars from KarmaInstrumentedApplication.Spec (original values)",
		"container", container.Name, "envCount", len(observedEnvs))

	// Step 1: check existing environment on the manifest and update them if needed
	newEnvs := make([]corev1.EnvVar, 0, len(container.Env))
	logger.V(0).Info("DEBUG ENV PATCH: Starting env patching for container", "container", container.Name, "totalEnvVars", len(container.Env), "language", programmingLanguage, "sdk", sdk)

	for _, envVar := range container.Env {

		// extract the observed value for this env var, which might be empty if not currently exists
		observedEnvValue := observedEnvs[envVar.Name]

		if envVar.Name == "JAVA_TOOL_OPTIONS" || envVar.Name == "JAVA_OPTS" {
			logger.V(0).Info("DEBUG ENV PATCH: Processing Java env var", "name", envVar.Name, "manifestValue", envVar.Value, "observedValue", observedEnvValue, "sdk", sdk, "language", programmingLanguage)
		}

		desiredEnvValue := envOverwrite.GetPatchedEnvValue(envVar.Name, observedEnvValue, sdk, programmingLanguage)

		if envVar.Name == "JAVA_TOOL_OPTIONS" || envVar.Name == "JAVA_OPTS" {
			if desiredEnvValue != nil {
				logger.V(0).Info("DEBUG ENV PATCH: GetPatchedEnvValue returned value", "name", envVar.Name, "desiredValue", *desiredEnvValue)
			} else {
				logger.V(0).Info("DEBUG ENV PATCH: GetPatchedEnvValue returned nil", "name", envVar.Name, "manifestValue", envVar.Value)
			}
		}

		if desiredEnvValue == nil {
			// no need to patch this env var, so make sure it is reverted to its original value

			// CRITICAL FIX: Don't remove env vars that contain our CodeKarma agent!
			// This happens during helm upgrade when SDK might be nil temporarily.
			// If the manifest value contains our agent, we want to keep it.
			if (envVar.Name == "JAVA_TOOL_OPTIONS" || envVar.Name == "JAVA_OPTS") && strings.Contains(envVar.Value, "ck-agent-universal.jar") {
				logger.V(0).Info("✅ DEBUG ENV PATCH: Preserving env var with ck-agent even though desiredEnvValue is nil", "name", envVar.Name, "value", envVar.Value)
				newEnvs = append(newEnvs, envVar)
				delete(observedEnvs, envVar.Name)
				continue
			}

			origValue, found := manifestEnvOriginal.RemoveOriginalValue(container.Name, envVar.Name)

			if envVar.Name == "JAVA_TOOL_OPTIONS" || envVar.Name == "JAVA_OPTS" {
				logger.V(0).Info("DEBUG ENV PATCH: desiredEnvValue is nil, checking original", "name", envVar.Name, "found", found, "origValue", origValue, "manifestValue", envVar.Value)
			}

			if !found {
				newEnvs = append(newEnvs, envVar)
				if envVar.Name == "JAVA_TOOL_OPTIONS" || envVar.Name == "JAVA_OPTS" {
					logger.V(0).Info("DEBUG ENV PATCH: No original found, keeping manifest value", "name", envVar.Name, "value", envVar.Value)
				}
			} else { // found, we need to update the env var to it's original value
				if origValue != nil {
					// this case reverts back the env var to it's original value
					newEnvs = append(newEnvs, corev1.EnvVar{
						Name:  envVar.Name,
						Value: *origValue,
					})
					if envVar.Name == "JAVA_TOOL_OPTIONS" || envVar.Name == "JAVA_OPTS" {
						logger.V(0).Info("DEBUG ENV PATCH: Reverting to original value", "name", envVar.Name, "value", *origValue)
					}
				} else {
					// if the original value was nil, then it was not set by the user.
					// we will simply not append it to the new envs to achieve the same effect.
					if envVar.Name == "JAVA_TOOL_OPTIONS" || envVar.Name == "JAVA_OPTS" {
						logger.V(0).Info("⚠️ DEBUG ENV PATCH: REMOVING env var - original was nil!", "name", envVar.Name, "manifestValue", envVar.Value)
					}
				}
			}
		} else { // there is a desired value to inject
			// if it's the first time we patch this env var, save the original value
			// InsertOriginalValue will NOT overwrite if it already exists
			manifestEnvOriginal.InsertOriginalValue(container.Name, envVar.Name, &envVar.Value)
			// update the env var to it's desired value
			newEnvs = append(newEnvs, corev1.EnvVar{
				Name:  envVar.Name,
				Value: *desiredEnvValue,
			})

			if envVar.Name == "JAVA_TOOL_OPTIONS" || envVar.Name == "JAVA_OPTS" {
				logger.V(0).Info("✅ DEBUG ENV PATCH: Applied desired value to manifest",
					"name", envVar.Name, "value", *desiredEnvValue)
			}
		}

		// If an env var is defined both in the container build and in the container spec, the value in the container spec will be used.
		delete(observedEnvs, envVar.Name)
	}

	// Step 2: add the new env vars which odigos might patch, but which are not defined in the manifest
	logger.V(0).Info("DEBUG ENV PATCH: Step 2 - processing observed envs not in manifest", "container", container.Name, "observedEnvCount", len(observedEnvs), "sdk", sdk)
	if sdk != nil {
		for envName, envValue := range observedEnvs {
			if envName == "JAVA_TOOL_OPTIONS" || envName == "JAVA_OPTS" {
				logger.V(0).Info("DEBUG ENV PATCH: Step 2 - Processing Java env from observed", "name", envName, "observedValue", envValue)
			}

			desiredEnvValue := envOverwrite.GetPatchedEnvValue(envName, envValue, sdk, programmingLanguage)

			if envName == "JAVA_TOOL_OPTIONS" || envName == "JAVA_OPTS" {
				if desiredEnvValue != nil {
					logger.V(0).Info("DEBUG ENV PATCH: Step 2 - GetPatchedEnvValue returned value", "name", envName, "desiredValue", *desiredEnvValue)
				} else {
					logger.V(0).Info("DEBUG ENV PATCH: Step 2 - GetPatchedEnvValue returned nil", "name", envName)
				}
			}

			if desiredEnvValue != nil {
				// store that it was empty to begin with
				manifestEnvOriginal.InsertOriginalValue(container.Name, envName, nil)
				// and add this new env var to the manifest
				newEnvs = append(newEnvs, corev1.EnvVar{
					Name:  envName,
					Value: *desiredEnvValue,
				})
			}
		}
	}

	// Step 3: update the container with the new env vars
	container.Env = newEnvs

	return nil
}

func SetInjectInstrumentationLabel(original *corev1.PodTemplateSpec) {

	if original.Labels == nil {
		original.Labels = make(map[string]string)
	}
	original.Labels[consts.OdigosInjectInstrumentationLabel] = "true"
}

// RemoveInjectInstrumentationLabel removes the "codekarma.tech/inject-instrumentation" label if it exists.
func RemoveInjectInstrumentationLabel(original *corev1.PodTemplateSpec) bool {
	if original.Labels != nil {
		if _, ok := original.Labels[consts.OdigosInjectInstrumentationLabel]; ok {
			delete(original.Labels, consts.OdigosInjectInstrumentationLabel)
			return true
		}
	}
	return false
}
