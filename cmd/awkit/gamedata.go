package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/silver2dream/ai-workflow-kit/internal/gamedata"
)

func usageGamedata() {
	fmt.Fprint(os.Stderr, `Check the game data, and show it for a person to review

Usage:
  awkit gamedata check [--state-root <path>]
  awkit gamedata review copy|tables [--out <file>] [--state-root <path>]

The game data is the numbers and the text the game reads, kept as Luban
reads it, in the folder of the project's luban.conf (gamedata/ in a game the
kit sets up): the schema in Defines/, one table per CSV file in Datas/, and the
text the player reads in Datas/text/, by key.

Commands:
  check    What Luban does not check: a number or a truth value left blank,
           which Luban reads as 0 or false; columns that are not the fields
           of their schema; a table or a field the client gets without its
           group marked; an odds or a weight the client gets; a text key the
           text tables lack; and the text tables themselves: each key once and
           well formed, the source text written, each translation there, up
           to date with the source, within its limit, with the same
           placeholders and markup. Exits 1 when it finds anything.
  review   Write a page for a person to read and review, in Markdown:
             copy    the text, every language side by side, where each key
                     is used, and what to look at
             tables  each table: what it is, which side has it, what each
                     column holds, its rows, each text key with its text
           To standard output, or to the file --out names.

Options:
  --out <file>         Write the page to a file
  --state-root <path>  The project's root (default: the git repository's)

awkit plan shows what a plan wrote to the game data on the same two pages, in
the plan's folder.
`)
}

func cmdGamedata(args []string) int {
	if len(args) == 0 {
		usageGamedata()
		return 2
	}
	command, rest := args[0], args[1:]
	switch command {
	case "--help", "-h", "help":
		usageGamedata()
		return 0
	case "check", "review":
	default:
		errorf("unknown gamedata command: %s\n\n", command)
		usageGamedata()
		return 2
	}

	fs := flag.NewFlagSet("gamedata "+command, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageGamedata
	stateRoot := fs.String("state-root", "", "")
	out := fs.String("out", "", "")
	showHelp := fs.Bool("help", false, "")
	showHelpShort := fs.Bool("h", false, "")
	flags, words := splitFlags(rest, map[string]bool{"--state-root": true, "-state-root": true, "--out": true, "-out": true})
	if err := fs.Parse(flags); err != nil {
		return 2
	}
	if *showHelp || *showHelpShort {
		usageGamedata()
		return 0
	}
	page := ""
	switch {
	case command == "check" && (len(words) > 0 || *out != ""):
		errorf("gamedata check takes no page and no --out\n\n")
		usageGamedata()
		return 2
	case command == "review" && len(words) != 1:
		errorf("gamedata review shows one page: copy or tables\n\n")
		usageGamedata()
		return 2
	case command == "review":
		page = words[0]
		if page != "copy" && page != "tables" {
			errorf("gamedata review shows copy or tables, not %s\n\n", page)
			usageGamedata()
			return 2
		}
	}

	root, code := planRoot(*stateRoot)
	if code != 0 {
		return code
	}
	project := os.DirFS(root)
	data, err := gamedata.Find(project)
	if err != nil {
		errorf("%v\n", err)
		return 1
	}
	if data == nil {
		errorf("the project keeps no game data: no folder at its root holds a %s\n", gamedata.ConfFile)
		return 1
	}
	d, findings, err := gamedata.Check(project, data)
	if err != nil {
		errorf("the game data cannot be read: %v\n", err)
		return 1
	}

	if command == "review" {
		var text string
		if page == "copy" {
			text = "# The copy\n\n" + d.CopyReview(nil, findings)
		} else {
			text = "# The tables\n\n" + d.DataReview(nil, findings)
		}
		if *out == "" {
			fmt.Print(text)
			return 0
		}
		if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
			errorf("%v\n", err)
			return 1
		}
		if err := os.WriteFile(*out, []byte(text), 0o644); err != nil {
			errorf("%v\n", err)
			return 1
		}
		success("Wrote %s\n", *out)
		return 0
	}

	keys := 0
	for _, text := range d.Texts {
		keys += len(text.Rows)
	}
	for _, finding := range findings {
		fmt.Println(finding)
	}
	if len(findings) > 0 {
		fmt.Println("")
		errorf("%d point(s) in the game data (%s): %d tables, %d text keys\n", len(findings), data.Dir, len(d.Tables), keys)
		return 1
	}
	success("The game data (%s) passes its check: %d tables, %d text keys\n", data.Dir, len(d.Tables), keys)
	return 0
}
