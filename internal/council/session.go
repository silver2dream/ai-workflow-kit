package council

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrBudget is returned when a plan has made as many model calls as its
// roster allows.
var ErrBudget = errors.New("the plan has used up the model calls its roster allows")

// session asks the model on behalf of the roles. It counts the calls, keeps
// every prompt and answer on disk, and remembers the answers it could use, so
// that a plan that was interrupted can go on where it stopped.
type session struct {
	model    Model
	limits   Limits
	stateDir string // every prompt and answer of the plan
	progress func(string)
	telling  sync.Mutex // one line of progress at a time

	mu      sync.Mutex
	answers map[string]string // step -> the answer that was used
	asked   map[string]bool   // the steps this run had to ask the model for
	calls   int               // model calls made in this run
	seq     int
}

func newSession(model Model, limits Limits, stateDir string, progress func(string)) (*session, error) {
	if progress == nil {
		progress = func(string) {}
	}
	s := &session{model: model, limits: limits, stateDir: stateDir, progress: progress,
		answers: map[string]string{}, asked: map[string]bool{}}
	if err := os.MkdirAll(filepath.Join(stateDir, "calls"), 0o755); err != nil {
		return nil, err
	}
	// The record of the calls only grows: a run that goes on with a plan
	// numbers its calls after the ones that are there.
	if records, err := os.ReadDir(filepath.Join(stateDir, "calls")); err == nil {
		for _, record := range records {
			if number, err := strconv.Atoi(strings.SplitN(record.Name(), "-", 2)[0]); err == nil && number > s.seq {
				s.seq = number
			}
		}
	}
	data, err := os.ReadFile(s.answersFile())
	switch {
	case os.IsNotExist(err):
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal(data, &s.answers); err != nil {
			return nil, fmt.Errorf("%s cannot be read: %w", s.answersFile(), err)
		}
	}
	return s, nil
}

func (s *session) answersFile() string {
	return filepath.Join(s.stateDir, "answers.json")
}

// remembered returns the answer a step was settled with in an earlier run.
func (s *session) remembered(step string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	answer, ok := s.answers[step]
	return answer, ok
}

// remember keeps the answer a step was settled with. Only answers that could
// be used are kept: a plan that is resumed asks again where it failed.
func (s *session) remember(step, answer string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.answers[step] = answer
	s.asked[step] = true
	data, err := json.MarshalIndent(s.answers, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.answersFile(), data, 0o644)
}

// wasAsked reports whether this run asked the model for a step, rather than
// taking the answer an earlier run remembered.
func (s *session) wasAsked(step string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.asked[step]
}

var unsafeInName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// call makes one model call and records it.
func (s *session) call(ctx context.Context, step, model, prompt string) (string, error) {
	s.mu.Lock()
	if s.calls >= s.limits.Calls {
		s.mu.Unlock()
		return "", ErrBudget
	}
	s.calls++
	s.seq++
	seq := s.seq
	s.mu.Unlock()

	callCtx, cancel := context.WithTimeout(ctx, time.Duration(s.limits.TimeoutSeconds)*time.Second)
	defer cancel()
	answer, err := s.model.Ask(callCtx, model, prompt)

	record := fmt.Sprintf("# %s (%s)\n\n## prompt\n\n%s\n\n## answer\n\n%s\n", step, model, prompt, answer)
	if err != nil {
		record += "\n## error\n\n" + err.Error() + "\n"
	}
	name := fmt.Sprintf("%03d-%s.md", seq, unsafeInName.ReplaceAllString(step, "-"))
	_ = os.WriteFile(filepath.Join(s.stateDir, "calls", name), []byte(record), 0o644)
	if err != nil {
		return "", fmt.Errorf("%s: %w", step, err)
	}
	return answer, nil
}

// broken marks a failure that is not the answer's fault: a file that cannot
// be written, say. The step fails with it; the role is not asked again.
type broken struct{ err error }

func (b broken) Error() string { return b.err.Error() }
func (b broken) Unwrap() error { return b.err }

// settle gets a usable answer for a step: the one remembered from an earlier
// run, or a new one from the model. use reads the answer and says whether it
// can be used; replay tells it that the answer is a remembered one, which was
// acted on when it was given. An answer that cannot be used is asked for once
// more, with what was wrong with it; a second unusable answer fails the step.
func (s *session) settle(ctx context.Context, step, model, prompt string, use func(answer string, replay bool) error) error {
	if answer, ok := s.remembered(step); ok {
		if err := use(answer, true); err == nil {
			s.say("  %s: kept from the earlier run", step)
			return nil
		}
		// What was remembered no longer fits (the roster changed, say): ask again.
	}
	started := time.Now()
	answer, err := s.call(ctx, step, model, prompt)
	if err != nil {
		return err
	}
	problem := use(answer, false)
	if problem != nil {
		var fatal broken
		if errors.As(problem, &fatal) {
			return fmt.Errorf("%s: %w", step, fatal.err)
		}
		s.say("  %s: the answer could not be used (%s); asking again", step, oneLine(problem.Error()))
		again := prompt + "\n\nYour previous answer could not be used: " + problem.Error() +
			"\nAnswer again, in exactly the form asked for and with nothing else."
		answer, err = s.call(ctx, step+" (again)", model, again)
		if err != nil {
			return err
		}
		if problem = use(answer, false); problem != nil {
			if errors.As(problem, &fatal) {
				return fmt.Errorf("%s: %w", step, fatal.err)
			}
			return fmt.Errorf("%s: the answer could not be used: %w", step, problem)
		}
	}
	if err := s.remember(step, answer); err != nil {
		return err
	}
	s.say("  %s (%s)", step, time.Since(started).Round(time.Second))
	return nil
}

// settleJSON settles a step whose answer is one JSON object.
func settleJSON[T any](ctx context.Context, s *session, step, model, prompt string, check func(*T) error) (*T, error) {
	var result *T
	err := s.settle(ctx, step, model, prompt, func(answer string, _ bool) error {
		value := new(T)
		if err := decodeJSON(answer, value); err != nil {
			return err
		}
		if check != nil {
			if err := check(value); err != nil {
				return err
			}
		}
		result = value
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// each runs n pieces of work, a few at a time, and returns the first error in
// the order of the pieces. Every piece runs even when one fails, so that what
// can be settled is settled and remembered.
func (s *session) each(n int, work func(i int) error) error {
	parallel := s.limits.Parallel
	if parallel < 1 {
		parallel = 1
	}
	errs := make([]error, n)
	gate := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		gate <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-gate }()
			errs[i] = work(i)
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *session) callsMade() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// say tells the progress of the meeting. Roles are asked several at a time;
// whoever listens is told one line at a time.
func (s *session) say(format string, args ...any) {
	s.telling.Lock()
	defer s.telling.Unlock()
	s.progress(strings.TrimRight(fmt.Sprintf(format, args...), "\n"))
}
