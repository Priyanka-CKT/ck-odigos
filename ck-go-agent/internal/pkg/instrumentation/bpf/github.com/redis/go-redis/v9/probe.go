// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package redisv9

import (
	"bytes"
	"log/slog"
	"strings"

	"go.opentelemetry.io/collector/pdata/ptrace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sys/unix"

	"go.opentelemetry.io/auto/internal/pkg/instrumentation/context"
	"go.opentelemetry.io/auto/internal/pkg/instrumentation/probe"
	"go.opentelemetry.io/auto/internal/pkg/telemetry"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target amd64,arm64 bpf ./bpf/probe.bpf.c

const (
	// pkg is the package being instrumented.
	pkgV9 = "github.com/redis/go-redis/v9"
	pkgV8 = "github.com/go-redis/redis/v8"
)

// RedisCommandType represents the type of Redis command
type RedisCommandType string

const (
	RedisCommandTypeRead      RedisCommandType = "READ"
	RedisCommandTypeWrite     RedisCommandType = "WRITE"
	RedisCommandTypeAggregate RedisCommandType = "AGGREGATE"
	RedisCommandTypeOther     RedisCommandType = "OTHER"
)

var redisCommandTypeMap = map[string]RedisCommandType{
	// READ Commands
	"GET": RedisCommandTypeRead, "EXISTS": RedisCommandTypeRead, "TYPE": RedisCommandTypeRead,
	"KEYS": RedisCommandTypeRead, "RANDOMKEY": RedisCommandTypeRead, "DBSIZE": RedisCommandTypeRead,
	"TTL": RedisCommandTypeRead, "STRLEN": RedisCommandTypeRead, "SUBSTR": RedisCommandTypeRead,
	"HGET": RedisCommandTypeRead, "HMGET": RedisCommandTypeRead, "HEXISTS": RedisCommandTypeRead,
	"HLEN": RedisCommandTypeRead, "HKEYS": RedisCommandTypeRead, "HVALS": RedisCommandTypeRead,
	"HGETALL": RedisCommandTypeRead, "LLEN": RedisCommandTypeRead, "LRANGE": RedisCommandTypeRead,
	"LINDEX": RedisCommandTypeRead, "SMEMBERS": RedisCommandTypeRead, "SCARD": RedisCommandTypeRead,
	"SISMEMBER": RedisCommandTypeRead, "SRANDMEMBER": RedisCommandTypeRead, "ZRANGE": RedisCommandTypeRead,
	"ZRANK": RedisCommandTypeRead, "ZREVRANK": RedisCommandTypeRead, "ZREVRANGE": RedisCommandTypeRead,
	"ZCARD": RedisCommandTypeRead, "ZSCORE": RedisCommandTypeRead, "ZCOUNT": RedisCommandTypeRead,
	"ZRANGEBYSCORE": RedisCommandTypeRead, "INFO": RedisCommandTypeRead,

	// WRITE Commands
	"SET": RedisCommandTypeWrite, "SETEX": RedisCommandTypeWrite, "SETNX": RedisCommandTypeWrite,
	"GETSET": RedisCommandTypeWrite, "DEL": RedisCommandTypeWrite, "EXPIRE": RedisCommandTypeWrite,
	"EXPIREAT": RedisCommandTypeWrite, "PERSIST": RedisCommandTypeWrite, "RENAME": RedisCommandTypeWrite,
	"RENAMENX": RedisCommandTypeWrite, "MOVE": RedisCommandTypeWrite, "DECR": RedisCommandTypeWrite,
	"DECRBY": RedisCommandTypeWrite, "INCR": RedisCommandTypeWrite, "INCRBY": RedisCommandTypeWrite,
	"APPEND": RedisCommandTypeWrite, "HSET": RedisCommandTypeWrite, "HSETNX": RedisCommandTypeWrite,
	"HMSET": RedisCommandTypeWrite, "HINCRBY": RedisCommandTypeWrite, "HDEL": RedisCommandTypeWrite,
	"RPUSH": RedisCommandTypeWrite, "RPUSHX": RedisCommandTypeWrite, "LPUSH": RedisCommandTypeWrite,
	"LPUSHX": RedisCommandTypeWrite, "LTRIM": RedisCommandTypeWrite, "LSET": RedisCommandTypeWrite,
	"LREM": RedisCommandTypeWrite, "LINSERT": RedisCommandTypeWrite, "LPOP": RedisCommandTypeWrite,
	"RPOP": RedisCommandTypeWrite, "RPOPLPUSH": RedisCommandTypeWrite, "SADD": RedisCommandTypeWrite,
	"SREM": RedisCommandTypeWrite, "SPOP": RedisCommandTypeWrite, "SMOVE": RedisCommandTypeWrite,
	"ZADD": RedisCommandTypeWrite, "ZREM": RedisCommandTypeWrite, "ZINCRBY": RedisCommandTypeWrite,
	"ZREMRANGEBYRANK": RedisCommandTypeWrite, "ZREMRANGEBYSCORE": RedisCommandTypeWrite,
	"FLUSHDB": RedisCommandTypeWrite, "FLUSHALL": RedisCommandTypeWrite, "SORT": RedisCommandTypeWrite,

	// AGGREGATE Commands
	"MGET": RedisCommandTypeAggregate, "MSET": RedisCommandTypeAggregate, "MSETNX": RedisCommandTypeAggregate,
	"SINTER": RedisCommandTypeAggregate, "SUNION": RedisCommandTypeAggregate, "SDIFF": RedisCommandTypeAggregate,
	"SINTERSTORE": RedisCommandTypeAggregate, "SUNIONSTORE": RedisCommandTypeAggregate,
	"SDIFFSTORE": RedisCommandTypeAggregate, "ZUNIONSTORE": RedisCommandTypeAggregate,
	"ZINTERSTORE": RedisCommandTypeAggregate,
}

func getRedisCommandType(command string) RedisCommandType {
	if cmdType, exists := redisCommandTypeMap[strings.ToUpper(command)]; exists {
		return cmdType
	}
	return RedisCommandTypeOther
}

// New returns a new [probe.Probe] for Redis v9.
func New(logger *slog.Logger, version string, serviceName string) probe.Probe {
	return newRedisProbe(logger, version, serviceName, pkgV9, "v9")
}

// NewV8 returns a new [probe.Probe] for Redis v8.
func NewV8(logger *slog.Logger, version string, serviceName string) probe.Probe {
	return newRedisProbe(logger, version, serviceName, pkgV8, "v8")
}

// newRedisProbe creates a probe for the specified Redis version
func newRedisProbe(logger *slog.Logger, version string, serviceName string, pkg string, redisVersion string) probe.Probe {
	id := probe.ID{
		SpanKind:        trace.SpanKindClient,
		InstrumentedPkg: pkg,
	}
	return &probe.SpanProducer[bpfObjects, event]{
		Base: probe.Base[bpfObjects, event]{
			ID:     id,
			Logger: logger,
			Consts: []probe.Const{
				probe.RegistersABIConst{},
				probe.AllocationConst{},
				// probe.StructFieldConst{
				// 	Key: "baseCmd_args_pos",
				// 	Val: structfield.NewID(pkg, pkg, "baseCmd", "args"),
				// },
			},
			Uprobes: []probe.Uprobe{
				{
					Sym:         pkg + ".(*baseClient)._process",
					EntryProbe:  "uprobe_redisProcess",
					ReturnProbe: "uprobe_redisProcess_Returns",
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
}

// request-response.
type event struct {
	context.BaseSpanProperties
	Cmdbuf    [32]byte
	ErrorCode [32]byte
	IsError   bool
}

func processFnC(e *event, logger *slog.Logger, serviceName string) telemetry.PathKey {
	logger.Info("=== Processing Redis event", "event", e)

	parentTraceId := e.ParentSpanContext.TraceID.String()
	//If the parentTraceId (incoming pathkey) is not set or its an empty pathkey, just return an empty pathkey
	if parentTraceId == "00000000000000000000000000000000" || parentTraceId == "" {
		return telemetry.PathKey{}
	}
	rawCmd := unix.ByteSliceToString(e.Cmdbuf[:])
	errorCode := unix.ByteSliceToString(e.ErrorCode[:])
	logger.Info("=== Raw command", "rawCmd", rawCmd)

	// Get the command type
	commandType := getRedisCommandType(rawCmd)
	logger.Info("=== Redis command type", "operation", rawCmd, "type", commandType)
	if commandType == RedisCommandTypeOther {
		return telemetry.PathKey{}
	}
	operation := strings.ToUpper(rawCmd)
	logger.Info("=== Creating PathKey for Redis",
		"service", serviceName,
		"operation", operation,
		"parentTraceID", parentTraceId,
	)
	typeName := "Redis"
	pathKeyStr := telemetry.GeneratePathKey(serviceName, typeName, parentTraceId, operation)
	logger.Info("=== Created Redis PathKey String", "pathKey", pathKeyStr)

	// Create PathKey instance
	pathKey := telemetry.PathKey{
		PathKey: pathKeyStr,
		GraphPathNode: telemetry.GraphPathNode{
			IncomingPath: &parentTraceId,
			Runtime:      "go",
			GraphPathElement: telemetry.RedisElement{
				Type:    typeName,
				Command: operation,
				Server:  "DB",
			},
			StartTime: e.StartTime,
			EndTime:   e.EndTime,
			IsError:   e.IsError,
			ErrorCode: errorCode,
		},
	}

	logger.Info("=== Created Redis PathKey", "pathKey", pathKey)
	return pathKey
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
