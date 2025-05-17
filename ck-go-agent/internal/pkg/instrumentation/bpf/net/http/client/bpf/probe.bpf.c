// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

#include "arguments.h"
#include "trace/span_context.h"
#include "go_context.h"
#include "go_types.h"
#include "uprobe.h"
#include "trace/span_output.h"
#include "trace/start_span.h"

char __license[] SEC("license") = "Dual MIT/GPL";

#define MAX_HOSTNAME_SIZE 128
#define MAX_PROTO_SIZE 8
#define MAX_PATH_SIZE 128
#define MAX_SCHEME_SIZE 8
#define MAX_OPAQUE_SIZE 8
#define MAX_RAWPATH_SIZE 8
#define MAX_RAWQUERY_SIZE 128
#define MAX_FRAGMENT_SIZE 56
#define MAX_RAWFRAGMENT_SIZE 56
#define MAX_USERNAME_SIZE 8
#define MAX_METHOD_SIZE 16
#define MAX_CONCURRENT MAX_CONCURRENT_REQUESTS
#define MAX_BUCKETS 8

#define AMZ_TARGET_HEADER_LEN 12  //X-Amz-Target
#define AMZ_TARGET_HEADER_VALUE_LEN 40

struct http_request_t {
    BASE_SPAN_PROPERTIES
    char host[MAX_HOSTNAME_SIZE];
    char proto[MAX_PROTO_SIZE];
    u64 status_code;
    char method[MAX_METHOD_SIZE];
    char path[MAX_PATH_SIZE];
    char scheme[MAX_SCHEME_SIZE];
    char opaque[MAX_OPAQUE_SIZE];
    char raw_path[MAX_RAWPATH_SIZE];
    char username[MAX_USERNAME_SIZE];
    char raw_query[MAX_RAWQUERY_SIZE];
    char fragment[MAX_FRAGMENT_SIZE];
    char raw_fragment[MAX_RAWFRAGMENT_SIZE];
    char amz_target[AMZ_TARGET_HEADER_VALUE_LEN];
    char server_path_key[CKR_VAL_LENGTH];
    u8 force_query;
    u8 omit_host;
};

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__type(key, void*);
	__type(value, struct http_request_t);
	__uint(max_entries, MAX_CONCURRENT);
} http_events SEC(".maps");

struct
{
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(key_size, sizeof(u32));
    __uint(value_size, sizeof(struct http_request_t));
    __uint(max_entries, 1);
} http_client_uprobe_storage_map SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__type(key, void*); // the headers ptr
	__type(value, void*); // request key, goroutine or context ptr
	__uint(max_entries, MAX_CONCURRENT);
} http_headers SEC(".maps");

MAP_BUCKET_DEFINITION(go_string_t, go_slice_t)

struct
{
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(key_size, sizeof(u32));
    __uint(value_size, sizeof(MAP_BUCKET_TYPE(go_string_t, go_slice_t)));
    __uint(max_entries, 1);
} golang_mapbucket_storage_map SEC(".maps");

struct
{
    __uint(type, BPF_MAP_TYPE_HASH);
	__type(key, u32);
	__type(value, u32);
    __uint(max_entries, 1);
} http_headers_size_map SEC(".maps");


// Injected in init
volatile const u64 method_ptr_pos;
volatile const u64 url_ptr_pos;
volatile const u64 path_ptr_pos;
volatile const u64 headers_ptr_pos;
volatile const u64 ctx_ptr_pos;
volatile const u64 status_code_pos;
volatile const u64 request_host_pos;
volatile const u64 request_proto_pos;
volatile const u64 scheme_pos;
volatile const u64 opaque_pos;
volatile const u64 user_ptr_pos;
volatile const u64 raw_path_pos;
volatile const u64 omit_host_pos;
volatile const u64 force_query_pos;
volatile const u64 raw_query_pos;
volatile const u64 fragment_pos;
volatile const u64 raw_fragment_pos;
volatile const u64 username_pos;
volatile const u64 io_writer_buf_ptr_pos;
volatile const u64 io_writer_n_pos;
volatile const u64 url_host_pos;
volatile const u64 buckets_ptr_pos;
volatile const u64 response_header_ptr_pos;

// Extracts the span context from the request headers by looking for the 'aws dynamo db' header(s).
//Header key : X-Amz-Target | 
//Possible values : DynamoDB_20120810.PutItem, DynamoDB_20120810.GetItem, DynamoDB_20120810.Query, DynamoDB_20120810.Scan, 
//DynamoDB_20120810.BatchGetItem, DynamoDB_20120810.BatchWriteItem, DynamoDB_20120810.DeleteItem, DynamoDB_20120810.UpdateItem, 
//DynamoDB_20120810.BatchDeleteItem, DynamoDB_20120810.BatchUpdateItem
// Returns 0 on success, negative value on error.
static __always_inline long extract_aws_context_from_req_headers(void *headers_ptr, char *aws_header_value, u64 max_len)
{
    long res;
    u64 headers_count = 0;
    res = bpf_probe_read(&headers_count, sizeof(headers_count), headers_ptr);
    if (res < 0){
        return res;
    }
    if (headers_count == 0){
        return -1;
    }
    unsigned char log_2_bucket_count;
    res = bpf_probe_read(&log_2_bucket_count, sizeof(log_2_bucket_count), headers_ptr + 9);
    if (res < 0){
        return -1;
    }
    u64 bucket_count = 1 << log_2_bucket_count;
    void *header_buckets;
    res = bpf_probe_read(&header_buckets, sizeof(header_buckets), (void*)(headers_ptr + buckets_ptr_pos));
    if (res < 0){
        return -1;
    }
    u32 map_id = 0;
    MAP_BUCKET_TYPE(go_string_t, go_slice_t) *map_value = bpf_map_lookup_elem(&golang_mapbucket_storage_map, &map_id);
    if (!map_value){
        return -1;
    }

    for (u64 j = 0; j < MAX_BUCKETS; j++){
        if (j >= bucket_count){
            break;
        }
        res = bpf_probe_read(map_value, sizeof(MAP_BUCKET_TYPE(go_string_t, go_slice_t)), header_buckets + (j * sizeof(MAP_BUCKET_TYPE(go_string_t, go_slice_t))));
        if (res < 0){
            continue;
        }
        for (u64 i = 0; i < 8; i++){
            if (map_value->tophash[i] == 0){
                continue;
            }
            if (map_value->keys[i].len != AMZ_TARGET_HEADER_LEN){
                continue;
            }
            char current_header_key[AMZ_TARGET_HEADER_LEN];
            bpf_probe_read(current_header_key, sizeof(current_header_key), map_value->keys[i].str);
            if (!bpf_memcmp(current_header_key, "x-amz-target", AMZ_TARGET_HEADER_LEN) && !bpf_memcmp(current_header_key, "X-Amz-Target", AMZ_TARGET_HEADER_LEN)){
                continue;
            }
            void *header_value_ptr = map_value->values[i].array;
            struct go_string header_value_go_str;
            res = bpf_probe_read(&header_value_go_str, sizeof(header_value_go_str), header_value_ptr);
            if (res < 0){
                return -1;
            }
            u64 len_to_read = header_value_go_str.len < max_len-1 ? header_value_go_str.len : max_len-1;
            res = bpf_probe_read(aws_header_value, len_to_read, header_value_go_str.str);
            if (res < 0){
                return res;
            }
            aws_header_value[len_to_read] = '\0';
            bpf_debug_printk("uprobe_Transport_roundTrip: AWS header len_to_read: %d, header_value: %s", len_to_read, aws_header_value);
            return 0;
        }
    }
    return -1;
}

// This instrumentation attaches uprobe to the following function:
// func net/http/transport.roundTrip(req *Request) (*Response, error)
SEC("uprobe/Transport_roundTrip")
int uprobe_Transport_roundTrip(struct pt_regs *ctx) {
    bpf_debug_printk("=== uprobe_Transport_roundTrip called with ctx: %p ===", ctx);
    u64 request_pos = 2;
    void *req_ptr = get_argument(ctx, request_pos);

    struct go_iface go_context = {0};
    get_Go_context(ctx, 2, ctx_ptr_pos, false, &go_context);

    void *key = get_consistent_key(ctx, go_context.data);
    bpf_debug_printk("uprobe_Transport_roundTrip: go_context_key: %p", key); 
    void *httpReq_ptr = bpf_map_lookup_elem(&http_events, &key);
    if (httpReq_ptr != NULL)
    {
        bpf_debug_printk("uprobe/Transport_RoundTrip already tracked with the current context");
        return 0;
    }

    u32 map_id = 0;
    struct http_request_t *httpReq = bpf_map_lookup_elem(&http_client_uprobe_storage_map, &map_id);
    if (httpReq == NULL)
    {
        bpf_debug_printk("uprobe/Transport_roundTrip: httpReq is NULL");
        return 0;
    }

    __builtin_memset(httpReq, 0, sizeof(struct http_request_t));
    httpReq->start_time = bpf_ktime_get_ns();

    start_span_params_t start_span_params = {
        .ctx = ctx,
        .go_context = &go_context,
        .psc = &httpReq->psc,
        .sc = &httpReq->sc,
        .get_parent_span_context_fn = get_parent_sc_glitch,
        .get_parent_span_context_arg = &go_context,
    };
    start_span(&start_span_params);

    if (!get_go_string_from_user_ptr((void *)(req_ptr+method_ptr_pos), httpReq->method, sizeof(httpReq->method))) {
        bpf_debug_printk("uprobe_Transport_roundTrip: Failed to get method from request");
        return 0;
    }

    // get path from Request.URL
    void *url_ptr = 0;

    bpf_probe_read(&url_ptr, sizeof(url_ptr), (void *)(req_ptr+url_ptr_pos));
    if (!get_go_string_from_user_ptr((void *)(url_ptr+path_ptr_pos), httpReq->path, sizeof(httpReq->path))) {
        bpf_debug_printk("uprobe_Transport_roundTrip: Failed to get path from Request.URL");
    }

    // get scheme from Request.URL
    if (!get_go_string_from_user_ptr((void *)(url_ptr+scheme_pos), httpReq->scheme, sizeof(httpReq->scheme))) {
        bpf_debug_printk("uprobe_Transport_roundTrip: Failed to get scheme from Request.URL");
    }

    // get opaque from Request.URL
    if (!get_go_string_from_user_ptr((void *)(url_ptr+opaque_pos), httpReq->opaque, sizeof(httpReq->opaque))) {
        bpf_debug_printk("uprobe_Transport_roundTrip: Failed to get opaque from Request.URL");
    }

    // get RawPath from Request.URL
    if (!get_go_string_from_user_ptr((void *)(url_ptr+raw_path_pos), httpReq->raw_path, sizeof(httpReq->raw_path))) {
        bpf_debug_printk("uprobe_Transport_roundTrip: Failed to get RawPath from Request.URL");
    }

    // get username from Request.URL.User
    void *user_ptr = 0;
    bpf_probe_read(&user_ptr, sizeof(user_ptr), (void *)(url_ptr+user_ptr_pos));
    if (!get_go_string_from_user_ptr((void *)(user_ptr+username_pos), httpReq->username, sizeof(httpReq->username))) {
        bpf_debug_printk("uprobe_Transport_roundTrip: Failed to get RawQuery from Request.URL");
    }

    // get RawQuery from Request.URL
    if (!get_go_string_from_user_ptr((void *)(url_ptr+raw_query_pos), httpReq->raw_query, sizeof(httpReq->raw_query))) {
        bpf_debug_printk("uprobe_Transport_roundTrip: Failed to get RawQuery from Request.URL");
    }

    // get Fragment from Request.URL
    if (!get_go_string_from_user_ptr((void *)(url_ptr+fragment_pos), httpReq->fragment, sizeof(httpReq->fragment))) {
        bpf_debug_printk("uprobe_Transport_roundTrip: Failed to get Fragment from Request.URL");
    }

    // get RawFragment from Request.URL
    if (!get_go_string_from_user_ptr((void *)(url_ptr+raw_fragment_pos), httpReq->raw_fragment, sizeof(httpReq->raw_fragment))) {
        bpf_debug_printk("uprobe_Transport_roundTrip: Failed to get RawFragment from Request.URL");
    }

    // get ForceQuery from Request.URL
    bpf_probe_read(&httpReq->force_query, sizeof(httpReq->force_query), (void *)(url_ptr+force_query_pos));

    // get OmitHost from Request.URL
    bpf_probe_read(&httpReq->omit_host, sizeof(httpReq->omit_host), (void *)(url_ptr+omit_host_pos));

    // get host from Request
    if (!get_go_string_from_user_ptr((void *)(req_ptr+request_host_pos), httpReq->host, sizeof(httpReq->host))) {
        // If host is not present in Request, get it from URL
        if (!get_go_string_from_user_ptr((void *)(url_ptr+url_host_pos), httpReq->host, sizeof(httpReq->host))) {
            bpf_debug_printk("uprobe_Transport_roundTrip: Failed to get host from Request and URL");
        }
    }

    // get proto from Request
    if (!get_go_string_from_user_ptr((void *)(req_ptr+request_proto_pos), httpReq->proto, sizeof(httpReq->proto))) {
        bpf_debug_printk("uprobe_Transport_roundTrip: Failed to get proto from Request");
    }

    // get headers from Request
    void *headers_ptr = 0;
    bpf_probe_read(&headers_ptr, sizeof(headers_ptr), (void *)(req_ptr+headers_ptr_pos));
    if (headers_ptr) {
        bpf_debug_printk("uprobe_Transport_roundTrip: update map : http_headers , key: headers_ptr %p, value type: context", headers_ptr );
        if(bpf_map_update_elem(&http_headers, &headers_ptr, &key, BPF_ANY) < 0) {
            bpf_debug_printk("uprobe_Transport_roundTrip: Failed to update map : http_headers , key: headers_ptr %p, value type: context", headers_ptr );
        }  else{
            // u32 *header_count = bpf_map_lookup_elem(&http_headers_size_map, &map_id);
            // if(header_count == NULL) {
            //     u32 new_header_count = 1;
            //     bpf_map_update_elem(&http_headers_size_map, &map_id, &new_header_count, 0);
            //     bpf_debug_printk("uprobe_Transport_roundTrip: header_count is NULL");
            // } else {
            //     bpf_debug_printk("uprobe_Transport_roundTrip: header_count: %d", *header_count);
            //     *header_count = *header_count + 1;
            // }
        }
    } else {
        bpf_debug_printk("uprobe_Transport_roundTrip: headers_ptr is NULL");
    }

    char aws_header_value[AMZ_TARGET_HEADER_VALUE_LEN] = {0};
    long res = extract_aws_context_from_req_headers(headers_ptr, aws_header_value, AMZ_TARGET_HEADER_VALUE_LEN);
    if (res >= 0) {
        bpf_probe_read_str(httpReq->amz_target, AMZ_TARGET_HEADER_VALUE_LEN, aws_header_value);
        bpf_debug_printk("uprobe_Transport_roundTrip: Received amz target: %s", httpReq->amz_target);
    }
    // Write event
    bpf_debug_printk("uprobe_Transport_roundTrip: update map : http_events , key: context %p, value type: http_request_t", key );

    res = bpf_map_update_elem(&http_events, &key, httpReq, 0);
    if(res < 0) {
        bpf_debug_printk("uprobe_Transport_roundTrip: Failed to update map : http_events , key: context %p, value type: http_request_t and res: %ld", key, res );
    }
    return 0;
}

// Extracts the span context from the response headers by looking for the 'ck-route' header.
// Fills the parent_span_context with the extracted span context.
// Returns 0 on success, negative value on error.
static __always_inline long extract_context_from_response_headers(void *headers_ptr_ptr, void * response_ck_header_value)
{
    void *headers_ptr;
    long res;
    res = bpf_probe_read(&headers_ptr, sizeof(headers_ptr), headers_ptr_ptr);
    if (res < 0){
        return res;
    }
    u64 headers_count = 0;
    res = bpf_probe_read(&headers_count, sizeof(headers_count), headers_ptr);
    if (res < 0){
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
            void *header_value_ptr = map_value->values[i].array;
            struct go_string header_value_go_str;
            res = bpf_probe_read(&header_value_go_str, sizeof(header_value_go_str), header_value_ptr);
            if (res < 0)
            {
                return -1;
            }
            if (header_value_go_str.len != CKR_VAL_LENGTH)
            {
                continue;
            }
            char header_value[CKR_VAL_LENGTH];
            res = bpf_probe_read(&header_value, sizeof(header_value), header_value_go_str.str);
            if (res < 0)
            {
                return res;
            }
            copy_byte_arrays(header_value, response_ck_header_value, CKR_VAL_LENGTH);
            return 0;
        }
    }
    return -1;
}

// This instrumentation attaches uretprobe to the following function:
// func net/http/transport.roundTrip(req *Request) (*Response, error)
SEC("uprobe/Transport_roundTrip")
int uprobe_Transport_roundTrip_Returns(struct pt_regs *ctx) {
    bpf_debug_printk("=== uprobe_Transport_roundTrip_Returns called with ctx: %p ===", ctx);
    u64 end_time = bpf_ktime_get_ns();
    struct go_iface go_context = {0};
    get_Go_context(ctx, 2, ctx_ptr_pos, false, &go_context);
    void *key = get_consistent_key(ctx, go_context.data);
    bpf_debug_printk("uprobe_Transport_roundTrip_Returns: go_context_key: %p", key); 

    struct http_request_t *http_req_span = bpf_map_lookup_elem(&http_events, &key);
    if (http_req_span == NULL) {
        bpf_debug_printk("probe_Transport_roundTrip_Returns: entry_state is NULL");
        return 0;
    }

    if (is_register_abi()) {
        // Getting the returned response
        void *resp_ptr = get_argument(ctx, 1);
        // Get status code from response
        bpf_probe_read(&http_req_span->status_code, sizeof(http_req_span->status_code), (void *)(resp_ptr + status_code_pos));

        char response_ck_header_value[CKR_VAL_LENGTH+1] = {0};
        long res = extract_context_from_response_headers((void *)(resp_ptr + response_header_ptr_pos), response_ck_header_value);        
        if (res >= 0) {
            copy_byte_arrays(response_ck_header_value, http_req_span->server_path_key, CKR_VAL_LENGTH);
            bpf_debug_printk("uprobe_Transport_roundTrip_Returns: server_path_key: %s", response_ck_header_value);
        }
    }

    http_req_span->end_time = end_time;
    output_span_event(ctx, http_req_span, sizeof(*http_req_span), &http_req_span->sc);
    bpf_debug_printk("uprobe_Transport_roundTrip_Returns: delete map : http_events , key: context %p, value type: http_request_t", key );
    long res = bpf_map_delete_elem(&http_events, &key);
    if(res < 0) {
        bpf_debug_printk("uprobe_Transport_roundTrip_Returns: Failed to delete map : http_events , key: context %p, value type: http_request_t and res: %ld", key, res );
    }
    return 0;
}

#ifndef NO_HEADER_PROPAGATION
// This instrumentation attaches uprobe to the following function:
// func (h Header) net/http.Header.writeSubset(w io.Writer, exclude map[string]bool, trace *httptrace.ClientTrace) error
SEC("uprobe/header_writeSubset")
int uprobe_writeSubset(struct pt_regs *ctx) {
    bpf_debug_printk("=== uprobe_writeSubset called with ctx: %p ===", ctx);
    u64 headers_pos = 1;
    void *headers_ptr = get_argument(ctx, headers_pos);

    u64 io_writer_pos = 3;
    void *io_writer_ptr = get_argument(ctx, io_writer_pos);

    void **key_ptr = bpf_map_lookup_elem(&http_headers, &headers_ptr);
    if (!key_ptr) {
        bpf_debug_printk("uprobe_writeSubset: key_ptr is NULL");
        return 0;
    }
    void *key = *key_ptr;
    struct http_request_t *http_req_span = bpf_map_lookup_elem(&http_events, &key);
    if (http_req_span) {
        bpf_debug_printk("uprobe_writeSubset: attempting to write ck-route header");
        if (!is_ck_instrumentation_enabled()) {
            bpf_debug_printk("uprobe_writeSubset: ck instrumentation is not enabled");
            goto done;
        }
            char tp[CKR_VAL_LENGTH];
            span_context_to_ckr_string(&http_req_span->psc, tp);

            void *buf_ptr = 0;
            bpf_probe_read(&buf_ptr, sizeof(buf_ptr), (void *)(io_writer_ptr + io_writer_buf_ptr_pos)); // grab buf ptr
            if (!buf_ptr) {
                bpf_debug_printk("uprobe_writeSubset: Failed to get buf from io writer");
                goto done;
            }

            s64 size = 0;
            if (bpf_probe_read(&size, sizeof(s64), (void *)(io_writer_ptr + io_writer_buf_ptr_pos + offsetof(struct go_slice, cap)))) { // grab capacity
                bpf_debug_printk("uprobe_writeSubset: Failed to get size from io writer");
                goto done;
            }

            s64 len = 0;
            if (bpf_probe_read(&len, sizeof(s64), (void *)(io_writer_ptr + io_writer_n_pos))) { // grab len
                bpf_debug_printk("uprobe_writeSubset: Failed to get len from io writer");
                goto done;
            }

            if (len < (size - CKR_VAL_LENGTH - CKR_KEY_LENGTH - 4)) { // 4 = strlen(":_") + strlen("\r\n")
                char tp_str[CKR_KEY_LENGTH + 2 + CKR_VAL_LENGTH + 2] = "ck-route: ";
                char end[2] = "\r\n";
                __builtin_memcpy(&tp_str[CKR_KEY_LENGTH + 2], tp, sizeof(tp));
                __builtin_memcpy(&tp_str[CKR_KEY_LENGTH + 2 + CKR_VAL_LENGTH], end, sizeof(end));
                if (bpf_probe_write_user(buf_ptr + (len & 0x0ffff), tp_str, sizeof(tp_str))) {
                    bpf_debug_printk("uprobe_writeSubset: Failed to write trace parent key in buffer");
                    goto done;
                }
                len += CKR_KEY_LENGTH + 2 + CKR_VAL_LENGTH + 2;
                if (bpf_probe_write_user((void *)(io_writer_ptr + io_writer_n_pos), &len, sizeof(len))) {
                    bpf_debug_printk("uprobe_writeSubset: Failed to change io writer n");
                    goto done;
                }
                bpf_debug_printk("uprobe_writeSubset: updated io writer with ck-route header");
            }
        } else {
            bpf_debug_printk("uprobe_writeSubset: http_req_span is NULL");
        }
done:
    bpf_debug_printk("uprobe_writeSubset: delete map : http_headers , key: headers_ptr %p, value type: http_headers", headers_ptr );
    if(bpf_map_delete_elem(&http_headers, &headers_ptr) < 0) {
        bpf_debug_printk("uprobe_writeSubset: Failed to delete map : http_headers , key: headers_ptr %p, value type: http_headers", headers_ptr );
    } else {
        bpf_debug_printk("uprobe_writeSubset: deleted map : http_headers , key: headers_ptr %p, value type: http_headers", headers_ptr );
            // u32 map_id = 0;
            // u32 *header_count = bpf_map_lookup_elem(&http_headers_size_map, &map_id);
            // if(header_count == NULL) {
            // bpf_debug_printk("uprobe_writeSubset: header_count is NULL");
            // } else {
            //     bpf_debug_printk("uprobe_writeSubset: header_count: %d", *header_count);
            //     *header_count = *header_count - 1;
            // }

    }
    return 0;
}
#else
// Not used at all, empty stub needed to ensure both versions of the bpf program are
// able to compile with bpf2go. The userspace code will avoid loading the probe if
// context propagation is not enabled.
SEC("uprobe/header_writeSubset")
int uprobe_writeSubset(struct pt_regs *ctx) {
    return 0;
}
#endif
