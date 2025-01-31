package ebpf

import (
	"context"
	"fmt"

	"github.com/odigos-io/odigos/instrumentation"
	odgiosK8s "github.com/odigos-io/odigos/k8sutils/pkg/container"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type podDeviceDistributionMatcher struct{}

func (dm *podDeviceDistributionMatcher) Distribution(ctx context.Context, e K8sProcessGroup) (instrumentation.OtelDistribution, error) {
	// get the language and sdk for this process event
	// based on the pod spec and the container name from the process event
	// TODO: We should have all the required information in the process event
	// to determine the language - hence in the future we can improve this
	lang, sdk, err := odgiosK8s.LanguageSdkFromPodContainer(e.pod, e.containerName)
	logger := log.FromContext(ctx)
	logger.Info("getting language and sdk from pod container",
		"pod", e.pod.Name,
		"container", e.containerName,
		"language", lang,
		"sdk", sdk)
	if err != nil {
		return instrumentation.OtelDistribution{}, fmt.Errorf("failed to get language and sdk: %w", err)
	}
	return instrumentation.OtelDistribution{Language: lang, OtelSdk: sdk}, nil
}
