package utils

import (
	"context"

	"github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/common/consts"
	"github.com/odigos-io/odigos/k8sutils/pkg/env"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

func GetCurrentCodekarmaConfig(ctx context.Context, k8sClient client.Client) (common.CodekarmaConfiguration, error) {
	var configMap v1.ConfigMap
	var codekarmaConfig common.CodekarmaConfiguration
	codekarmaSystemNamespaceName := env.GetCurrentNamespace()
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: codekarmaSystemNamespaceName, Name: consts.CodekarmaConfigurationName}, &configMap); err != nil {
		return codekarmaConfig, err
	}
	if err := yaml.Unmarshal([]byte(configMap.Data[consts.CodekarmaConfigurationFileName]), &codekarmaConfig); err != nil {
		return codekarmaConfig, err
	}
	return codekarmaConfig, nil
}

// Function alias for backward compatibility
func GetCurrentOdigosConfig(ctx context.Context, k8sClient client.Client) (common.CodekarmaConfiguration, error) {
	return GetCurrentCodekarmaConfig(ctx, k8sClient)
}
