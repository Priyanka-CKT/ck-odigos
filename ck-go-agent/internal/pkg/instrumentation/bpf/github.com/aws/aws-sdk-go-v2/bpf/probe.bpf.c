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
#define MAX_CONCURRENT 200
#define MAX_ERROR_MSG_SIZE 32

/**
 * This probe is not used at all. SQS instrumentation is not supported yet.
 */

typedef struct sqs_attribute_value {
    void * dataType;
    go_slice_t BinaryListValues;
    go_slice_t BinaryValue;
    go_slice_t StringListValues;
    void * StringValue;
} sqs_attribute_value_t;

MAP_BUCKET_DEFINITION(go_string_t, sqs_attribute_value_t)

struct
{
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(key_size, sizeof(u32));
    __uint(value_size, sizeof(MAP_BUCKET_TYPE(go_string_t, sqs_attribute_value_t)));
    __uint(max_entries, 1);
} golang_mapbucket_storage_map SEC(".maps");


//Injected at init
// volatile const u64 baseCmd_args_pos;

// This instrumentation attaches uprobe to the following function:
// github.com/aws/aws-sdk-go-v2/service/sqs.(*Client).SendMessage
// func (c *Client) SendMessage(ctx context.Context, params *SendMessageInput, optFns ...func(*Options)) (*SendMessageOutput, error) {
SEC("uprobe/SendMessage") 
int uprobe_SendMessage(struct pt_regs *ctx) {
    // argument positions
    bpf_printk("=== uprobe_SendMessage called with ctx: %p ===", ctx);

    void* sendmsg_input_ptr = get_argument(ctx, 4);
    if (!sendmsg_input_ptr) {
        bpf_printk("=== sendmsg_input_ptr is NULL ===");
        return 0;
    }

    bpf_printk("=== uprobe_SendMessage: sendmsg_input_ptr: %p ===", sendmsg_input_ptr);

    //Its at an offset of 0 from sendmsg_input_ptr
    void *sendmsg_msgbody_ptr_ptr = sendmsg_input_ptr; 
    void *sendmsg_msgbody_ptr = 0;
    bpf_probe_read(&sendmsg_msgbody_ptr, sizeof(sendmsg_msgbody_ptr), sendmsg_msgbody_ptr_ptr);

    bpf_printk("=== uprobe_SendMessage: sendmsg_msgbody_ptr: %p ===", sendmsg_msgbody_ptr);

    //Read the msgbody
    char msgbody[128] = {0};
    get_go_string_from_user_ptr(sendmsg_msgbody_ptr, msgbody, 127);
    bpf_printk("=== uprobe_SendMessage: msgbody: %s ===", msgbody);

    void *sendmsg_queueUrl_ptr_ptr = sendmsg_input_ptr + 8; 
    void *sendmsg_queueUrl_ptr = 0;
    bpf_probe_read(&sendmsg_queueUrl_ptr, sizeof(sendmsg_queueUrl_ptr), sendmsg_queueUrl_ptr_ptr);
    bpf_printk("=== uprobe_SendMessage: sendmsg_queueUrl_ptr: %p ===", sendmsg_queueUrl_ptr);

    //Read the queueUrl
    char queueUrl[128] = {0};
    get_go_string_from_user_ptr(sendmsg_queueUrl_ptr, queueUrl, 127);
    bpf_printk("=== uprobe_SendMessage: queueUrl: %s ===", queueUrl);

    //Next field is DelayInSeconds.. its a int32 field. The size of the field is 4 bytes.
    void *sendmsg_delayInSeconds_ptr = sendmsg_input_ptr + 8 + 8;

    u32 delayInSeconds = 0;
    bpf_probe_read(&delayInSeconds, sizeof(delayInSeconds), sendmsg_delayInSeconds_ptr);
    bpf_printk("=== uprobe_SendMessage: delayInSeconds: %d ===", delayInSeconds);

    //Now comes the message attributesmap..
    // though delayInSeconds is 4 bytes, we need to pad it to 8 bytes.
    void *sendmsg_messageAttributes_ptr_ptr = sendmsg_input_ptr + 8 + 8 + 8; 
    bpf_printk("=== uprobe_SendMessage: sendmsg_messageAttributes_ptr_ptr: %p ===", sendmsg_messageAttributes_ptr_ptr);

    void *sendmsg_messageAttributes_ptr = 0;
    bpf_probe_read(&sendmsg_messageAttributes_ptr, sizeof(sendmsg_messageAttributes_ptr), sendmsg_messageAttributes_ptr_ptr);
    bpf_printk("=== uprobe_SendMessage: sendmsg_messageAttributes_ptr: %p ===", sendmsg_messageAttributes_ptr);

    u64 attributes_count;
    bpf_probe_read(&attributes_count, sizeof(attributes_count), sendmsg_messageAttributes_ptr);
    bpf_printk("=== uprobe_SendMessage: attributes_count: %d ===", attributes_count);
    
    unsigned char log_2_bucket_count;
    bpf_probe_read(&log_2_bucket_count, sizeof(log_2_bucket_count), sendmsg_messageAttributes_ptr + 9);
    bpf_printk("=== uprobe_SendMessage: log_2_bucket_count: %d ===", log_2_bucket_count);

    u64 bucket_count = 1 << log_2_bucket_count;
    bpf_printk("=== uprobe_SendMessage: bucket_count: %d ===", bucket_count);
    
    long res;
    void *attribute_buckets;
    //use the hmap bucket ptr position instead of 16.
    bpf_probe_read(&attribute_buckets, sizeof(attribute_buckets), (void*)(sendmsg_messageAttributes_ptr + 16));
    res = bpf_printk("=== uprobe_SendMessage: attribute_buckets: %p ===", attribute_buckets);
    if (res < 0) {
        bpf_printk("=== uprobe_SendMessage: bpf_probe_read failed ===");
        return 0;
    }
    u32 map_id = 0;
    MAP_BUCKET_TYPE(go_string_t, sqs_attribute_value_t) *map_value = bpf_map_lookup_elem(&golang_mapbucket_storage_map, &map_id);
    if (!map_value) {
        bpf_printk("=== uprobe_SendMessage: map lookup failed ===");
        return 0;
    }


    for (u64 j = 0; j < 5; j++)
    {
        if (j >= bucket_count)
        {
            break;
        }

        // First read into local buffer
        res = bpf_probe_read(map_value, sizeof(MAP_BUCKET_TYPE(go_string_t, sqs_attribute_value_t)), 
                           attribute_buckets + (j * sizeof(MAP_BUCKET_TYPE(go_string_t, sqs_attribute_value_t))));
        if (res < 0)
        {
            bpf_printk("=== uprobe_SendMessage: bpf_probe_read failed: %d ===", res);
            continue;
        }

        for (u64 i = 0; i < 8; i++)
        {
            if (map_value->tophash[i] == 0)
            {
                continue;
            }
            bpf_printk("=== uprobe_SendMessage: map_value->keys[i].len: %d ===", map_value->keys[i].len);
        }
        // Then copy from local buffer to map value
    }


    return 0;
}


// This instrumentation attaches uprobe to the following function:
// github.com/aws/aws-sdk-go-v2/service/sqs.(*Client).SendMessage
// func (c *Client) SendMessage(ctx context.Context, params *SendMessageInput, optFns ...func(*Options)) (*SendMessageOutput, error) {
SEC("uprobe/SendMessage") 
int uprobe_SendMessage_Returns(struct pt_regs *ctx) {
    bpf_printk("=== uprobe_SendMessage_Returns called with ctx: %p ===", ctx);
    return 0;
}
