// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"fmt"
	"log/slog"

	"go.opentelemetry.io/collector/pdata/ptrace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sys/unix"

	"go.opentelemetry.io/auto/internal/pkg/instrumentation/context"
	"go.opentelemetry.io/auto/internal/pkg/instrumentation/probe"
	"go.opentelemetry.io/auto/internal/pkg/structfield"
	"go.opentelemetry.io/auto/internal/pkg/telemetry"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target amd64,arm64 bpf ./bpf/probe.bpf.c

const (
	// pkg is the package being instrumented.
	pkg = "github.com/segmentio/kafka-go"
	// TOPIC_NAME_SIZE is the maximum size for Kafka topic names
	TOPIC_NAME_SIZE = 128
)

// New returns a new [probe.Probe].
func New(logger *slog.Logger, version string, serviceName string) probe.Probe {
	id := probe.ID{
		SpanKind:        trace.SpanKindProducer,
		InstrumentedPkg: pkg,
	}
	return &probe.SpanProducer[bpfObjects, event]{
		Base: probe.Base[bpfObjects, event]{
			ID:     id,
			Logger: logger,
			Consts: []probe.Const{
				probe.RegistersABIConst{},
				probe.AllocationConst{},
				probe.StructFieldConst{
					Key: "writer_topic_pos",
					Val: structfield.NewID("github.com/segmentio/kafka-go", "github.com/segmentio/kafka-go", "Writer", "Topic"),
				},
				probe.StructFieldConst{
					Key: "message_headers_pos",
					Val: structfield.NewID("github.com/segmentio/kafka-go", "github.com/segmentio/kafka-go", "Message", "Headers"),
				},
				probe.StructFieldConst{
					Key: "message_key_pos",
					Val: structfield.NewID("github.com/segmentio/kafka-go", "github.com/segmentio/kafka-go", "Message", "Key"),
				},
				probe.StructFieldConst{
					Key: "message_time_pos",
					Val: structfield.NewID("github.com/segmentio/kafka-go", "github.com/segmentio/kafka-go", "Message", "Time"),
				},
			},
			Uprobes: []probe.Uprobe{
				{
					Sym:         "github.com/segmentio/kafka-go.(*Writer).WriteMessages",
					EntryProbe:  "uprobe_WriteMessages",
					ReturnProbe: "uprobe_WriteMessages_Returns",
				},
			},
			SpecFn: loadBpf,
		},
		Version:     version,
		SchemaURL:   semconv.SchemaURL,
		ProcessFn:   processFn,
		ProcessFnC:  processFnC,
		ServiceName: serviceName,
	}
}

type messageAttributes struct {
	SpanContext context.EBPFSpanContext
	Topic       [TOPIC_NAME_SIZE]byte
	Key         [TOPIC_NAME_SIZE]byte
}

type topicAttributes struct {
	TopicName     [TOPIC_NAME_SIZE]byte
	ValidMessages uint16
}

type topicName struct {
	TopicName [TOPIC_NAME_SIZE]byte
}

// event represents a batch of kafka messages being sent.
type event struct {
	context.BaseSpanProperties
	// Global topic for the batch
	GlobalTopic   [TOPIC_NAME_SIZE]byte
	ValidMessages uint16
	IsGlobalTopic bool
	IsError       bool
}

func processFnC(e *event, logger *slog.Logger, serviceName string) telemetry.PathKey {
	// Create a new instance of CustomSpan
	logger.Info("=== Processing Kafka Producer event ===")
	typeName := "KafkaProducerBridge"
	parentTraceId := e.ParentSpanContext.TraceID.String()
	topic := unix.ByteSliceToString(e.GlobalTopic[:])
	if len(topic) == 0 || !e.IsGlobalTopic {
		logger.Info("=== Global topic not set in Kafka Producer. It is set as message level.. Setting topic to be CK_UNKNOWN_TOPIC ===")
		topic = "CK_UNKNOWN_TOPIC"
	}
	pathKeyStr := telemetry.GeneratePathKey(serviceName, typeName, parentTraceId, topic)

	pathKey := telemetry.PathKey{
		PathKey: pathKeyStr,
		GraphPathNode: telemetry.GraphPathNode{
			IncomingPath: &parentTraceId,
			Runtime:      "go",
			GraphPathElement: telemetry.KafkaProducerBridgeElement{
				Type:      typeName,
				TopicName: topic,
			},
			StartTime:  e.StartTime,
			EndTime:    e.EndTime,
			IsError:    e.IsError,
			ErrorCode:  "",
			EventCount: int(e.ValidMessages),
		},
	}
	logger.Info("=== Created Kafka Producer Bridge PathKey", "pathKey", pathKey)
	return pathKey
}

func processFn(e *event) ptrace.SpanSlice {
	// globalTopic := unix.ByteSliceToString(e.GlobalTopic[:])

	// attrs := []attribute.KeyValue{semconv.MessagingSystemKafka, semconv.MessagingOperationTypePublish}
	// if len(globalTopic) > 0 {
	// 	attrs = append(attrs, semconv.MessagingDestinationName(globalTopic))
	// }

	// if e.ValidMessages > 0 {
	// 	attrs = append(attrs, semconv.MessagingBatchMessageCount(int(e.ValidMessages)))
	// }

	// traceID := pcommon.TraceID(e.Messages[0].SpanContext.TraceID)

	spans := ptrace.NewSpanSlice()

	// var msgTopic string
	// for i := uint64(0); i < e.ValidMessages; i++ {
	// 	key := unix.ByteSliceToString(e.Messages[i].Key[:])
	// 	var msgAttrs []attribute.KeyValue
	// 	if len(key) > 0 {
	// 		msgAttrs = append(msgAttrs, semconv.MessagingKafkaMessageKey(key))
	// 	}

	// 	// Topic is either the global topic or the message specific topic
	// 	if len(globalTopic) == 0 {
	// 		msgTopic = unix.ByteSliceToString(e.Messages[i].Topic[:])
	// 	} else {
	// 		msgTopic = globalTopic
	// 	}

	// 	msgAttrs = append(msgAttrs, semconv.MessagingDestinationName(msgTopic))
	// 	msgAttrs = append(msgAttrs, attrs...)

	// 	span := spans.AppendEmpty()
	// 	span.SetName(kafkaProducerSpanName(msgTopic))
	// 	span.SetKind(ptrace.SpanKindProducer)
	// 	span.SetStartTimestamp(utils.BootOffsetToTimestamp(e.StartTime))
	// 	span.SetEndTimestamp(utils.BootOffsetToTimestamp(e.EndTime))
	// 	span.SetTraceID(traceID)
	// 	span.SetSpanID(pcommon.SpanID(e.Messages[i].SpanContext.SpanID))
	// 	span.SetFlags(uint32(trace.FlagsSampled))

	// 	if e.ParentSpanContext.SpanID.IsValid() {
	// 		span.SetParentSpanID(pcommon.SpanID(e.ParentSpanContext.SpanID))
	// 	}

	// 	utils.Attributes(span.Attributes(), msgAttrs...)
	// }

	return spans
}

func kafkaProducerSpanName(topic string) string {
	return fmt.Sprintf("%s publish", topic)
}
