// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

#ifndef _START_SPAN_H_
#define _START_SPAN_H_

#include "common.h"
#include "span_context.h"
#include "sampling.h"
//#include "bpf_helper_defs.h"

#define MAX_INPUT_SIZE 160
#define MAX_ELEMENT_NAME_SMALL 16
#define MAX_ELEMENT_NAME 32
#define MAX_ELEMENT_NAME_BIG 128
#define MAX_PATH_KEY_BYTE 16
#define MAX_PATH_KEY_CHAR 32
// function for getting the parent span context, the result is stored in the passed span context.
// the function should return 0 if the parent span context is found, negative value otherwise.
// Each probe can potentially have a different way of getting the parent span context,
// this is useful for incoming requests (http, kafka, etc.) where the parent span context needs to be extracted from the
// incoming request.
// The handle param can be used to pass any data needed to get the parent span context.
typedef long (*get_parent_sc_fn)(void *handle, struct span_context *psc);

typedef struct start_span_params {
    struct pt_regs *ctx;
    struct go_iface *go_context;
    struct span_context *psc;
    struct span_context *sc;
    // function for getting the parent span context, the result is stored in the passed span context.
    get_parent_sc_fn get_parent_span_context_fn;
    // argument to be passed to the get_parent_span_context_fn
    void *get_parent_span_context_arg;
} start_span_params_t;

// Start a new span, setting the parent span context if found.
// Generate a new span context for the new span. Perform sampling decision and set the TraceFlags accordingly.
static __always_inline void start_span(start_span_params_t *params) {
    long found_parent = -1;
    if (params->get_parent_span_context_fn != NULL) {
        bpf_debug_printk("start_span: get_parent_span_context_fn is not NULL");
        found_parent = params->get_parent_span_context_fn(params->get_parent_span_context_arg, params->psc);
    } else {
        struct span_context *local_psc = get_parent_span_context(params->go_context);
        if (local_psc != NULL) {
            bpf_debug_printk("start_span: local_psc is not NULL");
            found_parent = 0;
            *(params->psc) = *local_psc;
        }
    }

    u8 parent_trace_flags = 0;
    if (found_parent == 0) {
        get_span_context_from_parent(params->psc, params->sc);
        parent_trace_flags = params->psc->TraceFlags;
    } else {
        get_root_span_context(params->sc);
    }

    sampling_parameters_t sampling_params = {
        .trace_id = params->sc->TraceID,
        .psc = (found_parent == 0) ? params->psc : NULL,
    };
    bool sample = should_sample(&sampling_params);
    if (sample) {
        params->sc->TraceFlags = (parent_trace_flags) | (FLAG_SAMPLED);
    } else {
        params->sc->TraceFlags = (parent_trace_flags) & (~FLAG_SAMPLED);
    }
}

// Function that calls get_parent_span_context and creates a glitch span context if parent not found
static __always_inline long get_parent_sc_glitch(void *handle, struct span_context *psc) {
    struct go_iface *go_context = (struct go_iface *)handle;
    struct span_context *parent_sc = get_parent_span_context(go_context);
    
    if (parent_sc != NULL) {
        // Parent found, copy it
        bpf_debug_printk("get_parent_sc_glitch: parent found, copying");
        *psc = *parent_sc;
        return 0;
    }
    
    // Parent not found, create a glitch span context
    bpf_debug_printk("get_parent_sc_glitch: parent not found, creating glitch");
        
    // Set each byte of TraceID to 0x11 so it appears as 16 "1"s in hex
    for (int i = 0; i < TRACE_ID_SIZE; i++) {
        psc->TraceID[i] = 0x11;
    }
    
    return 0;
}

// FNV-1a hash function
#define FNV_PRIME 0x1000193
#define FNV_OFFSET_BASIS 0x811C9DC5
static __always_inline u32 fnv1a_hash32(const u8 *data, int len) {
    u32 hash = FNV_OFFSET_BASIS; //seed
    for (int i = 0; i < len; i++) {
        // Convert to lowercase if uppercase
        u8 c = (data[i] >= 'A' && data[i] <= 'Z') ? (data[i] + 32) : data[i];
        hash ^= c;
        hash *= FNV_PRIME;
    }
    return hash;
}

static __always_inline u32 service_name_fnv1a_hash32() {
    u8 service_name[MAX_ELEMENT_NAME+1] = {0};

    u8 ins_config_key = 1;
    u8 *ins_value = bpf_map_lookup_elem(&instrumentation_config_map, &ins_config_key);
    if(ins_value == NULL){
        bpf_get_current_comm(&service_name, MAX_ELEMENT_NAME);
        bpf_debug_printk("service_name not found in map, reading from bpf_get_current_comm: %s", service_name);
        return fnv1a_hash32(service_name, MAX_ELEMENT_NAME);
    }
    copy_byte_arrays(ins_value, service_name, MAX_ELEMENT_NAME);
    return fnv1a_hash32(service_name, MAX_ELEMENT_NAME);
}

// Combine an array of u32 hashes into a 16-byte hash
static __always_inline void combine_to_16byte_hash(const u32 *hashes, int num_hashes, u8 output[MAX_PATH_KEY_BYTE]) {
    u32 hash1 = FNV_OFFSET_BASIS;
    u32 hash2 = FNV_OFFSET_BASIS;

    for (int i = 0; i < num_hashes; i++) {
        // Rotate and mix strategy for better distribution
        hash1 = (hash1 << 5) | (hash1 >> 27);  // 5-bit rotate left
        hash2 = (hash2 << 7) | (hash2 >> 25);  // 7-bit rotate left
        
        hash1 ^= hashes[i];
        hash1 *= FNV_PRIME;
        
        hash2 ^= (hashes[i] * 0x9e3779b9);  // multiply by golden ratio
        hash2 *= FNV_PRIME;
    }

    // Convert the two accumulators into a 16-byte hash
    // First 8 bytes are hash1, last 8 bytes are hash2
    output[0] = (hash1 >> 24) & 0xFF;
    output[1] = (hash1 >> 16) & 0xFF;
    output[2] = (hash1 >> 8) & 0xFF;
    output[3] = hash1 & 0xFF;
    output[4] = (hash2 >> 24) & 0xFF;
    output[5] = (hash2 >> 16) & 0xFF;
    output[6] = (hash2 >> 8) & 0xFF;
    output[7] = hash2 & 0xFF;

    // Optionally, repeat hashing for the second half to increase entropy
    u32 combined = hash1 ^ hash2;
    combined *= FNV_PRIME;

    output[8] = (combined >> 24) & 0xFF;
    output[9] = (combined >> 16) & 0xFF;
    output[10] = (combined >> 8) & 0xFF;
    output[11] = combined & 0xFF;
    output[12] = (hash2 >> 24) & 0xFF; // Spreading final impact
    output[13] = (hash2 >> 16) & 0xFF;
    output[14] = (hash1 >> 8) & 0xFF;
    output[15] = hash1 & 0xFF;
}

#endif
