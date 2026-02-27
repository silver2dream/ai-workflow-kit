//go:build windows

package hooks

import "os/exec"

// setProcGroup is a no-op on Windows.
// exec.CommandContext already handles process killing on Windows.
func setProcGroup(cmd *exec.Cmd) {}
