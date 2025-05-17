// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

#include "common.h"
#include "trace/sampling.h"

#ifndef _SPAN_OUTPUT_H_
#define _SPAN_OUTPUT_H_

struct
{
    __uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
} events SEC(".maps");

// Output a record to the perf buffer. If the span context is sampled, the record is outputted.
// Returns 0 on success, negative error code on failure.
static __always_inline long output_span_event(void *ctx, void *data, u64 size, struct span_context *sc) {
    bpf_debug_printk("output_span_event: Output event to perf buffer" );
    return bpf_perf_event_output(ctx, &events, BPF_F_CURRENT_CPU, data, size);
}

#endif
