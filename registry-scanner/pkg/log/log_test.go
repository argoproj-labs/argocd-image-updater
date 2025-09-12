package log

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/test/fixture"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_LogToStdout(t *testing.T) {
	// We need tracing level
	Log().SetLevel(logrus.TraceLevel)

	t.Run("Test for Tracef() to log to stdout", func(t *testing.T) {
		out, err := fixture.CaptureStdout(func() {
			Tracef("this is a test")
		})
		require.NoError(t, err)
		assert.Contains(t, out, "this is a test")
		assert.Contains(t, out, "level=trace")
	})
	t.Run("Test for Debugf() to log to stdout", func(t *testing.T) {
		out, err := fixture.CaptureStdout(func() {
			Debugf("this is a test")
		})
		require.NoError(t, err)
		assert.Contains(t, out, "this is a test")
		assert.Contains(t, out, "level=debug")
	})
	t.Run("Test for Infof() to log to stdout", func(t *testing.T) {
		out, err := fixture.CaptureStdout(func() {
			Infof("this is a test")
		})
		require.NoError(t, err)
		assert.Contains(t, out, "this is a test")
		assert.Contains(t, out, "level=info")
	})
	t.Run("Test for Warnf() to log to stdout", func(t *testing.T) {
		out, err := fixture.CaptureStdout(func() {
			Warnf("this is a test")
		})
		require.NoError(t, err)
		assert.Contains(t, out, "this is a test")
		assert.Contains(t, out, "level=warn")
	})
	t.Run("Test for Errorf() to not log to stdout", func(t *testing.T) {
		out, err := fixture.CaptureStdout(func() {
			Errorf("this is a test")
		})
		require.NoError(t, err)
		assert.Empty(t, out)
	})
}

func Test_LogToStderr(t *testing.T) {
	// We need tracing level
	Log().SetLevel(logrus.TraceLevel)

	t.Run("Test for Tracef() to log to stdout", func(t *testing.T) {
		out, err := fixture.CaptureStderr(func() {
			Tracef("this is a test")
		})
		require.NoError(t, err)
		assert.Empty(t, out)
	})
	t.Run("Test for Debugf() to log to stdout", func(t *testing.T) {
		out, err := fixture.CaptureStderr(func() {
			Debugf("this is a test")
		})
		require.NoError(t, err)
		assert.Empty(t, out)
	})
	t.Run("Test for Infof() to log to stdout", func(t *testing.T) {
		out, err := fixture.CaptureStderr(func() {
			Infof("this is a test")
		})
		require.NoError(t, err)
		assert.Empty(t, out)
	})
	t.Run("Test for Warnf() to log to stdout", func(t *testing.T) {
		out, err := fixture.CaptureStderr(func() {
			Warnf("this is a test")
		})
		require.NoError(t, err)
		assert.Empty(t, out)
	})
	t.Run("Test for Errorf() to not log to stdout", func(t *testing.T) {
		out, err := fixture.CaptureStderr(func() {
			Errorf("this is a test")
		})
		require.NoError(t, err)
		assert.Contains(t, out, "this is a test")
		assert.Contains(t, out, "level=error")
	})
}

func Test_LoggerFields(t *testing.T) {
	Log().SetLevel(logrus.TraceLevel)
	t.Run("Test for Tracef() to log correctly with fields", func(t *testing.T) {
		out, err := fixture.CaptureStdout(func() {
			WithContext().AddField("foo", "bar").Tracef("this is a test")
		})
		require.NoError(t, err)
		assert.Contains(t, out, "foo=bar")
		assert.Contains(t, out, "msg=\"this is a test\"")
	})
	t.Run("Test for Debugf() to log correctly with fields", func(t *testing.T) {
		out, err := fixture.CaptureStdout(func() {
			WithContext().AddField("foo", "bar").Debugf("this is a test")
		})
		require.NoError(t, err)
		assert.Contains(t, out, "foo=bar")
		assert.Contains(t, out, "msg=\"this is a test\"")
	})
	t.Run("Test for Infof() to log correctly with fields", func(t *testing.T) {
		out, err := fixture.CaptureStdout(func() {
			WithContext().AddField("foo", "bar").Infof("this is a test")
		})
		require.NoError(t, err)
		assert.Contains(t, out, "foo=bar")
		assert.Contains(t, out, "msg=\"this is a test\"")
	})
	t.Run("Test for Warnf() to log correctly with fields", func(t *testing.T) {
		out, err := fixture.CaptureStdout(func() {
			WithContext().AddField("foo", "bar").Warnf("this is a test")
		})
		require.NoError(t, err)
		assert.Contains(t, out, "foo=bar")
		assert.Contains(t, out, "msg=\"this is a test\"")
	})
	t.Run("Test for Errorf() to log correctly with fields", func(t *testing.T) {
		out, err := fixture.CaptureStderr(func() {
			WithContext().AddField("foo", "bar").Errorf("this is a test")
		})
		require.NoError(t, err)
		assert.Contains(t, out, "foo=bar")
		assert.Contains(t, out, "msg=\"this is a test\"")
	})
}

func Test_LogLevel(t *testing.T) {
	for _, level := range []string{"trace", "debug", "info", "warn", "error"} {
		t.Run(fmt.Sprintf("Test set loglevel %s", level), func(t *testing.T) {
			err := SetLogLevel(level)
			assert.NoError(t, err)
		})
	}
	t.Run("Test set invalid loglevel", func(t *testing.T) {
		err := SetLogLevel("invalid")
		assert.Error(t, err)
	})
}

func Test_LogFormatJSON(t *testing.T) {
	// We need tracing level
	Log().SetLevel(logrus.TraceLevel)

	t.Run("Test set text log format", func(t *testing.T) {
		SetLogFormat(LogFormatText)
		out, err := fixture.CaptureStdout(func() {
			WithContext().AddField("foo", "bar").Infof("this is a test")
		})
		assert.NoError(t, err)
		assert.Contains(t, out, "foo=bar")
		assert.Contains(t, out, "msg=\"this is a test\"")
	})
	t.Run("Test set JSON log format", func(t *testing.T) {
		SetLogFormat(LogFormatJSON)
		out, err := fixture.CaptureStdout(func() {
			WithContext().AddField("foo", "bar").Warnf("this is a test")
		})
		assert.NoError(t, err)

		var data map[string]any
		err = json.Unmarshal([]byte(out), &data)
		assert.NoError(t, err)
		assert.Equal(t, "bar", data["foo"])
		assert.Equal(t, "this is a test", data["msg"])
	})
}
