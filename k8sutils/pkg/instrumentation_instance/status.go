package instrumentation_instance

import (
	"context"
	"fmt"

	odigosv1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	"github.com/odigos-io/odigos/common/consts"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type KarmaInstrumentationInstanceOption interface {
	applyKarmaInstrumentationInstance(odigosv1.KarmaInstrumentationInstanceStatus) odigosv1.KarmaInstrumentationInstanceStatus
}

type updateKarmaInstrumentationInstanceStatusOpt func(odigosv1.KarmaInstrumentationInstanceStatus) odigosv1.KarmaInstrumentationInstanceStatus

func (o updateKarmaInstrumentationInstanceStatusOpt) applyKarmaInstrumentationInstance(s odigosv1.KarmaInstrumentationInstanceStatus) odigosv1.KarmaInstrumentationInstanceStatus {
	return o(s)
}

// set Healthy and related fields in KarmaInstrumentationInstanceStatus
func WithHealthy(healthy *bool, reason string, message *string) KarmaInstrumentationInstanceOption {
	return updateKarmaInstrumentationInstanceStatusOpt(func(s odigosv1.KarmaInstrumentationInstanceStatus) odigosv1.KarmaInstrumentationInstanceStatus {
		s.Healthy = healthy
		s.Reason = reason
		if message != nil {
			s.Message = *message
		} else {
			s.Message = ""
		}
		return s
	})
}

func WithAttributes(identifying []odigosv1.Attribute, nonIdentifying []odigosv1.Attribute) KarmaInstrumentationInstanceOption {
	return updateKarmaInstrumentationInstanceStatusOpt(func(s odigosv1.KarmaInstrumentationInstanceStatus) odigosv1.KarmaInstrumentationInstanceStatus {
		s.IdentifyingAttributes = identifying
		s.NonIdentifyingAttributes = nonIdentifying
		return s
	})
}

func updateKarmaInstrumentationInstanceStatus(status odigosv1.KarmaInstrumentationInstanceStatus, options ...KarmaInstrumentationInstanceOption) odigosv1.KarmaInstrumentationInstanceStatus {
	for _, option := range options {
		status = option.applyKarmaInstrumentationInstance(status)
	}
	status.LastStatusTime = metav1.Now()
	return status
}

func KarmaInstrumentationInstanceName(ownerName string, pid int) string {
	return fmt.Sprintf("%s-%d", ownerName, pid)
}

func UpdateKarmaInstrumentationInstanceStatus(ctx context.Context, owner client.Object, containerName string, kubeClient client.Client, instrumentedAppName string, pid int, scheme *runtime.Scheme, options ...KarmaInstrumentationInstanceOption) error {
	instrumentationInstanceName := KarmaInstrumentationInstanceName(owner.GetName(), pid)
	updatedInstance := &odigosv1.KarmaInstrumentationInstance{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "codekarma.tech/v1alpha1",
			Kind:       "KarmaInstrumentationInstance",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      instrumentationInstanceName,
			Namespace: owner.GetNamespace(),
			Labels: map[string]string{
				consts.InstrumentedAppNameLabel: instrumentedAppName,
			},
		},
		Spec: odigosv1.KarmaInstrumentationInstanceSpec{
			ContainerName: containerName,
		},
	}

	err := controllerutil.SetControllerReference(owner, updatedInstance, scheme)
	if err != nil {
		return err
	}

	if err = kubeClient.Create(ctx, updatedInstance); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
		err := kubeClient.Get(ctx, client.ObjectKeyFromObject(updatedInstance), updatedInstance)
		if err != nil {
			return err
		}
	}

	updatedInstance.Status = updateKarmaInstrumentationInstanceStatus(updatedInstance.Status, options...)

	err = kubeClient.Status().Update(ctx, updatedInstance)
	if err != nil {
		return err
	}
	return nil
}

// Function aliases for backward compatibility
func UpdateInstrumentationInstanceStatus(ctx context.Context, owner client.Object, containerName string, kubeClient client.Client, instrumentedAppName string, pid int, scheme *runtime.Scheme, options ...KarmaInstrumentationInstanceOption) error {
	return UpdateKarmaInstrumentationInstanceStatus(ctx, owner, containerName, kubeClient, instrumentedAppName, pid, scheme, options...)
}

func InstrumentationInstanceName(ownerName string, pid int) string {
	return KarmaInstrumentationInstanceName(ownerName, pid)
}

// Type alias for backward compatibility
type InstrumentationInstanceOption = KarmaInstrumentationInstanceOption
