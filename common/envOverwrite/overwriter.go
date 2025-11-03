package envOverwrite

import (
	"fmt"
	"os"
	"strings"

	"github.com/odigos-io/odigos/common"
)

type envValues struct {
	delim               string
	programmingLanguage common.ProgrammingLanguage
	values              map[common.OtelSdk]string
}

// EnvValuesMap is a map of environment variables odigos uses for various languages and goals.
// The key is the environment variable name and the value is the value to be set or appended
// to the environment variable. We need to make sure that in case any of these environment
// variables is already set, we append the value to it instead of overwriting it.
//
// Note: The values here needs to be in sync with the paths used in the odigos images.
// If the paths are changed in the odigos images, the values here should be updated accordingly.
var EnvValuesMap = map[string]envValues{
	"NODE_OPTIONS": {
		delim:               " ",
		programmingLanguage: common.JavascriptProgrammingLanguage,
		values: map[common.OtelSdk]string{
			common.OtelSdkNativeCommunity: "--require /var/codekarma/nodejs/autoinstrumentation.js",
			common.OtelSdkEbpfEnterprise:  "--require /var/codekarma/nodejs-ebpf/autoinstrumentation.js",
		},
	},
	"PYTHONPATH": {
		delim:               ":",
		programmingLanguage: common.PythonProgrammingLanguage,
		values: map[common.OtelSdk]string{
			common.OtelSdkNativeCommunity: "/var/codekarma/python:/var/codekarma/python/opentelemetry/instrumentation/auto_instrumentation",
			common.OtelSdkEbpfEnterprise:  "/var/codekarma/python-ebpf:/var/codekarma/python/opentelemetry/instrumentation/auto_instrumentation:/var/codekarma/python",
		},
	},
	"JAVA_OPTS": {
		delim:               " ",
		programmingLanguage: common.JavaProgrammingLanguage,
		values: map[common.OtelSdk]string{
			common.OtelSdkNativeCommunity: "-javaagent:/var/codekarma/java/ck-agent-universal.jar",
			common.OtelSdkEbpfEnterprise:  "-javaagent:/var/codekarma/java-ebpf/dtrace-injector.jar",
			common.OtelSdkNativeEnterprise: "-javaagent:/var/codekarma/java-ext-ebpf/javaagent.jar " +
				"-Dotel.javaagent.extensions=/var/codekarma/java-ext-ebpf/otel_agent_extension.jar",
		},
	},
	"JAVA_TOOL_OPTIONS": {
		delim:               " ",
		programmingLanguage: common.JavaProgrammingLanguage,
		values: map[common.OtelSdk]string{
			common.OtelSdkNativeCommunity: getConditionalJavaAgent("/var/codekarma/java/ck-agent-universal.jar"),
			common.OtelSdkEbpfEnterprise:  getConditionalJavaAgent("/var/codekarma/java-ebpf/dtrace-injector.jar"),
			common.OtelSdkNativeEnterprise: getConditionalJavaAgent("/var/codekarma/java-ext-ebpf/javaagent.jar") + " " +
				"-Dotel.javaagent.extensions=/var/codekarma/java-ext-ebpf/otel_agent_extension.jar",
		},
	},
}

func GetRelevantEnvVarsKeys() []string {
	keys := make([]string, 0, len(EnvValuesMap))
	for key := range EnvValuesMap {
		keys = append(keys, key)
	}
	return keys
}

// returns the current value that should be populated in a specific environment variable.
// if we should not patch the value, returns nil.
// the are 2 parts to the environment value: odigos part and user part.
// either one can be set or empty.
// so we have 4 cases to handle:
func GetPatchedEnvValue(envName string, observedValue string, currentSdk *common.OtelSdk, language common.ProgrammingLanguage) *string {
	envMetadata, ok := EnvValuesMap[envName]
	if !ok {
		// Odigos does not manipulate this environment variable, so ignore it
		return nil
	}

	if envMetadata.programmingLanguage != language {
		// Odigos does not manipulate this environment variable for the given language, so ignore it
		return nil
	}

	if currentSdk == nil {
		// When we have no sdk injected, we should not inject any odigos values.
		if envName == "JAVA_TOOL_OPTIONS" || envName == "JAVA_OPTS" {
			fmt.Printf("DEBUG GetPatchedEnvValue: SDK is nil, returning nil for %s\n", envName)
		}
		return nil
	}

	desiredOdigosPart, ok := envMetadata.values[*currentSdk]
	if !ok {
		// No specific overwrite is required for this SDK
		if envName == "JAVA_TOOL_OPTIONS" || envName == "JAVA_OPTS" {
			fmt.Printf("DEBUG GetPatchedEnvValue: No value for SDK %v, returning nil for %s\n", *currentSdk, envName)
		}
		return nil
	}
	
	if envName == "JAVA_TOOL_OPTIONS" || envName == "JAVA_OPTS" {
		fmt.Printf("DEBUG GetPatchedEnvValue: Processing %s - observed='%s', desired='%s', sdk=%v, language=%v\n", envName, observedValue, desiredOdigosPart, *currentSdk, language)
	}

	// For JAVA_TOOL_OPTIONS, validate all agents exist before processing
	if envName == "JAVA_TOOL_OPTIONS" && language == common.JavaProgrammingLanguage {
		// For environment overwrite, we don't have pod labels, so use file-only validation
		validatedValue := validateAllJavaAgentsFileOnly(observedValue)
		if validatedValue != observedValue {
			// If validation changed the value, return the validated version
			return &validatedValue
		}
	}

	// scenario 1: no user defined values and no odigos value
	// happens: might be the case right after the source is instrumented, and before the instrumentation is applied.
	// action: there are no user defined values, so no need to make any changes.
	// CRITICAL FIX: Return nil to avoid overwriting existing manifest values during helm upgrade
	if observedValue == "" {
		if envName == "JAVA_TOOL_OPTIONS" || envName == "JAVA_OPTS" {
			fmt.Printf("DEBUG GetPatchedEnvValue: Scenario 1 - empty observed, returning nil (no overwrite)\n")
		}
		return nil
	}

	// scenario 2: no user defined values, only odigos value
	// happens: when the user did not set any value to this env (either via manifest or dockerfile)
	// action: we don't need to overwrite the value, just let odigos handle it
	for _, sdkEnvValue := range envMetadata.values {
		if sdkEnvValue == observedValue {
			return nil
		}
	}

	// temporary fix clean up observed value from the known webhook injected value
	parts := strings.Split(observedValue, envMetadata.delim)
	const (
		ignoredJavaAgentValue       = "-javaagent:/opt/sre-agent/sre-agent.jar"
		ignoredNRPythonPathAddition = "newrelic/bootstrap"
	)
	newValues := []string{}
	for _, part := range parts {
		if part == ignoredJavaAgentValue || strings.Contains(part, ignoredNRPythonPathAddition) {
			continue
		}
		if strings.TrimSpace(part) == "" {
			continue
		}
		newValues = append(newValues, part)
	}
	observedValue = strings.Join(newValues, envMetadata.delim)

	// Scenario 3: Check if our DESIRED SDK value is already present
	// If it is, return as-is. If not, we need to add/replace it.
	if strings.Contains(observedValue, desiredOdigosPart) {
		// Our desired value is already present, no need to patch
		if envName == "JAVA_TOOL_OPTIONS" || envName == "JAVA_OPTS" {
			fmt.Printf("DEBUG GetPatchedEnvValue: Scenario 3 - desired already present, returning observed='%s'\n", observedValue)
		}
		return &observedValue
	}
	
	if envName == "JAVA_TOOL_OPTIONS" || envName == "JAVA_OPTS" {
		fmt.Printf("DEBUG GetPatchedEnvValue: Scenario 3 - desired NOT present, checking for other SDK values to replace\n")
	}
	
	// Scenario 3b: Check if OTHER SDK values are present that need to be replaced
	// This happens when switching between SDKs (e.g., from Odigos to CodeKarma)
	for _, sdkEnvValue := range envMetadata.values {
		if sdkEnvValue != desiredOdigosPart && strings.Contains(observedValue, sdkEnvValue) {
			// Replace the other SDK's value with our desired value
			patchedEvnValue := strings.ReplaceAll(observedValue, sdkEnvValue, desiredOdigosPart)
			if envName == "JAVA_TOOL_OPTIONS" || envName == "JAVA_OPTS" {
				fmt.Printf("DEBUG GetPatchedEnvValue: Scenario 3b - found other SDK value='%s', replacing with desired='%s', result='%s'\n", sdkEnvValue, desiredOdigosPart, patchedEvnValue)
			}
			return &patchedEvnValue
		}
	}
	
	if envName == "JAVA_TOOL_OPTIONS" || envName == "JAVA_OPTS" {
		fmt.Printf("DEBUG GetPatchedEnvValue: No other SDK values found, falling to Scenario 4\n")
	}

	// Scenario 4: only user defined values are present
	// happens: when the user set some values to this env (either via manifest or dockerfile) and odigos instrumentation not yet applied.
	// action: we want to keep the user defined values and prepend the odigos value for JAVA_TOOL_OPTIONS.
	if observedValue == "" {
		if envName == "JAVA_TOOL_OPTIONS" || envName == "JAVA_OPTS" {
			fmt.Printf("DEBUG GetPatchedEnvValue: Scenario 4 - observed empty (unexpected), returning desired='%s'\n", desiredOdigosPart)
		}
		return &desiredOdigosPart
	} else {
		// For JAVA_TOOL_OPTIONS, prepend the CodeKarma agent first, then add other values
		if envName == "JAVA_TOOL_OPTIONS" {
			mergedEnvValue := desiredOdigosPart + envMetadata.delim + observedValue
			fmt.Printf("DEBUG GetPatchedEnvValue: Scenario 4 - JAVA_TOOL_OPTIONS prepending, result='%s'\n", mergedEnvValue)
			return &mergedEnvValue
		} else {
			// For other environment variables, append the odigos value
			mergedEnvValue := observedValue + envMetadata.delim + desiredOdigosPart
			if envName == "JAVA_OPTS" {
				fmt.Printf("DEBUG GetPatchedEnvValue: Scenario 4 - JAVA_OPTS appending, result='%s'\n", mergedEnvValue)
			}
			return &mergedEnvValue
		}
	}
}

func ValToAppend(envName string, sdk common.OtelSdk) (string, bool) {
	env, exists := EnvValuesMap[envName]
	if !exists {
		return "", false
	}

	valToAppend, ok := env.values[sdk]
	if !ok {
		return "", false
	}

	return valToAppend, true
}

// validateAllJavaAgents checks if ALL agents in JAVA_TOOL_OPTIONS exist and removes invalid ones
// Uses both label presence and file existence for more accurate validation
func validateAllJavaAgents(javaToolOptions string, podLabels map[string]string) string {
	if javaToolOptions == "" {
		return ""
	}

	parts := strings.Split(javaToolOptions, " ")
	var validParts []string

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if strings.HasPrefix(part, "-javaagent:") {
			agentPath := strings.TrimPrefix(part, "-javaagent:")

			// Enhanced validation: Check both file existence AND label presence
			if shouldKeepJavaAgent(agentPath, podLabels) {
				validParts = append(validParts, part)
			}
			// Skip invalid agents - they will be removed
		} else {
			// Keep non-javaagent parts (like -Xmx512m, -XX:+UseG1GC, etc.)
			validParts = append(validParts, part)
		}
	}

	return strings.Join(validParts, " ")
}

// shouldKeepJavaAgent determines if a Java agent should be kept based on label presence only
// File existence check is removed to avoid race conditions with device plugin mounting
// JVM will handle missing files gracefully at startup
func shouldKeepJavaAgent(agentPath string, podLabels map[string]string) bool {
	// Enhanced logic: Check label presence for specific agents
	if strings.Contains(agentPath, "/var/odigos/") {
		// For Odigos agents:
		// - If Odigos label is present, keep it
		// - If Odigos label is NOT present, remove it
		if odigosLabel, exists := podLabels["odigos.io/inject-instrumentation"]; exists && odigosLabel == "true" {
			return true
		}
		// If Odigos label is not present, remove the Odigos agent
		return false
	}

	if strings.Contains(agentPath, "/var/codekarma/") {
		// For CodeKarma agents:
		// - If CodeKarma label is present, keep it
		// - If CodeKarma label is NOT present, remove it
		if codekarmaLabel, exists := podLabels["codekarma.tech/inject-instrumentation"]; exists && codekarmaLabel == "true" {
			return true
		}
		// If CodeKarma label is not present, remove the CodeKarma agent
		return false
	}

	return true
}

// validateAllJavaAgentsFileOnly keeps all agents (no file existence check)
// Used when pod labels are not available (e.g., in environment overwrite logic)
// File existence check is removed because files are only available inside containers at runtime
func validateAllJavaAgentsFileOnly(javaToolOptions string) string {
	if javaToolOptions == "" {
		return ""
	}

	// Keep all agents - let the JVM handle missing files at runtime
	return javaToolOptions
}

// agentExists checks if the agent file exists on the filesystem
func agentExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// getConditionalJavaAgent returns the javaagent option
// File existence check is removed because files are only available inside containers at runtime
func getConditionalJavaAgent(agentPath string) string {
	return fmt.Sprintf("-javaagent:%s", agentPath)
}
