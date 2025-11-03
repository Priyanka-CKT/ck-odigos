package utils

import (
	odigosv1alpha1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

type OtelSdkKarmaInstrumentationRulePredicate struct{}

func (o OtelSdkKarmaInstrumentationRulePredicate) Create(e event.CreateEvent) bool {
	// check if delete rule is for otel sdk
	instrumentationRule, ok := e.Object.(*odigosv1alpha1.KarmaInstrumentationRule)
	if !ok {
		return false
	}

	return instrumentationRule.Spec.OtelSdks != nil
}

func (i OtelSdkKarmaInstrumentationRulePredicate) Update(e event.UpdateEvent) bool {
	oldKarmaInstrumentationRule, oldOk := e.ObjectOld.(*odigosv1alpha1.KarmaInstrumentationRule)
	newKarmaInstrumentationRule, newOk := e.ObjectNew.(*odigosv1alpha1.KarmaInstrumentationRule)

	if !oldOk || !newOk {
		return false
	}

	// only handle rules for otel sdks
	return oldKarmaInstrumentationRule.Spec.OtelSdks != nil || newKarmaInstrumentationRule.Spec.OtelSdks != nil
}

func (i OtelSdkKarmaInstrumentationRulePredicate) Delete(e event.DeleteEvent) bool {
	// check if delete rule is for otel sdk
	instrumentationRule, ok := e.Object.(*odigosv1alpha1.KarmaInstrumentationRule)
	if !ok {
		return false
	}

	return instrumentationRule.Spec.OtelSdks != nil
}

func (i OtelSdkKarmaInstrumentationRulePredicate) Generic(e event.GenericEvent) bool {
	return false
}
