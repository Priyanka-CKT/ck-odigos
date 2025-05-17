// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package mux

import (
	"log/slog"

	"go.opentelemetry.io/collector/pdata/ptrace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"go.opentelemetry.io/auto/internal/pkg/instrumentation/context"
	"go.opentelemetry.io/auto/internal/pkg/instrumentation/probe"
	"go.opentelemetry.io/auto/internal/pkg/structfield"
	"go.opentelemetry.io/auto/internal/pkg/telemetry"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target amd64,arm64 bpf ./bpf/probe.bpf.c

const (
	// pkg is the package being instrumented.
	pkg = "github.com/gorilla/mux"
)

// event represents the data we collect from eBPF
type event struct {
	context.BaseSpanProperties
}

// muxProbe is our custom probe type that embeds the SpanProducer
type muxProbe struct {
	*probe.SpanProducer[bpfObjects, event]
}

// New returns a new probe.Probe.
func New(logger *slog.Logger, version string, serviceName string) probe.Probe {
	id := probe.ID{
		SpanKind:        trace.SpanKindInternal,
		InstrumentedPkg: pkg,
	}

	spanProducer := &probe.SpanProducer[bpfObjects, event]{
		Base: probe.Base[bpfObjects, event]{
			ID:     id,
			Logger: logger,
			Consts: []probe.Const{
				probe.RegistersABIConst{},
				probe.StructFieldConst{
					Key: "routematch_route_pos",
					Val: structfield.NewID("github.com/gorilla/mux", "github.com/gorilla/mux", "RouteMatch", "Route"),
				},
				probe.StructFieldConst{
					Key: "route_routeconf_pos",
					Val: structfield.NewID("github.com/gorilla/mux", "github.com/gorilla/mux", "Route", "routeConf"),
				},
				probe.StructFieldConst{
					Key: "routeconf_regexp_pos",
					Val: structfield.NewID("github.com/gorilla/mux", "github.com/gorilla/mux", "routeConf", "regexp"),
				},
				probe.StructFieldConst{
					Key: "routeregexpgroup_path_pos",
					Val: structfield.NewID("github.com/gorilla/mux", "github.com/gorilla/mux", "routeRegexpGroup", "path"),
				},
				probe.StructFieldConst{
					Key: "routeregexp_template_pos",
					Val: structfield.NewID("github.com/gorilla/mux", "github.com/gorilla/mux", "routeRegexp", "template"),
				},
				probe.StructFieldConst{
					Key: "routeregexp_reverse_pos",
					Val: structfield.NewID("github.com/gorilla/mux", "github.com/gorilla/mux", "routeRegexp", "reverse"),
				},
			},
			Uprobes: []probe.Uprobe{
				{
					Sym:         "github.com/gorilla/mux.(*Router).Match",
					EntryProbe:  "uprobe_Route_Match",
					ReturnProbe: "uprobe_Route_Match_Returns",
					FailureMode: probe.FailureModeIgnore,
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

	return &muxProbe{
		SpanProducer: spanProducer,
	}
}

// processFnC handles processing events for custom exporter
func processFnC(e *event, logger *slog.Logger, serviceName string) telemetry.PathKey {
	// Example implementation - modify as needed
	logger.Info("Processing gorilla/mux event")

	// This is a placeholder implementation
	// For now, return an empty PathKey since we don't have specific implementation yet
	return telemetry.PathKey{}
}

// processFn handles processing events
func processFn(e *event) ptrace.SpanSlice {
	// Return empty as we're not implementing this function yet
	return ptrace.SpanSlice{}
}
