# Context Compaction Strategy

## When Context Compaction Happens

Claude Code automatically compresses prior messages as your conversation approaches context limits. This is normal in long-running workflows and does not indicate an error.

## What To Do After Compaction

1. **Run `awkit context-snapshot`** to get a compact summary of current state
2. **Run `awkit analyze-next --json`** to determine the next action
3. **Re-read the main loop**: `.ai/skills/principal-workflow/phases/main-loop.md`
4. **Continue the main loop** from Step 1 as normal

## What Information Is Preserved

The workflow state is persisted on disk and in GitHub, so compaction does NOT lose:
- Issue and PR states (GitHub labels)
- Worker execution results (`.ai/results/`)
- Session data (`.ai/state/`)
- Review feedback history (`.ai/state/review_feedback.jsonl`)
- Loop count and failure tracking

## What You May Need To Re-establish

After compaction, you may need to:
- Re-read the spec being worked on (check `awkit status` for active spec)
- Re-read any in-progress issue tickets
- Remember the current position in the main loop

## Best Practices

- **Trust the state machine**: `awkit analyze-next` always knows the correct next action
- **Don't repeat completed work**: The analyzer tracks what's done
- **Keep iterations small**: Each loop iteration is self-contained
- **Use `awkit status`** instead of re-reading multiple state files manually
