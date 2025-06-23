package startlangdetection

import (
	"github.com/odigos-io/odigos/k8sutils/pkg/env"
	"github.com/odigos-io/odigos/k8sutils/pkg/workload"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// this predicate is used for workload reconciler, and will only pass events
// where the workload is changed to odigos instrumentation enabled.
// This way, we don't need to run language detection downstream when unnecessary.
// This also helps in managing race conditions, where we might re-add runtime details
// which were just deleted by instrumentor controller and generate unnecessary noise
// in the k8s eventual consistency model.
type WorkloadEnabledPredicate struct {
	predicate.Funcs
}

func (i *WorkloadEnabledPredicate) Create(e event.CreateEvent) bool {
	logger := log.Log.WithName("WorkloadEnabledPredicate")

	enabled := workload.IsObjectLabeledForInstrumentation(e.Object)
	w, err := workload.ObjectToWorkload(e.Object)
	if err != nil {
		logger.V(0).Info("Failed to convert object to workload",
			"object", e.Object.GetName(),
			"namespace", e.Object.GetNamespace(),
			"error", err)
		return false
	}

	availableReplicas := w.AvailableReplicas()
	logger.V(0).Info("Processing workload creation",
		"object", e.Object.GetName(),
		"namespace", e.Object.GetNamespace(),
		"enabled", enabled,
		"availableReplicas", availableReplicas)

	return enabled && availableReplicas > 0
}

func (i *WorkloadEnabledPredicate) Update(e event.UpdateEvent) bool {
	logger := log.Log.WithName("WorkloadEnabledPredicate")

	if e.ObjectOld == nil || e.ObjectNew == nil {
		logger.V(0).Info("Skipping update - missing old or new object")
		return false
	}

	// filter our own namespace
	if e.ObjectNew.GetNamespace() == env.GetCurrentNamespace() {
		logger.V(0).Info("Skipping update - object in odigos namespace",
			"object", e.ObjectNew.GetName(),
			"namespace", e.ObjectNew.GetNamespace())
		return false
	}

	wOld, err := workload.ObjectToWorkload(e.ObjectOld)
	if err != nil {
		logger.V(0).Info("Failed to convert old object to workload",
			"object", e.ObjectOld.GetName(),
			"namespace", e.ObjectOld.GetNamespace(),
			"error", err)
		return false
	}

	wNew, err := workload.ObjectToWorkload(e.ObjectNew)
	if err != nil {
		logger.V(0).Info("Failed to convert new object to workload",
			"object", e.ObjectNew.GetName(),
			"namespace", e.ObjectNew.GetNamespace(),
			"error", err)
		return false
	}

	oldEnabled := workload.IsObjectLabeledForInstrumentation(e.ObjectOld)
	newEnabled := workload.IsObjectLabeledForInstrumentation(e.ObjectNew)
	becameEnabled := !oldEnabled && newEnabled

	newReplicas := wNew.AvailableReplicas()
	oldReplicas := wOld.AvailableReplicas()

	logger.V(0).Info("Processing workload update",
		"object", e.ObjectNew.GetName(),
		"namespace", e.ObjectNew.GetNamespace(),
		"oldEnabled", oldEnabled,
		"newEnabled", newEnabled,
		"oldReplicas", oldReplicas,
		"newReplicas", newReplicas)

	// 1. workload became enabled and has available (running) replicas
	if becameEnabled && newReplicas > 0 {
		logger.Info("Workload became enabled with available replicas",
			"object", e.ObjectNew.GetName(),
			"namespace", e.ObjectNew.GetNamespace())
		return true
	}

	// 2. replicas became available
	replicasBecameAvailable := (oldReplicas == 0) && (newReplicas > 0)
	if replicasBecameAvailable {
		logger.Info("Workload replicas became available",
			"object", e.ObjectNew.GetName(),
			"namespace", e.ObjectNew.GetNamespace())
		return true
	}

	// The language detection process currently does 2 things:
	// 1. Detect the language of each container in the workload
	// 2. Detect the actual value of relevant environment variables for each container.
	//
	// thus, we need to re-run language detection if something that might affect
	// any of these 2 things has changed.
	//
	// currently, we only check if the enabled label has changed, or the pod become available,
	// but other events that might affect the language detection are not checked.
	// for example: if the container array changed, if an env var was added/removed, if the image was changed, etc.
	// we might need to add these checks in the future.
	// notice that the change alone is not enough - after the change, the workload running pods still
	// run an old manifest. we should re-calculate the runtime details only with up-to-date running pods.

	return false
}

func (i *WorkloadEnabledPredicate) Delete(e event.DeleteEvent) bool {
	// no need to calculate runtime details for deleted workloads
	return false
}

func (i *WorkloadEnabledPredicate) Generic(e event.GenericEvent) bool {
	// not sure when exactly this would be called, but we don't need to handle it
	return false
}
