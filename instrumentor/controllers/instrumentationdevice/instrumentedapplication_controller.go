/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package instrumentationdevice

import (
	"context"
	"fmt"

	odigosv1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	"github.com/odigos-io/odigos/k8sutils/pkg/utils"
	"github.com/odigos-io/odigos/k8sutils/pkg/workload"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type KarmaInstrumentedApplicationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *KarmaInstrumentedApplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var runtimeDetails odigosv1.KarmaInstrumentedApplication
	err := r.Client.Get(ctx, req.NamespacedName, &runtimeDetails)
	if err != nil {

		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}

		// runtime details deleted: remove instrumentation from resource requests
		workloadName, workloadKind, err := workload.ExtractWorkloadInfoFromRuntimeObjectName(req.Name)
		if err != nil {
			logger.Error(err, "error parsing workload info from runtime object name")
			return ctrl.Result{}, err
		}
		err = removeInstrumentationDeviceFromWorkload(ctx, r.Client, req.Namespace, workloadKind, workloadName, ApplyInstrumentationDeviceReasonNoRuntimeDetails)
		return utils.K8SUpdateErrorHandler(err)
	}

	// Always validate JAVA_TOOL_OPTIONS to ensure proper cleanup
	// This is especially important when labels are removed (uninstrumentation)
	err = validateAndCleanupJavaToolOptions(ctx, r.Client, &runtimeDetails)
	if err != nil {
		logger.Error(err, "Failed to validate JAVA_TOOL_OPTIONS")
	}

	isNodeCollectorReady := true
	err = reconcileSingleWorkload(ctx, r.Client, &runtimeDetails, isNodeCollectorReady)
	return utils.K8SUpdateErrorHandler(err)
}

// validateAndCleanupJavaToolOptions validates and cleans up JAVA_TOOL_OPTIONS in the workload
// AND updates the RuntimeDetails in KarmaInstrumentedApplication to keep them in sync
func validateAndCleanupJavaToolOptions(ctx context.Context, client client.Client, runtimeDetails *odigosv1.KarmaInstrumentedApplication) error {
	// Get the workload object
	workloadName, workloadKind, err := workload.ExtractWorkloadInfoFromRuntimeObjectName(runtimeDetails.Name)
	if err != nil {
		return err
	}

	workloadObj := workload.ClientObjectFromWorkloadKind(workloadKind)
	if workloadObj == nil {
		return fmt.Errorf("unknown workload kind: %s", workloadKind)
	}

	err = client.Get(ctx, types.NamespacedName{
		Namespace: runtimeDetails.Namespace,
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
	runtimeDetailsChanged := false

	for _, container := range podSpec.Spec.Containers {
		for _, envVar := range container.Env {
			if envVar.Name == "JAVA_TOOL_OPTIONS" {
				// Use label-aware validation for proper cleanup
				validatedValue := validateAllJavaAgents(envVar.Value, podLabels)
				if validatedValue != envVar.Value {
					// Note: We don't update the workload here to avoid infinite loops
					// The workload will be updated by the webhook during pod creation/restart
					log.FromContext(ctx).Info(
						"Detected JAVA_TOOL_OPTIONS cleanup needed in workload",
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

	// Also update RuntimeDetails in KarmaInstrumentedApplication to keep them in sync
	if runtimeDetails.Spec.RuntimeDetails != nil {
		for containerIndex, containerDetails := range runtimeDetails.Spec.RuntimeDetails {
			for envIndex, envVar := range containerDetails.EnvVars {
				if envVar.Name == "JAVA_TOOL_OPTIONS" {
					validatedValue := validateAllJavaAgents(envVar.Value, podLabels)
					if validatedValue != envVar.Value {
						runtimeDetails.Spec.RuntimeDetails[containerIndex].EnvVars[envIndex].Value = validatedValue
						runtimeDetailsChanged = true
						log.FromContext(ctx).Info(
							"Cleaned up JAVA_TOOL_OPTIONS in RuntimeDetails",
							"container", containerDetails.ContainerName,
							"original", envVar.Value,
							"validated", validatedValue,
							"podLabels", podLabels,
						)
					}
					break
				}
			}
		}
	}

	// Update the KarmaInstrumentedApplication if RuntimeDetails changed
	// Note: We don't update the workload here to avoid infinite loops
	// The workload will be updated by the webhook during pod creation/restart
	if runtimeDetailsChanged {
		err = client.Update(ctx, runtimeDetails)
		if err != nil {
			return err
		}
	}

	return nil
}
