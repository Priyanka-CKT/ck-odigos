# Frontend UI Architecture Summary

## Overview

The Odigos frontend is a GraphQL-based web application that provides a user interface for managing observability instrumentation in Kubernetes clusters. It follows a modern architecture with React/TypeScript frontend, GraphQL API layer, and direct Kubernetes integration.

## Architecture Components

### 1. **Frontend Layer (Web UI)**
- **Technology**: React/TypeScript (assumed based on modern web practices)
- **Communication**: GraphQL queries and mutations
- **State Management**: Apollo Client for GraphQL state management
- **Responsibilities**:
  - User interface for source management
  - Destination configuration
  - Instrumentation rule management
  - Real-time status monitoring

### 2. **GraphQL API Layer**
- **Framework**: gqlgen (Go GraphQL library)
- **Files**: 
  - `graph/schema.graphqls` - GraphQL schema definition
  - `graph/schema.resolvers.go` - Resolver implementations
  - `graph/generated.go` - Auto-generated GraphQL code
- **Responsibilities**:
  - API contract definition
  - Request validation and processing
  - Business logic orchestration

### 3. **Services Layer**
- **Files**:
  - `services/sources.go` - Source management
  - `services/namespaces.go` - Namespace operations
  - `services/instrumentationrule.go` - Rule management
  - `services/utils.go` - Utility functions
- **Responsibilities**:
  - Business logic implementation
  - Data transformation
  - Kubernetes API interactions

### 4. **Kubernetes Integration Layer**
- **Files**:
  - `kube/client.go` - Kubernetes client wrapper
- **Responsibilities**:
  - Kubernetes API communication
  - Resource management
  - Authentication and authorization

## Key Operations Flow

### 1. GetComputePlatform Operation

```mermaid
sequenceDiagram
    participant UI as Frontend UI
    participant GQL as GraphQL API
    participant Resolver as ComputePlatform Resolver
    participant Services as Services Layer
    participant K8s as Kubernetes API

    UI->>GQL: query { computePlatform { ... } }
    GQL->>Resolver: ComputePlatform()
    Resolver->>Resolver: Return static K8S platform type
    
    Note over UI,K8s: Nested field resolution
    
    UI->>GQL: k8sActualNamespaces
    GQL->>Resolver: K8sActualNamespaces()
    Resolver->>Services: GetK8SNamespaces(ctx)
    Services->>K8s: List namespaces
    K8s-->>Services: Namespace list
    Services->>K8s: Get instrumentation labels
    K8s-->>Services: Label values
    Services-->>Resolver: Processed namespaces
    Resolver-->>GQL: K8sActualNamespace[]
    GQL-->>UI: Platform data with namespaces
```

### 2. UpdateK8sActualSource Operation

```mermaid
sequenceDiagram
    participant UI as Frontend UI
    participant GQL as GraphQL API
    participant Resolver as Mutation Resolver
    participant Services as Services Layer
    participant K8s as Kubernetes API
    participant Controller as Instrumentor Controller

    UI->>GQL: mutation updateK8sActualSource(sourceId, patchRequest)
    GQL->>Resolver: UpdateK8sActualSource(ctx, sourceId, patchRequest)
    
    alt ReportedName Update
        Resolver->>Services: UpdateReportedName(ctx, ns, kind, name, reportedName)
        Services->>K8s: Get workload object
        K8s-->>Services: Workload object
        Services->>Services: updateAnnotations(annotations, reportedName)
        Services->>K8s: Update workload with odigos.io/reported-name
        K8s-->>Services: Success/Error
        
        Note over Controller: Async controller processing
        Controller->>K8s: Watch annotation changes
        Controller->>Controller: Update InstrumentationConfig
        Controller->>K8s: Update service name in config
    end
    
    Services-->>Resolver: Success/Error
    Resolver-->>GQL: Boolean response
    GQL-->>UI: Mutation result
```

### 3. PersistK8sSources Operation

```mermaid
sequenceDiagram
    participant UI as Frontend UI
    participant GQL as GraphQL API
    participant Resolver as Mutation Resolver
    participant Services as Services Layer
    participant K8s as Kubernetes API
    participant Controller as Instrumentor Controller

    UI->>GQL: mutation persistK8sSources(namespace, sources[])
    GQL->>Resolver: PersistK8sSources(ctx, namespace, sources)
    Resolver->>Services: SyncWorkloadsInNamespace(ctx, namespace, sources)
    
    Note over Services: Concurrent processing with errgroup
    
    loop For each workload
        Services->>Services: setWorkloadInstrumentationLabel(ctx, ns, name, kind, selected)
        Services->>K8s: Patch workload with odigos.io/instrumentation label
        K8s-->>Services: Success/Error
    end
    
    Note over Controller: Async controller chain reaction
    Controller->>K8s: Watch label changes
    Controller->>Controller: Start language detection
    Controller->>Controller: Apply instrumentation devices
    Controller->>K8s: Create InstrumentationConfig
    Controller->>K8s: Create InstrumentedApplication
    
    Services-->>Resolver: Aggregated success/error
    Resolver-->>GQL: Boolean response
    GQL-->>UI: Mutation result
```

## Error Handling and Failover Scenarios

### 1. **Kubernetes API Failures**

```mermaid
sequenceDiagram
    participant UI as Frontend UI
    participant GQL as GraphQL API
    participant Services as Services Layer
    participant K8s as Kubernetes API

    UI->>GQL: mutation persistK8sSources(...)
    GQL->>Services: SyncWorkloadsInNamespace(...)
    
    Services->>K8s: Patch workload 1
    K8s-->>Services: Success
    
    Services->>K8s: Patch workload 2
    K8s-->>Services: Error (API timeout)
    
    Services->>K8s: Patch workload 3
    K8s-->>Services: Success
    
    Note over Services: errgroup collects all errors
    Services-->>GQL: Partial failure with error details
    GQL-->>UI: Error response with failed workloads
    
    Note over UI: UI shows partial success state
    UI->>UI: Display retry option for failed workloads
```

**Failover Strategy:**
- **Concurrent Processing**: Uses `errgroup` to process workloads in parallel
- **Partial Success Handling**: Continues processing even if some workloads fail
- **Error Aggregation**: Collects and reports all errors to the user
- **Retry Mechanism**: UI can retry failed operations

### 2. **GraphQL Layer Failures**

```mermaid
sequenceDiagram
    participant UI as Frontend UI
    participant GQL as GraphQL API
    participant Services as Services Layer

    UI->>GQL: query computePlatform
    GQL->>Services: GetK8SNamespaces()
    Services-->>GQL: Error (service unavailable)
    
    alt GraphQL Error Handling
        GQL->>GQL: Log error details
        GQL-->>UI: GraphQL error response
        UI->>UI: Show error message
        UI->>UI: Disable affected UI components
        
        Note over UI: Retry with exponential backoff
        UI->>GQL: Retry query (after delay)
        GQL->>Services: GetK8SNamespaces()
        Services-->>GQL: Success
        GQL-->>UI: Data response
        UI->>UI: Re-enable UI components
    end
```

**Failover Strategy:**
- **Error Boundaries**: GraphQL errors are caught and handled gracefully
- **Graceful Degradation**: UI components disable when data is unavailable
- **Automatic Retry**: Exponential backoff retry mechanism
- **User Feedback**: Clear error messages and loading states

### 3. **Controller Synchronization Failures**

```mermaid
sequenceDiagram
    participant UI as Frontend UI
    participant GQL as GraphQL API
    participant K8s as Kubernetes API
    participant Controller as Instrumentor Controller

    UI->>GQL: mutation persistK8sSources(...)
    GQL->>K8s: Update workload labels
    K8s-->>GQL: Success
    GQL-->>UI: Success response
    
    Note over UI: User sees success, but...
    
    Controller->>K8s: Watch for label changes
    Controller->>Controller: Process instrumentation
    Controller-->>Controller: Error (resource conflict)
    
    Note over Controller: Controller retry mechanism
    Controller->>Controller: Retry with backoff
    Controller->>K8s: Retry instrumentation
    K8s-->>Controller: Success
    
    Note over UI: UI polling for status updates
    UI->>GQL: query instrumentationStatus
    GQL->>K8s: Get InstrumentedApplication status
    K8s-->>GQL: Status data
    GQL-->>UI: Updated status
    UI->>UI: Show instrumentation progress
```

**Failover Strategy:**
- **Eventual Consistency**: Controllers use Kubernetes retry mechanisms
- **Status Polling**: UI polls for status updates to show progress
- **Reconciliation**: Controllers continuously reconcile desired vs actual state
- **Health Monitoring**: UI shows instrumentation health and progress

### 4. **Network Connectivity Issues**

```mermaid
sequenceDiagram
    participant UI as Frontend UI
    participant GQL as GraphQL API
    participant K8s as Kubernetes API

    UI->>GQL: mutation updateK8sActualSource(...)
    
    Note over GQL,K8s: Network partition
    GQL->>K8s: Update workload annotation
    K8s-->>GQL: Timeout/Connection refused
    
    GQL->>GQL: Retry with exponential backoff
    GQL->>K8s: Retry update
    K8s-->>GQL: Timeout
    
    GQL->>GQL: Max retries exceeded
    GQL-->>UI: Error: "Unable to connect to Kubernetes API"
    
    UI->>UI: Show offline mode indicator
    UI->>UI: Cache failed operations
    
    Note over GQL,K8s: Network restored
    UI->>GQL: Retry cached operations
    GQL->>K8s: Update workload annotation
    K8s-->>GQL: Success
    GQL-->>UI: Success
    UI->>UI: Clear offline indicator
```

**Failover Strategy:**
- **Connection Pooling**: Kubernetes client uses connection pooling
- **Retry Logic**: Exponential backoff with jitter
- **Offline Mode**: UI caches operations when backend is unavailable
- **Health Checks**: Regular health checks to detect connectivity issues

## Performance Optimizations

### 1. **Concurrent Processing**
```go
// Example from services/namespaces.go
func SyncWorkloadsInNamespace(ctx context.Context, nsName string, workloads []model.PersistNamespaceSourceInput) error {
    g, ctx := errgroup.WithContext(ctx)
    g.SetLimit(kube.K8sClientDefaultBurst) // Limit: 100 concurrent operations
    
    for _, workload := range workloads {
        currWorkload := workload
        g.Go(func() error {
            return setWorkloadInstrumentationLabel(ctx, nsName, currWorkload.Name, WorkloadKind(currWorkload.Kind.String()), currWorkload.Selected)
        })
    }
    return g.Wait()
}
```

### 2. **Kubernetes Client Configuration**
```go
// From kube/client.go
const (
    K8sClientDefaultQPS   = 100  // Queries per second
    K8sClientDefaultBurst = 100  // Burst capacity
)
```

### 3. **GraphQL Field Resolution**
- **Lazy Loading**: Fields are resolved only when requested
- **Batching**: Multiple operations can be batched in a single request
- **Caching**: Apollo Client provides automatic caching

## Security Considerations

### 1. **Authentication & Authorization**
- **RBAC**: Kubernetes RBAC controls API access
- **Service Account**: Frontend uses Kubernetes service account
- **Context Isolation**: Operations are scoped to accessible namespaces

### 2. **Input Validation**
- **GraphQL Schema**: Enforces type safety and required fields
- **Business Logic**: Services layer validates business rules
- **Kubernetes Validation**: API server validates resource specifications

### 3. **Error Information Disclosure**
- **Sanitized Errors**: Internal errors are logged but not exposed to UI
- **User-Friendly Messages**: Clear, actionable error messages
- **Audit Logging**: All operations are logged for security auditing

## Monitoring and Observability

### 1. **Health Checks**
- **Kubernetes API**: Regular health checks to Kubernetes API
- **GraphQL Endpoint**: Health check endpoint for load balancers
- **Controller Status**: Monitor controller reconciliation loops

### 2. **Metrics**
- **Operation Latency**: Track GraphQL operation response times
- **Error Rates**: Monitor error rates by operation type
- **Resource Usage**: Track memory and CPU usage

### 3. **Logging**
- **Structured Logging**: JSON-formatted logs with correlation IDs
- **Error Tracking**: Detailed error logs with stack traces
- **Audit Trail**: Log all user operations for compliance

## Deployment Architecture

```mermaid
graph TB
    subgraph "Kubernetes Cluster"
        subgraph "Odigos Namespace"
            UI[Frontend UI Pod]
            API[GraphQL API Pod]
            Controllers[Instrumentor Controllers]
        end
        
        subgraph "Application Namespaces"
            Apps[Application Workloads]
            IA[InstrumentedApplications]
            IC[InstrumentationConfigs]
        end
        
        subgraph "Kubernetes API"
            APIServer[API Server]
            ETCD[etcd]
        end
    end
    
    UI --> API
    API --> APIServer
    Controllers --> APIServer
    APIServer --> ETCD
    Controllers --> IA
    Controllers --> IC
    Controllers --> Apps
```

## Best Practices

### 1. **Error Handling**
- Always provide meaningful error messages to users
- Implement retry logic with exponential backoff
- Use circuit breakers for external dependencies
- Log errors with sufficient context for debugging

### 2. **Performance**
- Use concurrent processing for bulk operations
- Implement proper pagination for large datasets
- Cache frequently accessed data
- Monitor and optimize slow GraphQL queries

### 3. **User Experience**
- Provide immediate feedback for user actions
- Show loading states during operations
- Implement optimistic updates where appropriate
- Gracefully handle partial failures

### 4. **Reliability**
- Design for eventual consistency
- Implement idempotent operations
- Use health checks and readiness probes
- Plan for graceful degradation scenarios

This architecture provides a robust, scalable, and user-friendly interface for managing Kubernetes observability instrumentation while handling various failure scenarios gracefully. 