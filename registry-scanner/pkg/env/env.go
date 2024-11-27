package env

import (
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/argoproj-labs/argocd-image-updater/pkg/log"
)

// Package env provides some utility functions to interact with the environment
// of the process.

// GetBoolVal retrieves a boolean value from given environment envVar.
// Returns default value if envVar is not set.
func GetBoolVal(envVar string, defaultValue bool) bool {
	if val := os.Getenv(envVar); val != "" {
		if strings.ToLower(val) == "true" {
			return true
		} else if strings.ToLower(val) == "false" {
			return false
		}
	}
	return defaultValue
}

// GetStringVal retrieves a string value from given environment envVar
// Returns default value if envVar is not set.
func GetStringVal(envVar string, defaultValue string) string {
	if val := os.Getenv(envVar); val != "" {
		return val
	} else {
		return defaultValue
	}
}

// Helper function to parse a number from an environment variable. Returns a
// default if env is not set, is not parseable to a number, exceeds max (if
// max is greater than 0) or is less than min.
//
// nolint:unparam
func ParseNumFromEnv(env string, defaultValue, min, max int) int {
	str := os.Getenv(env)
	if str == "" {
		return defaultValue
	}
	num, err := strconv.ParseInt(str, 10, 0)
	if err != nil {
		log.Warnf("Could not parse '%s' as a number from environment %s", str, env)
		return defaultValue
	}
	if num > math.MaxInt || num < math.MinInt {
		log.Warnf("Value in %s is %d is outside of the min and max %d allowed values. Using default %d", env, num, min, defaultValue)
		return defaultValue
	}
	if int(num) < min {
		log.Warnf("Value in %s is %d, which is less than minimum %d allowed", env, num, min)
		return defaultValue
	}
	if int(num) > max {
		log.Warnf("Value in %s is %d, which is greater than maximum %d allowed", env, num, max)
		return defaultValue
	}
	return int(num)
}
