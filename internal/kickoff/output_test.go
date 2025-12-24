package kickoff

import (
	"bytes"
	"strings"
	"testing"
)

// TestNewOutputFormatter tests formatter creation
func TestNewOutputFormatter(t *testing.T) {
	var buf bytes.Buffer
	formatter := NewOutputFormatter(&buf)

	if formatter == nil {
		t.Fatal("NewOutputFormatter returned nil")
	}

	if formatter.writer != &buf {
		t.Error("writer not set correctly")
	}
}

// TestNewOutputFormatter_NilWriter tests nil writer defaults to stdout
func TestNewOutputFormatter_NilWriter(t *testing.T) {
	formatter := NewOutputFormatter(nil)

	if formatter == nil {
		t.Fatal("NewOutputFormatter returned nil")
	}

	// Should not panic
	formatter.Info("test")
}

// TestOutputFormatter_Success tests success message output
func TestOutputFormatter_Success(t *testing.T) {
	var buf bytes.Buffer
	formatter := &OutputFormatter{writer: &buf, useColors: false}

	formatter.Success("Operation completed")

	output := buf.String()
	if !strings.Contains(output, "✓") {
		t.Error("Success should contain checkmark")
	}
	if !strings.Contains(output, "Operation completed") {
		t.Error("Success should contain message")
	}
}

// TestOutputFormatter_Error tests error message output
func TestOutputFormatter_Error(t *testing.T) {
	var buf bytes.Buffer
	formatter := &OutputFormatter{writer: &buf, useColors: false}

	formatter.Error("Something failed")

	output := buf.String()
	if !strings.Contains(output, "✗") {
		t.Error("Error should contain cross mark")
	}
	if !strings.Contains(output, "Something failed") {
		t.Error("Error should contain message")
	}
}

// TestOutputFormatter_Warning tests warning message output
func TestOutputFormatter_Warning(t *testing.T) {
	var buf bytes.Buffer
	formatter := &OutputFormatter{writer: &buf, useColors: false}

	formatter.Warning("Be careful")

	output := buf.String()
	if !strings.Contains(output, "⚠️") {
		t.Error("Warning should contain warning sign")
	}
	if !strings.Contains(output, "Be careful") {
		t.Error("Warning should contain message")
	}
}

// TestOutputFormatter_Info tests info message output
func TestOutputFormatter_Info(t *testing.T) {
	var buf bytes.Buffer
	formatter := &OutputFormatter{writer: &buf, useColors: false}

	formatter.Info("Information message")

	output := buf.String()
	if !strings.Contains(output, "Information message") {
		t.Error("Info should contain message")
	}
}

// TestOutputFormatter_WorkerMessage tests worker message format
func TestOutputFormatter_WorkerMessage(t *testing.T) {
	var buf bytes.Buffer
	formatter := &OutputFormatter{writer: &buf, useColors: false}

	formatter.WorkerMessage(42, "worker_start")

	output := buf.String()
	if !strings.Contains(output, "[#42]") {
		t.Error("WorkerMessage should contain issue ID")
	}
	if !strings.Contains(output, "Worker:") {
		t.Error("WorkerMessage should contain 'Worker:'")
	}
	if !strings.Contains(output, "worker_start") {
		t.Error("WorkerMessage should contain message")
	}
}

// TestOutputFormatter_WorkerComplete tests worker completion message
func TestOutputFormatter_WorkerComplete(t *testing.T) {
	var buf bytes.Buffer
	formatter := &OutputFormatter{writer: &buf, useColors: false}

	formatter.WorkerComplete(42, "45s")

	output := buf.String()
	if !strings.Contains(output, "✓") {
		t.Error("WorkerComplete should contain checkmark")
	}
	if !strings.Contains(output, "[#42]") {
		t.Error("WorkerComplete should contain issue ID")
	}
	if !strings.Contains(output, "45s") {
		t.Error("WorkerComplete should contain duration")
	}
}

// TestOutputFormatter_WorkerTimeout tests worker timeout warning
func TestOutputFormatter_WorkerTimeout(t *testing.T) {
	var buf bytes.Buffer
	formatter := &OutputFormatter{writer: &buf, useColors: false}

	formatter.WorkerTimeout(42, "worker_start")

	output := buf.String()
	if !strings.Contains(output, "[#42]") {
		t.Error("WorkerTimeout should contain issue ID")
	}
	if !strings.Contains(output, "timeout") {
		t.Error("WorkerTimeout should contain 'timeout'")
	}
	if !strings.Contains(output, "30 分鐘") {
		t.Error("WorkerTimeout should mention 30 minutes")
	}
}

// TestOutputFormatter_Bold tests bold formatting
func TestOutputFormatter_Bold(t *testing.T) {
	tests := []struct {
		name      string
		useColors bool
		input     string
		wantBold  bool
	}{
		{"with colors", true, "test", true},
		{"without colors", false, "test", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := &OutputFormatter{useColors: tt.useColors}
			result := formatter.Bold(tt.input)

			hasBold := strings.Contains(result, colorBold)
			if hasBold != tt.wantBold {
				t.Errorf("Bold() hasBold=%v, want %v", hasBold, tt.wantBold)
			}

			if !strings.Contains(result, tt.input) {
				t.Error("Bold should contain original text")
			}
		})
	}
}

// TestOutputFormatter_Cyan tests cyan formatting
func TestOutputFormatter_Cyan(t *testing.T) {
	tests := []struct {
		name      string
		useColors bool
		input     string
		wantCyan  bool
	}{
		{"with colors", true, "test", true},
		{"without colors", false, "test", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := &OutputFormatter{useColors: tt.useColors}
			result := formatter.Cyan(tt.input)

			hasCyan := strings.Contains(result, colorCyan)
			if hasCyan != tt.wantCyan {
				t.Errorf("Cyan() hasCyan=%v, want %v", hasCyan, tt.wantCyan)
			}

			if !strings.Contains(result, tt.input) {
				t.Error("Cyan should contain original text")
			}
		})
	}
}

// TestOutputFormatter_ColorsEnabled tests color output
func TestOutputFormatter_ColorsEnabled(t *testing.T) {
	var buf bytes.Buffer
	formatter := &OutputFormatter{writer: &buf, useColors: true}

	formatter.Success("test")

	output := buf.String()
	if !strings.Contains(output, colorGreen) {
		t.Error("Success with colors should contain green ANSI code")
	}
	if !strings.Contains(output, colorReset) {
		t.Error("Success with colors should contain reset ANSI code")
	}
}

// TestOutputFormatter_ColorsDisabled tests non-color output
func TestOutputFormatter_ColorsDisabled(t *testing.T) {
	var buf bytes.Buffer
	formatter := &OutputFormatter{writer: &buf, useColors: false}

	formatter.Success("test")

	output := buf.String()
	if strings.Contains(output, "\033[") {
		t.Error("Success without colors should not contain ANSI codes")
	}
}
