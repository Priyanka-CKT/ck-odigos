package services

import (
	"github.com/odigos-io/odigos/frontend/graph/model"
)

// ClusterInfo represents cluster information
type ClusterInfo struct {
	AttributeName        string  `json:"attributeName"`
	AttributeStringValue *string `json:"attributeStringValue,omitempty"`
}

// Action interface for all action types
type Action interface {
	GetID() string
	GetType() string
	GetName() *string
	GetNotes() *string
	GetDisable() bool
	GetSignals() []model.SignalType
}

// BaseAction provides common fields for all action types
type BaseAction struct {
	ID      string             `json:"id"`
	Type    string             `json:"type"`
	Name    *string            `json:"name,omitempty"`
	Notes   *string            `json:"notes,omitempty"`
	Disable bool               `json:"disable"`
	Signals []model.SignalType `json:"signals"`
}

// GetID returns the action ID
func (a BaseAction) GetID() string {
	return a.ID
}

// GetType returns the action type
func (a BaseAction) GetType() string {
	return a.Type
}

// GetName returns the action name
func (a BaseAction) GetName() *string {
	return a.Name
}

// GetNotes returns the action notes
func (a BaseAction) GetNotes() *string {
	return a.Notes
}

// GetDisable returns the action disable status
func (a BaseAction) GetDisable() bool {
	return a.Disable
}

// GetSignals returns the action signals
func (a BaseAction) GetSignals() []model.SignalType {
	return a.Signals
}

// AddClusterInfoAction represents an action to add cluster info
type AddClusterInfoAction struct {
	BaseAction
	Details []*ClusterInfo `json:"details"`
}

// DeleteAttributeAction represents an action to delete attributes
type DeleteAttributeAction struct {
	BaseAction
	Details []string `json:"details"`
}

// ErrorSamplerAction represents an error sampler action
type ErrorSamplerAction struct {
	BaseAction
	Details string `json:"details"`
}

// LatencySamplerAction represents a latency sampler action
type LatencySamplerAction struct {
	BaseAction
	Details []*string `json:"details"`
}

// PiiMaskingAction represents a PII masking action
type PiiMaskingAction struct {
	BaseAction
	Details []string `json:"details,omitempty"`
}

// ProbabilisticSamplerAction represents a probabilistic sampler action
type ProbabilisticSamplerAction struct {
	BaseAction
	Details string `json:"details"`
}

// RenameAttributeAction represents an action to rename attributes
type RenameAttributeAction struct {
	BaseAction
	Details string `json:"details"`
}
