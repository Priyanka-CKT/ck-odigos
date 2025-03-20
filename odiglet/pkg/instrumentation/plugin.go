package instrumentation

import (
	"context"

	"github.com/odigos-io/odigos/procdiscovery/pkg/libc"

	"github.com/kubevirt/device-plugin-manager/pkg/dpm"
	odigosclientset "github.com/odigos-io/odigos/api/generated/odigos/clientset/versioned"
	"github.com/odigos-io/odigos/common"
	k8sconsts "github.com/odigos-io/odigos/k8sutils/pkg/consts"
	"github.com/odigos-io/odigos/k8sutils/pkg/env"
	"github.com/odigos-io/odigos/odiglet/pkg/instrumentation/devices"
	"github.com/odigos-io/odigos/odiglet/pkg/log"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

type LangSpecificFunc func(deviceId string, enabledSignals map[common.ObservabilitySignal]struct{}) *v1beta1.ContainerAllocateResponse

type plugin struct {
	idsManager       devices.DeviceManager
	stopCh           chan struct{}
	LangSpecificFunc LangSpecificFunc
	odigosKubeClient *odigosclientset.Clientset
}

func NewPlugin(maxPods int64, lsf LangSpecificFunc, odigosKubeClient *odigosclientset.Clientset) dpm.PluginInterface {
	idManager := devices.NewIDManager(maxPods)

	return &plugin{
		idsManager:       idManager,
		stopCh:           make(chan struct{}),
		LangSpecificFunc: lsf,
		odigosKubeClient: odigosKubeClient,
	}
}

func NewMuslPlugin(lang common.ProgrammingLanguage, maxPods int64, lsf LangSpecificFunc, odigosKubeClient *odigosclientset.Clientset) dpm.PluginInterface {
	wrappedLsf := func(deviceId string, enabledSignals map[common.ObservabilitySignal]struct{}) *v1beta1.ContainerAllocateResponse {
		res := lsf(deviceId, enabledSignals)
		libc.ModifyEnvVarsForMusl(lang, res.Envs)
		return res
	}

	return NewPlugin(maxPods, wrappedLsf, odigosKubeClient)
}

func (p *plugin) GetDevicePluginOptions(ctx context.Context, empty *v1beta1.Empty) (*v1beta1.DevicePluginOptions, error) {
	return &v1beta1.DevicePluginOptions{
		PreStartRequired:                false,
		GetPreferredAllocationAvailable: false,
	}, nil
}

func (p *plugin) ListAndWatch(empty *v1beta1.Empty, server v1beta1.DevicePlugin_ListAndWatchServer) error {
	devicesList := p.idsManager.GetDevices()
	log.Logger.V(3).Info("ListAndWatch", "devices", devicesList)
	err := server.Send(&v1beta1.ListAndWatchResponse{
		Devices: devicesList,
	})

	if err != nil {
		log.Logger.Error(err, "Failed to send ListAndWatchResponse")
	}

	<-p.stopCh
	server.Send(&v1beta1.ListAndWatchResponse{
		Devices: []*v1beta1.Device{},
	})
	return nil
}

func (p *plugin) Stop() error {
	log.Logger.V(0).Info("Stopping Odigos Device Plugin ...")
	p.stopCh <- struct{}{}
	return nil
}

func (p *plugin) GetPreferredAllocation(ctx context.Context, request *v1beta1.PreferredAllocationRequest) (*v1beta1.PreferredAllocationResponse, error) {
	return &v1beta1.PreferredAllocationResponse{}, nil
}

func (p *plugin) Allocate(ctx context.Context, request *v1beta1.AllocateRequest) (*v1beta1.AllocateResponse, error) {
	res := &v1beta1.AllocateResponse{}

	// calculate the enabled signals from the collectors group status.
	// in any error, just use empty enabled signals.
	// If the Allocate returns an error, the pod will not be scheduled which we have to avoid no matter what.
	enabledSignals := make(map[common.ObservabilitySignal]struct{})

	odigosNs := env.GetCurrentNamespace()
	nodeCollectorGroup, err := p.odigosKubeClient.OdigosV1alpha1().CollectorsGroups(odigosNs).Get(ctx, k8sconsts.OdigosNodeCollectorCollectorGroupName, metav1.GetOptions{})
	if err != nil {
		// we should have collectors group created for odigos device to trigger.
		// however if we don't, just log and enable all signals by default.
		if apierrors.IsNotFound(err) {
			log.Logger.Info("Collector group not found. Enabling all signals by default.")
			// Enable all signals by default
			enabledSignals[common.TracesObservabilitySignal] = struct{}{}
			enabledSignals[common.MetricsObservabilitySignal] = struct{}{}
			enabledSignals[common.LogsObservabilitySignal] = struct{}{}
		} else {
			log.Logger.Error(err, "error getting node collectors group, enabling all signals by default")
			// Enable all signals by default in case of any error
			enabledSignals[common.TracesObservabilitySignal] = struct{}{}
			enabledSignals[common.MetricsObservabilitySignal] = struct{}{}
			enabledSignals[common.LogsObservabilitySignal] = struct{}{}
		}
	} else {
		for _, signal := range nodeCollectorGroup.Status.ReceiverSignals {
			enabledSignals[signal] = struct{}{}
		}
	}

	for _, req := range request.ContainerRequests {
		if len(req.DevicesIDs) != 1 {
			log.Logger.V(0).Info("got  instrumentation device not equal to 1, skipping", "devices", req.DevicesIDs)
			continue
		}

		deviceId := req.DevicesIDs[0]
		res.ContainerResponses = append(res.ContainerResponses, p.LangSpecificFunc(deviceId, enabledSignals))
	}

	return res, nil
}

func (p *plugin) PreStartContainer(ctx context.Context, request *v1beta1.PreStartContainerRequest) (*v1beta1.PreStartContainerResponse, error) {
	return &v1beta1.PreStartContainerResponse{}, nil
}
