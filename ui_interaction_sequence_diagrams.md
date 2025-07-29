# UI Interaction Sequence Diagrams

## Complete User Workflows with UI State Management

### 1. **Source Discovery and Selection Workflow**

```mermaid
sequenceDiagram
    participant User as User
    participant UI as Frontend UI
    participant Apollo as Apollo Client
    participant GQL as GraphQL API
    participant Services as Services Layer
    participant K8s as Kubernetes API

    User->>UI: Navigate to Sources page
    UI->>UI: Show loading spinner
    UI->>Apollo: query { computePlatform { k8sActualNamespaces } }
    Apollo->>GQL: Execute query
    GQL->>Services: GetK8SNamespaces(ctx)
    Services->>K8s: List namespaces with labels
    K8s-->>Services: Namespace data
    Services-->>GQL: Processed namespace list
    GQL-->>Apollo: GraphQL response
    Apollo->>Apollo: Cache response
    Apollo-->>UI: Namespace data
    UI->>UI: Render namespace list
    UI->>UI: Hide loading spinner

    User->>UI: Click on namespace "production"
    UI->>UI: Show namespace details loading
    UI->>Apollo: query { k8sActualNamespace(name: "production") { k8sActualSources } }
    Apollo->>GQL: Execute nested query
    GQL->>Services: GetWorkloadsInNamespace(ctx, "production", nil)
    Services->>K8s: List deployments, statefulsets, daemonsets
    K8s-->>Services: Workload objects
    Services->>Services: Process instrumentation status
    Services-->>GQL: Workload list with status
    GQL-->>Apollo: Source data
    Apollo-->>UI: Workload sources
    UI->>UI: Render source list with checkboxes
    UI->>UI: Show instrumentation status indicators
```

### 2. **Bulk Source Instrumentation Workflow**

```mermaid
sequenceDiagram
    participant User as User
    participant UI as Frontend UI
    participant Apollo as Apollo Client
    participant GQL as GraphQL API
    participant Services as Services Layer
    participant K8s as Kubernetes API
    participant Controller as Instrumentor Controller

    User->>UI: Select multiple sources (checkboxes)
    UI->>UI: Update local state (selected sources)
    UI->>UI: Enable "Apply Instrumentation" button

    User->>UI: Click "Apply Instrumentation"
    UI->>UI: Show confirmation dialog
    User->>UI: Confirm action
    UI->>UI: Show progress indicator
    UI->>UI: Disable form controls

    UI->>Apollo: mutation persistK8sSources(namespace, sources)
    Apollo->>GQL: Execute mutation
    GQL->>Services: SyncWorkloadsInNamespace(ctx, namespace, sources)
    
    Note over Services: Concurrent processing begins
    
    loop For each selected workload
        Services->>K8s: Patch workload with instrumentation label
        K8s-->>Services: Success/Error response
    end
    
    Services-->>GQL: Aggregated results
    GQL-->>Apollo: Mutation response
    Apollo-->>UI: Success/Error with details

    alt Success Case
        UI->>UI: Show success message
        UI->>UI: Update source status to "Instrumenting"
        UI->>UI: Start polling for status updates
        
        loop Status Polling
            UI->>Apollo: query { instrumentationStatus }
            Apollo->>GQL: Status query
            GQL->>K8s: Get InstrumentedApplication status
            K8s-->>GQL: Current status
            GQL-->>Apollo: Status data
            Apollo-->>UI: Updated status
            UI->>UI: Update progress indicators
        end
        
        Note over Controller: Async instrumentation process
        Controller->>K8s: Detect label changes
        Controller->>Controller: Start language detection
        Controller->>Controller: Apply instrumentation devices
        Controller->>K8s: Create InstrumentationConfig
        
        UI->>UI: Status changes to "Instrumented"
        UI->>UI: Stop polling
        UI->>UI: Show final success state
    else Error Case
        UI->>UI: Show error message with details
        UI->>UI: Highlight failed sources
        UI->>UI: Show retry button
        UI->>UI: Re-enable form controls
    end
```

### 3. **Individual Source Configuration Workflow**

```mermaid
sequenceDiagram
    participant User as User
    participant UI as Frontend UI
    participant Modal as Configuration Modal
    participant Apollo as Apollo Client
    participant GQL as GraphQL API
    participant Services as Services Layer
    participant K8s as Kubernetes API

    User->>UI: Click "Configure" on source row
    UI->>Modal: Open configuration modal
    Modal->>Modal: Load current configuration
    Modal->>Apollo: query { k8sActualSource(namespace, kind, name) }
    Apollo->>GQL: Execute query
    GQL->>Services: GetSourceDetails(ctx, namespace, kind, name)
    Services->>K8s: Get workload and annotations
    K8s-->>Services: Workload data with reported name
    Services-->>GQL: Source configuration
    GQL-->>Apollo: Configuration data
    Apollo-->>Modal: Current settings
    Modal->>Modal: Populate form fields

    User->>Modal: Update "Reported Name" field
    Modal->>Modal: Validate input (real-time)
    Modal->>Modal: Enable "Save" button

    User->>Modal: Click "Save"
    Modal->>Modal: Show saving indicator
    Modal->>Apollo: mutation updateK8sActualSource(sourceId, patchRequest)
    Apollo->>GQL: Execute mutation
    GQL->>Services: UpdateReportedName(ctx, ns, kind, name, reportedName)
    Services->>K8s: Get workload object
    K8s-->>Services: Workload object
    Services->>Services: Update annotations
    Services->>K8s: Update workload with new annotation
    K8s-->>Services: Success/Error
    Services-->>GQL: Operation result
    GQL-->>Apollo: Mutation response
    Apollo-->>Modal: Success/Error

    alt Success Case
        Modal->>Modal: Show success message
        Modal->>Modal: Update form with new values
        Modal->>UI: Trigger cache update
        UI->>UI: Update source list display
        
        Note over K8s: Controller detects annotation change
        Note over K8s: InstrumentationConfig updated with new service name
        
        Modal->>Modal: Auto-close after delay
    else Error Case
        Modal->>Modal: Show error message
        Modal->>Modal: Keep modal open
        Modal->>Modal: Re-enable form controls
    end
```

### 4. **Real-time Status Updates Workflow**

```mermaid
sequenceDiagram
    participant UI as Frontend UI
    participant Apollo as Apollo Client
    participant GQL as GraphQL API
    participant K8s as Kubernetes API
    participant Controller as Instrumentor Controller

    Note over UI: User is viewing sources page
    
    UI->>UI: Start status polling timer (every 5 seconds)
    
    loop Status Polling Loop
        UI->>Apollo: query { computePlatform { k8sActualSources } }
        Apollo->>Apollo: Check cache freshness
        
        alt Cache is fresh
            Apollo-->>UI: Cached data
        else Cache is stale
            Apollo->>GQL: Execute query
            GQL->>K8s: Get InstrumentedApplications with conditions
            K8s-->>GQL: Current instrumentation status
            GQL-->>Apollo: Updated source data
            Apollo->>Apollo: Update cache
            Apollo-->>UI: Fresh data
        end
        
        UI->>UI: Compare with previous state
        
        alt Status Changed
            UI->>UI: Update status indicators
            UI->>UI: Show notification (if significant change)
            UI->>UI: Update progress bars
        else No Change
            UI->>UI: No visual updates needed
        end
    end

    Note over Controller: Background controller activity
    Controller->>K8s: Reconcile instrumentation
    Controller->>K8s: Update InstrumentedApplication status
    
    Note over UI: Next polling cycle picks up changes
    UI->>Apollo: query { computePlatform { k8sActualSources } }
    Apollo->>GQL: Execute query
    GQL->>K8s: Get updated status
    K8s-->>GQL: New status (e.g., "Healthy" -> "Error")
    GQL-->>Apollo: Updated data
    Apollo-->>UI: Status change detected
    UI->>UI: Show error indicator
    UI->>UI: Display error details in tooltip
```

### 5. **Error Recovery and Retry Workflow**

```mermaid
sequenceDiagram
    participant User as User
    participant UI as Frontend UI
    participant Apollo as Apollo Client
    participant GQL as GraphQL API
    participant K8s as Kubernetes API

    User->>UI: Attempt to instrument sources
    UI->>Apollo: mutation persistK8sSources(...)
    Apollo->>GQL: Execute mutation
    GQL->>K8s: Patch workload labels
    K8s-->>GQL: Error (API server unavailable)
    GQL-->>Apollo: GraphQL error
    Apollo-->>UI: Error response

    UI->>UI: Show error notification
    UI->>UI: Display "Retry" button
    UI->>UI: Log error details
    UI->>UI: Keep failed operations in local state

    User->>UI: Click "Retry"
    UI->>UI: Show retry attempt indicator
    UI->>Apollo: Retry mutation with exponential backoff
    Apollo->>GQL: Execute mutation (retry #1)
    GQL->>K8s: Patch workload labels
    K8s-->>GQL: Error (still unavailable)
    GQL-->>Apollo: Error response
    Apollo-->>UI: Retry failed

    UI->>UI: Increment retry counter
    UI->>UI: Show "Retry in 2 seconds..." message
    UI->>UI: Wait with countdown timer

    UI->>Apollo: Retry mutation (retry #2)
    Apollo->>GQL: Execute mutation
    GQL->>K8s: Patch workload labels
    K8s-->>GQL: Success (API server recovered)
    GQL-->>Apollo: Success response
    Apollo-->>UI: Operation succeeded

    UI->>UI: Clear error state
    UI->>UI: Show success message
    UI->>UI: Update source status
    UI->>UI: Clear retry state
    UI->>UI: Resume normal polling
```

### 6. **Offline Mode and Connectivity Recovery**

```mermaid
sequenceDiagram
    participant User as User
    participant UI as Frontend UI
    participant Apollo as Apollo Client
    participant GQL as GraphQL API
    participant ServiceWorker as Service Worker

    Note over UI: Normal operation
    UI->>Apollo: query { computePlatform }
    Apollo->>GQL: Execute query
    GQL-->>Apollo: Success response
    Apollo-->>UI: Data received

    Note over GQL: Network connectivity lost
    UI->>Apollo: query { computePlatform } (next poll)
    Apollo->>GQL: Execute query
    GQL-->>Apollo: Network error
    Apollo-->>UI: Error response

    UI->>UI: Detect network error pattern
    UI->>UI: Enter offline mode
    UI->>UI: Show offline indicator
    UI->>UI: Disable mutation buttons
    UI->>ServiceWorker: Cache current state
    ServiceWorker-->>UI: State cached

    User->>UI: Attempt to instrument source
    UI->>UI: Show "Offline - operation queued" message
    UI->>UI: Add operation to pending queue
    UI->>ServiceWorker: Store pending operation
    ServiceWorker-->>UI: Operation queued

    Note over GQL: Network connectivity restored
    UI->>Apollo: query { computePlatform } (health check)
    Apollo->>GQL: Execute query
    GQL-->>Apollo: Success response
    Apollo-->>UI: Connection restored

    UI->>UI: Detect connectivity restoration
    UI->>UI: Hide offline indicator
    UI->>UI: Show "Reconnected - syncing..." message
    UI->>ServiceWorker: Get pending operations
    ServiceWorker-->>UI: Pending operations list

    loop Process Pending Operations
        UI->>Apollo: Execute queued mutation
        Apollo->>GQL: Execute mutation
        GQL-->>Apollo: Success/Error
        Apollo-->>UI: Operation result
        UI->>UI: Update operation status
        UI->>ServiceWorker: Remove from queue
    end

    UI->>UI: Show "Sync complete" message
    UI->>UI: Resume normal operation
    UI->>UI: Re-enable all controls
```

### 7. **Destination Management Workflow**

```mermaid
sequenceDiagram
    participant User as User
    participant UI as Frontend UI
    participant Modal as Destination Modal
    participant Apollo as Apollo Client
    participant GQL as GraphQL API
    participant Services as Services Layer
    participant K8s as Kubernetes API

    User->>UI: Click "Add Destination"
    UI->>Modal: Open destination creation modal
    Modal->>Apollo: query { destinationTypes }
    Apollo->>GQL: Execute query
    GQL->>Services: GetDestinationTypes()
    Services-->>GQL: Available destination types
    GQL-->>Apollo: Destination types
    Apollo-->>Modal: Type options
    Modal->>Modal: Render destination type selector

    User->>Modal: Select destination type (e.g., "Jaeger")
    Modal->>Apollo: query { destinationTypeDetails(type: "jaeger") }
    Apollo->>GQL: Execute query
    GQL->>Services: GetDestinationTypeConfig("jaeger")
    Services-->>GQL: Field configuration
    GQL-->>Apollo: Field definitions
    Apollo-->>Modal: Dynamic form schema
    Modal->>Modal: Render dynamic form fields

    User->>Modal: Fill in destination details
    Modal->>Modal: Validate fields in real-time
    Modal->>Modal: Enable "Test Connection" button

    User->>Modal: Click "Test Connection"
    Modal->>Modal: Show testing indicator
    Modal->>Apollo: mutation testConnectionForDestination(destination)
    Apollo->>GQL: Execute mutation
    GQL->>Services: TestConnection(ctx, configurer)
    Services->>Services: Attempt connection to destination
    Services-->>GQL: Test result
    GQL-->>Apollo: Connection test response
    Apollo-->>Modal: Test result

    alt Connection Successful
        Modal->>Modal: Show success indicator
        Modal->>Modal: Enable "Save" button
    else Connection Failed
        Modal->>Modal: Show error details
        Modal->>Modal: Keep "Save" button disabled
        Modal->>Modal: Suggest troubleshooting steps
    end

    User->>Modal: Click "Save"
    Modal->>Modal: Show saving indicator
    Modal->>Apollo: mutation createNewDestination(destination)
    Apollo->>GQL: Execute mutation
    GQL->>Services: CreateDestination(ctx, destination)
    Services->>K8s: Create Destination CRD
    Services->>K8s: Create associated Secret (if needed)
    K8s-->>Services: Resources created
    Services-->>GQL: Creation result
    GQL-->>Apollo: Success response
    Apollo-->>Modal: Destination created

    Modal->>Modal: Show success message
    Modal->>UI: Trigger destinations list refresh
    UI->>Apollo: Refetch destinations query
    Apollo->>GQL: Execute query
    GQL-->>Apollo: Updated destinations list
    Apollo-->>UI: New destination appears in list
    Modal->>Modal: Auto-close modal
```

## UI State Management Patterns

### 1. **Loading States**
```typescript
interface UIState {
  loading: {
    namespaces: boolean;
    sources: boolean;
    destinations: boolean;
    operations: {
      [operationId: string]: boolean;
    };
  };
  errors: {
    [component: string]: Error | null;
  };
  cache: {
    lastUpdated: Date;
    data: any;
  };
}
```

### 2. **Error Handling Patterns**
```typescript
interface ErrorState {
  type: 'network' | 'validation' | 'permission' | 'unknown';
  message: string;
  retryable: boolean;
  retryCount: number;
  lastRetry: Date;
  context: {
    operation: string;
    parameters: any;
  };
}
```

### 3. **Optimistic Updates**
```typescript
// Example: Optimistic source instrumentation
const handleInstrumentSource = async (sourceId: string) => {
  // 1. Optimistically update UI
  updateSourceStatus(sourceId, 'instrumenting');
  
  try {
    // 2. Execute mutation
    const result = await persistK8sSources(namespace, [source]);
    
    // 3. Confirm success
    updateSourceStatus(sourceId, 'instrumented');
  } catch (error) {
    // 4. Rollback on error
    updateSourceStatus(sourceId, 'not-instrumented');
    showErrorMessage(error);
  }
};
```

This comprehensive documentation covers all major UI interaction patterns, error scenarios, and state management approaches used in the Odigos frontend architecture. 