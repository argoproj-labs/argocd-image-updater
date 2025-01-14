package common

// This file contains a helper function to retrieve prefixed value of the constants

// Prefixed returns the annotation of the constant prefixed with the given prefix
func Prefixed(prefix string, annotation string) string {
	return prefix + annotation
}
