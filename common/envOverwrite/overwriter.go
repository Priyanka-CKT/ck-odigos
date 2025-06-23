package envOverwrite

import (
	"log"
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
			common.OtelSdkNativeCommunity: "--require /var/odigos/nodejs/autoinstrumentation.js",
			common.OtelSdkEbpfEnterprise:  "--require /var/odigos/nodejs-ebpf/autoinstrumentation.js",
		},
	},
	"PYTHONPATH": {
		delim:               ":",
		programmingLanguage: common.PythonProgrammingLanguage,
		values: map[common.OtelSdk]string{
			common.OtelSdkNativeCommunity: "/var/odigos/python:/var/odigos/python/opentelemetry/instrumentation/auto_instrumentation",
			common.OtelSdkEbpfEnterprise:  "/var/odigos/python-ebpf:/var/odigos/python/opentelemetry/instrumentation/auto_instrumentation:/var/odigos/python",
		},
	},
	"JAVA_OPTS": {
		delim:               " ",
		programmingLanguage: common.JavaProgrammingLanguage,
		values: map[common.OtelSdk]string{
			common.OtelSdkNativeCommunity: "-javaagent:/var/odigos/java/ck-agent-universal.jar",
			common.OtelSdkEbpfEnterprise:  "-javaagent:/var/odigos/java-ebpf/dtrace-injector.jar",
			common.OtelSdkNativeEnterprise: "-javaagent:/var/odigos/java-ext-ebpf/javaagent.jar " +
				"-Dotel.javaagent.extensions=/var/odigos/java-ext-ebpf/otel_agent_extension.jar",
		},
	},
	"JAVA_TOOL_OPTIONS": {
		delim:               " ",
		programmingLanguage: common.JavaProgrammingLanguage,
		values: map[common.OtelSdk]string{
			common.OtelSdkNativeCommunity: "-javaagent:/var/odigos/java/ck-agent-universal.jar",
			common.OtelSdkEbpfEnterprise:  "-javaagent:/var/odigos/java-ebpf/dtrace-injector.jar",
			common.OtelSdkNativeEnterprise: "-javaagent:/var/odigos/java-ext-ebpf/javaagent.jar " +
				"-Dotel.javaagent.extensions=/var/odigos/java-ext-ebpf/otel_agent_extension.jar",
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
	log.Printf("Getting patched env value: envName=%s, observedValue=%s, currentSdk=%v, language=%v",
		envName, observedValue, currentSdk, language)

	envMetadata, ok := EnvValuesMap[envName]
	if !ok {
		log.Printf("Environment variable not managed by Odigos: %s", envName)
		return nil
	}

	if envMetadata.programmingLanguage != language {
		log.Printf("Environment variable not managed for this language: %s, language=%v, expectedLanguage=%v",
			envName, language, envMetadata.programmingLanguage)
		return nil
	}

	if currentSdk == nil {
		log.Printf("No SDK specified, skipping environment variable patching: %s", envName)
		return nil
	}

	desiredOdigosPart, ok := envMetadata.values[*currentSdk]
	log.Printf("Retrieved desired Odigos part: envName=%s, desiredOdigosPart=%s, found=%v",
		envName, desiredOdigosPart, ok)
	if !ok {
		log.Printf("No specific overwrite required for this SDK: envName=%s, sdk=%v",
			envName, *currentSdk)
		return nil
	}

	// scenario 1: no user defined values and no odigos value
	if observedValue == "" {
		log.Printf("No observed value, skipping patching: %s", envName)
		return nil
	}

	// scenario 2: no user defined values, only odigos value
	for _, sdkEnvValue := range envMetadata.values {
		if sdkEnvValue == observedValue {
			log.Printf("Value already matches Odigos value, no patching needed: envName=%s, value=%s",
				envName, observedValue)
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
			log.Printf("Removing ignored value from environment variable: envName=%s, ignoredValue=%s",
				envName, part)
			continue
		}
		if strings.TrimSpace(part) == "" {
			continue
		}
		newValues = append(newValues, part)
	}
	observedValue = strings.Join(newValues, envMetadata.delim)
	log.Printf("Cleaned observed value: envName=%s, cleanedValue=%s",
		envName, observedValue)

	// Scenario 3: both odigos and user defined values are present
	for _, sdkEnvValue := range envMetadata.values {
		if strings.Contains(observedValue, sdkEnvValue) {
			if sdkEnvValue == desiredOdigosPart {
				log.Printf("Value already contains desired Odigos part: envName=%s, value=%s",
					envName, observedValue)
				return &observedValue
			} else {
				patchedEvnValue := strings.ReplaceAll(observedValue, sdkEnvValue, desiredOdigosPart)
				log.Printf("Replaced existing Odigos part with new value: envName=%s, oldValue=%s, newValue=%s",
					envName, observedValue, patchedEvnValue)
				return &patchedEvnValue
			}
		}
	}

	// Scenario 4: only user defined values are present
	if observedValue == "" {
		log.Printf("No observed value, using only Odigos part: envName=%s, value=%s",
			envName, desiredOdigosPart)
		return &desiredOdigosPart
	} else {
		mergedEnvValue := observedValue + envMetadata.delim + desiredOdigosPart
		log.Printf("Merged user value with Odigos part: envName=%s, mergedValue=%s",
			envName, mergedEnvValue)
		return &mergedEnvValue
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
