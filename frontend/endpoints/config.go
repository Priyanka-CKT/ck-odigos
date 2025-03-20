package endpoints

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
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
	Installation InstallationStatus `json:"installation"`
}

func GetConfig(c *gin.Context) {
	var response GetConfigResponse
	if !isSomethingLabeled(c.Request.Context()) {
		response.Installation = NewInstallation
	} else {
		// Skip destination check since destinations are being deprecated
		response.Installation = Finished
	}

	c.JSON(http.StatusOK, response)
}

// isDestinationChosen is simplified to always return true since destinations are being deprecated
func isDestinationChosen(ctx context.Context) bool {
	return true
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
