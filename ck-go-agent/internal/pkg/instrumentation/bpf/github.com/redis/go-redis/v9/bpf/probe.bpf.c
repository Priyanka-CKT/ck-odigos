// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

#include "arguments.h"
#include "trace/span_context.h"
#include "go_context.h"
#include "go_types.h"
#include "uprobe.h"
#include "trace/start_span.h"

char __license[] SEC("license") = "Dual MIT/GPL";

#define MAX_QUERY_SIZE 32
#define MAX_CONCURRENT MAX_CONCURRENT_REQUESTS
#define MAX_ERROR_MSG_SIZE 32
struct redis_request_t {
    BASE_SPAN_PROPERTIES
    char cmdbuf[MAX_QUERY_SIZE];
    char error_code[MAX_ERROR_MSG_SIZE];
    bool is_error;
};

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__type(key, void*);
	__type(value, struct redis_request_t);
	__uint(max_entries, MAX_CONCURRENT);
} redis_events SEC(".maps");

//Injected at init
// volatile const u64 baseCmd_args_pos;


static __always_inline void get_cmd_from_args_slice(struct pt_regs *ctx, struct redis_request_t *request, int cmder_pos) {

    //cmder is an interface. So the data pointer is the next argument position from where it is defined.
    void *cmder_data_ptr = get_argument(ctx, cmder_pos + 1);

    if(cmder_data_ptr == NULL) {
        bpf_printk("uprobe_redisProcess: cmder_data_ptr is NULL");
        return;
    }

    struct go_iface cmder_iface = {0};
    bpf_probe_read(&cmder_iface, sizeof(cmder_iface), cmder_data_ptr);


    //16 is the offset of the baseCmd_args field in the baseCmd struct
    //There is a context.Context in the first position of the baseCmd struct
    //We are hardcoding this offset for now as we were not able to generate this 
    //offset for v9 somehow.
    void *baseCmd_args_ptr = cmder_data_ptr + 16;

    struct go_slice slice = {0};
    bpf_probe_read(&slice, sizeof(slice), baseCmd_args_ptr);

    u64 slice_len = slice.len;
    u64 slice_cap = slice.cap;
    if (slice_len > 0 && slice.array != NULL)
    {
        //the array is a slice of interfaces. We are interested in the zero index element of the array.
        struct go_iface args_iface = {};
        bpf_probe_read(&args_iface, sizeof(args_iface), slice.array);

        if(args_iface.data == NULL) {
            bpf_debug_printk("uprobe_redisProcess: args_iface.data is NULL");
            return;
        }

        get_go_string_from_user_ptr(args_iface.data, request->cmdbuf, MAX_QUERY_SIZE-1);
        bpf_debug_printk("uprobe_redisProcess: cmd_go_str: %s", request->cmdbuf);
    }

}


static __always_inline void init_request_and_start_span(struct pt_regs *ctx, struct redis_request_t *request, int context_pos, int cmder_pos) {
    request->start_time = bpf_ktime_get_ns();
    bpf_debug_printk("uprobe: redis init_request_and_start_span: request->start_time: %lld", request->start_time);
    struct go_iface go_context = {0};
    get_Go_context(ctx, context_pos, 0, true, &go_context);
    
    start_span_params_t start_span_params = {
        .ctx = ctx,
        .go_context = &go_context,
        .psc = &request->psc,
        .sc = &request->sc,
        .get_parent_span_context_fn = NULL,
        .get_parent_span_context_arg = NULL,
    };
    start_span(&start_span_params);
    u8 path_key[MAX_PATH_KEY_CHAR+1] = {0};
    bytes_to_hex_string(request->psc.TraceID, MAX_PATH_KEY_BYTE, path_key);
    bpf_debug_printk("uprobe: redis parent path_key: %s", path_key);
    bytes_to_hex_string(request->sc.TraceID, MAX_PATH_KEY_BYTE, path_key);

    get_cmd_from_args_slice(ctx, request, cmder_pos);
    // Get key
    void *key = get_consistent_key(ctx, NULL);
    bpf_debug_printk("uprobe: redis init_request_and_start_span, key: go_context.data %p, consistent_key %p, value: redis request %p", go_context.data , key, request);
    bpf_map_update_elem(&redis_events, &key, request, 0);
}


static __always_inline void get_error_from_process_response(struct pt_regs *ctx, struct redis_request_t *redis_request) {
    if (is_register_abi()) {
        // Getting the returned response
        void *resp_ptr = get_argument(ctx, 3);
        if (resp_ptr != NULL) {
            bpf_debug_printk("uprobe_redisProcessReturns: Some error has occurred in process returns.");
            redis_request->is_error = true;
            u32 error_msg_len = get_go_string_from_user_ptr_with_len((void*) (resp_ptr), redis_request->error_code, MAX_ERROR_MSG_SIZE-1);
            bpf_debug_printk("uprobe_redisProcessReturns: error_msg: %s, error_msg_len: %d", redis_request->error_code, error_msg_len);
        }
    }
}


// This instrumentation attaches uprobe to the following function:
// github.com/redis/go-redis/v9.(*baseClient)._process
// func (c *baseClient) _process(ctx context.Context, cmd Cmder, attempt int) (bool, error)
SEC("uprobe/redisProcess") 
int uprobe_redisProcess(struct pt_regs *ctx) {
    // argument positions
    bpf_debug_printk("=== uprobe_redisProcess called with ctx: %p ===", ctx);
    struct redis_request_t redis_request = {0};

    //2 is contextContext pointer
    //3 is contextContext data pointer
    //4 is cmder pointer
    //5 is cmder data pointer
    init_request_and_start_span(ctx, &redis_request, 2, 4);
    return 0;
}


// This instrumentation attaches uprobe to the following function:
// github.com/redis/go-redis/v9.(*baseClient)._process
// func (c *baseClient) _process(ctx context.Context, cmd Cmder, attempt int) (bool, error)
SEC("uprobe/redisProcess") 
int uprobe_redisProcess_Returns(struct pt_regs *ctx) {
    bpf_debug_printk("=== uprobe_redisProcessReturn called with ctx: %p ===", ctx);
    void *key = get_consistent_key(ctx, NULL);

    struct redis_request_t *redis_request = bpf_map_lookup_elem(&redis_events, &key);
    if (redis_request == NULL) {
        bpf_debug_printk("uprobe_redisProcessReturns: redis_request is NULL in process returns.");
        return 0;
    }
    bpf_debug_printk("uprobe_redisProcessReturns: redis_request->cmdbuf: %s", redis_request->cmdbuf);
    
    redis_request->end_time = bpf_ktime_get_ns();  
    get_error_from_process_response(ctx, redis_request);

    output_span_event(ctx, redis_request, sizeof(struct redis_request_t), &redis_request->sc);    
    stop_tracking_span(&redis_request->sc, &redis_request->psc);
    bpf_debug_printk("uprobe_redisProcessReturns: delete map : redis_events , key - consistent_key: %p, value type: redis_request %p", key, &redis_request);
    bpf_map_delete_elem(&redis_events, &key);
    return 0;
}
