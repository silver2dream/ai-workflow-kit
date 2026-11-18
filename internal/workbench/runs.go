package workbench

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/council"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	"github.com/silver2dream/ai-workflow-kit/internal/status"
	"github.com/silver2dream/ai-workflow-kit/internal/stopmark"
)

// A run of a project is `awkit run`. The page starts one in a process of its
// own, as it does a meeting (spawnAwkit): closing the page or the workbench
// does not end it. It ends when the work is done or needs a person, or when
// it is asked to: once the step it is on is done, as `awkit stop` asks, or
// at once, which ends it and everything it started.

// runLog is where a run the page started writes what it says: what the
// terminal would have shown.
func runLog(root string) string { return filepath.Join(layout.Logs(root), "workbench-run.log") }

// runGoing reports whether a run works on the project, and its process.
func (s *Server) runGoing(root string) (bool, int) {
	report, err := status.Collect(root, status.Options{Now: s.opts.Now()})
	if err != nil || report.Run.State != "running" {
		return false, 0
	}
	return true, report.Run.PID
}

// approvalAnywhere names the plan of the project that is being approved, if
// one is: it labels the tickets a run would take up.
func (s *Server) approvalAnywhere(root string) string {
	for _, id := range planIDs(root) {
		if approval := s.approvalOf(root, id); approval != nil && approval.State == council.ApprovalWorking {
			return id
		}
	}
	return ""
}

func (s *Server) handleStartRun(w http.ResponseWriter, r *http.Request, root string) {
	var body struct {
		// Issue limits the run to one ticket; none when zero.
		Issue int `json:"issue"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Issue < 0 {
		writeError(w, http.StatusBadRequest, "a run is limited to a ticket by its issue number")
		return
	}
	if going, _ := s.runGoing(root); going {
		writeError(w, http.StatusConflict, "a run is going in this project already")
		return
	}
	if plan := s.meetingAnywhere(root); plan != "" {
		writeError(w, http.StatusConflict, "the meeting of plan "+plan+" is going on: it writes the documents a run starts from")
		return
	}
	if plan := s.approvalAnywhere(root); plan != "" {
		writeError(w, http.StatusConflict, "plan "+plan+" is being approved: its tickets are not ready yet")
		return
	}
	// What the last run the page started said is not this one's.
	_ = os.Remove(runLog(root))
	args := []string{"run"}
	if body.Issue > 0 {
		args = append(args, "--issue", strconv.Itoa(body.Issue))
	}
	if _, err := s.opts.Spawn(root, args, runLog(root)); err != nil {
		writeError(w, http.StatusInternalServerError, "the run cannot be started: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleStopRun(w http.ResponseWriter, r *http.Request, root string) {
	var body struct {
		// Now ends the run at once, and what it started with it; else it
		// ends once the step it is on is done.
		Now bool `json:"now"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	going, pid := s.runGoing(root)
	if !going {
		writeError(w, http.StatusConflict, "no run is going in this project")
		return
	}
	// The marker first: whatever of the run outlives the end, its next
	// step ends it.
	if err := stopmark.Write(root, "the workbench", stopmark.ByUser); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if body.Now {
		if err := s.opts.Kill(pid); err != nil {
			writeError(w, http.StatusInternalServerError, "the run cannot be ended at once: "+err.Error())
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// RunLogView is a log of a run that can be read: the run's own, the
// Principal's, or the Worker's of a ticket.
type RunLogView struct {
	Name    string    `json:"name"`
	Issue   int       `json:"issue,omitempty"`
	Size    int64     `json:"size"`
	Updated time.Time `json:"updated"`
}

var workerLogName = regexp.MustCompile(`^issue-(\d+)\.worker\.log$`)

// runLogFile is the file of a log by its name: run, principal, or issue-N.
func runLogFile(root, name string) (string, bool) {
	switch {
	case name == "run":
		return runLog(root), true
	case name == "principal":
		return layout.PrincipalLog(root), true
	case strings.HasPrefix(name, "issue-"):
		issue, err := strconv.Atoi(strings.TrimPrefix(name, "issue-"))
		if err != nil || issue <= 0 {
			return "", false
		}
		return layout.WorkerLog(root, issue), true
	}
	return "", false
}

func (s *Server) handleRunLogs(w http.ResponseWriter, r *http.Request, root string) {
	logs := []RunLogView{}
	add := func(name string, issue int, file string) {
		if info, err := os.Stat(file); err == nil && !info.IsDir() {
			logs = append(logs, RunLogView{Name: name, Issue: issue, Size: info.Size(), Updated: info.ModTime()})
		}
	}
	add("run", 0, runLog(root))
	add("principal", 0, layout.PrincipalLog(root))
	entries, _ := os.ReadDir(layout.Logs(root))
	var workers []RunLogView
	for _, entry := range entries {
		if match := workerLogName.FindStringSubmatch(entry.Name()); match != nil {
			issue, _ := strconv.Atoi(match[1])
			if info, err := entry.Info(); err == nil {
				workers = append(workers, RunLogView{Name: "issue-" + match[1], Issue: issue, Size: info.Size(), Updated: info.ModTime()})
			}
		}
	}
	// The Workers' logs newest first: the ticket being worked on leads.
	sort.Slice(workers, func(i, j int) bool { return workers[i].Updated.After(workers[j].Updated) })
	writeJSON(w, http.StatusOK, map[string]any{"logs": append(logs, workers...)})
}

// RunOutput is a piece of a log of a run, from where the page read last.
type RunOutput struct {
	Exists bool   `json:"exists"`
	Text   string `json:"text"`
	// Next is where the next piece starts.
	Next int64 `json:"next"`
	// Fresh says the text does not follow what the page read before: the log
	// started over, or only its end is given.
	Fresh bool `json:"fresh"`
}

// maxOutput is the most of a log given at once: the end of a long one.
const maxOutput = 64 * 1024

// ansiEscape matches the escapes of colours and cursor moves a terminal
// would have read.
var ansiEscape = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]")

func (s *Server) handleRunOutput(w http.ResponseWriter, r *http.Request, root string) {
	file, ok := runLogFile(root, r.URL.Query().Get("log"))
	if !ok {
		writeError(w, http.StatusBadRequest, "name a log: run, principal or issue-<number>")
		return
	}
	since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	out, err := readOutput(file, since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// readOutput reads a log from an offset: from its start again when it is
// shorter than that, and only its end when much more was written since.
func readOutput(file string, since int64) (RunOutput, error) {
	f, err := os.Open(file)
	if errors.Is(err, os.ErrNotExist) {
		return RunOutput{}, nil
	}
	if err != nil {
		return RunOutput{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return RunOutput{}, err
	}
	size := info.Size()
	out := RunOutput{Exists: true, Next: size}
	if since < 0 || since > size {
		since, out.Fresh = 0, true
	}
	cut := false
	if size-since > maxOutput {
		since, out.Fresh, cut = size-maxOutput, true, true
	}
	if _, err := f.Seek(since, io.SeekStart); err != nil {
		return RunOutput{}, err
	}
	data, err := io.ReadAll(io.LimitReader(f, size-since))
	if err != nil {
		return RunOutput{}, err
	}
	text := strings.ReplaceAll(ansiEscape.ReplaceAllString(string(data), ""), "\r", "")
	if cut {
		// The first line was cut in two: it starts with the next one.
		if at := strings.IndexByte(text, '\n'); at >= 0 {
			text = text[at+1:]
		}
		text = fmt.Sprintf("…（前面的省略了）\n%s", text)
	}
	out.Text = text
	return out, nil
}
