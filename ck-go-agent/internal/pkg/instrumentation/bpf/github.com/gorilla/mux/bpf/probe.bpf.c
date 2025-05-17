// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

#include "arguments.h"
#include "go_context.h"
#include "go_types.h"
#include "uprobe.h"
#include "mux.h"
#include "trace/span_output.h"

char __license[] SEC("license") = "Dual MIT/GPL";

volatile const u64 routematch_route_pos;
volatile const u64 route_routeconf_pos;
volatile const u64 routeconf_regexp_pos;
volatile const u64 routeregexpgroup_path_pos;
volatile const u64 routeregexp_template_pos;
volatile const u64 routeregexp_reverse_pos;
volatile const u64 ctx_ptr_pos; //http request context ptr position

// Attaches to: func (r *Route) Match(req *http.Request, match *RouteMatch) bool
SEC("uprobe/Route_Match")
int uprobe_Route_Match(struct pt_regs *ctx)
{
    bpf_debug_printk("=== uprobe_Route_Match called with ctx: %p ===", ctx);
    
    // void *route_ptr = get_argument(ctx, 1);  // r *Route
    // void *req_ptr = get_argument(ctx, 2);    // req *http.Request
    // void *match_ptr = get_argument(ctx, 3);  // match *RouteMatch

    // void *key = get_consistent_key(ctx, NULL);
    // bpf_debug_printk("Route_Match: route_ptr: 1=%p req_ptr: 2=%p match_ptr: 3=%p key: %p", route_ptr, req_ptr, match_ptr, key);
    
    return 0;
}

// Return probe for Route.Match
//func (r *Route) Match(req *http.Request, match *RouteMatch) bool
SEC("uprobe/Route_Match")
int uprobe_Route_Match_Returns(struct pt_regs *ctx)
{
    bpf_debug_printk("=== uprobe_Route_Match_Returns called with ctx: %p ===", ctx);

    //These are the arguments passed to the function and are now only in stack
    void *req_ptr = get_argument_by_stack(ctx, 2);    // req *http.Request
    void *route_match_ptr = get_argument_by_stack(ctx, 3);  // match *RouteMatch
    if (route_match_ptr == NULL) {
        bpf_debug_printk("Route_Match_Returns: match_ptr is NULL");
        return 0;
    }
    //RouteMatch -> Route -> RouteConf-> regexp(routeRegexGroup) -> path (routeRegexP)
    // Get the Route field from RouteMatch using the offset
    void *route_ptr = 0;
    bpf_probe_read(&route_ptr, sizeof(void *), (void *)(route_match_ptr + routematch_route_pos));
    if (route_ptr == NULL) {
        bpf_debug_printk("Route_Match_Returns: routeptr  is NULL");
        return 0;
    }

    //Get the RouteConf field from Route using the offset
    void *path_ptr = 0;
    bpf_probe_read(&path_ptr, sizeof(path_ptr), (void *)(route_ptr +  route_routeconf_pos+routeconf_regexp_pos+routeregexpgroup_path_pos));
    if (path_ptr == NULL) {
        bpf_debug_printk("Route_Match_Returns: pathptr  is NULL");
        return 0;
    }

    // Get the template field from Route using the offset
    struct mux_route_match_t mux_route_match = {0};
    u32 mux_path_len = get_go_string_from_user_ptr_with_len((void*) (path_ptr +routeregexp_template_pos), mux_route_match.path, sizeof(mux_route_match.path));
    bpf_debug_printk("Route_Match_Returns: mux_path=%s, mux_path_len=%d", mux_route_match.path, mux_path_len );

    //get_Go_context_from_struct
    struct go_iface go_context = {0};
    get_Go_context_from_struct(req_ptr, ctx_ptr_pos, &go_context);
    void *key = get_consistent_key(ctx, go_context.data);
    bpf_debug_printk("Route_Match_Returns: go_context.data: %p, consistent_key: %p", go_context.data, key);
    if (key == NULL) {
        bpf_debug_printk("Route_Match_Returns: failed to get consistent key");
        return 0;
    }

    if (mux_path_len > 0) {
        mux_route_match.has_match = true;
        mux_route_match.path_len = mux_path_len;
        bpf_debug_printk("Route_Match_Returns: update map : mux_route_match_map , key - consistent_key: %p, value type: mux_route_match_t", key );
        update_mux_route_match_map(key, &mux_route_match);
    }else{
        bpf_debug_printk("Route_Match_Returns: mux_path_len is 0");
    }

    struct mux_route_match_t *mux_route_match2 = get_mux_route_match(key);
    if (mux_route_match2 == NULL) {
        bpf_debug_printk("Route_Match_Returns: mux_route_match is NULL");
        return 0;
    }
    bpf_debug_printk("Route_Match_Returns: mux_route_match after writing: %s", mux_route_match2->path);

    return 0;
}