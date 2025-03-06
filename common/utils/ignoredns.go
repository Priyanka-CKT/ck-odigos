package utils

func arrayContainsString(arr []string, str string) bool {
	for _, elem := range arr {
		if elem == str {
			return true
		}
	}
	return false
}

func MergeDefaultIgnoreWithUserInput(userInputIgnore []string, defaultIgnored []string) []string {

	mergedList := make([]string, len(userInputIgnore))
	copy(mergedList, userInputIgnore)

	for _, ns := range defaultIgnored {
		if !arrayContainsString(mergedList, ns) {
			mergedList = append(mergedList, ns)
		}
	}

	return mergedList
}

func IsItemIgnored(item string, ignoredList []string) bool {
	for _, ignoredListItem := range ignoredList {
		if item == ignoredListItem {
			return true
		}
	}
	return false
}

// ShouldIncludeNamespace checks if a namespace should be included based on includeNamespaces and ignoredNamespaces
// If includeNamespaces is not empty, only namespaces in that list will be included
// If includeNamespaces is empty, all namespaces except those in ignoredNamespaces will be included
func ShouldIncludeNamespace(namespace string, includeNamespaces []string, ignoredNamespaces []string) bool {
	// If includeNamespaces is not empty, only include namespaces in that list
	if len(includeNamespaces) > 0 {
		return arrayContainsString(includeNamespaces, namespace)
	}

	// Otherwise, include all namespaces except those in ignoredNamespaces
	return !IsItemIgnored(namespace, ignoredNamespaces)
}
