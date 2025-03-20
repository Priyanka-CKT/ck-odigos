package endpoints

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/odigos-io/odigos/frontend/endpoints/destination_recognition"
	"github.com/odigos-io/odigos/k8sutils/pkg/env"

	"github.com/gin-gonic/gin"
	"github.com/odigos-io/odigos/api/odigos/v1alpha1"
	"github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/common/config"
	"github.com/odigos-io/odigos/destinations"
	"github.com/odigos-io/odigos/frontend/kube"
	k8s "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type GetDestinationTypesResponse struct {
	Categories []DestinationsCategory `json:"categories"`
}

type DestinationsCategory struct {
	Name  string                         `json:"name"`
	Items []DestinationTypesCategoryItem `json:"items"`
}

type DestinationTypesCategoryItem struct {
	Type                    common.DestinationType `json:"type"`
	DisplayName             string                 `json:"display_name"`
	ImageUrl                string                 `json:"image_url"`
	SupportedSignals        SupportedSignals       `json:"supported_signals"`
	TestConnectionSupported bool                   `json:"test_connection_supported"`
}

type SupportedSignals struct {
	Traces  ObservabilitySignalSupport `json:"traces"`
	Metrics ObservabilitySignalSupport `json:"metrics"`
	Logs    ObservabilitySignalSupport `json:"logs"`
}

type ObservabilitySignalSupport struct {
	Supported bool `json:"supported"`
}

type ExportedSignals struct {
	Traces  bool `json:"traces"`
	Metrics bool `json:"metrics"`
	Logs    bool `json:"logs"`
}

type Destination struct {
	Id              string                       `json:"id"`
	Name            string                       `json:"name"`
	Type            common.DestinationType       `json:"type"`
	ExportedSignals ExportedSignals              `json:"signals"`
	Fields          map[string]string            `json:"fields"`
	DestinationType DestinationTypesCategoryItem `json:"destination_type"`
	Conditions      []metav1.Condition           `json:"conditions,omitempty"`
}

var _ config.ExporterConfigurer = &Destination{}

func (dest Destination) GetID() string {
	return dest.Name
}
func (dest Destination) GetType() common.DestinationType {
	return dest.Type
}
func (dest Destination) GetConfig() map[string]string {
	return dest.Fields
}
func (dest Destination) GetSignals() []common.ObservabilitySignal {
	return exportedSignalsObjectToSlice(dest.ExportedSignals)
}

func GetDestinationTypes(c *gin.Context) {
	var resp GetDestinationTypesResponse
	itemsByCategory := make(map[string][]DestinationTypesCategoryItem)
	for _, destConfig := range destinations.Get() {
		item := DestinationTypeConfigToCategoryItem(destConfig)
		itemsByCategory[destConfig.Metadata.Category] = append(itemsByCategory[destConfig.Metadata.Category], item)
	}

	for category, items := range itemsByCategory {
		resp.Categories = append(resp.Categories, DestinationsCategory{
			Name:  category,
			Items: items,
		})
	}

	c.JSON(200, resp)
}

type GetDestinationDetailsResponse struct {
	Fields []Field `json:"fields"`
}

type Field struct {
	Name                string                 `json:"name"`
	DisplayName         string                 `json:"display_name"`
	ComponentType       string                 `json:"component_type"`
	ComponentProperties map[string]interface{} `json:"component_properties"`
	VideoUrl            string                 `json:"video_url,omitempty"`
	ThumbnailURL        string                 `json:"thumbnail_url,omitempty"`
	InitialValue        string                 `json:"initial_value,omitempty"`
}

func GetDestinationTypeDetails(c *gin.Context) {
	destType := common.DestinationType(c.Param("type"))
	destTypeConfig, err := getDestinationTypeConfig(destType)
	if err != nil {
		c.JSON(404, gin.H{
			"error": fmt.Sprintf("destination type %s not found", destType),
		})
		return
	}

	var resp GetDestinationDetailsResponse
	for _, field := range destTypeConfig.Spec.Fields {
		resp.Fields = append(resp.Fields, Field{
			Name:                field.Name,
			DisplayName:         field.DisplayName,
			ComponentType:       field.ComponentType,
			ComponentProperties: field.ComponentProps,
			VideoUrl:            field.VideoURL,
			ThumbnailURL:        field.ThumbnailURL,
			InitialValue:        field.InitialValue,
		})
	}

	c.JSON(200, resp)
}

func GetDestinations(c *gin.Context, odigosns string) {
	// Return empty array as destinations are being deprecated
	c.JSON(200, []Destination{})
}

func GetDestinationById(c *gin.Context, odigosns string) {
	// Return error as destinations are being deprecated
	c.JSON(404, gin.H{
		"error": "Destinations are deprecated and no longer supported",
	})
}

func CreateNewDestination(c *gin.Context, odigosns string) {
	// Return error as destinations are being deprecated
	c.JSON(400, gin.H{
		"error": "Destinations are deprecated and no longer supported",
	})
}

func TestConnectionForDestination(c *gin.Context, odigosns string) {
	// Return error as destinations are being deprecated
	c.JSON(400, gin.H{
		"error": "Destinations are deprecated and no longer supported",
	})
}

func DeleteDestination(c *gin.Context, odigosns string) {
	// Return error as destinations are being deprecated
	c.JSON(400, gin.H{
		"error": "Destinations are deprecated and no longer supported",
	})
}

func UpdateExistingDestination(c *gin.Context, odigosns string) {
	// Return error as destinations are being deprecated
	c.JSON(400, gin.H{
		"error": "Destinations are deprecated and no longer supported",
	})
}

func k8sDestinationToEndpointFormat(k8sDest v1alpha1.Destination, secretFields map[string]string) Destination {
	destType := k8sDest.Spec.Type
	destName := k8sDest.Spec.DestinationName
	mergedFields := mergeDataAndSecrets(k8sDest.Spec.Data, secretFields)
	destTypeConfig := DestinationTypeConfigToCategoryItem(destinations.GetDestinationByType(string(destType)))

	var conditions []metav1.Condition
	for _, condition := range k8sDest.Status.Conditions {
		conditions = append(conditions, metav1.Condition{
			Type:               condition.Type,
			Status:             condition.Status,
			Message:            condition.Message,
			LastTransitionTime: condition.LastTransitionTime,
		})
	}

	return Destination{
		Id:   k8sDest.Name,
		Name: destName,
		Type: destType,
		ExportedSignals: ExportedSignals{
			Traces:  isSignalExported(k8sDest, common.TracesObservabilitySignal),
			Metrics: isSignalExported(k8sDest, common.MetricsObservabilitySignal),
			Logs:    isSignalExported(k8sDest, common.LogsObservabilitySignal),
		},
		Fields:          mergedFields,
		DestinationType: destTypeConfig,
		Conditions:      conditions,
	}
}

func mergeDataAndSecrets(data map[string]string, secrets map[string]string) map[string]string {
	merged := map[string]string{}

	for k, v := range data {
		merged[k] = v
	}

	for k, v := range secrets {
		merged[k] = v
	}

	return merged
}

func isSignalExported(dest v1alpha1.Destination, signal common.ObservabilitySignal) bool {
	for _, s := range dest.Spec.Signals {
		if s == signal {
			return true
		}
	}

	return false
}

func exportedSignalsObjectToSlice(signals ExportedSignals) []common.ObservabilitySignal {
	var resp []common.ObservabilitySignal
	if signals.Traces {
		resp = append(resp, common.TracesObservabilitySignal)
	}
	if signals.Metrics {
		resp = append(resp, common.MetricsObservabilitySignal)
	}
	if signals.Logs {
		resp = append(resp, common.LogsObservabilitySignal)
	}

	return resp
}

func verifyDestinationDataScheme(destType common.DestinationType, destTypeConfig *destinations.Destination, data map[string]string) []error {

	errors := []error{}

	// verify all fields in config are present in data (assuming here all fields are required)
	for _, field := range destTypeConfig.Spec.Fields {
		required, ok := field.ComponentProps["required"].(bool)
		if !ok || !required {
			continue
		}
		fieldValue, found := data[field.Name]
		if !found || fieldValue == "" {
			errors = append(errors, fmt.Errorf("field %s is required", field.Name))
		}
	}

	// verify data fields are found in config
	for dataField := range data {
		found := false
		// iterating all fields in config every time, assuming it's a small list
		for _, field := range destTypeConfig.Spec.Fields {
			if dataField == field.Name {
				found = true
				break
			}
		}
		if !found {
			errors = append(errors, fmt.Errorf("field %s is not found in config for destination type '%s'", dataField, destType))
		}
	}

	return errors
}

func getDestinationTypeConfig(destType common.DestinationType) (*destinations.Destination, error) {
	for _, dest := range destinations.Get() {
		if dest.Metadata.Type == destType {
			return &dest, nil
		}
	}

	return nil, fmt.Errorf("destination type %s not found", destType)
}

func transformFieldsToDataAndSecrets(destTypeConfig *destinations.Destination, fields map[string]string) (map[string]string, map[string]string) {

	dataFields := map[string]string{}
	secretFields := map[string]string{}

	for fieldName, fieldValue := range fields {

		// it is possible that some fields are not required and are empty.
		// we should treat them as empty
		if fieldValue == "" {
			continue
		}

		// for each field in the data, find it's config
		// assuming the list is small so it's ok to iterate it
		for _, fieldConfig := range destTypeConfig.Spec.Fields {
			if fieldName == fieldConfig.Name {
				if fieldConfig.Secret {
					secretFields[fieldName] = fieldValue
				} else {
					dataFields[fieldName] = fieldValue
				}
			}
		}
	}

	return dataFields, secretFields
}

func getDestinationSecretFields(c *gin.Context, odigosns string, dest *v1alpha1.Destination) (map[string]string, error) {

	secretFields := map[string]string{}
	secretRef := dest.Spec.SecretRef

	if secretRef == nil {
		return secretFields, nil
	}

	secret, err := kube.DefaultClient.CoreV1().Secrets(odigosns).Get(c, secretRef.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	for k, v := range secret.Data {
		secretFields[k] = string(v)
	}

	return secretFields, nil
}

func DestinationTypeConfigToCategoryItem(destConfig destinations.Destination) DestinationTypesCategoryItem {
	return DestinationTypesCategoryItem{
		Type:                    destConfig.Metadata.Type,
		DisplayName:             destConfig.Metadata.DisplayName,
		ImageUrl:                GetImageURL(destConfig.Spec.Image),
		TestConnectionSupported: destConfig.Spec.TestConnectionSupported,
		SupportedSignals: SupportedSignals{
			Traces: ObservabilitySignalSupport{
				Supported: destConfig.Spec.Signals.Traces.Supported,
			},
			Metrics: ObservabilitySignalSupport{
				Supported: destConfig.Spec.Signals.Metrics.Supported,
			},
			Logs: ObservabilitySignalSupport{
				Supported: destConfig.Spec.Signals.Logs.Supported,
			},
		},
	}
}

func createDestinationSecret(ctx context.Context, destType common.DestinationType, secretFields map[string]string, odigosns string) (*k8s.LocalObjectReference, error) {
	generateNamePrefix := "odigos.io.dest." + string(destType) + "-"
	secret := k8s.Secret{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: generateNamePrefix,
		},
		StringData: secretFields,
	}
	newSecret, err := kube.DefaultClient.CoreV1().Secrets(odigosns).Create(ctx, &secret, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	return &k8s.LocalObjectReference{
		Name: newSecret.Name,
	}, nil
}

func addDestinationOwnerReferenceToSecret(ctx context.Context, odigosns string, dest *v1alpha1.Destination) error {
	destOwnerRef := metav1.OwnerReference{
		APIVersion: "odigos.io/v1alpha1",
		Kind:       "Destination",
		Name:       dest.Name,
		UID:        dest.UID,
	}

	secretPatch := []struct {
		Op    string                  `json:"op"`
		Path  string                  `json:"path"`
		Value []metav1.OwnerReference `json:"value"`
	}{{
		Op:    "add",
		Path:  "/metadata/ownerReferences",
		Value: []metav1.OwnerReference{destOwnerRef},
	},
	}

	secretPatchBytes, err := json.Marshal(secretPatch)
	if err != nil {
		return err
	}

	_, err = kube.DefaultClient.CoreV1().Secrets(odigosns).Patch(ctx, dest.Spec.SecretRef.Name, types.JSONPatchType, secretPatchBytes, metav1.PatchOptions{})
	if err != nil {
		return err
	}
	return nil
}

func potentialDestinations(c *gin.Context, odigosns string) []destination_recognition.DestinationDetails {
	relevantNamespaces, err := getRelevantNameSpaces(c, env.GetCurrentNamespace())
	if err != nil {
		return nil
	}

	// Existing Destinations
	existingDestination, err := kube.DefaultClient.OdigosClient.Destinations(odigosns).List(c, metav1.ListOptions{})
	if err != nil {
		return nil
	}

	destinationDetails, err := destination_recognition.GetAllPotentialDestinationDetails(c, relevantNamespaces, existingDestination)
	if err != nil {
		return nil
	}

	return destinationDetails
}
