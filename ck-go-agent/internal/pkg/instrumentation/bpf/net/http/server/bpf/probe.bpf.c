// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

#include "arguments.h"
#include "trace/span_context.h"
#include "go_context.h"
#include "go_types.h"
#include "uprobe.h"
#include "trace/span_output.h"
#include "trace/start_span.h"
#include "mux.h"

char __license[] SEC("license") = "Dual MIT/GPL";

#define PATH_MAX_LEN HTTP_PATH_MAX_LEN
#define MAX_BUCKETS 8
#define METHOD_MAX_LEN 8
#define MAX_CONCURRENT MAX_CONCURRENT_REQUESTS
#define REMOTE_ADDR_MAX_LEN 256
#define HOST_MAX_LEN 256
#define PROTO_MAX_LEN 8

struct http_server_span_t
{
    BASE_SPAN_PROPERTIES
    u64 status_code;
    char method[METHOD_MAX_LEN];
    char path[PATH_MAX_LEN];
    char path_pattern[PATH_MAX_LEN];
    char remote_addr[REMOTE_ADDR_MAX_LEN];
    char host[HOST_MAX_LEN];
    char proto[PROTO_MAX_LEN];
};

struct uprobe_data_t
{
    struct http_server_span_t span;
    // bpf2go doesn't support pointers fields
    // saving the response pointer in the entry probe
    // and using it in the return probe
    u64 resp_ptr;
};

MAP_BUCKET_DEFINITION(go_string_t, go_slice_t)

struct
{
    __uint(type, BPF_MAP_TYPE_HASH);
    __type(key, void *);
    __type(value, struct uprobe_data_t);
    __uint(max_entries, MAX_CONCURRENT);
} http_server_uprobes SEC(".maps");

struct
{
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(key_size, sizeof(u32));
    __uint(value_size, sizeof(MAP_BUCKET_TYPE(go_string_t, go_slice_t)));
    __uint(max_entries, 1);
} golang_mapbucket_storage_map SEC(".maps");

struct
{
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(key_size, sizeof(u32));
    __uint(value_size, sizeof(struct uprobe_data_t));
    __uint(max_entries, 1);
} http_server_uprobe_storage_map SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__type(key, void*); // the headers ptr
	__type(value, char[MAX_PATH_KEY_BYTE]); //the ck-route value of the server.
	__uint(max_entries, MAX_CONCURRENT);
} http_return_headers SEC(".maps");

// Injected in init
volatile const u64 method_ptr_pos;
volatile const u64 url_ptr_pos;
volatile const u64 path_ptr_pos;
volatile const u64 ctx_ptr_pos;
volatile const u64 headers_ptr_pos;
volatile const u64 buckets_ptr_pos;
volatile const u64 req_ptr_pos;
volatile const u64 status_code_pos;
volatile const u64 remote_addr_pos;
volatile const u64 host_pos;
volatile const u64 proto_pos;

//Required for HTTP Server return headers.
volatile const u64 io_writer_buf_ptr_pos;
volatile const u64 io_writer_n_pos;


// A flag indicating whether pattern handlers are supported
volatile const bool pattern_path_supported;
// In case pattern handlers are supported the following offsets will be used:
volatile const u64 req_pat_pos;
volatile const u64 pat_str_pos;

// Required for HTTP Server return headers.
volatile const u64 chunk_writer_pos;
volatile const u64 chunk_writer_header_pos;

// Extracts the span context from the request headers by looking for the 'traceparent' header.
// Fills the parent_span_context with the extracted span context.
// Returns 0 on success, negative value on error.
static __always_inline long extract_context_from_req_headers(void *headers_ptr_ptr, struct span_context *parent_span_context)
{
    void *headers_ptr;
    long res;
    res = bpf_probe_read(&headers_ptr, sizeof(headers_ptr), headers_ptr_ptr);
    if (res < 0)
    {
        return res;
    }
    u64 headers_count = 0;
    res = bpf_probe_read(&headers_count, sizeof(headers_count), headers_ptr);
    if (res < 0)
    {
        return res;
    }
    if (headers_count == 0)
    {
        return -1;
    }
    unsigned char log_2_bucket_count;
    res = bpf_probe_read(&log_2_bucket_count, sizeof(log_2_bucket_count), headers_ptr + 9);
    if (res < 0)
    {
        return -1;
    }
    u64 bucket_count = 1 << log_2_bucket_count;
    void *header_buckets;
    res = bpf_probe_read(&header_buckets, sizeof(header_buckets), (void*)(headers_ptr + buckets_ptr_pos));
    if (res < 0)
    {
        return -1;
    }
    u32 map_id = 0;
    MAP_BUCKET_TYPE(go_string_t, go_slice_t) *map_value = bpf_map_lookup_elem(&golang_mapbucket_storage_map, &map_id);
    if (!map_value)
    {
        return -1;
    }

    for (u64 j = 0; j < MAX_BUCKETS; j++)
    {
        if (j >= bucket_count)
        {
            break;
        }
        res = bpf_probe_read(map_value, sizeof(MAP_BUCKET_TYPE(go_string_t, go_slice_t)), header_buckets + (j * sizeof(MAP_BUCKET_TYPE(go_string_t, go_slice_t))));
        if (res < 0)
        {
            continue;
        }
        for (u64 i = 0; i < 8; i++)
        {
            if (map_value->tophash[i] == 0)
            {
                continue;
            }
            if (map_value->keys[i].len != CKR_KEY_LENGTH)
            {
                continue;
            }
            char current_header_key[CKR_KEY_LENGTH];
            bpf_probe_read(current_header_key, sizeof(current_header_key), map_value->keys[i].str);
            if (!bpf_memcmp(current_header_key, "ck-route", CKR_KEY_LENGTH) && !bpf_memcmp(current_header_key, "Ck-Route", CKR_KEY_LENGTH))
            {
                continue;
            }
            void *traceparent_header_value_ptr = map_value->values[i].array;
            struct go_string traceparent_header_value_go_str;
            res = bpf_probe_read(&traceparent_header_value_go_str, sizeof(traceparent_header_value_go_str), traceparent_header_value_ptr);
            if (res < 0)
            {
                return -1;
            }
            if (traceparent_header_value_go_str.len != CKR_VAL_LENGTH)
            {
                continue;
            }
            char traceparent_header_value[CKR_VAL_LENGTH];
            res = bpf_probe_read(&traceparent_header_value, sizeof(traceparent_header_value), traceparent_header_value_go_str.str);
            if (res < 0)
            {
                return res;
            }
            w3c_string_to_span_context(traceparent_header_value, parent_span_context);
            return 0;
        }
    }
    return -1;
}

static __always_inline u32 read_go_string(void *base, int offset, char *output, int maxLen, const char *errorMsg) {
    void *ptr = (void *)(base + offset);
    u32 res = get_go_string_from_user_ptr_with_len(ptr, output, maxLen);
    if (res == 0) {
        bpf_debug_printk("Failed to get %s", errorMsg);
        return 0;
    }else{
        bpf_debug_printk("read_go_string: %s %s", errorMsg, output);
        return res;
    }
}

static __always_inline long get_last_slash_length(char *http_path, u32 max_len){
    for(int i = max_len - 1; i > 0; i--)
    {
        if(http_path[i] == '/')
        {
            return i+1;
        }
    }
    return 0;
}

static __always_inline int get_first_slash(char *http_path, u32 max_len){
    for(int i = 0; i < max_len; i++)
    {
        if(http_path[i] == '/')
        {
            return i;
        }
    }
    return -1;
}

// This instrumentation attaches uprobe to the following function:
// func (sh serverHandler) ServeHTTP(rw ResponseWriter, req *Request)
SEC("uprobe/serverHandler_ServeHTTP")
int uprobe_serverHandler_ServeHTTP(struct pt_regs *ctx)
{
    bpf_debug_printk("=== uprobe_serverHandler_ServeHTTP called with ctx: %p ===", ctx);

    struct go_iface go_context = {0};
    get_Go_context(ctx, 4, ctx_ptr_pos, false, &go_context);
    void *key = get_consistent_key(ctx, go_context.data);
    bpf_debug_printk("uprobe_serverHandler_ServeHTTP: go_context.data: %p, consistent_key: %p", go_context.data, key); 
    void *httpReq_ptr = bpf_map_lookup_elem(&http_server_uprobes, &key);
    if (httpReq_ptr != NULL)
    {
        bpf_debug_printk("uprobe/HandlerFunc_ServeHTTP already tracked with the current request");
        return 0;
    }

    u32 map_id = 0;
    struct uprobe_data_t *uprobe_data = bpf_map_lookup_elem(&http_server_uprobe_storage_map, &map_id);
    if (uprobe_data == NULL)
    {
        bpf_printk("uprobe/HandlerFunc_ServeHTTP: http_server_span is NULL");
        return 0;
    }

    __builtin_memset(uprobe_data, 0, sizeof(struct uprobe_data_t));

    // Save response writer
    void *resp_impl = get_argument(ctx, 3);
    uprobe_data->resp_ptr = (u64)resp_impl;

    struct http_server_span_t *http_server_span = &uprobe_data->span;
    http_server_span->start_time = bpf_ktime_get_ns();

    // Propagate context
    void *req_ptr = get_argument(ctx, 4);
    start_span_params_t start_span_params = {
        .ctx = ctx,
        .go_context = &go_context,
        .psc = &http_server_span->psc,
        .sc = &http_server_span->sc,
        .get_parent_span_context_fn = extract_context_from_req_headers,
        .get_parent_span_context_arg = (void*)(req_ptr + headers_ptr_pos),
    };
    start_span(&start_span_params);
    //get method and path for creating pathkey
    read_go_string(req_ptr, method_ptr_pos, http_server_span->method, sizeof(http_server_span->method), "method from request");
    void *url_ptr = 0;
    bpf_probe_read(&url_ptr, sizeof(url_ptr), (void *)(req_ptr + url_ptr_pos));
    u32 path_len = read_go_string(url_ptr, path_ptr_pos, http_server_span->path, sizeof(http_server_span->path), "path from Request.URL");
    u32 pattern_available = 0;
    struct mux_route_match_t *mux_route_match = get_mux_route_match(key);
    if (mux_route_match == NULL) {
        if (pattern_path_supported) {
            bpf_debug_printk("mux null & pattern_path_supported: %d", pattern_path_supported);
            void *pat_ptr = NULL;
            bpf_probe_read(&pat_ptr, sizeof(pat_ptr), (void *)(req_ptr + req_pat_pos));
            if (pat_ptr != NULL) {
                read_go_string(pat_ptr, pat_str_pos, http_server_span->path_pattern, sizeof(http_server_span->path), "patterned path from Request");
                bpf_debug_printk("patterned path found %s", http_server_span->path_pattern);
                u32 trimmed_path_len = get_first_slash(http_server_span->path_pattern, HTTP_PATH_MAX_LEN);
                if(trimmed_path_len != -1){
                    copy_byte_arrays(http_server_span->path_pattern + trimmed_path_len, http_server_span->path_pattern, HTTP_PATH_MAX_LEN - trimmed_path_len);
                    if(trimmed_path_len > 0) {
                        http_server_span->path_pattern[HTTP_PATH_MAX_LEN - trimmed_path_len] = '\0';
                    }
                    bpf_debug_printk("trimmed_path_pattern: %s", http_server_span->path_pattern);
                }
                pattern_available = 1;
            }
        }
    }else{
        bpf_debug_printk("mux_route_match: %s", mux_route_match->path);
        copy_byte_arrays(mux_route_match->path, http_server_span->path_pattern, HTTP_PATH_MAX_LEN);
        pattern_available = 1;
        //We can potentially delete the entry from the mux map here itself. However, I am not sure
        //For now, we are deleting at the time of returns.
        // delete_mux_route_match_map(key);

    }
    bpf_debug_printk("http_server_span->path_pattern: %s, http_server_span->path: %s", http_server_span->path_pattern, http_server_span->path);
    
    // Prepare concatenated input string    
    u8 path_key[MAX_PATH_KEY_BYTE] = {0};
    u8 inc_path_key[MAX_PATH_KEY_CHAR+1] = {0};
    bytes_to_hex_string(http_server_span->psc.TraceID, MAX_PATH_KEY_BYTE, inc_path_key);

    u8 * http_path = NULL;
    if(pattern_available)
    {
        http_path = http_server_span->path_pattern;
    }else{
        http_path = http_server_span->path;
    }

    //This is the stragest shit I have ever seen.
    //If you comment out this line, the program compiliation will fail saying 512 bytes have been exceeded.
    // bpf_debug_printk("http server: ipath=%s http_method=%s http_path=%s",inc_path_key, http_server_span->method , http_server_span->path);
    
    u32 inc_path_key_hash = fnv1a_hash32(inc_path_key, MAX_PATH_KEY_CHAR);
    u32 http_method_hash = fnv1a_hash32(http_server_span->method, METHOD_MAX_LEN);
    u32 http_path_hash = fnv1a_hash32(http_path, PATH_MAX_LEN);
    u32 service_name_hash = service_name_fnv1a_hash32();
    bpf_debug_printk("service_name_hash: %u, inc_path_key_hash: %u, http_method_hash: %u, http_path_hash: %u", service_name_hash, inc_path_key_hash, http_method_hash, http_path_hash);
    u32 hashes[4] = {service_name_hash, inc_path_key_hash, http_method_hash, http_path_hash};
    combine_to_16byte_hash(hashes, 4, path_key);
    copy_byte_arrays(path_key, http_server_span->sc.TraceID, MAX_PATH_KEY_BYTE);
    
    // Print debug info
    bytes_to_hex_string(path_key, MAX_PATH_KEY_BYTE, inc_path_key);
    bpf_debug_printk("http server: Generated pathkey : %s", inc_path_key);



    bpf_debug_printk("uprobe_serverHandler_ServeHTTP: update map : http_server_uprobes , key - consistent_key: %p, value type: uprobe_data_t", key );

    long res = bpf_map_update_elem(&http_server_uprobes, &key, uprobe_data, 0);
    if(res < 0){
        bpf_printk("uprobe_serverHandler_ServeHTTP: Failed to update map : http_server_uprobes , key - consistent_key: %p, value type: uprobe_data_t, response: %ld", key, res);
    }
    start_tracking_span(go_context.data, &http_server_span->sc);
    return 0;
}

// This instrumentation attaches uprobe to the following function:
// func (sh serverHandler) ServeHTTP(rw ResponseWriter, req *Request)
SEC("uprobe/serverHandler_ServeHTTP")
int uprobe_serverHandler_ServeHTTP_Returns(struct pt_regs *ctx) {
    bpf_debug_printk("=== uprobe_serverHandler_ServeHTTP_Returns called with ctx: %p ===", ctx);
    u64 end_time = bpf_ktime_get_ns();
    struct go_iface go_context = {0};
    get_Go_context(ctx, 4, ctx_ptr_pos, false, &go_context);
    void *key = get_consistent_key(ctx, go_context.data);
    bpf_debug_printk("uprobe_serverHandler_ServeHTTP_Returns: consistent_key: %p", key); 

    struct uprobe_data_t *uprobe_data = bpf_map_lookup_elem(&http_server_uprobes, &key);
    if (uprobe_data == NULL) {
        bpf_debug_printk("uprobe/HandlerFunc_ServeHTTP_Returns: entry_state is NULL");
        return 0;
    }

    struct http_server_span_t *http_server_span = &uprobe_data->span;

    void *resp_ptr = (void *)uprobe_data->resp_ptr;
    void *req_ptr = NULL;
    bpf_probe_read(&req_ptr, sizeof(req_ptr), (void *)(resp_ptr + req_ptr_pos));


    // get headers from Request
    void *headers_ptr = 0;
    bpf_probe_read(&headers_ptr, sizeof(headers_ptr), (void *)(resp_ptr+chunk_writer_pos+chunk_writer_header_pos));
    if (headers_ptr) {
        bpf_debug_printk("uprobe_serverHandler_ServeHTTP_Returns: update map : http_headers , key: headers_ptr %p, value type: headerString", headers_ptr );
        long res = bpf_map_update_elem(&http_return_headers, &headers_ptr, http_server_span->sc.TraceID, 0);
        if(res < 0){
            bpf_printk("uprobe_serverHandler_ServeHTTP_Returns: Failed to update map : http_headers , key: headers_ptr %p, value type: headerString, response: %ld", headers_ptr, res);
        }
    }


    http_server_span->end_time = end_time;

    if (pattern_path_supported) {
        void *pat_ptr = NULL;
        bpf_probe_read(&pat_ptr, sizeof(pat_ptr), (void *)(req_ptr + req_pat_pos));
        if (pat_ptr != NULL) {
            read_go_string(pat_ptr, pat_str_pos, http_server_span->path_pattern, sizeof(http_server_span->path), "patterned path from Request");
        }
    }
    read_go_string(req_ptr, remote_addr_pos, http_server_span->remote_addr, sizeof(http_server_span->remote_addr), "remote addr from Request.RemoteAddr");
    read_go_string(req_ptr, host_pos, http_server_span->host, sizeof(http_server_span->host), "host from Request.Host");
    read_go_string(req_ptr, proto_pos, http_server_span->proto, sizeof(http_server_span->proto), "proto from Request.Proto");

    // status code
    bpf_probe_read(&http_server_span->status_code, sizeof(http_server_span->status_code), (void *)(resp_ptr + status_code_pos));

    output_span_event(ctx, http_server_span, sizeof(*http_server_span), &http_server_span->sc);

    stop_tracking_span(&http_server_span->sc, &http_server_span->psc);
    bpf_debug_printk("uprobe_serverHandler_ServeHTTP_Returns: delete map : http_server_uprobes , consistent_key: %p, value type: uprobe_data_t", key);
    bpf_map_delete_elem(&http_server_uprobes, &key);
    delete_mux_route_match_map(key);
    return 0;
}

// This instrumentation attaches uprobe to the following function:
// func (h Header) net/http.Header.writeSubset(w io.Writer, exclude map[string]bool, trace *httptrace.ClientTrace) error
SEC("uprobe/header_writeSubset_server")
int uprobe_writeSubset_server(struct pt_regs *ctx) {
    bpf_debug_printk("=== uprobe_writeSubset_server called with ctx: %p ===", ctx);
    u64 headers_pos = 1;
    void *headers_ptr = get_argument(ctx, headers_pos);

    bpf_debug_printk("uprobe_writeSubset_server: headers_ptr: %p", headers_ptr);
    u64 io_writer_pos = 3;
    void *io_writer_ptr = get_argument(ctx, io_writer_pos);

    char ck_return_header[MAX_PATH_KEY_CHAR+1] = {0};
    void *ck_return_header_ptr = bpf_map_lookup_elem(&http_return_headers, &headers_ptr);

    bpf_debug_printk("uprobe_writeSubset_server: ck_return_header_ptr: %p", ck_return_header_ptr);

    if (ck_return_header_ptr) {
        bytes_to_hex_string(ck_return_header_ptr, MAX_PATH_KEY_BYTE, ck_return_header);

        bpf_debug_printk("uprobe_writeSubset_server: ck_return_header: %s", ck_return_header);
        
            if (!is_ck_instrumentation_enabled()) {
                bpf_debug_printk("uprobe_writeSubset_server: ck instrumentation is not enabled");
                goto done;
            }

            void *buf_ptr = 0;
            bpf_probe_read(&buf_ptr, sizeof(buf_ptr), (void *)(io_writer_ptr + io_writer_buf_ptr_pos)); // grab buf ptr
            if (!buf_ptr) {
                bpf_debug_printk("uprobe_writeSubset_server: Failed to get buf from io writer");
                goto done;
            }

            s64 size = 0;
            if (bpf_probe_read(&size, sizeof(s64), (void *)(io_writer_ptr + io_writer_buf_ptr_pos + offsetof(struct go_slice, cap)))) { // grab capacity
                bpf_debug_printk("uprobe_writeSubset_server: Failed to get size from io writer");
                goto done;
            }

            s64 len = 0;
            if (bpf_probe_read(&len, sizeof(s64), (void *)(io_writer_ptr + io_writer_n_pos))) { // grab len
                bpf_debug_printk("uprobe_writeSubset_server: Failed to get len from io writer");
                goto done;
            }

            if (len < (size - CKR_VAL_LENGTH - CKR_KEY_LENGTH - 4)) { // 4 = strlen(":_") + strlen("\r\n")
                char tp_str[CKR_KEY_LENGTH + 2 + CKR_VAL_LENGTH + 2] = "ck-route: ";
                char end[2] = "\r\n";
                __builtin_memcpy(&tp_str[CKR_KEY_LENGTH + 2], ck_return_header, MAX_PATH_KEY_CHAR);
                __builtin_memcpy(&tp_str[CKR_KEY_LENGTH + 2 + CKR_VAL_LENGTH], end, sizeof(end));
                if (bpf_probe_write_user(buf_ptr + (len & 0x0ffff), tp_str, sizeof(tp_str))) {
                    bpf_debug_printk("uprobe_writeSubset_server: Failed to write trace parent key in buffer");
                    goto done;
                }
                len += CKR_KEY_LENGTH + 2 + CKR_VAL_LENGTH + 2;
                if (bpf_probe_write_user((void *)(io_writer_ptr + io_writer_n_pos), &len, sizeof(len))) {
                    bpf_debug_printk("uprobe_writeSubset_server: Failed to change io writer n");
                    goto done;
                }
            }
    }

done:
    bpf_debug_printk("uprobe_writeSubset_server: delete map : http_return_headers , key: headers_ptr %p, value type: http_return_headers", headers_ptr );
    bpf_map_delete_elem(&http_return_headers, &headers_ptr);
    return 0;
}

