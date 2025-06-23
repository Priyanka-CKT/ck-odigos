package ebpf

import (
	"context"
	"fmt"

	"github.com/odigos-io/odigos/instrumentation"
	"github.com/odigos-io/odigos/instrumentation/detector"
	"github.com/odigos-io/odigos/k8sutils/pkg/consts"
	"github.com/odigos-io/odigos/k8sutils/pkg/workload"
	"github.com/odigos-io/odigos/odiglet/pkg/log"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type k8sDetailsResolver struct {
	client client.Client
}

func (dr *k8sDetailsResolver) Resolve(ctx context.Context, event detector.ProcessEvent) (K8sProcessGroup, error) {
	log.Logger.Info("Resolve in resolvers.go", "event", event)
	pod, err := dr.podFromProcEvent(ctx, event)
	if err != nil {
		return K8sProcessGroup{}, err
	}
	log.Logger.Info("pod from proc event", "pod", pod)
	containerName, found := containerNameFromProcEvent(event)
	if !found {
		return K8sProcessGroup{}, errContainerNameNotReported
	}
	log.Logger.Info("container name from proc event", "container name", containerName)
	podWorkload, err := workload.PodWorkloadObjectOrError(ctx, pod)
	if err != nil {
		return K8sProcessGroup{}, fmt.Errorf("failed to find workload object from pod manifest owners references: %w", err)
	}

	return K8sProcessGroup{
		pod:           pod,
		containerName: containerName,
		pw:            podWorkload,
	}, nil
}

func (dr *k8sDetailsResolver) podFromProcEvent(ctx context.Context, e detector.ProcessEvent) (*corev1.Pod, error) {
	log.Logger.Info("podFromProcEvent in resolvers.go", "event", e)
	eventEnvs := e.ExecDetails.Environments
	log.Logger.Info("eventEnvs in podFromProcEvent", "eventEnvs", eventEnvs)
	podName, ok := eventEnvs[consts.OdigosEnvVarPodName]
	if !ok {
		return nil, errPodNameNotReported
	}
	log.Logger.Info("podName in podFromProcEvent", "podName", podName)
	podNamespace, ok := eventEnvs[consts.OdigosEnvVarNamespace]
	log.Logger.Info("podNamespace in podFromProcEvent", "podNamespace", podNamespace)
	if !ok {
		return nil, errPodNameSpaceNotReported
	}
	log.Logger.Info("podNamespace in podFromProcEvent", "podNamespace", podNamespace)
	pod := corev1.Pod{}
	err := dr.client.Get(ctx, client.ObjectKey{Namespace: podNamespace, Name: podName}, &pod)
	if err != nil {
		return nil, fmt.Errorf("error fetching pod object: %w", err)
	}
	log.Logger.Info("pod in podFromProcEvent", "pod", pod)
	return &pod, nil
}

func containerNameFromProcEvent(event detector.ProcessEvent) (string, bool) {
	containerName, ok := event.ExecDetails.Environments[consts.OdigosEnvVarContainerName]
	return containerName, ok
}

type k8sConfigGroupResolver struct{}

func (cr *k8sConfigGroupResolver) Resolve(ctx context.Context, d K8sProcessGroup, dist instrumentation.OtelDistribution) (K8sConfigGroup, error) {
	log.Logger.Info("Resolve in resolvers.go", "d", d, "dist", dist)
	if d.pw == nil {
		return K8sConfigGroup{}, fmt.Errorf("podWorkload is not provided, cannot resolve config group")
	}
	log.Logger.Info("Resolve in resolvers.go after pw check", "pw", d.pw)
	return K8sConfigGroup{
		Pw:   *d.pw,
		Lang: dist.Language,
	}, nil
}
