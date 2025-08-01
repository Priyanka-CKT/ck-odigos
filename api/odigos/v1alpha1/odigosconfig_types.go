// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

package v1alpha1

import (
	"github.com/odigos-io/odigos/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Deprecated: Use common.CodekarmaConfiguration instead
type CollectorGatewayConfiguration struct {
	// RequestMemoryMiB is the memory request for the cluster gateway collector deployment.
	// it will be embedded in the deployment as a resource request of the form "memory: <value>Mi"
	// default value is 500Mi
	RequestMemoryMiB int `json:"requestMemoryMiB,omitempty"`

	// this parameter sets the "limit_mib" parameter in the memory limiter configuration for the collector gateway.
	// it is the hard limit after which a force garbage collection will be performed.
	// if not set, it will be 50Mi below the memory request.
	MemoryLimiterLimitMiB int `json:"memoryLimiterLimitMiB,omitempty"`

	// this parameter sets the "spike_limit_mib" parameter in the memory limiter configuration for the collector gateway.
	// note that this is not the processor soft limit, but the diff in Mib between the hard limit and the soft limit.
	// if not set, this will be set to 20% of the hard limit (so the soft limit will be 80% of the hard limit).
	MemoryLimiterSpikeLimitMiB int `json:"memoryLimiterSpikeLimitMiB,omitempty"`

	// the GOMEMLIMIT environment variable value for the collector gateway deployment.
	// this is when go runtime will start garbage collection.
	// if not specified, it will be set to 80% of the hard limit of the memory limiter.
	GoMemLimitMib int `json:"goMemLimitMiB,omitempty"`
}

// CodekarmaConfigurationSpec defines the desired state of CodekarmaConfiguration
//
// Deprecated: Use common.CodekarmaConfiguration instead
type CodekarmaConfigurationSpec struct {
	OdigosVersion     string                         `json:"odigosVersion"`
	ConfigVersion     int                            `json:"configVersion"`
	TelemetryEnabled  bool                           `json:"telemetryEnabled,omitempty"`
	OpenshiftEnabled  bool                           `json:"openshiftEnabled,omitempty"`
	IgnoredNamespaces []string                       `json:"ignoredNamespaces,omitempty"`
	IgnoredContainers []string                       `json:"ignoredContainers,omitempty"`
	Psp               bool                           `json:"psp,omitempty"`
	ImagePrefix       string                         `json:"imagePrefix,omitempty"`
	OdigletImage      string                         `json:"odigletImage,omitempty"`
	InstrumentorImage string                         `json:"instrumentorImage,omitempty"`
	AutoscalerImage   string                         `json:"autoscalerImage,omitempty"`
	CollectorGateway  *CollectorGatewayConfiguration `json:"collectorGateway,omitempty"`

	// this is internal currently, and is not exposed on the CLI / helm
	// used for odigos enterprise
	GoAutoIncludeCodeAttributes bool `json:"goAutoIncludeCodeAttributes,omitempty"`
}

//+genclient
//+kubebuilder:object:root=true
//+kubebuilder:metadata:labels=codekarma.tech/config=1
//+kubebuilder:metadata:labels=codekarma.tech/system-object=true

// CodekarmaConfiguration is the Schema for the codekarma configuration
//
// Deprecated: Use common.CodekarmaConfiguration instead
type CodekarmaConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec CodekarmaConfigurationSpec `json:"spec,omitempty"`
}

//+kubebuilder:object:root=true

// CodekarmaConfigurationList contains a list of CodekarmaConfiguration
type CodekarmaConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CodekarmaConfiguration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CodekarmaConfiguration{}, &CodekarmaConfigurationList{})
}

func (codekarmaConfig *CodekarmaConfiguration) ToCommonConfig() *common.CodekarmaConfiguration {
	var collectorGateway common.CollectorGatewayConfiguration
	if codekarmaConfig.Spec.CollectorGateway != nil {
		collectorGateway = common.CollectorGatewayConfiguration{
			RequestMemoryMiB:           codekarmaConfig.Spec.CollectorGateway.RequestMemoryMiB,
			MemoryLimiterLimitMiB:      codekarmaConfig.Spec.CollectorGateway.MemoryLimiterLimitMiB,
			MemoryLimiterSpikeLimitMiB: codekarmaConfig.Spec.CollectorGateway.MemoryLimiterSpikeLimitMiB,
			GoMemLimitMib:              codekarmaConfig.Spec.CollectorGateway.GoMemLimitMib,
		}
	}
	return &common.CodekarmaConfiguration{
		ConfigVersion:               codekarmaConfig.Spec.ConfigVersion,
		TelemetryEnabled:            codekarmaConfig.Spec.TelemetryEnabled,
		OpenshiftEnabled:            codekarmaConfig.Spec.OpenshiftEnabled,
		IgnoredNamespaces:           codekarmaConfig.Spec.IgnoredNamespaces,
		IgnoredContainers:           codekarmaConfig.Spec.IgnoredContainers,
		Psp:                         codekarmaConfig.Spec.Psp,
		ImagePrefix:                 codekarmaConfig.Spec.ImagePrefix,
		OdigletImage:                codekarmaConfig.Spec.OdigletImage,
		InstrumentorImage:           codekarmaConfig.Spec.InstrumentorImage,
		AutoscalerImage:             codekarmaConfig.Spec.AutoscalerImage,
		CollectorGateway:            &collectorGateway,
		GoAutoIncludeCodeAttributes: codekarmaConfig.Spec.GoAutoIncludeCodeAttributes,
	}
}
