// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

#include "arguments.h"
#include "go_types.h"
#include "trace/span_context.h"
#include "go_context.h"
#include "uprobe.h"
#include "trace/start_span.h"

char __license[] SEC("license") = "Dual MIT/GPL";

#define TARGET_SIZE 50
#define METHOD_SIZE GRPC_METHOD_SIZE
#define MAX_CONCURRENT MAX_CONCURRENT_REQUESTS

struct grpc_request_t
{
    BASE_SPAN_PROPERTIES
    char method[METHOD_SIZE];
    char target[TARGET_SIZE];
    u32 status_code;
    bool is_stream;
};

struct composite_http_stream_key
{
    u64 conn_ptr;
    u32 nextid;
    u32 padding;  // Add padding to ensure 8-byte alignment
};

struct hpack_header_field
{
    struct go_string name;
    struct go_string value;
    bool sensitive;
};

struct
{
    __uint(type, BPF_MAP_TYPE_HASH);
    __type(key, void *);
    __type(value, struct grpc_request_t);
    __uint(max_entries, MAX_CONCURRENT);
} grpc_events SEC(".maps");

struct
{
    __uint(type, BPF_MAP_TYPE_LRU_HASH);
    __type(key, u32);
    __type(value, struct grpc_request_t);
    __uint(max_entries, MAX_CONCURRENT);
} hf_to_grpc_request SEC(".maps");



// Injected in init
volatile const u64 clientconn_target_ptr_pos;
volatile const u64 httpclient_nextid_pos;
volatile const u64 headerFrame_streamid_pos;
volatile const u64 headerFrame_hf_pos;
volatile const u64 error_status_pos;
volatile const u64 status_s_pos;
volatile const u64 status_code_pos;
volatile const u64 loopywriter_conn_pos;
volatile const u64 httpclient_conn_pos;

volatile const bool write_status_supported;


static __always_inline int process_grpc_client_request_start(char * probe_name, struct pt_regs *ctx, u64 method_ptr_pos, u64 method_len_pos, bool is_stream) {
    struct go_iface go_context = {0};
    get_Go_context(ctx, 2, 0, true, &go_context);

    // Get key
    void *key = (void *)GOROUTINE(ctx);
    bpf_debug_printk("%s: go_context_key: %p", probe_name, key); 
    void *grpcReq_ptr = bpf_map_lookup_elem(&grpc_events, &key);
    if (grpcReq_ptr != NULL)
    {
        bpf_printk("%s: already tracked with the current context", probe_name);
        return 0;
    }

    struct grpc_request_t grpcReq = {};
    grpcReq.start_time = bpf_ktime_get_ns();

    // Read Method
    void *method_ptr = get_argument(ctx, method_ptr_pos);
    u64 method_len = (u64)get_argument(ctx, method_len_pos);
    u64 method_size = sizeof(grpcReq.method);
    method_size = method_size < method_len ? method_size : method_len;
    bpf_probe_read(&grpcReq.method, method_size, method_ptr);
    grpcReq.is_stream = is_stream;
    // Read ClientConn.Target
    // void *clientconn_ptr = get_argument(ctx, clientconn_pos);
    // if (!get_go_string_from_user_ptr((void*)(clientconn_ptr + clientconn_target_ptr_pos), grpcReq.target, sizeof(grpcReq.target)))
    // {
    //     bpf_printk("target write failed, aborting ebpf probe");
    //     return 0;
    // }

    start_span_params_t start_span_params = {
        .ctx = ctx,
        .go_context = &go_context,
        .psc = &grpcReq.psc,
        .sc = &grpcReq.sc,
        .get_parent_span_context_fn = NULL,
        .get_parent_span_context_arg = NULL,
    };
    start_span(&start_span_params);

    // Write event
    long res = bpf_map_update_elem(&grpc_events, &key, &grpcReq, 0);
    if( res < 0) {
        bpf_printk("%s: Failed to update map : (client)grpc_events , key: context %p, value type: grpc_request_t and res: %ld", probe_name, key, res );
    } else {
        bpf_debug_printk("%s: Updated map : (client)grpc_events , key: context %p, value type: grpc_request_t and res: %ld", probe_name, key, res );
    }
    return 0;
}

static __always_inline int process_grpc_client_request_end(char* probe_name, struct pt_regs *ctx, int response_pos, void* err, bool is_stream) {

    void *key = (void *)GOROUTINE(ctx);
    struct grpc_request_t *grpc_span = bpf_map_lookup_elem(&grpc_events, &key);
    if (grpc_span == NULL) {
        bpf_printk("%s: process_grpc_client_request_end: grpc_request_t is NULL in ret probe", probe_name);
        return 0;
    }

    if(grpc_span->is_stream != is_stream) {
        bpf_debug_printk("%s: process_grpc_client_request_end: grpc_span->is_stream is %d and is_stream is %d, Not continuing with ebpf probe", probe_name, grpc_span->is_stream, is_stream);
        return 0;
    }

    if(!write_status_supported || !is_ck_instrumentation_enabled()) {
        goto done;
    }

    //This means we can read the response and potentially read the error code also.
    if(response_pos > 0) {
        // Getting the returned response (error)
        // The status code is embedded 3 layers deep:
        // Invoke() error
        // the `error` interface concrete type here is a gRPC `internal.Error` struct
        // type Error struct {
        //   s *Status
        // }
        // The `Error` struct embeds a `Status` proto object
        // type Status struct {
        //   s *Status
        // }
        // The `Status` proto object contains a `Code` int32 field, which is what we want
        void *resp_ptr = get_argument(ctx, 2);
        if(resp_ptr == 0) {
            bpf_debug_printk("%s: process_grpc_client_request_end: resp_ptr is 0, Not continuing with ebpf probe", probe_name);
            // err == nil
            goto done;
        }
        void *status_ptr = 0;
        // get `s` (Status pointer field) from Error struct
        bpf_probe_read_user(&status_ptr, sizeof(status_ptr), (void *)(resp_ptr+error_status_pos));
        // get `s` field from Status object pointer
        void *s_ptr = 0;
        bpf_probe_read_user(&s_ptr, sizeof(s_ptr), (void *)(status_ptr + status_s_pos));
        // Get status code from Status.s pointer
        bpf_probe_read_user(&grpc_span->status_code, sizeof(grpc_span->status_code), (void *)(s_ptr + status_code_pos));
    }

    //This means some error has occured. But we do not know the error code.
    if(err != NULL) {
        //We will set the code to 2 (Unknown) if the status code is not set.
        if(grpc_span->status_code == 0) {
            grpc_span->status_code = 2;
        }
    }
done:
    grpc_span->end_time = bpf_ktime_get_ns();
    output_span_event(ctx, grpc_span, sizeof(*grpc_span), &grpc_span->sc);
    bpf_debug_printk("%s: process_grpc_client_request_end: delete map : (client)grpc_events , key: context %p, value type: grpc_request_t", probe_name, key );
    long res = bpf_map_delete_elem(&grpc_events, &key);
    if( res < 0) {
        bpf_printk("%s: process_grpc_client_request_end: Failed to delete map : (client)grpc_events , key: context %p, value type: grpc_request_t and res: %ld", probe_name, key, res );
    }

    return 0;
}



// This instrumentation attaches uprobe to the following function:
// func (cc *ClientConn) Invoke(ctx context.Context, method string, args, reply interface{}, opts ...CallOption) error
SEC("uprobe/ClientConn_Invoke")
int uprobe_ClientConn_Invoke(struct pt_regs *ctx)
{
    bpf_debug_printk("=== uprobe_ClientConn_Invoke called with ctx: %p ===", ctx);

    // positions
    u64 clientconn_pos = 1;
    u64 method_ptr_pos = 4;
    u64 method_len_pos = 5;
    process_grpc_client_request_start("uprobe_ClientConn_Invoke", ctx, method_ptr_pos, method_len_pos, false);
    return 0;
}

// This instrumentation attaches uprobe to the following function:
// func (cc *ClientConn) Invoke(ctx context.Context, method string, args, reply interface{}, opts ...CallOption) error
SEC("uprobe/ClientConn_Invoke")
int uprobe_ClientConn_Invoke_Returns(struct pt_regs *ctx) {

    bpf_debug_printk("=== uprobe_ClientConn_Invoke_Returns called with ctx: %p ===", ctx);
    int response_pos = 2;
    void *err = NULL;

    process_grpc_client_request_end("uprobe_ClientConn_Invoke_Returns", ctx, response_pos, err, false);
    return 0;
}


// Same as ClientConn_Invoke, registers for the method are offset by one
SEC("uprobe/ClientConn_NewStream")
int uprobe_ClientConn_NewStream(struct pt_regs *ctx) {
    bpf_debug_printk("=== uprobe_ClientConn_NewStream called with ctx: %p ===", ctx);
    u64 method_ptr_pos = 5;
    u64 method_len_pos = 6;

    process_grpc_client_request_start("uprobe_ClientConn_NewStream", ctx, method_ptr_pos, method_len_pos, true);
    return 0;
}

SEC("uprobe/ClientConn_NewStream")
int uprobe_ClientConn_NewStream_Returns(struct pt_regs *ctx) {
    bpf_debug_printk("=== uprobe/proc grpc ClientConn.NewStream return === ");

    void *stream = get_argument(ctx, 1);

    //This means there is some error and we need to set the status code to 2 (Unknown)
    //The first param is a stream and its null.
    if (!stream) {
        process_grpc_client_request_end("uprobe_ClientConn_NewStream_Returns", ctx, 0, (void*)1, true);
    }

    return 0;
}

// google.golang.org/grpc.(*clientStream).RecvMsg
SEC("uprobe/clientStream_RecvMsg")
int uprobe_clientStream_RecvMsg_Returns(struct pt_regs *ctx) {
    bpf_debug_printk("=== uprobe/proc grpc clientStream.RecvMsg return === ");
    void *err = get_argument(ctx, 1);
    process_grpc_client_request_end("uprobe_clientStream_RecvMsg_Returns", ctx, 0, err, true);
    return 0;
}

SEC("uprobe/ClientConn_Close")
int uprobe_ClientConn_Close(struct pt_regs *ctx) {
    bpf_debug_printk("=== uprobe/proc grpc ClientConn.Close === ");

    void *key = (void *)GOROUTINE(ctx);
    long res = bpf_map_delete_elem(&grpc_events, &key);
    if(res < 0) {
        bpf_printk("uprobe_ClientConn_Close: Failed to delete map : grpc_events , key: context %p, value type: grpc_request_t and res: %ld", key, res);
    }
    return 0;
}

// func (c *controlBuffer) executeAndPut(f func() bool, it cbItem) (bool, error) 
SEC("uprobe/controlBuffer_executeAndPut")
int uprobe_controlBuffer_executeAndPut(struct pt_regs *ctx) {
    bpf_debug_printk("=== uprobe_controlBuffer_executeAndPut called with ctx: %p ===", ctx);

    void * hf_ptr = get_argument(ctx, 4);
    if(hf_ptr == NULL) {
        bpf_debug_printk("uprobe_controlBuffer_executeAndPut: hf_ptr is NULL, Not continuing with ebpf probe");
        return 0;
    }

    void * key = (void *)GOROUTINE(ctx);
    struct grpc_request_t *grpcReq = bpf_map_lookup_elem(&grpc_events, &key);
    if(grpcReq == NULL) {
        bpf_debug_printk("uprobe_controlBuffer_executeAndPut: grpcReq is NULL, Not continuing with ebpf probe");
        return 0;
    }

    long res = bpf_map_update_elem(&hf_to_grpc_request, &hf_ptr, grpcReq, 0);
    if(res < 0) {
        bpf_debug_printk("uprobe_controlBuffer_executeAndPut: Failed to update map : hf_to_grpc_request , consistent_key: %p, hf_ptr %p, value type: span_context and res: %ld", key, hf_ptr, res );
    } else {
        bpf_debug_printk("uprobe_controlBuffer_executeAndPut: Updated map : hf_to_grpc_request , consistent_key: %p, hf_ptr %p, value type: span_context and res: %ld", key, hf_ptr, res );
    }
    return 0;
}


// func (l *loopyWriter) headerHandler(h *headerFrame) error
SEC("uprobe/loopyWriter_headerHandler")
int uprobe_LoopyWriter_HeaderHandler(struct pt_regs *ctx)
{
    bpf_debug_printk("=== uprobe_LoopyWriter_HeaderHandler called with ctx: %p ===", ctx);
    
    void *headerFrame_ptr = get_argument(ctx, 2);
    bpf_debug_printk("uprobe_LoopyWriter_HeaderHandler: headerFrame_ptr: %p", headerFrame_ptr);

    long res = 0;

    struct grpc_request_t *grpcReq = bpf_map_lookup_elem(&hf_to_grpc_request, &headerFrame_ptr);
    if (grpcReq == NULL)
    {
        bpf_debug_printk("uprobe_LoopyWriter_HeaderHandler: hf_ptr %p not found in map : hf_to_span_contexts", headerFrame_ptr);
        return 0;
    }
    bpf_debug_printk("uprobe_LoopyWriter_HeaderHandler: Found span context for headerFrame_ptr %p", headerFrame_ptr);
    

    char ck_key[CKR_KEY_LENGTH] = "ck-route";
    struct go_string key_str = write_user_go_string(ck_key, sizeof(ck_key));
    if (key_str.len == 0) {
        bpf_printk("key write failed, aborting ebpf probe");
        goto done;
    }

    // Write headers
    char val[CKR_VAL_LENGTH];
    span_context_to_ckr_string(&grpcReq->psc, val);
    struct go_string val_str = write_user_go_string(val, sizeof(val));
    if (val_str.len == 0) {
        bpf_printk("val write failed, aborting ebpf probe");
        goto done;
    }
    struct hpack_header_field hf = {};
    hf.name = key_str;
    hf.value = val_str;
    hf.sensitive = true;
    bpf_debug_printk("Writing CK Header - value: %s", hf.value.str);

    append_item_to_slice(&hf, sizeof(hf), (void *)(headerFrame_ptr + (headerFrame_hf_pos)));    
done:
    res = bpf_map_delete_elem(&hf_to_grpc_request, &headerFrame_ptr); 
    if( res < 0) {
        bpf_printk("uprobe_LoopyWriter_HeaderHandler: Failed to delete map : hf_to_grpc_request , key: headerFrame_ptr %p, value type: span_context and res: %ld", headerFrame_ptr, res);
    }
    return 0;
}

// func (t *http2Client) NewStream(ctx context.Context, callHdr *CallHdr) (*Stream, error)
// SEC("uprobe/http2Client_NewStream")
// int uprobe_http2Client_NewStream(struct pt_regs *ctx)
// {
//     bpf_debug_printk("=== uprobe_http2Client_NewStream called with ctx: %p ===", ctx);
//     void *httpclient_ptr = get_argument(ctx, 1);
//     u32 nextid = 0;
//     void *httpclient_conn_ptr = NULL;

//     bpf_probe_read(&httpclient_conn_ptr, sizeof(httpclient_conn_ptr), (void *)(httpclient_ptr + (httpclient_conn_pos)));

//     if(httpclient_conn_ptr == NULL) {
//         bpf_printk("uprobe_http2Client_NewStream: httpclient_conn_ptr is NULL, Not continuing with ebpf probe");
//         return 0;
//     }

//     bpf_probe_read(&nextid, sizeof(nextid), (void *)(httpclient_ptr + (httpclient_nextid_pos)));
    

//     // Get the span context from go context. The mapping is created in the Invoke probe,
//     // the context here is derived from the Invoke context.
//     void *key = (void *)GOROUTINE(ctx);
//     bpf_printk("uprobe_http2Client_NewStream: key: %p", key);
//     struct grpc_request_t *grpcReq = bpf_map_lookup_elem(&grpc_events, &key);
//     // struct span_context *current_span_context = get_parent_span_context(&go_context);
//     if (grpcReq != NULL) {
//         struct composite_http_stream_key composite_key = {
//             .conn_ptr = (u64)httpclient_conn_ptr,
//             .nextid = nextid,
//             .padding = 0
//         };
//         bpf_debug_printk("uprobe_http2Client_NewStream: KEY-DETAILS conn_ptr raw=%p, as-u64=%llx, nextid=%u", 
//                   httpclient_conn_ptr, composite_key.conn_ptr, composite_key.nextid);
//         // bpf_map_update_elem(&streamid_to_span_contexts, &nextid, current_span_context, 0);
//         long res = bpf_map_update_elem(&composite_http_stream_key_to_span_contexts, &composite_key, grpcReq, 0);
//         if( res < 0) {
//             bpf_printk("uprobe_http2Client_NewStream: Failed to update map : composite_http_stream_key_to_span_contexts , key: conn_ptr %p, nextid %u, value type: span_context and res: %ld", httpclient_conn_ptr, nextid, res );
//         } else {
//             bpf_debug_printk("uprobe_http2Client_NewStream: Updated map : composite_http_stream_key_to_span_contexts , key: conn_ptr %p, nextid %u, value type: span_context and res: %ld", httpclient_conn_ptr, nextid, res );
//         }
//     } else {
//         bpf_printk("uprobe_http2Client_NewStream: grpcReq is NULL, Not continuing with ebpf probe. Consistent Key: %p", key);
//     }

//     return 0;
// }
