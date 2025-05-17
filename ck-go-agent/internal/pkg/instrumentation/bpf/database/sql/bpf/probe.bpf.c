// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

#include "arguments.h"
#include "trace/span_context.h"
#include "go_context.h"
#include "go_types.h"
#include "uprobe.h"
#include "trace/start_span.h"

char __license[] SEC("license") = "Dual MIT/GPL";

#define MAX_QUERY_SIZE 256
#define MAX_CONCURRENT MAX_CONCURRENT_REQUESTS

struct sql_request_t {
    BASE_SPAN_PROPERTIES
    char query[MAX_QUERY_SIZE];
    u16 error_code;
    bool is_error;
};

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__type(key, void*);
	__type(value, struct sql_request_t);
	__uint(max_entries, MAX_CONCURRENT);
} sql_events SEC(".maps");

// Injected in init
volatile const bool should_include_db_statement;

// This instrumentation attaches uprobe to the following function:
// func (db *DB) queryDC(ctx, txctx context.Context, dc *driverConn, releaseConn func(error), query string, args []any)
SEC("uprobe/queryDC")
int uprobe_queryDC(struct pt_regs *ctx) {
    // argument positions
    bpf_debug_printk("=== uprobe_queryDC called with ctx: %p ===", ctx);
    u64 context_ptr_pos = 3;
    u64 query_str_ptr_pos = 8;
    u64 query_str_len_pos = 9;

    struct sql_request_t sql_request = {0};
    sql_request.start_time = bpf_ktime_get_ns();

    //if (should_include_db_statement) {
        // Read Query string
        void *query_str_ptr = get_argument(ctx, query_str_ptr_pos);
        u64 query_str_len = (u64)get_argument(ctx, query_str_len_pos);
        u64 query_size = MAX_QUERY_SIZE < query_str_len ? MAX_QUERY_SIZE : query_str_len;
        bpf_debug_printk("uprobe_queryDC : will read query also with len %d  ===", query_size);
        bpf_probe_read(sql_request.query, query_size, query_str_ptr);
    //}

    struct go_iface go_context = {0};
    get_Go_context(ctx, 2, 0, true, &go_context);
    start_span_params_t start_span_params = {
        .ctx = ctx,
        .go_context = &go_context,
        .psc = &sql_request.psc,
        .sc = &sql_request.sc,
        .get_parent_span_context_fn = NULL,
        .get_parent_span_context_arg = NULL,
    };
    start_span(&start_span_params);

    // // Prepare concatenated input string
    // u8 path_key[MAX_PATH_KEY_BYTE] = {0};
    // u8 inc_path_key[MAX_PATH_KEY_CHAR+1] = {0};
    // bytes_to_hex_string(sql_request.psc.TraceID, MAX_PATH_KEY_BYTE, inc_path_key);

    // u8 sql_method[6+1] = {0};
    // copy_byte_arrays(sql_request.query, sql_method, 6);

    // bpf_printk("uprobe_queryDC: inc_path_key=%s sql_method=%s", inc_path_key, sql_method);

    // u32 inc_path_key_hash = fnv1a_hash32(inc_path_key, MAX_PATH_KEY_CHAR);
    // u32 sql_method_hash = fnv1a_hash32(sql_method, MAX_ELEMENT_NAME_SMALL);
    // u32 service_name_hash = service_name_fnv1a_hash32();
    // u32 hashes[3] = {service_name_hash, inc_path_key_hash, sql_method_hash};
    // combine_to_16byte_hash(hashes, 3, path_key);
    // copy_byte_arrays(path_key, sql_request.sc.TraceID, MAX_PATH_KEY_BYTE);

    // // Print debug info
    // bytes_to_hex_string(path_key, MAX_PATH_KEY_BYTE, inc_path_key);
    // bpf_printk("uprobe_queryDC: Generated pathkey : %s", inc_path_key);

    // Get key
    void *key = get_consistent_key(ctx, go_context.data);
    bpf_debug_printk("uprobe_queryDC: context_key: %p", key); 
    bpf_debug_printk("uprobe_queryDC: update map : sql_events , key: context %p, value type: sql_request_t", key );
    long res = bpf_map_update_elem(&sql_events, &key, &sql_request, 0);
    if(res != 0) {
        bpf_printk("uprobe_queryDC: failed to update map : sql_events , key: context %p, value type: sql_request_t. Return value: %ld", key, res );
    }
    return 0;
}

// This instrumentation attaches uprobe to the following function:
// func (db *DB) queryDC(ctx, txctx context.Context, dc *driverConn, releaseConn func(error), query string, args []any)
UPROBE_RETURN(queryDC, struct sql_request_t, sql_events, 3)

// This instrumentation attaches uprobe to the following function:
// func (db *DB) execDC(ctx context.Context, dc *driverConn, release func(error), query string, args []any)
SEC("uprobe/execDC")
int uprobe_execDC(struct pt_regs *ctx) {
    // argument positions
    bpf_debug_printk("=== uprobe_execDC called with ctx: %p ===", ctx);
    u64 context_ptr_pos = 3;
    u64 query_str_ptr_pos = 6;
    u64 query_str_len_pos = 7;

    struct sql_request_t sql_request = {0};
    sql_request.start_time = bpf_ktime_get_ns();

    //if (should_include_db_statement) {
        // Read Query string
        void *query_str_ptr = get_argument(ctx, query_str_ptr_pos);
        u64 query_str_len = (u64)get_argument(ctx, query_str_len_pos);
        u64 query_size = MAX_QUERY_SIZE < query_str_len ? MAX_QUERY_SIZE : query_str_len;
        bpf_probe_read(sql_request.query, query_size, query_str_ptr);
    //}

    struct go_iface go_context = {0};
    get_Go_context(ctx, 2, 0, true, &go_context);
    start_span_params_t start_span_params = {
        .ctx = ctx,
        .go_context = &go_context,
        .psc = &sql_request.psc,
        .sc = &sql_request.sc,
        .get_parent_span_context_fn = NULL,
        .get_parent_span_context_arg = NULL,
    };
    start_span(&start_span_params);

    // Get key
    void *key = get_consistent_key(ctx, go_context.data);
    bpf_debug_printk("uprobe_execDC: context_key: %p", key); 
    bpf_debug_printk("uprobe_execDC: update map : sql_events , key: context %p, value type: sql_request_t", key );
    long res = bpf_map_update_elem(&sql_events, &key, &sql_request, 0);
    if(res != 0) {
        bpf_printk("uprobe_execDC: failed to update map : sql_events , key: context %p, value type: sql_request_t. Return value: %ld", key, res );
    }
    return 0;
}

// This instrumentation attaches uprobe to the following function:
// func (db *DB) execDC(ctx context.Context, dc *driverConn, release func(error), query string, args []any)
UPROBE_RETURN(execDC, struct sql_request_t, sql_events, 3)
