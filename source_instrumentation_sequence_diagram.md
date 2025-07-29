# Source Instrumentation Flow - Sequence Diagram

## Complete Flow: UI → Frontend → Instrumentor

```mermaid
sequenceDiagram
    participant UI as React UI Components
    participant Hook as useSourceCRUD Hook
    participant Apollo as Apollo GraphQL Client
    participant Resolver as GraphQL Resolvers
    participant Service as Frontend Services
    participant K8sAPI as Kubernetes API
    participant Controller as Instrumentor Controllers
    participant Odiglet as Odiglet Agents
    participant Workload as K8s Workloads

    Note over UI, Workload: Source Instrumentation Flow

    %% PersistK8sSources Flow
    rect rgb(240, 248, 255)
        Note over UI, Service: PersistK8sSources Operation
        
        UI->>Hook: User selects/deselects sources
        Hook->>Apollo: persistK8sSources mutation
        Apollo->>Resolver: PersistK8sSources(namespace, sources[])
        
        Resolver->>Service: SyncWorkloadsInNamespace(ctx, namespace, workloads)
        
        loop For each workload
            Service->>Service: setWorkloadInstrumentationLabel()
            Service->>K8sAPI: Patch workload with odigos.io/instrumentation label
            K8sAPI-->>Service: Success/Error
        end
        
        Service-->>Resolver: Success/Error
        Resolver-->>Apollo: Boolean response
        Apollo-->>Hook: Mutation result
        Hook-->>UI: Update UI state
    end

    %% UpdateK8sActualSource Flow
    rect rgb(255, 248, 240)
        Note over UI, Service: UpdateK8sActualSource Operation
        
        UI->>Hook: User updates reportedName
        Hook->>Apollo: updateK8sActualSource mutation
        Apollo->>Resolver: UpdateK8sActualSource(sourceId, patchRequest)
        
        Resolver->>Service: UpdateReportedName(ctx, ns, kind, name, reportedName)
        Service->>K8sAPI: Get workload object
        K8sAPI-->>Service: Workload object
        Service->>Service: updateAnnotations(annotations, reportedName)
        Service->>K8sAPI: Update workload with odigos.io/reported-name annotation
        K8sAPI-->>Service: Success/Error
        
        Service-->>Resolver: Success/Error
        Resolver-->>Apollo: Boolean response
        Apollo-->>Hook: Mutation result
        Hook-->>UI: Update UI state
    end

    %% Instrumentor Controller Processing
    rect rgb(248, 255, 248)
        Note over Controller, Workload: Instrumentor Module Processing
        
        K8sAPI->>Controller: Workload change event (label/annotation)
        
        %% Language Detection
        Controller->>Controller: StartLangDetection Controller
        Controller->>Workload: Check if instrumentation enabled
        Workload-->>Controller: Instrumentation status
        
        alt Instrumentation Enabled
            Controller->>Odiglet: Request runtime details calculation
            Odiglet->>Workload: Inspect running containers
            Workload-->>Odiglet: Runtime information
            Odiglet-->>Controller: Runtime details
            
            Controller->>K8sAPI: Create/Update InstrumentedApplication CR
            K8sAPI-->>Controller: Success
            
            %% Configuration Creation
            Controller->>Controller: InstrumentationConfig Controller
            Controller->>K8sAPI: Create/Update InstrumentationConfig CR
            K8sAPI-->>Controller: Success
            
            %% Device Injection
            Controller->>Controller: InstrumentationDevice Controller
            Controller->>K8sAPI: Get workload object
            K8sAPI-->>Controller: Workload object
            Controller->>Controller: addInstrumentationDeviceToWorkload()
            Controller->>K8sAPI: Update workload pod template with instrumentation
            K8sAPI-->>Controller: Success
            
            K8sAPI->>Workload: Apply updated pod template
            Workload->>Workload: Restart pods with instrumentation
        else Instrumentation Disabled
            Controller->>K8sAPI: Delete InstrumentedApplication CR
            Controller->>K8sAPI: Delete InstrumentationConfig CR
            Controller->>Controller: removeInstrumentationDeviceFromWorkload()
            Controller->>K8sAPI: Update workload pod template (remove instrumentation)
            K8sAPI->>Workload: Apply clean pod template
        end
    end

    %% Final State
    rect rgb(255, 255, 240)
        Note over UI, Workload: Final State Update
        
        Workload-->>K8sAPI: Pod status changes
        K8sAPI-->>Apollo: GraphQL subscription/polling
        Apollo-->>UI: Update source status in UI
    end
```

## Key Components Breakdown

### Frontend Components
- **React UI**: Source management interface
- **useSourceCRUD Hook**: Custom hook managing source operations
- **Apollo Client**: GraphQL client for API communication
- **GraphQL Resolvers**: Backend resolvers handling mutations

### Service Layer
- **SyncWorkloadsInNamespace**: Bulk workload labeling
- **UpdateReportedName**: Individual source annotation updates
- **setWorkloadInstrumentationLabel**: Kubernetes label management

### Instrumentor Controllers
- **StartLangDetection Controller**: Detects newly labeled workloads
- **InstrumentationConfig Controller**: Creates SDK configurations
- **InstrumentationDevice Controller**: Injects instrumentation into pods

### Kubernetes Resources
- **Labels**: `odigos.io/instrumentation=enabled/disabled`
- **Annotations**: `odigos.io/reported-name=<custom-name>`
- **Custom Resources**: InstrumentedApplication, InstrumentationConfig
- **Workload Updates**: Pod templates with instrumentation devices

## Flow Summary

1. **UI Interaction**: User selects sources or updates properties
2. **GraphQL Layer**: Mutations processed by resolvers
3. **Service Layer**: Kubernetes API calls to update workloads
4. **Controller Watching**: Instrumentor controllers detect changes
5. **Instrumentation Injection**: Automatic injection of observability tools
6. **Pod Restart**: Workloads restart with instrumentation enabled
7. **Status Update**: UI reflects the new instrumentation status 