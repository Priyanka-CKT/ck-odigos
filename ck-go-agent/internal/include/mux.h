// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

#ifndef _MUX_H_
#define _MUX_H_

#include "trace/span_context.h"
#include "bpf_helpers.h"

#define MAX_CONCURRENT MAX_CONCURRENT_REQUESTS

/*  This will store the path as detected by external mux 
    This handles both normal and patterned path when mux is used
    The bool value will be true if the path is detected by external mux 
*/
struct mux_route_match_t {
    char path[HTTP_PATH_MAX_LEN]; //path if detected by external mux 
    u32 path_len;
    bool has_match;   // Result of the Match function (true if route matched)
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __type(key, void *);
    __type(value, struct mux_route_match_t);
    __uint(max_entries, MAX_CONCURRENT);
    __uint(pinning, LIBBPF_PIN_BY_NAME);
} mux_route_match_map SEC(".maps");


static __always_inline void update_mux_route_match_map(void *contextContext, struct mux_route_match_t *mux_route_match) {
    long err = 0;
    bpf_debug_printk("update_mux_route_match_map: map : mux_route_match_map , key - consistent_key: %p, value type: mux_route_match_t", contextContext );
    err = bpf_map_update_elem(&mux_route_match_map, &contextContext, mux_route_match, BPF_ANY);
    if (err != 0)
    {
        bpf_debug_printk("Failed to update mux_route_match_map: %ld", err);
        return;
    }
}

static __always_inline struct mux_route_match_t *get_mux_route_match(void *contextContext) {
    bpf_debug_printk("get_mux_route_match: map : mux_route_match_map , key - consistent_key: %p", contextContext );
    return bpf_map_lookup_elem(&mux_route_match_map, &contextContext);
}

static __always_inline void delete_mux_route_match_map(void *contextContext) {
    bpf_debug_printk("delete_mux_route_match_map: map : mux_route_match_map , key - consistent_key: %p", contextContext );
    bpf_map_delete_elem(&mux_route_match_map, &contextContext);
}

#endif
