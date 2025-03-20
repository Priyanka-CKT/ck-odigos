package watchers

import (
	"context"
	"fmt"
	"log"

	"github.com/odigos-io/odigos/api/odigos/v1alpha1"
	"github.com/odigos-io/odigos/frontend/endpoints/sse"
	"k8s.io/apimachinery/pkg/watch"
)

func StartDestinationWatcher(ctx context.Context, namespace string) error {
	// Return early since destinations are being deprecated
	log.Printf("Destination watcher is disabled as destinations are deprecated")
	return nil
}

func handleDestinationWatchEvents(ctx context.Context, watcher watch.Interface) {
	ch := watcher.ResultChan()
	for {
		select {
		case <-ctx.Done():
			watcher.Stop()
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			switch event.Type {
			case watch.Added:
				handleAddedDestination(event)
			case watch.Modified:
				handleModifiedDestination(event)
			case watch.Deleted:
				handleDeletedDestination(event)
			default:
				log.Printf("unexpected type: %T", event.Object)
			}
		}
	}
}

func handleAddedDestination(event watch.Event) {
	destination, ok := event.Object.(*v1alpha1.Destination)
	if !ok {
		genericErrorMessage(sse.MessageEventAdded, "Destination", "error type assertion")
	}
	data := fmt.Sprintf("Destination %s created", destination.Spec.DestinationName)
	sse.SendMessageToClient(sse.SSEMessage{Event: sse.MessageEventAdded, Type: "success", Target: destination.Name, Data: data, CRDType: "Destination"})
}

func handleModifiedDestination(event watch.Event) {
	destination, ok := event.Object.(*v1alpha1.Destination)
	if !ok {
		genericErrorMessage(sse.MessageEventModified, "Destination", "error type assertion")
	}
	if len(destination.Status.Conditions) == 0 {
		return
	}

	lastCondition := destination.Status.Conditions[len(destination.Status.Conditions)-1]
	data := lastCondition.Message
	conditionType := sse.MessageTypeSuccess
	if lastCondition.Status == "False" {
		conditionType = sse.MessageTypeError
	}
	sse.SendMessageToClient(sse.SSEMessage{Event: sse.MessageEventModified, Type: conditionType, Target: destination.Name, Data: data, CRDType: "Destination"})
}

func handleDeletedDestination(event watch.Event) {
	destination, ok := event.Object.(*v1alpha1.Destination)
	if !ok {
		genericErrorMessage(sse.MessageEventDeleted, "Destination", "error type assertion")
	}
	data := fmt.Sprintf("Destination %s deleted successfully", destination.Spec.DestinationName)
	sse.SendMessageToClient(sse.SSEMessage{Event: sse.MessageEventDeleted, Type: "success", Target: "", Data: data, CRDType: "Destination"})
}
