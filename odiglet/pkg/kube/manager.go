package kube

import (
	"github.com/odigos-io/odigos/common/consts"
	"github.com/odigos-io/odigos/instrumentation"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/odigos-io/odigos/odiglet/pkg/ebpf"
	"github.com/odigos-io/odigos/odiglet/pkg/env"
	"github.com/odigos-io/odigos/odiglet/pkg/kube/instrumentation_ebpf"
	"github.com/odigos-io/odigos/odiglet/pkg/kube/runtime_details"
	"github.com/odigos-io/odigos/odiglet/pkg/log"
	ctrl "sigs.k8s.io/controller-runtime"

	odigosv1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	// Instead of adding all the Odigos API types, we'll only register the specific ones we need
	// We're intentionally NOT registering the CollectorsGroup CRD to avoid watching for it
	// since it's being removed from the system.
	schemeBuilder := runtime.NewSchemeBuilder(
		func(scheme *runtime.Scheme) error {
			// Create the GroupVersion for odigos.io/v1alpha1
			groupVersion := schema.GroupVersion{Group: "odigos.io", Version: "v1alpha1"}
			scheme.AddKnownTypes(groupVersion,
				&odigosv1.InstrumentedApplication{},
				&odigosv1.InstrumentedApplicationList{},
				&odigosv1.InstrumentationConfig{},
				&odigosv1.InstrumentationConfigList{},
				&odigosv1.InstrumentationInstance{},
				&odigosv1.InstrumentationInstanceList{},
				&odigosv1.OdigosConfiguration{},
				&odigosv1.OdigosConfigurationList{},
			)
			metav1.AddToGroupVersion(scheme, groupVersion)
			return nil
		},
	)
	utilruntime.Must(schemeBuilder.AddToScheme(scheme))
}

func CreateManager() (ctrl.Manager, error) {
	log.Logger.V(0).Info("Starting reconcileres for runtime details")
	ctrl.SetLogger(log.Logger)
	return manager.New(config.GetConfigOrDie(), manager.Options{
		Scheme: scheme,
		Cache: cache.Options{
			// ManagedFields are removed to save space. This can save a lot of space and recommended in the cache package.
			// running `kubectl get .... --show-managed-fields` will show the managed fields.
			DefaultTransform: cache.TransformStripManagedFields(),
			ByObject: map[client.Object]cache.ByObject{
				&corev1.Pod{}: {
					// only watch and list pods in the current node
					Field: fields.OneTermEqualSelector("spec.nodeName", env.Current.NodeName),
				},
				&corev1.Namespace{}: {
					Label: labels.Set{consts.OdigosInstrumentationLabel: consts.InstrumentationEnabled}.AsSelector(),
				},
			},
		},
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
	})
}

func SetupWithManager(mgr ctrl.Manager, ebpfDirectors ebpf.DirectorsMap, clientset *kubernetes.Clientset, configUpdates chan<- instrumentation.ConfigUpdate[ebpf.K8sConfigGroup]) error {
	err := runtime_details.SetupWithManager(mgr, clientset)
	if err != nil {
		return err
	}

	err = instrumentation_ebpf.SetupWithManager(mgr, ebpfDirectors, configUpdates)
	if err != nil {
		return err
	}

	return nil
}
