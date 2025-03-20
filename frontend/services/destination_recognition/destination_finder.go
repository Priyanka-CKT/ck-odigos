package destination_recognition

import (
	"context"

	"github.com/odigos-io/odigos/common"
	k8s "k8s.io/api/core/v1"
)

var SupportedDestinationType = []common.DestinationType{common.JaegerDestinationType, common.ElasticsearchDestinationType}

type DestinationDetails struct {
	Type   common.DestinationType `json:"type"`
	Fields map[string]string      `json:"fields"`
}

type IDestinationFinder interface {
	isPotentialService(k8s.Service) bool
	fetchDestinationDetails(k8s.Service) DestinationDetails
	getServiceURL() string
}

// GetAllPotentialDestinationDetails returns an empty list as destinations are being deprecated
func GetAllPotentialDestinationDetails(ctx context.Context, namespaces []k8s.Namespace, _ interface{}) ([]DestinationDetails, error) {
	// Return empty list as destinations are being deprecated
	return []DestinationDetails{}, nil
}

func getDestinationFinder(destinationType common.DestinationType) IDestinationFinder {
	switch destinationType {
	case common.JaegerDestinationType:
		return &JaegerDestinationFinder{}
	case common.ElasticsearchDestinationType:
		return &ElasticSearchDestinationFinder{}
	}

	return nil
}

// destinationExist is no longer needed as we're not checking against destination CRD anymore
func destinationExist(_ interface{}, _ DestinationDetails, _ IDestinationFinder) bool {
	return false
}
