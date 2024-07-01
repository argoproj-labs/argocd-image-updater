package main

import (
	"testing"
	"time"
)

func TestGetPrintableInterval(t *testing.T) {
	tests := []struct {
		input    time.Duration
		expected string
	}{
		{0, "once"},
		{time.Second, "1s"},
		{time.Minute, "1m0s"},
		{time.Hour, "1h0m0s"},
		{time.Hour + 30*time.Minute, "1h30m0s"},
		{24 * time.Hour, "24h0m0s"},
	}

	for _, test := range tests {
		result := getPrintableInterval(test.input)
		if result != test.expected {
			t.Errorf("For input %v, expected %v, but got %v", test.input, test.expected, result)
		}
	}
}

func TestGetPrintableHealthPort(t *testing.T) {
	testPorts := []struct {
		input    int
		expected string
	}{
		{0, "off"},
		{8080, "8080"},
		{9090, "9090"},
	}

	for _, testPort := range testPorts {
		result := getPrintableHealthPort(testPort.input)

		if result != testPort.expected {
			t.Errorf("For input %v, expected %v, but got %v", testPort.input, testPort.expected, result)
		}
	}

}
