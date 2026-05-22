package log

import (
	"os"
	"testing"
)

func TestInit(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-log-*.log")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	if err := Init("debug", tmpFile.Name(), 7); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	Info("test info message")
	Debug("test debug message")
	Error("test error message")

	Infof("test infof %s", "message")
	Errorf("test errorf %s", "message")

	Infow("test infow", "key1", "value1", "key2", 42)
	Errorw("test errorw", "key1", "value1")

	data, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	if len(data) == 0 {
		t.Error("log file is empty")
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"debug", "debug"},
		{"info", "info"},
		{"warn", "warn"},
		{"error", "error"},
		{"unknown", "info"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			level := parseLevel(tt.input)
			if level.String() != tt.want {
				t.Errorf("parseLevel(%q) = %v, want %v", tt.input, level, tt.want)
			}
		})
	}
}
