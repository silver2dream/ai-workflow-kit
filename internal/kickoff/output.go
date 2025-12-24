package kickoff

import (
	"fmt"
	"io"
	"os"
	"runtime"

	"golang.org/x/term"
)

// OutputFormatter handles formatted console output with colors
type OutputFormatter struct {
	writer    io.Writer
	useColors bool
}

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

// NewOutputFormatter creates a new OutputFormatter
func NewOutputFormatter(w io.Writer) *OutputFormatter {
	if w == nil {
		w = os.Stdout
	}

	// Detect if colors should be used
	useColors := true

	// Disable colors on Windows (unless using Windows Terminal)
	if runtime.GOOS == "windows" && os.Getenv("WT_SESSION") == "" {
		useColors = false
	}

	// Disable colors if NO_COLOR is set
	if os.Getenv("NO_COLOR") != "" {
		useColors = false
	}

	// Disable colors if not a TTY
	if f, ok := w.(*os.File); ok {
		if !term.IsTerminal(int(f.Fd())) {
			useColors = false
		}
	}

	return &OutputFormatter{
		writer:    w,
		useColors: useColors,
	}
}

// Success prints a success message with green checkmark
func (o *OutputFormatter) Success(msg string) {
	if o.useColors {
		fmt.Fprintf(o.writer, "%s✓%s %s\n", colorGreen, colorReset, msg)
	} else {
		fmt.Fprintf(o.writer, "✓ %s\n", msg)
	}
}

// Error prints an error message with red cross
func (o *OutputFormatter) Error(msg string) {
	if o.useColors {
		fmt.Fprintf(o.writer, "%s✗%s %s\n", colorRed, colorReset, msg)
	} else {
		fmt.Fprintf(o.writer, "✗ %s\n", msg)
	}
}

// Warning prints a warning message with yellow warning sign
func (o *OutputFormatter) Warning(msg string) {
	if o.useColors {
		fmt.Fprintf(o.writer, "%s⚠️%s %s\n", colorYellow, colorReset, msg)
	} else {
		fmt.Fprintf(o.writer, "⚠️ %s\n", msg)
	}
}

// Info prints an info message
func (o *OutputFormatter) Info(msg string) {
	fmt.Fprintln(o.writer, msg)
}

// WorkerMessage prints a Worker progress message
func (o *OutputFormatter) WorkerMessage(issueID int, msg string) {
	fmt.Fprintf(o.writer, "[#%d] Worker: %s\n", issueID, msg)
}

// WorkerComplete prints a Worker completion message
func (o *OutputFormatter) WorkerComplete(issueID int, duration string) {
	if o.useColors {
		fmt.Fprintf(o.writer, "%s✓%s [#%d] Worker 完成 (%s)\n",
			colorGreen, colorReset, issueID, duration)
	} else {
		fmt.Fprintf(o.writer, "✓ [#%d] Worker 完成 (%s)\n", issueID, duration)
	}
}

// WorkerTimeout prints a Worker timeout warning
func (o *OutputFormatter) WorkerTimeout(issueID int, lastStatus string) {
	msg := fmt.Sprintf("[#%d] Worker timeout: 30 分鐘無回應，最後狀態: %s", issueID, lastStatus)
	o.Warning(msg)
}

// Bold returns the string wrapped in bold formatting
func (o *OutputFormatter) Bold(s string) string {
	if o.useColors {
		return colorBold + s + colorReset
	}
	return s
}

// Cyan returns the string wrapped in cyan formatting
func (o *OutputFormatter) Cyan(s string) string {
	if o.useColors {
		return colorCyan + s + colorReset
	}
	return s
}
