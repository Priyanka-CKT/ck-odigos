// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package awssqs

import (
	"bytes"
	"log/slog"

	"go.opentelemetry.io/collector/pdata/ptrace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"go.opentelemetry.io/auto/internal/pkg/instrumentation/context"
	"go.opentelemetry.io/auto/internal/pkg/instrumentation/probe"
	"go.opentelemetry.io/auto/internal/pkg/telemetry"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target amd64,arm64 bpf ./bpf/probe.bpf.c

// New returns a new [probe.Probe].
func New(logger *slog.Logger, version string, serviceName string) probe.Probe {
	id := probe.ID{
		SpanKind:        trace.SpanKindProducer,
		InstrumentedPkg: "github.com/aws/aws-sdk-go-v2",
	}
	return &probe.SpanProducer[bpfObjects, event]{
		Base: probe.Base[bpfObjects, event]{
			ID:     id,
			Logger: logger,
			Consts: []probe.Const{
				probe.RegistersABIConst{},
				probe.AllocationConst{},
				// probe.StructFieldConst{
				// 	Key: "message_time_pos",
				// 	Val: structfield.NewID("github.com/segmentio/kafka-go", "github.com/segmentio/kafka-go", "Message", "Time"),
				// },
			},
			Uprobes: []probe.Uprobe{
				{
					Sym:         "github.com/aws/aws-sdk-go-v2/service/sqs.(*Client).SendMessage",
					EntryProbe:  "uprobe_SendMessage",
					ReturnProbe: "uprobe_SendMessage_Returns",
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

// request-response.
type event struct {
	context.BaseSpanProperties
	QueueUrl [128]byte
	IsError  bool
}

func processFnC(e *event, logger *slog.Logger, serviceName string) telemetry.PathKey {
	logger.Info("=== Processing SQS event", "event", e)

	// _ := e.ParentSpanContext.TraceID.String()
	// rawCmd := unix.ByteSliceToString(e.QueueUrl[:])
	// logger.Info("=== Raw command", "rawCmd", rawCmd)
	return telemetry.PathKey{}

	// Get the command type
}

func isRedisError(buf []uint8) bool {
	return bytes.HasPrefix(buf, []byte("ERR ")) ||
		bytes.HasPrefix(buf, []byte("WRONGTYPE ")) ||
		bytes.HasPrefix(buf, []byte("MOVED ")) ||
		bytes.HasPrefix(buf, []byte("ASK ")) ||
		bytes.HasPrefix(buf, []byte("BUSY ")) ||
		bytes.HasPrefix(buf, []byte("NOSCRIPT ")) ||
		bytes.HasPrefix(buf, []byte("CLUSTERDOWN "))
}

func processFn(e *event) ptrace.SpanSlice {
	spans := ptrace.NewSpanSlice()
	return spans
}
