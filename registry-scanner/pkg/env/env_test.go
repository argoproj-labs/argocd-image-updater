package env

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_GetBoolVal(t *testing.T) {
	t.Run("Get 'true' value from existing env var", func(t *testing.T) {
		_ = os.Setenv("TEST_BOOL_VAL", "true")
		defer os.Setenv("TEST_BOOL_VAL", "")
		assert.True(t, GetBoolVal("TEST_BOOL_VAL", false))
	})
	t.Run("Get 'false' value from existing env var", func(t *testing.T) {
		_ = os.Setenv("TEST_BOOL_VAL", "false")
		defer os.Setenv("TEST_BOOL_VAL", "")
		assert.False(t, GetBoolVal("TEST_BOOL_VAL", true))
	})
	t.Run("Get default value from non-existing env var", func(t *testing.T) {
		_ = os.Setenv("TEST_BOOL_VAL", "")
		assert.True(t, GetBoolVal("TEST_BOOL_VAL", true))
	})
}

func Test_GetStringVal(t *testing.T) {
	t.Run("Get string value from existing env var", func(t *testing.T) {
		_ = os.Setenv("TEST_STRING_VAL", "test")
		defer os.Setenv("TEST_STRING_VAL", "")
		assert.Equal(t, "test", GetStringVal("TEST_STRING_VAL", "invalid"))
	})
	t.Run("Get default value from non-existing env var", func(t *testing.T) {
		_ = os.Setenv("TEST_STRING_VAL", "")
		defer os.Setenv("TEST_STRING_VAL", "")
		assert.Equal(t, "invalid", GetStringVal("TEST_STRING_VAL", "invalid"))
	})
}

func Test_ParseNumFromEnv(t *testing.T) {
	t.Run("Get number from existing env var within range", func(t *testing.T) {
		_ = os.Setenv("TEST_NUM_VAL", "5")
		defer os.Setenv("TEST_NUM_VAL", "")
		assert.Equal(t, 5, ParseNumFromEnv("TEST_NUM_VAL", 0, 1, 10))
	})
	t.Run("Get default value from non-existing env var", func(t *testing.T) {
		_ = os.Setenv("TEST_NUM_VAL", "")
		assert.Equal(t, 10, ParseNumFromEnv("TEST_NUM_VAL", 10, 1, 20))
	})
	t.Run("Get default value from env var with non-numeric value", func(t *testing.T) {
		_ = os.Setenv("TEST_NUM_VAL", "abc")
		defer os.Setenv("TEST_NUM_VAL", "")
		assert.Equal(t, 10, ParseNumFromEnv("TEST_NUM_VAL", 10, 1, 20))
	})
	t.Run("Get default value from env var with value less than min", func(t *testing.T) {
		_ = os.Setenv("TEST_NUM_VAL", "0")
		defer os.Setenv("TEST_NUM_VAL", "")
		assert.Equal(t, 10, ParseNumFromEnv("TEST_NUM_VAL", 10, 1, 20))
	})
	t.Run("Get default value from env var with value greater than max", func(t *testing.T) {
		_ = os.Setenv("TEST_NUM_VAL", "30")
		defer os.Setenv("TEST_NUM_VAL", "")
		assert.Equal(t, 10, ParseNumFromEnv("TEST_NUM_VAL", 10, 1, 20))
	})
}
