// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

#ifndef _UPROBE_H_
#define _UPROBE_H_

#include "common.h"
#include "trace/span_context.h"
#include "go_context.h"
#include "go_types.h"
#include "trace/span_output.h"

#define BASE_SPAN_PROPERTIES \
    u64 start_time;          \
    u64 end_time;            \
    struct span_context sc;  \
    struct span_context psc;

// Common flow for uprobe return:
// 1. Find consistent key for the current uprobe context
// 2. Use the key to lookup for the uprobe context in the uprobe_context_map
// 3. Update the end time of the found span
// 4. Submit the constructed event to the agent code using perf buffer events_map
// 5. Delete the span from the global active spans map (in case the span is not tracked in the active spans map, this will be a no-op)
// 6. Delete the span from the uprobe_context_map
// 7. If an argument is present in the error pointer, set the error code and error message.
// 8. The error message is assumed to be a u16 number. This works for MySQL where this is used. If some other method is using this, we will need to revisit this.
#define UPROBE_RETURN(name, event_type, uprobe_context_map, error_pos)  \
SEC("uprobe/##name##")                                                                                               \
int uprobe_##name##_Returns(struct pt_regs *ctx) {                                                                   \
    bpf_debug_printk("=== G %s_Returns called with ctx: %p ===", #name, ctx);                                              \
    void *key = (void *)GOROUTINE(ctx);                                                                              \
    bpf_debug_printk("uprobe_%s_returns: go_context_key: %p",#name , key);                                                 \
    event_type *event = bpf_map_lookup_elem(&uprobe_context_map, &key);                                              \
    if (event == NULL) {                                                                                             \
        bpf_printk("event is NULL in ret probe");                                                                    \
        return 0;                                                                                                    \
    }                                                                                                                \
    if (is_register_abi()) {                                                                                         \
        if(error_pos > 0) {                                                                                          \
            void *error_ptr = get_argument(ctx, error_pos);                                                          \
            if (error_ptr != NULL) {                                                                                 \
                u16 error_code = 0;                                                                                  \
                bpf_probe_read(&error_code, sizeof(u16), error_ptr);                                                 \
                bpf_debug_printk("name: %s, Error occurred. Error code: %d", #name, error_code);                           \
                event->is_error = true;                                                                              \
                event->error_code = error_code;                                                                      \
            }                                                                                                        \
        }                                                                                                            \
    }                                                                                                                \
    event->end_time = bpf_ktime_get_ns();                                                                            \
    output_span_event(ctx, event, sizeof(event_type), &event->sc);                                                   \
    stop_tracking_span(&event->sc, &event->psc);                                                                     \
    bpf_debug_printk("G %s_Returns: delete map : %s , key: context %p, value type: %s", #name, #uprobe_context_map, key, #event_type);  \
    long res = bpf_map_delete_elem(&uprobe_context_map, &key);                                                                  \
    if( res < 0) {                                                                                                        \
        bpf_printk("G %s_Returns: Failed to delete map : %s , key: context %p, value type: %s", #name, #uprobe_context_map, key, #event_type);  \
    }                                                                                                                    \
    return 0;                                                                                                            \
} 

#endif
