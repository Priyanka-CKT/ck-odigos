package instrumentationdevice

import (
	odigosv1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	"github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/instrumentor/controllers/utils"
	odigospredicate "github.com/odigos-io/odigos/k8sutils/pkg/predicate"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func countOdigosResources(resources corev1.ResourceList) int {
	numOdigosResources := 0
	for resourceName := range resources {
		if common.IsResourceNameOdigosInstrumentation(resourceName.String()) {
			numOdigosResources = numOdigosResources + 1
		}
	}
	return numOdigosResources
}

type workloadPodTemplatePredicate struct {
	predicate.Funcs
}

func (w workloadPodTemplatePredicate) Create(e event.CreateEvent) bool {
	// when instrumentor restarts, this case will be triggered as workloads objects are being added to the cache.
	// in this case, we need to reconcile the workload, and guarantee that the device is injected or removed
	// based on the current state of the cluster.
	// if the instrumented application is deleted but the device is not cleaned,
	// the instrumented application controller will not be invoked after restart, which is why we need to handle this case here.
	return true
}

func (w workloadPodTemplatePredicate) Update(e event.UpdateEvent) bool {

	if e.ObjectOld == nil || e.ObjectNew == nil {
		return false
	}

	oldPodSpec, err := getPodSpecFromObject(e.ObjectOld)
	if err != nil {
		return false
	}
	newPodSpec, err := getPodSpecFromObject(e.ObjectNew)
	if err != nil {
		return false
	}

	// Check if the change was made by CodeKarma itself by comparing the inject-instrumentation label
	oldHasCodeKarmaLabel := hasCodeKarmaInjectInstrumentationLabel(oldPodSpec)
	newHasCodeKarmaLabel := hasCodeKarmaInjectInstrumentationLabel(newPodSpec)

	// Check if the change was made by Odigos itself by comparing the inject-instrumentation label
	oldHasOdigosLabel := hasOdigosInjectInstrumentationLabel(oldPodSpec)
	newHasOdigosLabel := hasOdigosInjectInstrumentationLabel(newPodSpec)

	// Allow reconciliation when ANY labels change (both addition and removal)
	// This enables proper setup when labels are added and cleanup when labels are removed
	if oldHasCodeKarmaLabel != newHasCodeKarmaLabel || oldHasOdigosLabel != newHasOdigosLabel {
		return true
	}

	// Prevent double reconciliation when CodeKarma is making changes
	// Only trigger reconciliation for environment changes if CodeKarma is not present
	// BUT: Allow reconciliation for label changes even when CodeKarma is present
	if oldHasCodeKarmaLabel || newHasCodeKarmaLabel {
		// CodeKarma is present, don't trigger on env changes to avoid double reconciliation
		// BUT: Label changes are already handled above, so this only affects env changes
		return false
	}

	// only handle workloads if any env changed (only when CodeKarma is not present)
	if len(oldPodSpec.Spec.Containers) != len(newPodSpec.Spec.Containers) {
		return true
	}
	for i := range oldPodSpec.Spec.Containers {
		if len(oldPodSpec.Spec.Containers[i].Env) != len(newPodSpec.Spec.Containers[i].Env) {
			return true
		}

		// Use the minimum length to avoid index out of range
		minEnvLen := len(oldPodSpec.Spec.Containers[i].Env)
		if len(newPodSpec.Spec.Containers[i].Env) < minEnvLen {
			minEnvLen = len(newPodSpec.Spec.Containers[i].Env)
		}

		for j := 0; j < minEnvLen; j++ {
			prevEnv := &newPodSpec.Spec.Containers[i].Env[j]
			newEnv := &oldPodSpec.Spec.Containers[i].Env[j]
			if prevEnv.Name != newEnv.Name || prevEnv.Value != newEnv.Value {
				return true
			}
		}

		// user might apply a change to workload which will overwrite odigos injected resources
		// but only trigger if the change is not made by Odigos itself
		prevNumOdigosResources := countOdigosResources(oldPodSpec.Spec.Containers[i].Resources.Limits)
		newNumOdigosResources := countOdigosResources(newPodSpec.Spec.Containers[i].Resources.Limits)
		if prevNumOdigosResources != newNumOdigosResources {
			return true
		}
	}

	return false
}

func (w workloadPodTemplatePredicate) Delete(e event.DeleteEvent) bool {
	return false
}

func (w workloadPodTemplatePredicate) Generic(e event.GenericEvent) bool {
	return false
}

// Helper function to check if pod template has the CodeKarma inject-instrumentation label
func hasCodeKarmaInjectInstrumentationLabel(podSpec *corev1.PodTemplateSpec) bool {
	if podSpec.Labels == nil {
		return false
	}
	_, exists := podSpec.Labels["codekarma.tech/inject-instrumentation"]
	return exists
}

// Helper function to check if pod template has the Odigos inject-instrumentation label
func hasOdigosInjectInstrumentationLabel(podSpec *corev1.PodTemplateSpec) bool {
	if podSpec.Labels == nil {
		return false
	}
	_, exists := podSpec.Labels["odigos.io/inject-instrumentation"]
	return exists
}

func SetupWithManager(mgr ctrl.Manager) error {
	err := builder.
		ControllerManagedBy(mgr).
		Named("instrumentationdevice-collectorsgroup").
		For(&odigosv1.CollectorsGroup{}).
		WithEventFilter(predicate.And(&odigospredicate.OdigosCollectorsGroupNodePredicate, &odigospredicate.CgBecomesReadyPredicate{})).
		Complete(&CollectorsGroupReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		})
	if err != nil {
		return err
	}

	err = builder.
		ControllerManagedBy(mgr).
		Named("instrumentationdevice-instrumentedapplication").
		For(&odigosv1.KarmaInstrumentedApplication{}).
		WithEventFilter(&predicate.GenerationChangedPredicate{}).
		Complete(&KarmaInstrumentedApplicationReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		})
	if err != nil {
		return err
	}

	err = builder.
		ControllerManagedBy(mgr).
		Named("instrumentationdevice-deployment").
		For(&appsv1.Deployment{}).
		WithEventFilter(workloadPodTemplatePredicate{}).
		Complete(&DeploymentReconciler{
			Client: mgr.GetClient(),
		})
	if err != nil {
		return err
	}

	err = builder.
		ControllerManagedBy(mgr).
		Named("instrumentationdevice-daemonset").
		For(&appsv1.DaemonSet{}).
		WithEventFilter(workloadPodTemplatePredicate{}).
		Complete(&DaemonSetReconciler{
			Client: mgr.GetClient(),
		})
	if err != nil {
		return err
	}

	err = builder.
		ControllerManagedBy(mgr).
		For(&appsv1.StatefulSet{}).
		WithEventFilter(workloadPodTemplatePredicate{}).
		Complete(&StatefulSetReconciler{
			Client: mgr.GetClient(),
		})
	if err != nil {
		return err
	}

	err = builder.
		ControllerManagedBy(mgr).
		Named("instrumentationdevice-instrumentationrules").
		For(&odigosv1.KarmaInstrumentationRule{}).
		WithEventFilter(&utils.OtelSdkKarmaInstrumentationRulePredicate{}).
		Complete(&KarmaInstrumentationRuleReconciler{
			Client: mgr.GetClient(),
		})
	if err != nil {
		return err
	}

	err = builder.
		WebhookManagedBy(mgr).
		For(&corev1.Pod{}).
		WithDefaulter(&PodsWebhook{
			Client: mgr.GetClient(),
		}).
		Complete()
	if err != nil {
		return err
	}

	return nil
}
