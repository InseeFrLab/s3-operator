package utils

import (
	"math/rand"
	"time"
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

const (
	letterBytes  = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	specialBytes = "!@#$%^&*()_+-=[]{}\\|;':\",.<>/?`~"
	numBytes     = "0123456789"
)

// func GeneratePassword(length int, useLetters bool, useSpecial bool, useNum bool) string {
func GeneratePassword(length int, useLetters bool, useSpecial bool, useNum bool) string {
	b := make([]byte, length)
	for i := range b {
		if useLetters {
			b[i] = letterBytes[rand.Intn(len(letterBytes))]
		} else if useSpecial {
			b[i] = specialBytes[rand.Intn(len(specialBytes))]
		} else if useNum {
			b[i] = numBytes[rand.Intn(len(numBytes))]
		}
	}
	return string(b)
}
