# Odiglet Process Detection and Language Detection Flow

## Table of Contents
1. [Overview](#overview)
2. [Process Discovery Architecture](#process-discovery-architecture)
3. [Container Process Filtering](#container-process-filtering)
4. [Process Details Extraction](#process-details-extraction)
5. [Language Detection Flow](#language-detection-flow)
6. [Language-Specific Inspectors](#language-specific-inspectors)
7. [Unknown Language Handling](#unknown-language-handling)
8. [Process Events and Runtime Detection](#process-events-and-runtime-detection)
9. [Failure Scenarios and Edge Cases](#failure-scenarios-and-edge-cases)
10. [Complete Flow Diagram](#complete-flow-diagram)

## Overview

Odiglet's runtime detection system is responsible for discovering running processes within Kubernetes containers and determining their programming languages and runtime versions. This system operates at the Linux process level using the `/proc` filesystem and employs multiple detection strategies to identify different programming languages.

## Process Discovery Architecture

### 1. Entry Point: Runtime Inspection

**File**: `odiglet/pkg/kube/runtime_details/inspection.go`

```go
func runtimeInspection(pods []corev1.Pod, ignoredContainers []string) ([]odigosv1.RuntimeDetailsByContainer, error) {
    resultsMap := make(map[string]odigosv1.RuntimeDetailsByContainer)
    
    for _, pod := range pods {
        for _, container := range pod.Spec.Containers {
            // Skip ignored containers
            if utils.IsItemIgnored(container.Name, ignoredContainers) {
                resultsMap[container.Name] = odigosv1.RuntimeDetailsByContainer{
                    ContainerName: container.Name,
                    Language:      common.IgnoredProgrammingLanguage,
                }
                continue
            }

            // Find all processes in this container
            processes, err := process.FindAllInContainer(string(pod.UID), container.Name)
            if err != nil {
                log.Logger.Error(err, "failed to find processes in pod container")
                return nil, err
            }
            
            if len(processes) == 0 {
                log.Logger.V(0).Info("no processes found in pod container")
                continue
            }

            // Detect language for each process
            programLanguageDetails := common.ProgramLanguageDetails{Language: common.UnknownProgrammingLanguage}
            var inspectProc *procdiscovery.Details
            
            for _, proc := range processes {
                containerURL := kubeutils.GetPodExternalURL(pod.Status.PodIP, container.Ports)
                programLanguageDetails, detectErr = inspectors.DetectLanguage(proc, containerURL)
                
                if detectErr == nil && programLanguageDetails.Language != common.UnknownProgrammingLanguage {
                    inspectProc = &proc
                    break // Found a recognizable language, stop searching
                }
            }
            
            // Process the results...
        }
    }
}
```

### 2. Container Process Discovery

**File**: `odiglet/pkg/process/process_linux.go`

```go
func FindAllInContainer(podUID string, containerName string) ([]procdiscovery.Details, error) {
    return procdiscovery.FindAllProcesses(isPodContainerPredicate(podUID, containerName))
}

func isPodContainerPredicate(podUID string, containerName string) func(string) bool {
    expectedMountRoot := fmt.Sprintf("%s/containers/%s", podUID, containerName)
    
    return func(procDirName string) bool {
        mountInfoFile := path.Join("/proc", procDirName, "mountinfo")
        f, err := os.Open(mountInfoFile)
        if err != nil {
            return false
        }
        defer f.Close()

        infos, err := mount.GetMountsFromReader(f, func(m *mount.Info) (skip, stop bool) {
            if strings.Contains(m.Root, expectedMountRoot) {
                return false, true // Found the mount, stop searching
            }
            return true, false // Keep looking
        })
        
        return len(infos) > 0
    }
}
```

**Process**:
1. **Mount Point Analysis**: Examines `/proc/[pid]/mountinfo` to determine container ownership
2. **Container Filtering**: Only processes with mount roots matching the expected container path are included
3. **Process Collection**: Returns all processes belonging to the specific container

## Container Process Filtering

### Mount Info Analysis

The system uses Linux mount information to determine which processes belong to which container:

```bash
# Example mount info for a container process
# /proc/1234/mountinfo contains:
1234 1235 0:123 /var/lib/kubelet/pods/abc-123/containers/app-container/... /app rw,relatime - overlay overlay rw,lowerdir=...
```

**Key Components**:
- **Pod UID**: `abc-123` (extracted from mount path)
- **Container Name**: `app-container` (extracted from mount path)
- **Mount Root**: Used to filter processes belonging to specific containers

## Process Details Extraction

### Core Process Discovery

**File**: `procdiscovery/pkg/process/process.go`

```go
func FindAllProcesses(predicate func(string) bool) ([]Details, error) {
    dirs, err := os.ReadDir("/proc")
    if err != nil {
        return nil, err
    }

    var result []Details
    for _, di := range dirs {
        if !di.IsDir() {
            continue
        }

        dirName := di.Name()
        pid, isProcessDirectory := isDirectoryPid(dirName)
        if !isProcessDirectory {
            continue
        }

        // Apply container filtering predicate
        if predicate != nil && !predicate(dirName) {
            continue
        }

        details := GetPidDetails(pid)
        result = append(result, details)
    }

    return result, nil
}
```

### Process ID Validation

**File**: `procdiscovery/pkg/process/utils.go`

```go
func isDirectoryPid(procDirectoryName string) (int, bool) {
    // Quick check: first character must be a digit
    if procDirectoryName[0] < '0' || procDirectoryName[0] > '9' {
        return 0, false
    }

    pid, err := strconv.Atoi(procDirectoryName)
    if err != nil {
        return 0, false
    }

    return pid, true
}
```

**Process**:
1. **Directory Filtering**: Only numeric directories in `/proc` are considered
2. **PID Validation**: Converts directory name to process ID
3. **Quick Rejection**: Non-numeric directories are immediately skipped

### Process Details Collection

```go
func GetPidDetails(pid int) Details {
    exeName := getExecName(pid)        // Read /proc/[pid]/exe symlink
    cmdLine := getCommandLine(pid)     // Read /proc/[pid]/cmdline
    envVars := getRelevantEnvVars(pid) // Read /proc/[pid]/environ

    return Details{
        ProcessID:    pid,
        ExeName:      exeName,
        CmdLine:      cmdLine,
        Environments: envVars,
    }
}
```

#### Executable Name Extraction

```go
func getExecName(pid int) string {
    exeFileName := fmt.Sprintf("/proc/%d/exe", pid)
    exeName, err := os.Readlink(exeFileName)
    if err != nil {
        // Read link may fail if target process runs not as root
        return ""
    }
    return exeName
}
```

**Examples**:
- Java: `/usr/lib/jvm/java-11-openjdk/bin/java`
- Go: `/app/mygoapp`
- Python: `/usr/bin/python3.9`
- Node.js: `/usr/local/bin/node`

#### Command Line Extraction

```go
func getCommandLine(pid int) string {
    cmdLineFileName := fmt.Sprintf("/proc/%d/cmdline", pid)
    fileContent, err := os.ReadFile(cmdLineFileName)
    if err != nil {
        return ""
    }
    return string(fileContent)
}
```

**Examples**:
- Java: `java\x00-jar\x00app.jar\x00`
- Go: `/app/mygoapp\x00--config\x00/etc/config.yaml\x00`
- Python: `python3\x00app.py\x00`
- Node.js: `node\x00server.js\x00`

#### Environment Variables Extraction

```go
func getRelevantEnvVars(pid int) ProcessEnvs {
    envFileName := fmt.Sprintf("/proc/%d/environ", pid)
    fileContent, err := os.ReadFile(envFileName)
    if err != nil {
        return ProcessEnvs{}
    }

    // Parse null-separated environment variables
    r := bufio.NewReader(strings.NewReader(string(fileContent)))
    
    overWriteEnvsResult := make(map[string]string)
    detailedEnvsResult := make(map[string]string)

    for {
        str, err := r.ReadString(0) // Read until null byte
        if err == io.EOF {
            break
        }
        
        str = strings.TrimRight(str, "\x00")
        envParts := strings.SplitN(str, "=", 2)
        if len(envParts) != 2 {
            continue
        }

        // Collect language version environment variables
        if _, ok := LangsVersionEnvs[envParts[0]]; ok {
            detailedEnvsResult[envParts[0]] = envParts[1]
        }

        // Collect other agent detection environment variables
        if _, ok := OtherAgentEnvs[envParts[0]]; ok {
            detailedEnvsResult[envParts[0]] = envParts[1]
        }

        // Collect Odigos overwrite environment variables
        if _, ok := relevantOverwriteEnvVars[envParts[0]]; ok {
            overWriteEnvsResult[envParts[0]] = envParts[1]
        }
    }

    return ProcessEnvs{
        OverwriteEnvs: overWriteEnvsResult,
        DetailedEnvs:  detailedEnvsResult,
    }
}
```

**Tracked Environment Variables**:
- **Language Versions**: `NODE_VERSION`, `PYTHON_VERSION`, `JAVA_VERSION`
- **Other Agents**: `NEW_RELIC_CONFIG_FILE` (detects New Relic agent)
- **Odigos Variables**: Variables that Odigos might overwrite during instrumentation

## Language Detection Flow

### Detection Orchestrator

**File**: `procdiscovery/pkg/inspectors/langdetect.go`

```go
var inspectorsList = []LanguageInspector{
    &golang.GolangInspector{},
    &java.JavaInspector{},
    &dotnet.DotnetInspector{},
    &nodejs.NodejsInspector{},
    &python.PythonInspector{},
    &mysql.MySQLInspector{},
    &nginx.NginxInspector{},
}

func DetectLanguage(process process.Details, containerURL string) (common.ProgramLanguageDetails, error) {
    detectedProgramLanguageDetails := common.ProgramLanguageDetails{
        Language: common.UnknownProgrammingLanguage,
    }

    for _, inspector := range inspectorsList {
        languageDetected, detected := inspector.Inspect(&process)
        if detected {
            if detectedProgramLanguageDetails.Language == common.UnknownProgrammingLanguage {
                // First detection
                detectedProgramLanguageDetails.Language = languageDetected
                
                // Get runtime version if inspector supports it
                if versionInspector, ok := inspector.(VersionInspector); ok {
                    detectedProgramLanguageDetails.RuntimeVersion = versionInspector.GetRuntimeVersion(&process, containerURL)
                }
                continue
            }
            
            // Conflict: multiple languages detected for same process
            return common.ProgramLanguageDetails{
                Language: common.UnknownProgrammingLanguage,
            }, ErrLanguageDetectionConflict{
                languages: [2]common.ProgrammingLanguage{
                    detectedProgramLanguageDetails.Language,
                    languageDetected,
                },
            }
        }
    }

    return detectedProgramLanguageDetails, nil
}
```

**Detection Strategy**:
1. **Sequential Inspection**: Each inspector examines the process
2. **First Match Wins**: First successful detection is accepted
3. **Conflict Detection**: Multiple detections result in "unknown" classification
4. **Version Resolution**: Runtime version extracted if supported

## Language-Specific Inspectors

### Java Inspector

**File**: `procdiscovery/pkg/inspectors/java/java.go`

```go
type JavaInspector struct{}

const processName = "java"
const JavaVersionRegex = `\d+\.\d+\.\d+\+\d+`

var re = regexp.MustCompile(JavaVersionRegex)

func (j *JavaInspector) Inspect(proc *process.Details) (common.ProgrammingLanguage, bool) {
    // Check executable name
    if strings.Contains(proc.ExeName, processName) {
        return common.JavaProgrammingLanguage, true
    }
    
    // Check command line
    if strings.Contains(proc.CmdLine, processName) {
        return common.JavaProgrammingLanguage, true
    }

    return "", false
}

func (j *JavaInspector) GetRuntimeVersion(proc *process.Details, containerURL string) *version.Version {
    if value, exists := proc.GetDetailedEnvsValue(process.JavaVersionConst); exists {
        javaVersion := re.FindString(value)
        return common.GetVersion(javaVersion)
    }
    return nil
}
```

**Java Detection Methods**:
1. **Executable Path**: Checks if `/proc/[pid]/exe` contains "java"
2. **Command Line**: Checks if `/proc/[pid]/cmdline` contains "java"
3. **Version Detection**: Extracts version from `JAVA_VERSION` environment variable

**Examples**:
- **Executable**: `/usr/lib/jvm/java-11-openjdk/bin/java`
- **Command Line**: `java -jar app.jar`
- **Version**: `11.0.16+8` (from `JAVA_VERSION=11.0.16+8`)

### Go Inspector

**File**: `procdiscovery/pkg/inspectors/golang/golang.go`

```go
type GolangInspector struct{}

const GolangVersionRegex = `go(\d+\.\d+\.\d+)`

var re = regexp.MustCompile(GolangVersionRegex)

func (g *GolangInspector) Inspect(p *process.Details) (common.ProgrammingLanguage, bool) {
    file := fmt.Sprintf("/proc/%d/exe", p.ProcessID)
    _, err := buildinfo.ReadFile(file)
    if err != nil {
        return "", false
    }

    return common.GoProgrammingLanguage, true
}

func (g *GolangInspector) GetRuntimeVersion(p *process.Details, containerURL string) *version.Version {
    file := fmt.Sprintf("/proc/%d/exe", p.ProcessID)
    buildInfo, err := buildinfo.ReadFile(file)
    if err != nil || buildInfo == nil {
        return nil
    }
    
    match := re.FindStringSubmatch(buildInfo.GoVersion)
    if len(match) < 2 {
        return nil
    }
    
    return common.GetVersion(match[1])
}
```

**Go Detection Methods**:
1. **Build Info Analysis**: Uses `debug/buildinfo` to read Go build information from executable
2. **ELF Binary Inspection**: Directly examines the binary for Go-specific metadata
3. **Version Extraction**: Extracts Go version from embedded build information

**Examples**:
- **Build Info**: Go build information embedded in binary
- **Version**: `1.19.5` (from `go1.19.5`)

### Python Inspector

**File**: `procdiscovery/pkg/inspectors/python/python.go`

```go
type PythonInspector struct{}

const (
    pythonProcessName = "python"
    libPythonStr      = "libpython3"
)

func (p *PythonInspector) Inspect(proc *process.Details) (common.ProgrammingLanguage, bool) {
    // Check executable name
    if strings.Contains(proc.ExeName, pythonProcessName) || strings.Contains(proc.CmdLine, pythonProcessName) {
        return common.PythonProgrammingLanguage, true
    }

    // Check for linked libpython
    if p.isLibPythonLinked(proc) {
        return common.PythonProgrammingLanguage, true
    }

    return "", false
}

func (p *PythonInspector) isLibPythonLinked(proc *process.Details) bool {
    f := fmt.Sprintf("/proc/%d/exe", proc.ProcessID)
    file, err := os.Open(f)
    if err != nil {
        return false
    }
    defer file.Close()

    elfFile, err := elf.NewFile(file)
    if err != nil {
        return false
    }
    defer elfFile.Close()

    // Check dynamic dependencies
    dynamicSection, err := elfFile.DynString(elf.DT_NEEDED)
    if err != nil {
        return false
    }

    for _, dep := range dynamicSection {
        if strings.Contains(dep, libPythonStr) {
            return true
        }
    }

    return false
}
```

**Python Detection Methods**:
1. **Process Name**: Checks executable and command line for "python"
2. **Library Linking**: Examines ELF dynamic dependencies for `libpython3`
3. **Version Detection**: Extracts version from `PYTHON_VERSION` environment variable

### Node.js Inspector

**File**: `procdiscovery/pkg/inspectors/nodejs/nodejs.go`

```go
type NodejsInspector struct{}

const nodeProcessName = "nodejs"

func (n *NodejsInspector) Inspect(proc *process.Details) (common.ProgrammingLanguage, bool) {
    if strings.Contains(proc.ExeName, nodeProcessName) || strings.Contains(proc.CmdLine, nodeProcessName) {
        return common.JavascriptProgrammingLanguage, true
    }

    return "", false
}

func (n *NodejsInspector) GetRuntimeVersion(proc *process.Details, containerURL string) *version.Version {
    if value, exists := proc.GetDetailedEnvsValue(process.NodeVersionConst); exists {
        return common.GetVersion(value)
    }
    return nil
}
```

**Node.js Detection Methods**:
1. **Process Name**: Checks for "nodejs" in executable path or command line
2. **Version Detection**: Extracts version from `NODE_VERSION` environment variable

## Unknown Language Handling

### When Languages Cannot Be Detected

```go
// In runtime inspection
if inspectProc == nil {
    log.Logger.V(0).Info("unable to detect language for any process", 
        "pod", pod.Name, "container", container.Name, "namespace", pod.Namespace)
    programLanguageDetails.Language = common.UnknownProgrammingLanguage
}

// Result stored in InstrumentedApplication
resultsMap[container.Name] = odigosv1.RuntimeDetailsByContainer{
    ContainerName:  container.Name,
    Language:       common.UnknownProgrammingLanguage, // Marked as unknown
    RuntimeVersion: "",
    EnvVars:        []odigosv1.EnvVar{},
    OtherAgent:     nil,
    LibCType:       nil,
}
```

### Supported vs Unsupported Languages

```go
func isSupportedLanguage(language common.ProgrammingLanguage) bool {
    switch language {
    case common.JavaProgrammingLanguage, common.GoProgrammingLanguage:
        return true
    default:
        return false
    }
}

// In persistRuntimeResults
containsUnsupportedLanguage := false
for _, result := range results {
    if !isSupportedLanguage(result.Language) {
        containsUnsupportedLanguage = true
        log.Logger.Info("Detected unsupported language, skipping instrumentation",
            "language", result.Language,
            "name", owner.GetName(),
            "namespace", owner.GetNamespace())
    }
}
```

### Unknown Language Scenarios

1. **No Process Detection**: Container has no running processes
2. **Unrecognized Runtime**: Process doesn't match any inspector patterns
3. **Detection Conflict**: Multiple inspectors claim the same process
4. **Unsupported Language**: Language detected but not supported for instrumentation
5. **Permission Issues**: Cannot read process information due to security restrictions

### Handling Unknown Languages

```go
// Language detection priorities
const (
    JavaProgrammingLanguage       ProgrammingLanguage = "java"        // Supported
    PythonProgrammingLanguage     ProgrammingLanguage = "python"      // Detected but unsupported
    GoProgrammingLanguage         ProgrammingLanguage = "go"          // Supported
    DotNetProgrammingLanguage     ProgrammingLanguage = "dotnet"      // Detected but unsupported
    JavascriptProgrammingLanguage ProgrammingLanguage = "javascript"  // Detected but unsupported
    MySQLProgrammingLanguage      ProgrammingLanguage = "mysql"       // Experimental
    NginxProgrammingLanguage      ProgrammingLanguage = "nginx"       // Experimental
    UnknownProgrammingLanguage    ProgrammingLanguage = "unknown"     // Cannot detect
    IgnoredProgrammingLanguage    ProgrammingLanguage = "ignored"     // Explicitly ignored
)
```

**Behavior for Each Type**:
- **Supported Languages** (Java, Go): Full instrumentation applied
- **Detected but Unsupported** (Python, .NET, Node.js): Detection recorded, no instrumentation
- **Unknown**: No detection, no instrumentation
- **Ignored**: Explicitly skipped by configuration

## Process Events and Runtime Detection

### Process Event Detection

**File**: `instrumentation/detector/detector.go`

```go
const (
    ProcessExecEvent = detector.ProcessExecEvent  // New process started
    ProcessExitEvent = detector.ProcessExitEvent  // Process terminated
)

type ProcessEvent = detector.ProcessEvent

// Process event structure
type ProcessEvent struct {
    PID         int
    EventType   EventType
    ExecDetails *ExecDetails  // Only for exec events
}

type ExecDetails struct {
    CmdLine      string
    Environments map[string]string
}
```

### Event-Based Runtime Detection

```go
// In instrumentation manager
func (m *manager) handleProcessExecEvent(ctx context.Context, e detector.ProcessEvent) error {
    // Resolve process group (pod, container, workload)
    pg, err := m.handler.ProcessGroupResolver.Resolve(ctx, e)
    if err != nil {
        return errors.Join(err, errFailedToGetDetails)
    }

    // Determine OTel distribution based on detected language
    otelDisto, err := m.handler.DistributionMatcher.Distribution(ctx, pg)
    if err != nil {
        return errors.Join(err, errFailedToGetDistribution)
    }

    // Apply instrumentation if supported
    factory, found := m.factories[otelDisto]
    if !found {
        return errNoInstrumentationFactory
    }

    // Create and start instrumentation
    inst, err := factory.CreateEbpfInstrumentation(ctx, e.PID, ...)
    if err != nil {
        return err
    }

    return inst.Load(ctx)
}
```

## Failure Scenarios and Edge Cases

### 1. Permission Denied Scenarios

```go
// Reading /proc/[pid]/exe
func getExecName(pid int) string {
    exeFileName := fmt.Sprintf("/proc/%d/exe", pid)
    exeName, err := os.Readlink(exeFileName)
    if err != nil {
        // Read link may fail if target process runs not as root
        return ""
    }
    return exeName
}
```

**Common Causes**:
- Process running as different user
- Security policies preventing access
- Process already terminated

### 2. Container Mount Detection Failures

```go
// Mount info parsing
infos, err := mount.GetMountsFromReader(f, func(m *mount.Info) (skip, stop bool) {
    if strings.Contains(m.Root, expectedMountRoot) {
        return false, true
    }
    return true, false
})
if err != nil {
    return false  // Cannot determine container ownership
}
```

**Failure Cases**:
- Corrupted mount information
- Non-standard container runtimes
- Process not in expected container

### 3. Language Detection Conflicts

```go
type ErrLanguageDetectionConflict struct {
    languages [2]common.ProgrammingLanguage
}

func (e ErrLanguageDetectionConflict) Error() string {
    return fmt.Sprintf("language detection conflict between %v and %v", 
        e.languages[0], e.languages[1])
}
```

**Conflict Scenarios**:
- Java process with embedded Python interpreter
- Go binary with Java libraries
- Multi-language containers

### 4. Process Lifecycle Issues

```go
// Process may exit during inspection
details := GetPidDetails(pid)
// Process might be gone by now, resulting in empty details
```

**Race Conditions**:
- Process exits during inspection
- Container restarts during detection
- Pod deletion during analysis

## Complete Flow Diagram

```mermaid
graph TD
    A[Runtime Inspection Triggered] --> B[Get Running Pods]
    B --> C[For Each Container]
    C --> D[Check if Ignored]
    D -->|Ignored| E[Mark as IgnoredProgrammingLanguage]
    D -->|Not Ignored| F[Find All Processes in Container]
    
    F --> G[Read /proc Directory]
    G --> H[Filter by PID Pattern]
    H --> I[Apply Container Mount Filter]
    I --> J[Extract Process Details]
    
    J --> K[Read /proc/[pid]/exe]
    J --> L[Read /proc/[pid]/cmdline]
    J --> M[Read /proc/[pid]/environ]
    
    K --> N[Process Details Collected]
    L --> N
    M --> N
    
    N --> O[For Each Process]
    O --> P[Run Language Inspectors]
    
    P --> Q{Java Inspector}
    Q -->|Match| R[Java Detected]
    Q -->|No Match| S{Go Inspector}
    
    S -->|Match| T[Go Detected]
    S -->|No Match| U{Python Inspector}
    
    U -->|Match| V[Python Detected]
    U -->|No Match| W{Other Inspectors}
    
    W -->|Match| X[Other Language Detected]
    W -->|No Match| Y[Unknown Language]
    
    R --> Z[Extract Runtime Version]
    T --> Z
    V --> Z
    X --> Z
    Y --> AA[No Version Available]
    
    Z --> BB[Check for Other Agents]
    AA --> BB
    
    BB --> CC[Detect LibC Type if Needed]
    CC --> DD[Create RuntimeDetailsByContainer]
    
    DD --> EE{Language Supported?}
    EE -->|Yes| FF[Apply Instrumentation]
    EE -->|No| GG[Record Detection Only]
    
    E --> HH[Store Results]
    FF --> HH
    GG --> HH
    
    HH --> II[Create/Update InstrumentedApplication CRD]
    II --> JJ[Persist to InstrumentationConfig]
```

## Summary

The Odiglet process detection and language detection system is a sophisticated multi-layered approach that:

1. **Discovers Processes**: Uses Linux `/proc` filesystem to find container processes
2. **Filters by Container**: Uses mount information to isolate container-specific processes
3. **Extracts Details**: Reads executable paths, command lines, and environment variables
4. **Detects Languages**: Employs multiple inspection strategies for different languages
5. **Handles Failures**: Gracefully manages unknown languages and detection failures
6. **Supports Instrumentation**: Only applies instrumentation to supported languages (Java, Go)

The system is designed to be resilient, handling edge cases like permission issues, process lifecycle races, and unknown languages while providing comprehensive runtime information for supported languages. 