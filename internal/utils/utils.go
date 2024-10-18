package utils

import (
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// func UpdateConditions(existingConditions []metav1.Condition, conditionType string, status metav1.ConditionStatus, reason string, message string, srcError error) []metav1.Condition {
func UpdateConditions(existingConditions []metav1.Condition, newCondition metav1.Condition) []metav1.Condition {

	// Comparing reason to existing conditions' reason.
	// If a match is found, only the lastTransitionTime is updated
	// If not, a new condition is added to the existing list
	var hasMatch, matchingIndex = false, -1
	for i, condition := range existingConditions {
		if condition.Reason == newCondition.Reason {
			matchingIndex = i
			hasMatch = true
		}
	}
	if hasMatch {
		existingConditions[matchingIndex].LastTransitionTime = metav1.NewTime(time.Now())
		existingConditions[matchingIndex].ObservedGeneration = newCondition.ObservedGeneration
		return existingConditions
	}

	return append([]metav1.Condition{newCondition}, existingConditions...)
}

func IsAllowedNamespaces(namespace string, namespaces []string) bool {
	for _, allowedNamespace := range namespaces {
		if strings.HasPrefix(allowedNamespace, "*") && strings.HasSuffix(allowedNamespace, "*") {
			return strings.Contains(namespace, strings.TrimSuffix(strings.TrimPrefix(allowedNamespace, "*"), "*"))
		} else if strings.HasPrefix(allowedNamespace, "*") {
			return strings.HasSuffix(namespace, strings.TrimPrefix(allowedNamespace, "*"))
		} else if strings.HasSuffix(allowedNamespace, "*") {
			return strings.HasPrefix(namespace, strings.TrimSuffix(allowedNamespace, "*"))
		} else {
			return namespace == allowedNamespace
		}
	}
	return false
}
