package council

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// ApprovalRecord is where an approval that brings the plan's documents to
// the integration branch is: `awkit plan approve --merge-docs` writes it as
// it goes, the workbench follows it. It is in the plan's state folder.
type ApprovalRecord struct {
	// PID is the process of the approval.
	PID int `json:"pid"`
	// State is working, done or failed.
	State string `json:"state"`
	// Step is where it is, or stopped: check, issues, documents, checks,
	// label, sync, done.
	Step string `json:"step"`
	// Why names, for a page, how it stopped when it failed: run (a run is
	// going), branch (the integration branch is not on origin), checks (the
	// pull request's checks failed), noci (the project has CI for pull
	// requests, and GitHub ran none for this one), slow (the checks took too
	// long), refused (GitHub would not merge), or error.
	Why   string `json:"why,omitempty"`
	Error string `json:"error,omitempty"`
	// Created counts the issues this approval created; Issues are all the
	// issues of the plan's tickets.
	Created int   `json:"created"`
	Issues  []int `json:"issues"`
	// PR is the pull request the documents went through, when there was one.
	PR  int    `json:"pr,omitempty"`
	URL string `json:"url,omitempty"`
	// Left says why the working tree was not brought up to the integration
	// branch, when it was not.
	Left string `json:"left,omitempty"`
	// Lines are what the approval said, the last ones.
	Lines   []string  `json:"lines"`
	Started time.Time `json:"started"`
	Updated time.Time `json:"updated"`
}

// The states of an approval.
const (
	ApprovalWorking = "working"
	ApprovalDone    = "done"
	ApprovalFailed  = "failed"
)

// ApprovalFile is the name of the record in the plan's state folder.
const ApprovalFile = "approval.json"

// maxApprovalLines is how many lines of what an approval said are kept.
const maxApprovalLines = 40

// Say keeps a line of what the approval said.
func (r *ApprovalRecord) Say(line string) {
	r.Lines = append(r.Lines, line)
	if len(r.Lines) > maxApprovalLines {
		r.Lines = r.Lines[len(r.Lines)-maxApprovalLines:]
	}
}

// ReadApproval reads where the approval of a plan is.
func ReadApproval(root, id string) (*ApprovalRecord, error) {
	data, err := os.ReadFile(filepath.Join(StateDir(root, id), ApprovalFile))
	if err != nil {
		return nil, err
	}
	var record ApprovalRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

// WriteApproval writes where the approval of a plan is, whole: a reader never
// sees half of it.
func WriteApproval(root, id string, record *ApprovalRecord) error {
	record.Updated = time.Now()
	if record.Lines == nil {
		record.Lines = []string{}
	}
	if record.Issues == nil {
		record.Issues = []int{}
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	dir := StateDir(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	file := filepath.Join(dir, ApprovalFile)
	temp := file + ".tmp"
	if err := os.WriteFile(temp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(temp, file)
}
