package main

import (
	"fmt"
	"os"

	"github.com/silver2dream/ai-workflow-kit/internal/workbench"
)

// cmdDetach starts awkit with the arguments after -- in a process of its own,
// in the folder it runs in, says its pid, and ends at once: the process it
// started has no parent that is still there. The workbench starts work this
// way so that closing it does not end the work. It is not for a person.
//
//	awkit detach --log <file> -- <args>
func cmdDetach(args []string) int {
	if len(args) < 4 || args[0] != "--log" || args[2] != "--" {
		errorf("awkit detach --log <file> -- <args>: the workbench starts work with it\n")
		return 2
	}
	dir, err := os.Getwd()
	if err != nil {
		errorf("%v\n", err)
		return 1
	}
	pid, err := workbench.StartAlone(dir, args[3:], args[1])
	if err != nil {
		errorf("%v\n", err)
		return 1
	}
	fmt.Println(pid)
	return 0
}
