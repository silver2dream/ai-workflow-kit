package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/ticket"
)

// The scheduler's rules (design.md §5.3, task 2.2a): which lanes take a slot,
// which hold the paths of their ticket, whether a ticket that could start is
// kept off by a lane's paths, and which tickets wait for each other in a
// circle. Which tickets can start, and in what order, the analyzer finds
// (Candidates); the engine passes over those these rules keep off.

// maxParallel is how many lanes take a slot at once. Task 2.2b runs lanes
// side by side and reads it from engine.max_parallel.
const maxParallel = 1

// takesSlot says whether a lane in a state takes one of the slots: an agent
// is at work on it, or is about to be. A lane to be done again (Rework)
// waits for a slot with the tickets not started, by number, as the
// Principal's workflow has it; one that waits for a person, an approval or
// the merge queue takes none.
func takesSlot(s State) bool {
	switch s {
	case Implementing, Checking, Reviewing, ReviewRetry, Resolving:
		return true
	}
	return false
}

// taken counts the lanes that take a slot.
func taken(lanes []Lane) int {
	n := 0
	for _, l := range lanes {
		if takesSlot(l.State) {
			n++
		}
	}
	return n
}

// holdsPaths says whether a lane holds the paths of its ticket: work was
// begun on them and is not over. A lane that waits for a person holds them
// when it has a pull request, or may have work in its worktree: every one but
// a ticket that could not be used, or waits for tickets in a circle, which
// was never dispatched. A paused lane holds what it held before.
func holdsPaths(l Lane) bool {
	switch l.State {
	case Implementing, Checking, Reviewing, ReviewRetry, Rework, Resolving, AwaitingApproval, Queued:
		return true
	case NeedsYou:
		return l.PR != 0 || (l.Why != WhyInvalidTicket && l.Why != WhyDependencyCycle)
	case Paused:
		before := l
		before.State, before.Why = l.Resume, ""
		return l.Resume != Paused && holdsPaths(before)
	}
	return false
}

// pathsOf are the paths a ticket declares, lowered: on a file system that
// ignores case, Docs/a.md is docs/a.md. nil is none declared (a ticket
// without a block, a block that cannot be read, schema 1): it may change
// every file.
func pathsOf(meta ticket.Meta, found bool) []string {
	if !found || len(meta.Paths) == 0 {
		return nil
	}
	paths := make([]string, len(meta.Paths))
	for i, path := range meta.Paths {
		paths[i] = strings.ToLower(path)
	}
	return paths
}

// pathsOfBody are the paths the block of an issue body declares (pathsOf).
func pathsOfBody(body string) []string {
	meta, found, err := ticket.Parse(body)
	if err != nil {
		return nil
	}
	return pathsOf(meta, found)
}

// hold is a lane that holds paths, and its paths (nil: every file).
type hold struct {
	Issue int
	Paths []string
}

// overlap says whether two tickets' paths may take in the same file. A ticket
// that declares none may take in any.
func overlap(a, b []string) bool {
	if a == nil || b == nil {
		return true
	}
	for _, x := range a {
		for _, y := range b {
			if ticket.Overlap(x, y) {
				return true
			}
		}
	}
	return false
}

// heldBy is the first lane, by number, whose paths keep a ticket from
// starting, or 0 when none does. A lane does not keep its own ticket off.
func heldBy(holds []hold, issue int, paths []string) int {
	for _, h := range holds {
		if h.Issue != issue && overlap(h.Paths, paths) {
			return h.Issue
		}
	}
	return 0
}

// Cycle is tickets that wait for each other in a circle: none can start.
type Cycle struct {
	// Members are the tickets of the circle, by number.
	Members []int
	// Round is one way round it, from the lowest: each waits for the next,
	// and the last for the first.
	Round []int
}

// Note says the circle for a person.
func (c Cycle) Note() string {
	steps := make([]string, 0, len(c.Round)+1)
	for _, issue := range append(append([]int{}, c.Round...), c.Round[0]) {
		steps = append(steps, fmt.Sprintf("#%d", issue))
	}
	return fmt.Sprintf("%s wait for each other in a circle (%s, each waiting for the next), so none of them can start: correct depends_on in their awk-ticket blocks",
		issueList(c.Members), strings.Join(steps, " → "))
}

func issueList(numbers []int) string {
	names := make([]string, len(numbers))
	for i, n := range numbers {
		names[i] = fmt.Sprintf("#%d", n)
	}
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// Cycles are the circles among tickets: waits maps a ticket to the tickets
// it waits for. A circle is a strongly connected set of two tickets or more
// (Tarjan's algorithm); the circles come by their lowest ticket. A ticket
// that waits for itself alone is not one: it cannot be used, and is handed to
// a person as such.
func Cycles(waits map[int][]int) []Cycle {
	var nodes []int
	for n := range waits {
		nodes = append(nodes, n)
	}
	sort.Ints(nodes)
	index, low := map[int]int{}, map[int]int{}
	onStack := map[int]bool{}
	var stack []int
	var cycles []Cycle
	next := 0
	var connect func(n int)
	connect = func(n int) {
		index[n], low[n] = next, next
		next++
		stack = append(stack, n)
		onStack[n] = true
		for _, m := range sortedCopy(waits[n]) {
			if _, seen := index[m]; !seen {
				connect(m)
				low[n] = min(low[n], low[m])
			} else if onStack[m] {
				low[n] = min(low[n], index[m])
			}
		}
		if low[n] != index[n] {
			return
		}
		var members []int
		for {
			m := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[m] = false
			members = append(members, m)
			if m == n {
				break
			}
		}
		if len(members) > 1 {
			sort.Ints(members)
			cycles = append(cycles, Cycle{Members: members, Round: round(waits, members)})
		}
	}
	for _, n := range nodes {
		if _, seen := index[n]; !seen {
			connect(n)
		}
	}
	sort.Slice(cycles, func(i, j int) bool { return cycles[i].Members[0] < cycles[j].Members[0] })
	return cycles
}

// round is the shortest way round a circle from its lowest ticket, within
// its members.
func round(waits map[int][]int, members []int) []int {
	in := map[int]bool{}
	for _, m := range members {
		in[m] = true
	}
	start := members[0]
	from := map[int]int{}
	queue := []int{start}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, m := range sortedCopy(waits[n]) {
			if !in[m] || m == n {
				continue
			}
			if m == start {
				path := []int{n}
				for p := n; p != start; {
					p = from[p]
					path = append(path, p)
				}
				for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
					path[i], path[j] = path[j], path[i]
				}
				return path
			}
			if _, seen := from[m]; !seen && m != start {
				from[m] = n
				queue = append(queue, m)
			}
		}
	}
	return members
}

func sortedCopy(numbers []int) []int {
	out := append([]int(nil), numbers...)
	sort.Ints(out)
	return out
}
