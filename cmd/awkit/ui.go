package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/silver2dream/ai-workflow-kit/internal/buildinfo"
	"github.com/silver2dream/ai-workflow-kit/internal/workbench"
)

func usageUI() {
	fmt.Fprint(os.Stderr, `Open the AWK workbench in your browser

Usage:
  awkit ui [options] [project folder]

The workbench is a page on this computer for following the work: where
every ticket is, what needs you, what each run did, and what awkit doctor
finds. Plans are made, runs started and stopped, and the settings changed
on it too, as awkit plan, awkit run, awkit stop and awkit settings do. It
listens on 127.0.0.1 only, with a new token each time it starts.

The project of the folder it starts in, or the one named, is added to it;
others are added from the page. Closing the workbench does not stop work
that is going on: that ends when it is done, or when someone stops it.

Options:
  --port N      The port to listen on (default 4747; another free one when
                it is taken)
  --no-open     Print the address without opening a browser
  -h, --help    Show this help

What you have learned of the workbench, and which projects you opened, is
kept in your own settings folder, not in a project.
`)
}

// openBrowser opens an address in the person's browser.
var openBrowser = func(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

// serveWorkbench serves until the context ends; a variable so tests do not
// wait on it.
var serveWorkbench = func(ctx context.Context, server *workbench.Server, port int, opened func(url string)) error {
	ln, err := server.Listen(port)
	if err != nil {
		return err
	}
	opened(server.URL())
	return server.Serve(ctx, ln)
}

func cmdUI(args []string) int {
	fs := flag.NewFlagSet("ui", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageUI
	port := fs.Int("port", 4747, "")
	noOpen := fs.Bool("no-open", false, "")
	showHelp := fs.Bool("help", false, "")
	showHelpShort := fs.Bool("h", false, "")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showHelp || *showHelpShort {
		usageUI()
		return 0
	}
	if fs.NArg() > 1 {
		errorf("name one project folder at most\n")
		return 2
	}

	settings, err := workbench.DefaultSettingsPath()
	if err != nil {
		errorf("%v\n", err)
		return 1
	}
	server, err := workbench.New(workbench.Options{Store: workbench.NewStore(settings), Assets: workbench.Assets(), Version: buildinfo.Version})
	if err != nil {
		errorf("%v\n", err)
		return 1
	}

	// The project it starts in is added; one that is named must be one.
	dir := "."
	if fs.NArg() == 1 {
		dir = fs.Arg(0)
	}
	if root, err := workbench.ResolveProject(dir); err == nil {
		if _, err := server.AddProject(root); err != nil {
			errorf("%v\n", err)
			return 1
		}
	} else if fs.NArg() == 1 {
		errorf("%v\n", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err = serveWorkbench(ctx, server, *port, func(url string) {
		fmt.Printf("AWK workbench: %s\n", url)
		fmt.Println("Press Ctrl+C to close it. Work that is going on goes on.")
		if workbench.Assets() == nil {
			warn("this awkit was built without the workbench's page; the address shows how to build it\n")
		}
		if !*noOpen {
			if err := openBrowser(url); err != nil {
				warn("the browser could not be opened (%v): open the address yourself\n", err)
			}
		}
	})
	if err != nil {
		errorf("the workbench cannot listen: %v\n", err)
		return 1
	}
	return 0
}
