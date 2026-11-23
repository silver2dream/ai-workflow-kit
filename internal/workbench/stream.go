package workbench

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"path/filepath"
	"slices"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/council"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	projectsettings "github.com/silver2dream/ai-workflow-kit/internal/settings"
)

// area is a part of a project a page is told has changed, and where on disk
// it is.
type area struct {
	name string
	dir  string
	// skip are folders of dir that change without changing the area.
	skip []string
	// only, when set, are the names of the files that count, wherever they
	// are in dir; every other file, and every calls folder, is passed over.
	only []string
}

// areas are what a stream watches. The tickets are on GitHub, not on disk:
// a page is told every ticketsEvery to ask again.
var areas = []area{
	// The run: its lock, its session, how many loops and failures, the
	// result of each ticket. Not its logs and events, which grow all the
	// time a run goes on.
	{name: "status", dir: layout.StateDir, skip: []string{"logs", "events", "council", "tmp", "settings"}},
	{name: "events", dir: layout.StateDir + "/events"},
	{name: "plans", dir: council.PlansDir},
	// A meeting that goes on: where it is, what was said, what the owner
	// said. Not every prompt and answer it keeps, which grow all the time.
	{name: "meeting", dir: layout.StateDir + "/council", only: []string{council.LiveFile, council.TranscriptFile, council.RecordFile, "owner.jsonl", "questions.json", council.ApprovalFile}},
	// A change of the settings on its way to the integration branch.
	{name: "settings", dir: layout.StateDir + "/settings", only: []string{projectsettings.RecordFile}},
}

const (
	ticketsEvery = 30 * time.Second
	pingEvery    = 15 * time.Second
	// maxWatched bounds how many files a look counts, so that a state
	// folder that has grown large does not make every second slow.
	maxWatched = 5000
)

// handleStream tells the page what changed in the project, as it changes:
// the page asks again for what it shows of it.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request, root string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "this server cannot stream")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "retry: 2000\n\n")
	flusher.Flush()

	send := func(changed []string) {
		data, _ := json.Marshal(map[string][]string{"areas": changed})
		fmt.Fprintf(w, "event: change\ndata: %s\n\n", data)
		flusher.Flush()
	}
	seen := map[string]string{}
	for _, a := range areas {
		seen[a.name] = fingerprint(root, a)
	}
	poll := time.NewTicker(s.opts.Poll)
	tickets := time.NewTicker(ticketsEvery)
	ping := time.NewTicker(pingEvery)
	defer poll.Stop()
	defer tickets.Stop()
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-poll.C:
			var changed []string
			for _, a := range areas {
				if now := fingerprint(root, a); now != seen[a.name] {
					seen[a.name] = now
					changed = append(changed, a.name)
				}
			}
			if len(changed) > 0 {
				send(changed)
			}
		case <-tickets.C:
			send([]string{"tickets"})
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

// fingerprint sums up the files of an area: their names, sizes and times. It
// changes when a file is written, added or removed, and not when only a
// folder's time does.
func fingerprint(root string, a area) string {
	hash := sha256.New()
	base := filepath.Join(root, filepath.FromSlash(a.dir))
	count := 0
	_ = filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if path != base && slices.Contains(a.skip, entry.Name()) && filepath.Dir(path) == base {
				return filepath.SkipDir
			}
			if len(a.only) > 0 && entry.Name() == "calls" {
				return filepath.SkipDir
			}
			// A folder changes when anything is made in it, a skipped folder
			// too: its files are what count.
			return nil
		}
		if len(a.only) > 0 && !slices.Contains(a.only, entry.Name()) {
			return nil
		}
		if count++; count > maxWatched {
			return filepath.SkipAll
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(base, path)
		fmt.Fprintf(hash, "%s\x00%d\x00%d\n", rel, info.Size(), info.ModTime().UnixNano())
		return nil
	})
	return hex.EncodeToString(hash.Sum(nil))
}
