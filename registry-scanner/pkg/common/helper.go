package common

// This file contains a helper function to retrieve prefixed value of the constants

// DEPRECATED: This function has been removed in the CRD branch and will be deprecated in a future release.
// The CRD branch introduces a new architecture that eliminates the need for this annotation-based approach.
// Prefixed returns the annotation of the constant prefixed with the given prefix
func Prefixed(prefix string, annotation string) string {
	return prefix + annotation
}
