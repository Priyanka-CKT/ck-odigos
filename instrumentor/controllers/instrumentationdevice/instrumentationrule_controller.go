package instrumentationdevice

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// InstrumentationRuleReconciler is a placeholder struct
// We no longer use this controller since we've removed the InstrumentationRule dependency
type InstrumentationRuleReconciler struct {
	client.Client
}
