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

#define MAX_CONCURRENT MAX_CONCURRENT_REQUESTS
// https://github.com/segmentio/kafka-go/blob/main/writer.go#L118
// TODO: (this value is directly impact the map sizes as well as the verification complexity)
// limitation on map entry size: https://github.com/iovisor/bcc/issues/2519#issuecomment-534359316
// the default value is 100, but it can be changed by the user
// we must specify a limit for the verifier
// 100 is the limit.. If you want to add more statements since the verifier is failing, you can reduce this value.
#define MAX_BATCH_SIZE 100
// https://github.com/apache/kafka/blob/0.10.2/core/src/main/scala/kafka/common/Topic.scala#L30C3-L30C34
#define MAX_TOPIC_SIZE 128

struct topic_name_t {
    char topic[MAX_TOPIC_SIZE];
};

struct kafka_request_t {
    BASE_SPAN_PROPERTIES
    char global_topic[MAX_TOPIC_SIZE];
    u16 valid_messages;
    bool is_global_topic;
    bool isError;
}__attribute__((packed));

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__type(key, void*);
	__type(value, struct kafka_request_t);
	__uint(max_entries, MAX_CONCURRENT);
} kafka_events SEC(".maps");


struct
{
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(key_size, sizeof(u32));
    __uint(value_size, sizeof(struct kafka_request_t));
    __uint(max_entries, 1);
} kafka_request_storage_map SEC(".maps");


struct {
	__uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
	__uint(key_size, sizeof(u32));
	__uint(value_size, sizeof(struct topic_name_t));
	__uint(max_entries, 1);
} kafka_topic_name SEC(".maps");


// https://github.com/segmentio/kafka-go/blob/main/protocol/record.go#L48
struct kafka_header_t {
    struct go_string key;
    struct go_slice value;
};

// Injected in init
volatile const u64 message_key_pos;
volatile const u64 message_topic_pos;
volatile const u64 message_headers_pos;
volatile const u64 message_time_pos;

volatile const u64 writer_topic_pos;

static __always_inline int  build_context_header(struct kafka_header_t *header, struct span_context *span_ctx) {
    if (header == NULL || span_ctx == NULL) {
        bpf_printk("uprobe_WriteMessages: build_contxt_header: Invalid arguments");
        return -1;
    }

    // Prepare the key string for the user
    char key[CKR_KEY_LENGTH] = "ck-route";
    void *ptr = write_target_data(key, CKR_KEY_LENGTH);
    if (ptr == NULL) {
        bpf_printk("uprobe_WriteMessages: build_contxt_header: Failed to write key to user");
        return -1;
    }

    // build the go string of the key
    header->key.str = ptr;
    header->key.len = CKR_KEY_LENGTH;

    // Prepare the value string for the user
    char val[CKR_VAL_LENGTH];
    span_context_to_ckr_string(span_ctx, val);
    ptr = write_target_data(val, sizeof(val));
    if (ptr == NULL) {
        bpf_printk("uprobe_WriteMessages: build_contxt_header: Failed to write value to user");
        return -1;
    }

    // build the go slice of the value
    header->value.array = ptr;
    header->value.len = CKR_VAL_LENGTH;
    header->value.cap = CKR_VAL_LENGTH;
    return 0;
}

static __always_inline int inject_kafka_header(void *message, struct kafka_header_t *header) {
    append_item_to_slice(header, sizeof(*header), (void *)(message + message_headers_pos));
    return 0;
}

static __always_inline u32 collect_kafka_topic(void *message, char *topic_name) {
    return get_go_string_from_user_ptr_with_len((void *)(message + message_topic_pos), topic_name, MAX_TOPIC_SIZE);
}


// This instrumentation attaches uprobe to the following function:
// func (w *Writer) WriteMessages(ctx context.Context, msgs ...Message) error
SEC("uprobe/WriteMessages")
int uprobe_WriteMessages(struct pt_regs *ctx) {
    // In Go, "..." is equivalent to passing a slice: https://go.dev/ref/spec#Passing_arguments_to_..._parameters
    bpf_debug_printk("=== uprobe_WriteMessages called with ctx: %p ===", ctx);

    void *writer = get_argument(ctx, 1);
    void *msgs_array = get_argument(ctx, 4);
    u64 msgs_array_len = (u64)get_argument(ctx, 5);

    struct go_iface go_context = {0};
    get_Go_context(ctx, 2, 0, true, &go_context);
    void *key = get_consistent_key(ctx, go_context.data);

    void *kafka_request_ptr = bpf_map_lookup_elem(&kafka_events, &key);
    if (kafka_request_ptr != NULL)
    {
        bpf_debug_printk("uprobe/WriteMessages already tracked with the current context");
        return 0;
    }

    u32 map_id = 0;
    struct kafka_request_t *kafka_request = bpf_map_lookup_elem(&kafka_request_storage_map, &map_id);
    if (kafka_request == NULL)
    {
        bpf_printk("uprobe_WriteMessages: kafka_request is NULL");
        return 0;
    }

    __builtin_memset(kafka_request, 0, sizeof(struct kafka_request_t));
    kafka_request->start_time = bpf_ktime_get_ns();

    start_span_params_t start_span_params = {
        .ctx = ctx,
        .go_context = &go_context,
        .psc = &kafka_request->psc,
        .sc = &kafka_request->sc,
        .get_parent_span_context_fn = NULL,
        .get_parent_span_context_arg = NULL,
    };
    start_span(&start_span_params);

    if (is_ck_instrumentation_enabled()) {

        struct topic_name_t *topic_name_data = bpf_map_lookup_elem(&kafka_topic_name, &map_id);
        if(topic_name_data == NULL){
            return 0;
        }
        __builtin_memset(topic_name_data, 0, sizeof(struct topic_name_t));

        // Try to get a global topic from Writer
        bool global_topic = get_go_string_from_user_ptr((void *)(writer + writer_topic_pos), kafka_request->global_topic, sizeof(kafka_request->global_topic));
        kafka_request->is_global_topic = global_topic;

        bpf_debug_printk("uprobe/WriteMessages: global_topic: %s", kafka_request->global_topic);

        void *msg_ptr = msgs_array;
        struct kafka_header_t header = {0};
        // This is hack to get the message size. This calculation is based on the following assumptions:
        // 1. "Time" is the last field in the message struct. This looks to be correct for all the versions according to
        //      https://github.com/segmentio/kafka-go/blob/v0.2.3/message.go#L24C2-L24C6
        // 2. the time.Time struct is 24 bytes. This looks to be correct for all the reasanobaly latest versions according to
        //      https://github.com/golang/go/blame/master/src/time/time.go#L135
        // In the future if more libraries will need to get structs sizes we probably want to have similar
        // mechanism to the one we have for the offsets
        u16 msg_size = message_time_pos + 8 + 8 + 8;
        kafka_request->valid_messages = 0;

        if (build_context_header(&header, &kafka_request->psc) != 0) {
            bpf_printk("uprobe_WriteMessages: Failed to build header");
            return 0;
        }

        u32 global_topic_length = 0;
        bool is_all_msgs_same_topic = true;
        // Iterate over the messages
        for (u64 i = 0; i < MAX_BATCH_SIZE; i++) {
            if (i >= msgs_array_len) {
                break;
            }
            //If the topic is not set at a Producer level, we will check if all the messages have the same topics.
            //If they do, then the topic will be considered as a global topics. If they don't, then we will set a 
            //default CK_UNKNOWN_TOPIC topic.
            if(!global_topic){
                if(i == 0){
                    global_topic_length = collect_kafka_topic(msg_ptr, kafka_request->global_topic);
                } else {
                    u32 topic_length = collect_kafka_topic(msg_ptr, topic_name_data->topic);
                    if(topic_length != global_topic_length || !bpf_memcmp(kafka_request->global_topic, topic_name_data->topic, topic_length)){
                        is_all_msgs_same_topic = false;
                    }
                }
            }
                
            // Inject the header
            inject_kafka_header(msg_ptr, &header);
            bpf_debug_printk("uprobe/WriteMessages: injected header");
            kafka_request->valid_messages++;
            msg_ptr = msg_ptr + msg_size;
        }

        kafka_request->is_global_topic = is_all_msgs_same_topic;
    }

    bpf_debug_printk("uprobe_WriteMessages: update map : kafka_events , key: context %p, value type: kafka_request_t", key );
    long res = bpf_map_update_elem(&kafka_events, &key, kafka_request, 0);
    if (res != 0) {
        bpf_printk("uprobe_WriteMessages: failed to update map : kafka_events , key: context %p, value type: kafka_request_t, error: %ld", key, res);
    }
    // don't need to start tracking the span, as we don't have a context to propagate locally
    return 0;
}

// This instrumentation attaches uprobe to the following function:
// func (w *Writer) WriteMessages(ctx context.Context, msgs ...Message) error
SEC("uprobe/WriteMessages")
int uprobe_WriteMessages_Returns(struct pt_regs *ctx) {
    
    bpf_debug_printk("=== uprobe_WriteMessages_Returns called with ctx: %p ===", ctx);

    u64 end_time = bpf_ktime_get_ns();
    struct go_iface go_context = {0};
    get_Go_context(ctx, 2, 0, true, &go_context);
    void *key = get_consistent_key(ctx, go_context.data);
    bpf_debug_printk("uprobe_WriteMessages_Returns: consistent_key: %p", key); 

    struct kafka_request_t *kafka_request = bpf_map_lookup_elem(&kafka_events, &key);
    if (kafka_request == NULL) {
        bpf_printk("uprobe_WriteMessages_Returns: kafka_request is null\n");
        return 0;
    }
    
    //Check if there is an error in the response. This error is returned if any of the messagse have an error in them.
    //However, do not not try to find how many messages have error and how many don't as its quite complex and will also
    //eat a lot of instructions.
    if (is_register_abi()) {
        void *error_response = get_argument(ctx, 1);
        if (error_response != NULL) {
            kafka_request->isError = true;
        }
    }

    kafka_request->end_time = end_time;

    output_span_event(ctx, kafka_request, sizeof(*kafka_request), &kafka_request->sc);
    bpf_debug_printk("uprobe_WriteMessages_Returns: delete map : kafka_events , key - consistent_key: %p, value type: kafka_request_t", key );
    long res = bpf_map_delete_elem(&kafka_events, &key);
    if (res != 0) {
        bpf_printk("uprobe_WriteMessages_Returns: failed to delete map : kafka_events , key - consistent_key: %p, value type: kafka_request_t, error: %ld", key, res);
    }
    // don't need to stop tracking the span, as we don't have a context to propagate locally
    return 0;
}
