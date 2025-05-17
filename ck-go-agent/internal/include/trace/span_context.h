// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

#ifndef _SPAN_CONTEXT_H_
#define _SPAN_CONTEXT_H_

#include "utils.h"

#define SPAN_CONTEXT_STRING_SIZE 55
#define W3C_KEY_LENGTH 11 // length of the "traceparent" key
#define CKR_KEY_LENGTH 8 // length of the "ckroute" key
#define CKR_VAL_LENGTH 32
#define W3C_VAL_LENGTH 55
#define TRACE_ID_SIZE 16
#define TRACE_ID_STRING_SIZE 32
#define SPAN_ID_SIZE 8
#define SPAN_ID_STRING_SIZE 16
#define TRACE_FLAGS_SIZE 1
#define TRACE_FLAGS_STRING_SIZE 2
#define GRPC_METHOD_SIZE 128
#define MAX_CONCURRENT_REQUESTS 5000
#define HTTP_PATH_MAX_LEN 128



struct span_context
{
    u8 TraceID[TRACE_ID_SIZE];
    u8 SpanID[SPAN_ID_SIZE];
    u8 TraceFlags;
    u8 padding[7];
};

// Fill the child span context based on the parent span context,
// generating a new span id and copying the trace id and trace flags
static __always_inline void get_span_context_from_parent(struct span_context *parent, struct span_context *child) {
    copy_byte_arrays(parent->TraceID, child->TraceID, TRACE_ID_SIZE);
    generate_random_bytes(child->SpanID, SPAN_ID_SIZE);
}

// Fill the passed span context as root span context
static __always_inline void get_root_span_context(struct span_context *sc) {
    generate_random_bytes(sc->TraceID, TRACE_ID_SIZE);
    generate_random_bytes(sc->SpanID, SPAN_ID_SIZE);
}

static __always_inline void span_context_to_ckr_string(struct span_context *ctx, char *buff)
{
    // W3C format: version (2 chars) - trace id (32 chars) - span id (16 chars) - sampled (2 chars)
    char *out = buff;

    // Write trace id
    bytes_to_hex_string(ctx->TraceID, TRACE_ID_SIZE, out);
}


static __always_inline void span_context_to_w3c_string(struct span_context *ctx, char *buff)
{
    // W3C format: version (2 chars) - trace id (32 chars) - span id (16 chars) - sampled (2 chars)
    char *out = buff;

    // Write version
    *out++ = '0';
    *out++ = '0';
    *out++ = '-';

    // Write trace id
    bytes_to_hex_string(ctx->TraceID, TRACE_ID_SIZE, out);
    out += TRACE_ID_STRING_SIZE;
    *out++ = '-';

    // Write span id
    bytes_to_hex_string(ctx->SpanID, SPAN_ID_SIZE, out);
    out += SPAN_ID_STRING_SIZE;
    *out++ = '-';

    // Write trace flags
    bytes_to_hex_string(&ctx->TraceFlags, TRACE_FLAGS_SIZE, out);
}

static __always_inline void w3c_string_to_span_context(char *str, struct span_context *ctx)
{
    //bpf_printk("w3c_string_to_span_context: str: %s", str);
    hex_string_to_bytes(str, TRACE_ID_STRING_SIZE, ctx->TraceID);
    hex_string_to_bytes("0000000000000000", SPAN_ID_STRING_SIZE, ctx->SpanID);
    hex_string_to_bytes("00", TRACE_FLAGS_STRING_SIZE, &ctx->TraceFlags);
}

#endif
