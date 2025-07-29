# Predicate Filtering & Pod Generation Logic - Summary

## Quick Overview

**Predicate Filtering** and **Pod Generation Logic** in Odigos work together to efficiently manage runtime detection across Kubernetes clusters while preventing duplicate work and optimizing resource usage.

## Key Concepts

### 🔄 Pod Generation Logic
- **Purpose**: Track workload updates to avoid redundant runtime detection
- **Mechanism**: Each pod has a generation number that increases with workload updates
- **Storage**: InstrumentationConfig stores the last processed generation (`ObservedWorkloadGeneration`)
- **Decision**: Only process pods with generation > observed generation

### 🚪 Predicate Filtering
- **Purpose**: Filter Kubernetes events to only process relevant ones
- **Mechanism**: Event filters that act as gatekeepers before controller reconciliation
- **Benefits**: Reduces CPU/memory usage, prevents unnecessary processing, improves scalability

## Dual Controller Generation Checks

### Why Two Controllers Check Pod Generation?

Both **PodsController** and **InstrumentationConfigReconciler** check pod generation, but they serve different purposes:

| Controller | Trigger | Frequency | Purpose |
|------------|---------|-----------|---------|
| **PodsController** | Pod becomes ready | High (every pod event) | Handle workload updates & new pods |
| **InstrumentationConfigReconciler** | New empty InstrumentationConfig | Low (config creation only) | Handle initial workload deployment |

### Controller Behavior Comparison

#### PodsController
- **Watches**: `corev1.Pod` events
- **Predicate**: `AllContainersReadyPredicate` (only ready pods)
- **Generation Check**: Every pod ready event
- **Processing**: Single pod (the one that triggered)
- **Use Case**: Workload updates, pod restarts, odiglet restarts

#### InstrumentationConfigReconciler  
- **Watches**: `odigosv1.InstrumentationConfig` events
- **Predicate**: `instrumentationConfigPredicate` (only empty configs)
- **Generation Check**: Only for new/empty configs
- **Processing**: All pods (selects highest generation)
- **Use Case**: New workload deployments, initial setup

### Real-World Scenarios

#### Scenario 1: New Workload
```
1. Workload deployed → InstrumentationConfig created (empty)
2. InstrumentationConfigReconciler → Processes all pods → Detects runtime
3. PodsController → Sees pods already processed → Skips
```

#### Scenario 2: Workload Update
```
1. Deployment updated → New generation (N)
2. PodsController → Detects generation N > observed → Processes
3. InstrumentationConfigReconciler → Doesn't trigger (config has data)
```

#### Scenario 3: Odiglet Restart
```
1. Controllers restart → Cache warm-up
2. PodsController → Checks all pods → Skips (generations already processed)
3. InstrumentationConfigReconciler → Only processes truly empty configs
```

### Why This Design Works

✅ **Comprehensive Coverage**: All scenarios handled by at least one controller
✅ **No Duplication**: Generation checks prevent redundant work
✅ **High Efficiency**: Predicates filter out unnecessary events
✅ **Fault Tolerance**: Multiple controllers provide backup coverage
✅ **Performance**: Generation checks are lightweight (~1ms vs ~100-1000ms for detection)

## Core Predicates in Odigos

| Predicate | Purpose | Trigger Condition |
|-----------|---------|-------------------|
| `instrumentationConfigPredicate` | Process new InstrumentationConfigs | Empty runtime details |
| `AllContainersReadyPredicate` | Runtime detection on ready pods | All containers ready |
| `WorkloadEnabledPredicate` | Language detection on enabled workloads | Labeled + has replicas |
| `CgBecomesReadyPredicate` | React to collector readiness | Becomes ready |

## Cross-Node Coordination

### Problem
Multiple nodes might try to perform runtime detection for the same workload, causing:
- Duplicate work
- Resource waste
- Race conditions

### Solution
1. **Generation Comparison**: Check if pod generation > observed generation
2. **Atomic Updates**: Update InstrumentationConfig atomically with results + generation
3. **Predicate Filtering**: Only process InstrumentationConfigs with empty runtime details

### Flow
```
Node A: Detects new generation → Performs detection → Updates config
Node B: Sees updated config → Skips detection (generation already processed)
```

## Key Benefits

### Performance
- ✅ Reduces unnecessary reconciliation cycles
- ✅ Minimizes CPU and memory usage
- ✅ Prevents redundant API calls

### Reliability
- ✅ Prevents duplicate processing across nodes
- ✅ Handles race conditions gracefully
- ✅ Ensures eventual consistency

### Scalability
- ✅ Works efficiently in large clusters
- ✅ Reduces load on Kubernetes API server
- ✅ Scales with number of nodes

## Implementation Pattern

```go
// 1. Setup controller with predicate
err = builder.
    ControllerManagedBy(mgr).
    For(&corev1.Pod{}).
    WithEventFilter(&AllContainersReadyPredicate{}).
    Complete(&PodsReconciler{...})

// 2. Check generation in reconciler
podGeneration := GetPodGeneration(ctx, clientset, pod)
if podGeneration <= observedGeneration {
    return // Skip processing
}

// 3. Perform work and update atomically
results := performRuntimeDetection(pod)
updateInstrumentationConfig(results, podGeneration)
```

## Real-World Example

**Scenario**: Deployment with 3 replicas updated across 2 nodes

1. **Deployment Update**: New generation (N) created
2. **Node 1**: Detects pod with generation N, performs runtime detection, updates InstrumentationConfig
3. **Node 2**: Detects pod with generation N, sees InstrumentationConfig already has generation N, skips detection
4. **Result**: Runtime detection happens only once, not duplicated across nodes

## Best Practices

### Predicate Design
- Be specific about which events to process
- Keep predicate logic fast and simple
- Handle edge cases (nil objects, type assertions)

### Generation Handling
- Always compare generations before processing
- Update generation and results atomically
- Handle cases where generation can't be determined

### Error Handling
- Implement graceful degradation
- Use appropriate retry mechanisms
- Provide detailed logging for debugging

## Conclusion

This system demonstrates how to build efficient, distributed Kubernetes controllers that:
- Avoid duplicate work across nodes
- Use resources efficiently
- Scale well in large clusters
- Handle concurrent operations safely

The combination of predicate filtering and generation tracking provides a robust foundation for managing distributed operations in Kubernetes environments. 