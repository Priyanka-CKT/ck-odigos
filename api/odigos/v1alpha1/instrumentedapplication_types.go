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

package v1alpha1

import (
	"github.com/odigos-io/odigos/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:generate=true
type ConfigOption struct {
	OptionKey string          `json:"optionKey"`
	SpanKind  common.SpanKind `json:"spanKind"`
}

// +kubebuilder:object:generate=true
type InstrumentationLibraryOptions struct {
	LibraryName string         `json:"libraryName"`
	Options     []ConfigOption `json:"options"`
}

// +kubebuilder:object:generate=true
type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type OtherAgent struct {
	Name string `json:"name,omitempty"`
}

// +kubebuilder:object:generate=true
type RuntimeDetailsByContainer struct {
	ContainerName  string                     `json:"containerName"`
	Language       common.ProgrammingLanguage `json:"language"`
	RuntimeVersion string                     `json:"runtimeVersion,omitempty"`
	EnvVars        []EnvVar                   `json:"envVars,omitempty"`
	OtherAgent     *OtherAgent                `json:"otherAgent,omitempty"`
	LibCType       *common.LibCType           `json:"libCType,omitempty"`
}

// +kubebuilder:object:generate=true
type OptionByContainer struct {
	ContainerName            string                          `json:"containerName"`
	InstrumentationLibraries []InstrumentationLibraryOptions `json:"instrumentationsLibraries"`
}

// KarmaInstrumentedApplicationSpec defines the desired state of KarmaInstrumentedApplication
type KarmaInstrumentedApplicationSpec struct {
	RuntimeDetails []RuntimeDetailsByContainer `json:"runtimeDetails,omitempty"`
	Options        []OptionByContainer         `json:"options,omitempty"`
}

// KarmaInstrumentedApplicationStatus defines the observed state of KarmaInstrumentedApplication
type KarmaInstrumentedApplicationStatus struct {
	// Represents the observations of a nstrumentedApplication's current state.
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" protobuf:"bytes,1,rep,name=conditions"`
}

//+genclient
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:metadata:labels=codekarma.tech/config=1
//+kubebuilder:metadata:labels=codekarma.tech/system-object=true

// KarmaInstrumentedApplication is the Schema for the karmainstrumentedapplications API
type KarmaInstrumentedApplication struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KarmaInstrumentedApplicationSpec   `json:"spec,omitempty"`
	Status KarmaInstrumentedApplicationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// KarmaInstrumentedApplicationList contains a list of KarmaInstrumentedApplication
type KarmaInstrumentedApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KarmaInstrumentedApplication `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KarmaInstrumentedApplication{}, &KarmaInstrumentedApplicationList{})
}

// Type aliases for backward compatibility
type InstrumentedApplication = KarmaInstrumentedApplication
type InstrumentedApplicationSpec = KarmaInstrumentedApplicationSpec
type InstrumentedApplicationStatus = KarmaInstrumentedApplicationStatus
type InstrumentedApplicationList = KarmaInstrumentedApplicationList
