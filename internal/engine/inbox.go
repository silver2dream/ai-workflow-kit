package engine

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

// The command channel (design.md §5.9). A person, the workbench or the CLI
// writes a command into the engine's inbox, a file of its own; the engine
// takes the commands in the order they were written, and answers each in its
// outbox: applied, or rejected and why. Every command is an event. P1 takes
// the two stops, and 2.5 the owner's approval and return of a high-risk
// change (approval.go); the other kinds come with the workbench's controls
// (task 2.6), and are rejected until then.

// Where the commands and their replies are, under the project.
const (
	InboxDir  = layout.StateDir + "/engine/inbox"
	OutboxDir = layout.StateDir + "/engine/outbox"
)

// Command kinds.
const (
	// CommandStop stops the run when the step it is on is done.
	CommandStop = "stop"
	// CommandStopNow stops it at once: what is running is ended with it.
	CommandStopNow = "stop_now"
)

// What became of a command.
const (
	Applied  = "applied"
	Rejected = "rejected"
)

// TypeCommand is the event of a command and what became of it.
const TypeCommand = "engine_command"

// Command is one instruction to the engine.
type Command struct {
	ID    string            `json:"id"`
	Kind  string            `json:"kind"`
	Issue int               `json:"issue,omitempty"`
	Args  map[string]string `json:"args,omitempty"`
	// By says who gave it: "awkit stop", "workbench", a person's login.
	By string    `json:"by"`
	At time.Time `json:"at"`
}

// Reply is what became of a command.
type Reply struct {
	ID     string    `json:"id"`
	Kind   string    `json:"kind"`
	Result string    `json:"result"`
	Reason string    `json:"reason,omitempty"`
	At     time.Time `json:"at"`
}

// Send puts a command in a project's inbox, and gives it an id and a time
// when it has none. The id sorts in the order commands are written.
func Send(root string, c Command) (Command, error) {
	if c.At.IsZero() {
		c.At = time.Now().UTC()
	}
	if c.ID == "" {
		suffix := make([]byte, 4)
		_, _ = rand.Read(suffix)
		at := c.At.UTC()
		c.ID = fmt.Sprintf("%s%09dZ-%s", at.Format("20060102T150405"), at.Nanosecond(), hex.EncodeToString(suffix))
	}
	if strings.ContainsAny(c.ID, `/\.`) {
		return c, fmt.Errorf("a command's id %q cannot name a file", c.ID)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return c, err
	}
	return c, writeAtomic(filepath.Join(root, filepath.FromSlash(InboxDir)), c.ID+".json", data)
}

// Pending are the commands in a project's inbox, oldest first. A file that
// is not a command is given back with only its id, and Kind "".
func Pending(root string) ([]Command, error) {
	dir := filepath.Join(root, filepath.FromSlash(InboxDir))
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var commands []Command
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".json") {
			continue
		}
		id := strings.TrimSuffix(name, ".json")
		var c Command
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || json.Unmarshal(data, &c) != nil {
			c = Command{}
		}
		c.ID = id
		commands = append(commands, c)
	}
	sort.Slice(commands, func(i, j int) bool { return commands[i].ID < commands[j].ID })
	return commands, nil
}

// answer writes what became of a command to the outbox, and takes it out of
// the inbox.
func answer(root string, c Command, result, reason string, at time.Time) (Reply, error) {
	r := Reply{ID: c.ID, Kind: c.Kind, Result: result, Reason: reason, At: at.UTC()}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return r, err
	}
	if err := writeAtomic(filepath.Join(root, filepath.FromSlash(OutboxDir)), c.ID+".json", data); err != nil {
		return r, err
	}
	return r, os.Remove(filepath.Join(root, filepath.FromSlash(InboxDir), c.ID+".json"))
}

// ReplyOf is what became of a command, once the engine took it.
func ReplyOf(root, id string) (Reply, bool) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(OutboxDir), id+".json"))
	if err != nil {
		return Reply{}, false
	}
	var r Reply
	if json.Unmarshal(data, &r) != nil {
		return Reply{}, false
	}
	return r, true
}

// askPoll is how often Ask looks for the answer.
const askPoll = 200 * time.Millisecond

// Ask gives the engine a command, and waits up to wait for its answer:
// answered is false when it has not answered by then. The engine looks at
// its inbox every two seconds.
func Ask(root string, c Command, wait time.Duration) (sent Command, reply Reply, answered bool, err error) {
	sent, err = Send(root, c)
	if err != nil {
		return sent, Reply{}, false, err
	}
	deadline := time.Now().Add(wait)
	for {
		if r, ok := ReplyOf(root, sent.ID); ok {
			return sent, r, true, nil
		}
		if !time.Now().Before(deadline) {
			return sent, Reply{}, false, nil
		}
		time.Sleep(askPoll)
	}
}

// writeAtomic writes a file whole or not at all: a reader never sees half
// of it.
func writeAtomic(dir, name string, data []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".write-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, name)); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}
