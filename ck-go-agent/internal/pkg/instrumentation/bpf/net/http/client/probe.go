// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/cilium/ebpf"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sys/unix"

	"go.opentelemetry.io/auto/internal/pkg/instrumentation/bpf/net/http"
	"go.opentelemetry.io/auto/internal/pkg/instrumentation/context"
	"go.opentelemetry.io/auto/internal/pkg/instrumentation/probe"
	"go.opentelemetry.io/auto/internal/pkg/instrumentation/utils"
	"go.opentelemetry.io/auto/internal/pkg/structfield"
	"go.opentelemetry.io/auto/internal/pkg/telemetry"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target amd64,arm64 bpf ./bpf/probe.bpf.c
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target amd64,arm64 bpf_no_tp ./bpf/probe.bpf.c -- -DNO_HEADER_PROPAGATION

const (
	// pkg is the package being instrumented.
	pkg = "net/http"
)

// New returns a new [probe.Probe].
func New(logger *slog.Logger, version string, serviceName string) probe.Probe {
	id := probe.ID{
		SpanKind:        trace.SpanKindClient,
		InstrumentedPkg: pkg,
	}

	uprobes := []probe.Uprobe{
		{
			Sym:         "net/http.(*Transport).roundTrip",
			EntryProbe:  "uprobe_Transport_roundTrip",
			ReturnProbe: "uprobe_Transport_roundTrip_Returns",
		},
	}

	// If the kernel supports context propagation, we enable the
	// probe which writes the data in the outgoing buffer.
	if utils.SupportsContextPropagation() {
		uprobes = append(uprobes,
			probe.Uprobe{
				Sym:        "net/http.Header.writeSubset",
				EntryProbe: "uprobe_writeSubset",
				// We mark this probe as dependent on roundTrip, so we don't accidentally
				// enable this bpf program, if the executable has compiled in writeSubset,
				// but doesn't have any http roundTrip.
				DependsOn: []string{"net/http.(*Transport).roundTrip", "net/http.Header.writeSubset"},
			},
		)
	}

	return &probe.SpanProducer[bpfObjects, event]{
		Base: probe.Base[bpfObjects, event]{
			ID:     id,
			Logger: logger,
			Consts: []probe.Const{
				probe.RegistersABIConst{},
				probe.AllocationConst{},
				probe.StructFieldConst{
					Key: "method_ptr_pos",
					Val: structfield.NewID("std", "net/http", "Request", "Method"),
				},
				probe.StructFieldConst{
					Key: "url_ptr_pos",
					Val: structfield.NewID("std", "net/http", "Request", "URL"),
				},
				probe.StructFieldConst{
					Key: "path_ptr_pos",
					Val: structfield.NewID("std", "net/url", "URL", "Path"),
				},
				probe.StructFieldConst{
					Key: "headers_ptr_pos",
					Val: structfield.NewID("std", "net/http", "Request", "Header"),
				},
				probe.StructFieldConst{
					Key: "ctx_ptr_pos",
					Val: structfield.NewID("std", "net/http", "Request", "ctx"),
				},
				probe.StructFieldConst{
					Key: "status_code_pos",
					Val: structfield.NewID("std", "net/http", "Response", "StatusCode"),
				},
				probe.StructFieldConst{
					Key: "response_header_ptr_pos",
					Val: structfield.NewID("std", "net/http", "Response", "Header"),
				},
				probe.StructFieldConst{
					Key: "request_host_pos",
					Val: structfield.NewID("std", "net/http", "Request", "Host"),
				},
				probe.StructFieldConst{
					Key: "request_proto_pos",
					Val: structfield.NewID("std", "net/http", "Request", "Proto"),
				},
				probe.StructFieldConst{
					Key: "io_writer_buf_ptr_pos",
					Val: structfield.NewID("std", "bufio", "Writer", "buf"),
				},
				probe.StructFieldConst{
					Key: "io_writer_n_pos",
					Val: structfield.NewID("std", "bufio", "Writer", "n"),
				},
				probe.StructFieldConst{
					Key: "scheme_pos",
					Val: structfield.NewID("std", "net/url", "URL", "Scheme"),
				},
				probe.StructFieldConst{
					Key: "opaque_pos",
					Val: structfield.NewID("std", "net/url", "URL", "Opaque"),
				},
				probe.StructFieldConst{
					Key: "user_ptr_pos",
					Val: structfield.NewID("std", "net/url", "URL", "User"),
				},
				probe.StructFieldConst{
					Key: "raw_path_pos",
					Val: structfield.NewID("std", "net/url", "URL", "RawPath"),
				},
				probe.StructFieldConst{
					Key: "omit_host_pos",
					Val: structfield.NewID("std", "net/url", "URL", "OmitHost"),
				},
				probe.StructFieldConst{
					Key: "force_query_pos",
					Val: structfield.NewID("std", "net/url", "URL", "ForceQuery"),
				},
				probe.StructFieldConst{
					Key: "raw_query_pos",
					Val: structfield.NewID("std", "net/url", "URL", "RawQuery"),
				},
				probe.StructFieldConst{
					Key: "fragment_pos",
					Val: structfield.NewID("std", "net/url", "URL", "Fragment"),
				},
				probe.StructFieldConst{
					Key: "raw_fragment_pos",
					Val: structfield.NewID("std", "net/url", "URL", "RawFragment"),
				},
				probe.StructFieldConst{
					Key: "username_pos",
					Val: structfield.NewID("std", "net/url", "Userinfo", "username"),
				},
				probe.StructFieldConst{
					Key: "url_host_pos",
					Val: structfield.NewID("std", "net/url", "URL", "Host"),
				},
				probe.StructFieldConst{
					Key: "buckets_ptr_pos",
					Val: structfield.NewID("std", "runtime", "hmap", "buckets"),
				},
			},
			Uprobes: uprobes,
			SpecFn:  verifyAndLoadBpf,
		},
		Version:     version,
		SchemaURL:   semconv.SchemaURL,
		ProcessFn:   processFn,
		ProcessFnC:  processFnC,
		ServiceName: serviceName,
	}
}

func verifyAndLoadBpf() (*ebpf.CollectionSpec, error) {
	if !utils.SupportsContextPropagation() {
		fmt.Fprintf(os.Stderr, "the Linux Kernel doesn't support context propagation, please check if the kernel is in lockdown mode (/sys/kernel/security/lockdown)")
		return loadBpf_no_tp()
	}

	return loadBpf()
}

type event struct {
	context.BaseSpanProperties
	Host          [128]byte
	Proto         [8]byte
	StatusCode    uint64
	Method        [16]byte
	Path          [128]byte
	Scheme        [8]byte
	Opaque        [8]byte
	RawPath       [8]byte
	Username      [8]byte
	RawQuery      [128]byte
	Fragment      [56]byte
	RawFragment   [56]byte
	AmzTarget     [40]byte
	ServerPathKey [32]byte
	ForceQuery    uint8
	OmitHost      uint8
}

func processFnC(e *event, logger *slog.Logger, serviceName string) telemetry.PathKey {
	if e == nil {
		logger.Info("=== Processing HTTP client event", "event", e)
		return telemetry.PathKey{}
	}
	// Parse the Path string which contains "DynamoDB_20120810.PutItem" type string
	amzTargetStr := unix.ByteSliceToString(e.AmzTarget[:])

	if len(amzTargetStr) > 0 {
		//this means that its an AWS Request
		return processAwsRequest(amzTargetStr, logger, serviceName, e)
	}

	//This is  bridge request where the serverPathKey is received from the server.
	return processBridgeRequest(logger, serviceName, e)

}

func processBridgeRequest(logger *slog.Logger, serviceName string, e *event) telemetry.PathKey {

	serverPathKey := unix.ByteSliceToString(e.ServerPathKey[:])

	if len(serverPathKey) == 0 {
		serverPathKey = "external" //This is a default agreed contract.
	}
	httpMethod := strings.ToUpper(unix.ByteSliceToString(e.Method[:]))

	if httpMethod == "" {
		logger.Debug("=== HTTP Method is empty, returning empty pathKey")
		return telemetry.PathKey{}
	}

	httpScheme := strings.ToUpper(unix.ByteSliceToString(e.Scheme[:]))

	if httpScheme == "" {
		httpScheme = "HTTP"
	}

	parentTraceId := e.ParentSpanContext.TraceID.String()

	typeName := "HTTPClientBridge"

	logger.Info("=== Creating PathKey for ClientBridge",
		"type", typeName,
		"service", serviceName,
		"httpMethod", httpMethod,
		"httpScheme", httpScheme,
		"serverPathKey", serverPathKey,
		"parentTraceID", parentTraceId,
	)

	pathKeyStr := telemetry.GeneratePathKey(serviceName, typeName, parentTraceId, httpMethod, httpScheme, serverPathKey)
	logger.Info("=== Created HTTPClientBridge PathKey String", "pathKeyStr", pathKeyStr)

	pathKey := telemetry.PathKey{
		PathKey: pathKeyStr,
		GraphPathNode: telemetry.GraphPathNode{
			IncomingPath: &parentTraceId,
			Runtime:      "go",
			GraphPathElement: telemetry.HttpClientBridgeElement{
				Type:          typeName,
				HttpMethod:    httpMethod,
				HttpScheme:    httpScheme,
				ServerPathKey: serverPathKey,
			},
			StartTime: e.StartTime,
			EndTime:   e.EndTime,
			IsError:   e.StatusCode >= 400 && e.StatusCode < 600, //We are only considering 4xx and 5xx errors for now
			ErrorCode: strconv.FormatUint(e.StatusCode, 10),
		},
	}

	logger.Info("=== Created HTTPClientBridge PathKey", "pathKey", pathKey)
	return pathKey

}

func processAwsRequest(amzTargetStr string, logger *slog.Logger, serviceName string, e *event) telemetry.PathKey {

	parts := strings.FieldsFunc(amzTargetStr, func(r rune) bool {
		return r == '_' || r == '.'
	})
	// Now parts will be []string{"DynamoDB", "20120810", "PutItem"}
	var awsServiceName, action string
	if len(parts) == 3 {
		if len(parts[0]) > 0 && len(parts[2]) > 0 {
			awsServiceName = strings.ToLower(parts[0])
			action = strings.ToLower(parts[2])
		} else {
			logger.Info("=== No AWS service name or action found, returning empty pathKey")
			return telemetry.PathKey{}
		}
	}
	parentTraceId := e.ParentSpanContext.TraceID.String()

	if parentTraceId == "" || parentTraceId == "00000000000000000000000000000000" {
		logger.Debug("=== ParentTraceId is empty, returning empty pathKey while processing AWS request")
		return telemetry.PathKey{}
	}

	typeName := "AWSService"
	logger.Info("=== Creating PathKey for DynamoDB",
		"type", typeName,
		"service", serviceName,
		"awsServiceName", awsServiceName,
		"action", action,
		"parentTraceID", parentTraceId,
	)
	pathKeyStr := telemetry.GeneratePathKey(serviceName, typeName, parentTraceId, awsServiceName, action)
	logger.Info("=== Created AWS Service PathKey String", "pathKeyStr", pathKeyStr)

	pathKey := telemetry.PathKey{
		PathKey: pathKeyStr,
		GraphPathNode: telemetry.GraphPathNode{
			IncomingPath: &parentTraceId,
			Runtime:      "go",
			GraphPathElement: telemetry.AWSServiceElement{
				Type:        typeName,
				ServiceName: awsServiceName,
				Action:      action,
			},
			StartTime: e.StartTime,
			EndTime:   e.EndTime,
			IsError:   e.StatusCode >= 400 && e.StatusCode < 600, //We are only considering 4xx and 5xx errors for now
			ErrorCode: strconv.FormatUint(e.StatusCode, 10),
		},
	}

	logger.Info("=== Created AWSService PathKey", "pathKey", pathKey)
	return pathKey

}

func processFn(e *event) ptrace.SpanSlice {
	method := unix.ByteSliceToString(e.Method[:])
	path := unix.ByteSliceToString(e.Path[:])
	scheme := unix.ByteSliceToString(e.Scheme[:])
	opaque := unix.ByteSliceToString(e.Opaque[:])
	host := unix.ByteSliceToString(e.Host[:])
	rawPath := unix.ByteSliceToString(e.RawPath[:])
	rawQuery := unix.ByteSliceToString(e.RawQuery[:])
	username := unix.ByteSliceToString(e.Username[:])
	fragment := unix.ByteSliceToString(e.Fragment[:])
	rawFragment := unix.ByteSliceToString(e.RawFragment[:])
	forceQuery := e.ForceQuery != 0
	omitHost := e.OmitHost != 0
	var user *url.Userinfo
	if len(username) > 0 {
		// check that username!="", otherwise url.User will instantiate
		// an empty, non-nil *Userinfo object which url.String() will parse
		// to just "@" in the final fullUrl
		user = url.User(username)
	}

	attrs := []attribute.KeyValue{
		semconv.HTTPRequestMethodKey.String(method),
		semconv.HTTPResponseStatusCodeKey.Int(int(e.StatusCode)),
	}

	if path != "" {
		attrs = append(attrs, semconv.URLPath(path))
	}

	urlObj := &url.URL{
		Path:        path,
		Scheme:      scheme,
		Opaque:      opaque,
		Host:        host,
		RawPath:     rawPath,
		User:        user,
		RawQuery:    rawQuery,
		Fragment:    fragment,
		RawFragment: rawFragment,
		ForceQuery:  forceQuery,
		OmitHost:    omitHost,
	}

	fullURL := urlObj.String()
	attrs = append(attrs, semconv.URLFull(fullURL))

	// Server address and port
	serverAddr, serverPort := http.ServerAddressPortAttributes(e.Host[:])
	if serverAddr.Valid() {
		attrs = append(attrs, serverAddr)
	}
	if serverPort.Valid() {
		attrs = append(attrs, serverPort)
	}

	proto := unix.ByteSliceToString(e.Proto[:])
	if proto != "" {
		parts := strings.Split(proto, "/")
		if len(parts) == 2 {
			if parts[0] != "HTTP" {
				attrs = append(attrs, semconv.NetworkProtocolName(parts[0]))
			}
			attrs = append(attrs, semconv.NetworkProtocolVersion(parts[1]))
		}
	}

	spans := ptrace.NewSpanSlice()
	span := spans.AppendEmpty()
	span.SetName(method)
	span.SetKind(ptrace.SpanKindClient)
	span.SetStartTimestamp(utils.BootOffsetToTimestamp(e.StartTime))
	span.SetEndTimestamp(utils.BootOffsetToTimestamp(e.EndTime))
	span.SetTraceID(pcommon.TraceID(e.SpanContext.TraceID))
	span.SetSpanID(pcommon.SpanID(e.SpanContext.SpanID))
	span.SetFlags(uint32(trace.FlagsSampled))
	span.SetKind(ptrace.SpanKindClient)

	if e.ParentSpanContext.SpanID.IsValid() {
		span.SetParentSpanID(pcommon.SpanID(e.ParentSpanContext.SpanID))
	}

	utils.Attributes(span.Attributes(), attrs...)

	if int(e.StatusCode) >= 400 && int(e.StatusCode) < 600 {
		span.Status().SetCode(ptrace.StatusCodeError)
	}

	return spans
}
