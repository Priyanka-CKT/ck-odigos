package conditions

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UpdateStatusConditions updates the status conditions of the object if the passed conditions have changed.
// conditions is a pointer to the conditions of the object.
func UpdateStatusConditions(ctx context.Context, c client.Client, obj client.Object, conditions *[]metav1.Condition,
	status metav1.ConditionStatus, conditionType string, reason string, msg string) error {
	cond := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: obj.GetGeneration(),
	}
	logger := log.FromContext(ctx)
	logger.V(0).Info("Updating status conditions",
		"object", obj.GetName(),
		"namespace", obj.GetNamespace(),
		"conditionType", conditionType,
		"status", status,
		"reason", reason,
		"message", msg,
		"generation", obj.GetGeneration())

	changed := meta.SetStatusCondition(conditions, cond)

	if changed {
		logger.Info("Condition status changed",
			"object", obj.GetName(),
			"namespace", obj.GetNamespace(),
			"conditionType", conditionType,
			"oldStatus", meta.FindStatusCondition(*conditions, conditionType).Status,
			"newStatus", status)

		err := c.Status().Update(ctx, obj)
		if err != nil {
			return err
		}
	}
	return nil
}
