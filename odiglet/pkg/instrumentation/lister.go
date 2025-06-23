package instrumentation

import (
	"context"

	odigosclientset "github.com/odigos-io/odigos/api/generated/odigos/clientset/versioned"
	"k8s.io/client-go/rest"

	"github.com/odigos-io/odigos/procdiscovery/pkg/libc"

	"github.com/kubevirt/device-plugin-manager/pkg/dpm"
	"github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/odiglet/pkg/env"
	"github.com/odigos-io/odigos/odiglet/pkg/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	defaultMaxDevices = 100
)

type lister struct {
	plugins map[string]dpm.PluginInterface
}

func (l *lister) GetResourceNamespace() string {
	return common.OdigosResourceNamespace
}

func (l *lister) Discover(pluginNameLists chan dpm.PluginNameList) {
	var pluginNames []string
	for name := range l.plugins {
		pluginNames = append(pluginNames, name)
	}

	pluginNameLists <- pluginNames
}

func (l *lister) NewPlugin(s string) dpm.PluginInterface {
	return l.plugins[s]
}

// with this type, odiglet can determine which language specific function to use
// for each otel sdk in a each programming language.
// Otel SDKs frequently requires to set some environment variables and mount some fs dirs for it to work.
type OtelSdksLsf map[common.ProgrammingLanguage]map[common.OtelSdk]LangSpecificFunc

func NewLister(ctx context.Context, clientset *kubernetes.Clientset, otelSdksLsf OtelSdksLsf) (dpm.ListerInterface, error) {
	log.Logger.Info("Creating new lister with arguments", "clientset", clientset, "otelSdksLsf", otelSdksLsf)
	maxPods, err := getInitialDeviceAmount(clientset)
	if err != nil {
		return nil, err
	}

	isEbpfSupported := env.Current.IsEBPFSupported()
	log.Logger.Info("EBPF supported", "isEbpfSupported", isEbpfSupported)
	cfg, err := rest.InClusterConfig()
	if err != nil {
		log.Logger.Error(err, "Failed to init Kubernetes API client")
	}
	odigosKubeClient, err := odigosclientset.NewForConfig(cfg)
	log.Logger.Info("Odigos kube client", "odigosKubeClient", odigosKubeClient)
	if err != nil {
		log.Logger.Error(err, "Failed to init odigos client")
	}

	availablePlugins := map[string]dpm.PluginInterface{}

	log.Logger.Info("Available plugins", "availablePlugins", availablePlugins)
	for lang, otelSdkLsfMap := range otelSdksLsf {
		log.Logger.Info("Language", "lang", lang)
		log.Logger.Info("Otel SDK language specific functions", "otelSdkLsfMap", otelSdkLsfMap)
		for otelSdk, lsf := range otelSdkLsfMap {
			log.Logger.Info("Otel SDK", "otelSdk", otelSdk)
			if otelSdk.SdkType == common.EbpfOtelSdkType && !isEbpfSupported {
				continue
			}
			pluginName := common.InstrumentationPluginName(lang, otelSdk, nil)
			log.Logger.Info("Plugin name in lister.go", "pluginName", pluginName)
			availablePlugins[pluginName] = NewPlugin(maxPods, lsf, odigosKubeClient)
			log.Logger.Info("NewPlugin", "pluginName", pluginName)
			if libc.ShouldInspectForLanguage(lang) {
				musl := common.Musl
				pluginNameMusl := common.InstrumentationPluginName(lang, otelSdk, &musl)
				availablePlugins[pluginNameMusl] = NewMuslPlugin(lang, maxPods, lsf, odigosKubeClient)
			}
		}
	}
	log.Logger.Info("Available plugins after initialization", "plugins", availablePlugins)
	return &lister{
		plugins: availablePlugins,
	}, nil
}

func getInitialDeviceAmount(clientset *kubernetes.Clientset) (int64, error) {
	// get max pods per current node
	node, err := clientset.CoreV1().Nodes().Get(context.Background(), env.Current.NodeName, metav1.GetOptions{})
	if err != nil {
		return 0, err
	}

	maxPods, ok := node.Status.Allocatable.Pods().AsInt64()
	if !ok {
		log.Logger.V(0).Info("Failed to get max pods from node status, using default value", "default", defaultMaxDevices)
		maxPods = defaultMaxDevices
	}
	log.Logger.Info("Got max pods from node status", "maxPods", maxPods)
	return maxPods, nil
}
