package services

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/odigos-io/odigos/common/consts"
	"github.com/odigos-io/odigos/frontend/kube"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type InstallationStatus string

const (
	NewInstallation InstallationStatus = "NEW"
	AppsSelected    InstallationStatus = "APPS_SELECTED"
	Finished        InstallationStatus = "FINISHED"
)

type GetConfigResponse struct {
	Installation  InstallationStatus `json:"installation"`
	NexusEndpoint string             `json:"nexusEndpoint"`
}

func GetConfig(c context.Context) GetConfigResponse {
	var response GetConfigResponse
	if !isSomethingLabeled(c) {
		response.Installation = NewInstallation
	} else if !isDestinationChosen(c) {
		response.Installation = AppsSelected
	} else {
		response.Installation = Finished
	}

	// Read endpoint from environment, prefer CK_ENDPOINT over CK_NEXUS_ENDPOINT
	ckEndpoint := os.Getenv("CK_ENDPOINT")
	if ckEndpoint != "" {
		response.NexusEndpoint = ckEndpoint
	} else {
		response.NexusEndpoint = os.Getenv("CK_NEXUS_ENDPOINT")
	}

	return response
}

func isDestinationChosen(ctx context.Context) bool {
	dests, err := kube.DefaultClient.CodekarmaClient.Destinations("").List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("Error listing destinations: %v\n", err)
		return false
	}

	return len(dests.Items) > 0
}

func isSomethingLabeled(ctx context.Context) bool {
	labelSelector := fmt.Sprintf("%s=%s", consts.OdigosInstrumentationLabel, consts.InstrumentationEnabled)
	ns, err := kube.DefaultClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})

	if err != nil {
		log.Printf("Error listing namespaces: %v\n", err)
		return false
	}

	if len(ns.Items) > 0 {
		return true
	}

	deps, err := kube.DefaultClient.AppsV1().Deployments("").List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})

	if err != nil {
		log.Printf("Error listing deployments: %v\n", err)
		return false
	}

	if len(deps.Items) > 0 {
		return true
	}

	ss, err := kube.DefaultClient.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})

	if err != nil {
		log.Printf("Error listing statefulsets: %v\n", err)
		return false
	}

	if len(ss.Items) > 0 {
		return true
	}

	ds, err := kube.DefaultClient.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})

	if err != nil {
		log.Printf("Error listing daemonsets: %v\n", err)
		return false
	}

	if len(ds.Items) > 0 {
		return true
	}

	return false
}
