package watchers

import (
	"context"
	"fmt"
	"time"

	"github.com/odigos-io/odigos/api/odigos/v1alpha1"
	"github.com/odigos-io/odigos/frontend/endpoints/sse"
	"github.com/odigos-io/odigos/frontend/kube"
	commonutils "github.com/odigos-io/odigos/k8sutils/pkg/workload"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

var addedEventBatcher *EventBatcher
var deletedEventBatcher *EventBatcher

func StartKarmaInstrumentedApplicationWatcher(ctx context.Context, namespace string) error {
	addedEventBatcher = NewEventBatcher(
		EventBatcherConfig{
			Event:        sse.MessageEventAdded,
			CRDType:      "KarmaInstrumentedApplication",
			MinBatchSize: 4,
			Duration:     5000 * time.Millisecond,
			SuccessBatchMessageFunc: func(count int, crdType string) string {
				return fmt.Sprintf("successfully added %d sources", count)
			},
			FailureBatchMessageFunc: func(count int, crdType string) string {
				return fmt.Sprintf("failed to add %d sources", count)
			},
		},
	)

	deletedEventBatcher = NewEventBatcher(
		EventBatcherConfig{
			Event:        sse.MessageEventDeleted,
			CRDType:      "KarmaInstrumentedApplication",
			MinBatchSize: 4,
			Duration:     5000 * time.Millisecond,
			SuccessBatchMessageFunc: func(count int, crdType string) string {
				return fmt.Sprintf("successfully deleted %d sources", count)
			},
			FailureBatchMessageFunc: func(count int, crdType string) string {
				return fmt.Sprintf("failed to delete %d sources", count)
			},
		},
	)

	watcher, err := kube.DefaultClient.CodekarmaClient.KarmaInstrumentedApplications(namespace).Watch(context.Background(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("error creating watcher: %v", err)
	}

	go handleKarmaInstrumentedApplicationWatchEvents(ctx, watcher)
	return nil
}

func handleKarmaInstrumentedApplicationWatchEvents(ctx context.Context, watcher watch.Interface) {
	ch := watcher.ResultChan()
	defer addedEventBatcher.Cancel()
	defer deletedEventBatcher.Cancel()
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
				handleAddedEvent(event.Object.(*v1alpha1.KarmaInstrumentedApplication))
			case watch.Deleted:
				handleDeletedEvent(event.Object.(*v1alpha1.KarmaInstrumentedApplication))
			}
		}
	}
}

func handleAddedEvent(app *v1alpha1.KarmaInstrumentedApplication) {
	name, kind, err := commonutils.ExtractWorkloadInfoFromRuntimeObjectName(app.Name)
	if err != nil {
		genericErrorMessage(sse.MessageEventAdded, "KarmaInstrumentedApplication", "error getting workload info")
		return
	}
	namespace := app.Namespace
	target := fmt.Sprintf("name=%s&kind=%s&namespace=%s", name, kind, namespace)
	data := fmt.Sprintf("KarmaInstrumentedApplication %s created", name)
	fmt.Printf("Sending added event for source %s\n", name)
	addedEventBatcher.AddEvent(sse.MessageTypeSuccess, data, target)
}

func handleDeletedEvent(app *v1alpha1.KarmaInstrumentedApplication) {
	name, _, err := commonutils.ExtractWorkloadInfoFromRuntimeObjectName(app.Name)
	if err != nil {
		genericErrorMessage(sse.MessageEventDeleted, "KarmaInstrumentedApplication", "error getting workload info")
		return
	}
	data := fmt.Sprintf("Source %s deleted successfully", name)
	fmt.Printf("Sending deleted event for source %s\n", name)
	deletedEventBatcher.AddEvent(sse.MessageTypeSuccess, data, "")
}
