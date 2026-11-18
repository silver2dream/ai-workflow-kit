package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/council"
	"github.com/silver2dream/ai-workflow-kit/internal/kickoff"
)

// meetingGoesOn reports whether a plan's meeting is being held now: it says
// so, and the process that holds it runs.
func meetingGoesOn(root, id string) bool {
	live, err := council.ReadLive(root, id)
	return err == nil && live.State == council.LiveMeeting && kickoff.ProcessAlive(live.PID)
}

// waitsForOwner says what a meeting that stopped for its owner waits for,
// and how it goes on. It is no failure.
func waitsForOwner(root string, plan *council.Plan, waiting *council.Waiting) int {
	id := plan.ID
	switch waiting.State {
	case council.LiveAgenda:
		fmt.Printf("The agenda is set: %s/%s/%s\n\n", council.PlansDir, id, council.AgendaFile)
		if plan.Agenda != nil {
			for i, topic := range plan.Agenda.Topics {
				fmt.Printf("  %d. %s (%s leads; %s speak)\n", i+1, oneLine(topic.Title), topic.Lead, strings.Join(topic.Speakers, ", "))
			}
			fmt.Println("")
		}
		fmt.Println("Change it if you want: the topics, their order, who speaks. Then go on with:")
	case council.LiveQuestions:
		questions, _ := council.Questions(root, id)
		fmt.Printf("The council has %d question(s) only you can answer:\n\n", len(questions))
		for i, question := range questions {
			fmt.Printf("  %d. %s\n", i+1, oneLine(question))
		}
		fmt.Println("")
		fmt.Printf("Answer them in the workbench (%s), or here, then the meeting goes on:\n", cyan("awkit ui"))
		fmt.Printf("  %s\n", cyan("awkit plan --resume "+id+" --ask"))
		return 0
	default:
		where := ""
		if live, err := council.ReadLive(root, id); err == nil && live.Topic > 0 {
			where = fmt.Sprintf(", in topic %d of %d", live.Topic, live.Topics)
		}
		fmt.Printf("The meeting paused, as you asked%s. What was said is kept: going on asks for none of it again.\n", where)
		fmt.Println("Go on with:")
	}
	fmt.Printf("  %s\n", cyan("awkit plan --resume "+id))
	return 0
}

func cmdPlanSay(args []string) int {
	fs := flag.NewFlagSet("plan say", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usagePlan
	to := fs.String("to", "", "")
	stateRoot := fs.String("state-root", "", "")
	flags, words := splitFlags(args, map[string]bool{"--to": true, "--state-root": true, "-to": true, "-state-root": true})
	if err := fs.Parse(flags); err != nil {
		return 2
	}
	if len(words) < 2 {
		errorf("name the plan, then say what you want the meeting to know: awkit plan say <plan> \"...\"\n")
		return 2
	}
	id, text := words[0], strings.TrimSpace(strings.Join(words[1:], " "))
	root, code := planRoot(*stateRoot)
	if code != 0 {
		return code
	}
	word, err := council.Say(root, id, text, *to, time.Now())
	if err != nil {
		errorf("%v\n", err)
		return 1
	}
	if meetingGoesOn(root, id) {
		success("Said to the meeting of %s (%s). The next role to speak is told; what was decided is read again against it first.\n", id, word.ID)
		return 0
	}
	success("Said to plan %s (%s). The meeting is not going on: it takes it in when it goes on.\n", id, word.ID)
	fmt.Printf("  %s\n", cyan("awkit plan --resume "+id))
	return 0
}

func cmdPlanPause(args []string) int {
	fs := flag.NewFlagSet("plan pause", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usagePlan
	stateRoot := fs.String("state-root", "", "")
	flags, words := splitFlags(args, map[string]bool{"--state-root": true, "-state-root": true})
	if err := fs.Parse(flags); err != nil {
		return 2
	}
	if len(words) != 1 {
		errorf("name the plan whose meeting is to pause\n")
		return 2
	}
	root, code := planRoot(*stateRoot)
	if code != 0 {
		return code
	}
	id := words[0]
	if !meetingGoesOn(root, id) {
		errorf("the meeting of plan %s is not going on\n", id)
		return 1
	}
	if err := council.Pause(root, id); err != nil {
		errorf("%v\n", err)
		return 1
	}
	success("The meeting of %s stops after the step it is at.\n", id)
	return 0
}
