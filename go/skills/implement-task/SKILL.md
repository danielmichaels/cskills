---
name: implement-task
description: "Execute a planned task by spawning an agent team from todos.md. Supports parallel work and resumption."
argument-hint: "<task-name-or-number>"
disable-model-invocation: true
category: always
---

# implement-task — Team Execution Workflow

Execute a planned task by spawning an agent team that works through todos in parallel where possible.

---

## Directory Scope

Reads from and writes to `.tasks/NNN-slug/` relative to the current working directory.

```
.tasks/
  NNN-slug/
    research.md     # Context (read-only)
    plan.md         # Context (must be status: reviewed)
    todos.md        # Work items (read/write)
    log.md          # Execution log (append-only)
```

---

## Task Resolution

Same resolution as plan-task: match `$ARGUMENTS` against number, slug, or partial match.

---

## Prerequisites

Before starting:

1. `plan.md` must exist with `status: reviewed`
2. `todos.md` must exist with `status: pending` or `status: in_progress`

If either is missing, direct the user to the appropriate prior step (`/research-task` → `/plan-task` → `/implement-task`).

**Worktree awareness check:** If the current working directory is not inside a git worktree dedicated to this task, warn the user: `"Warning: Not inside a worktree. Consider isolating this work before proceeding."` Allow proceeding regardless.

---

## The Job

1. **Assess state** — Fresh start or resumption?
2. **Spawn team** — Create agent team sized to max parallelism (capped at 4)
3. **Assign work** — Give each agent a todo, respecting dependency order
4. **Monitor progress** — Track completions, update files, assign next items
5. **Handle failures** — Escalate blockers to user
6. **Wrap up** — Finalize todos.md, write summary to log.md, shutdown team
7. **Validate** — Build, test, and offer manual verification
8. **Extract learnings** — Review work for reusable knowledge (optional)

---

## Step 1: Assess State

Read `todos.md` and determine the execution state:

**Fresh start** — All items are `pending`:
- Proceed normally

**Resumption** — Some items are `in_progress` or `completed`:
- Reset any `in_progress` items back to `pending` (previous agents are gone)
- Preserve `completed` items
- Log the resumption in `log.md`

```
Resuming 001-add-rate-limiting:
  2 of 5 items completed
  1 item reset from in_progress → pending
  2 items remaining
```

---

## Step 2: Spawn Team

1. Parse the parallelism guide from `todos.md`
2. Determine max concurrent agents needed (capped at 4)
3. Spawn an agent team:

```
Team: task-001
Agents: up to N (based on max parallel group size, capped at 4)
```

---

## Step 3: Assign Work

For each agent, provide:

1. **Plan summary** — The approach section from plan.md (not the full plan)
2. **Todo details** — The specific todo item with full description
3. **File list** — Which files to modify
4. **Acceptance criteria** — What defines "done"
5. **Context** — Relevant sections from research.md (key files, patterns)

Assignment follows the parallelism guide:

- Start with Group 1 (all items can run in parallel)
- Wait for the gate (all Group 1 items complete) before starting Group 2
- Within a group, assign items as agents become available

### Agent Instructions Template

```
You are implementing a todo item for task NNN-slug.

## Plan Context
<approach section from plan.md>

## Your Assignment
<todo item details: title, description, files, acceptance criteria>

## Codebase Patterns
<relevant patterns from research.md>

## Instructions
1. Read the files listed in your assignment
2. Implement the changes described
3. Verify acceptance criteria
4. Run relevant tests if acceptance criteria include "tests pass"
5. Report completion with a summary of changes made

Do NOT modify files outside your assignment unless strictly necessary.
If you encounter a blocker, report it immediately rather than working around it.
```

---

## Step 4: Monitor Progress

As each agent completes:

1. **Update todos.md** — Set the item status to `completed`, check acceptance criteria boxes
2. **Append to log.md** — Record what was done, files changed, any notes
3. **Check gates** — If all items in the current group are done, unlock the next group
4. **Assign next** — Give the now-free agent the next available item

### log.md Format

```markdown
# Execution Log: <Task Title>

## T01: <Title>
- **Completed:** YYYY-MM-DD HH:MM
- **Agent:** agent-name
- **Files changed:** `path/to/file.go`
- **Notes:** Brief summary of what was done

## T02: <Title>
- **Completed:** YYYY-MM-DD HH:MM
- **Agent:** agent-name
- **Files changed:** `path/to/file.go`, `path/to/other.go`
- **Notes:** Brief summary
```

---

## Step 5: Handle Failures

If an agent reports a blocker or fails:

1. Mark the todo as `blocked` in todos.md
2. Log the failure in log.md with details
3. Escalate to the user with options:

```
T03 is blocked: "Cannot add middleware — router interface doesn't
support the expected pattern"

Options:
  A. Retry with additional guidance
  B. Skip this item and continue
  C. Pause execution (resume later with /implement-task 001)
```

If the user provides guidance for retry, re-assign the item with the additional context.

---

## Step 6: Wrap Up

When all items are complete (or the user pauses):

1. Update `todos.md` frontmatter status:
   - All done → `status: completed`
   - Paused → `status: in_progress`
2. Write completion summary to `log.md`:

```markdown
## Summary

- **Started:** YYYY-MM-DD HH:MM
- **Completed:** YYYY-MM-DD HH:MM
- **Items completed:** 5/5
- **Items blocked:** 0
- **Files modified:** 6

### Changes Overview
<Brief narrative of what was accomplished>
```

3. Shutdown the agent team
4. Report to the user:

```
Task 001-add-rate-limiting complete.

  5/5 items done
  6 files modified
  Log: .tasks/001-add-rate-limiting/log.md

Proceeding to validation...
```

---

## Step 7: Validate

Validation runs after wrap-up and before extracting learnings. Specific commands depend on the project — inspect the repo for common entry points or ask the user.

### Phase A — Build / Compile

If the project has a build or type-check command, run it. Detect from common signals:

- `go.mod` → `go build ./...`
- `Cargo.toml` → `cargo build`
- `package.json` with a `build` script → `npm run build` (or pnpm/yarn)
- `Makefile` with a `build` target → `make build`
- `Taskfile.yml` with a `build` task → `task build`

If no build command applies, skip this phase and note it in the log.

```
Build: passed (go build ./...)
```

Or:

```
Build failed:
  <error output>

Fix the errors before proceeding.
```

Halt on failure — do not proceed to tests until the build is clean.

### Phase B — Tests

If any todo had an acceptance criterion mentioning tests, run the project's test command:

- Go → `go test ./...`
- Rust → `cargo test`
- Node → `npm test` (or equivalent)
- Python → `pytest`

Report the result. If tests fail, halt — do not proceed to manual verification.

### Phase C — Manual Verification

Ask the user how they'd like to verify end-to-end:

```
Implementation complete and tests pass.

How would you like to verify?
  A. I'll verify manually — let me know when done
  B. Start the application for me (provide the start command)
  C. Skip — automated checks are sufficient for this change
```

If B, ask for the start command, run it, and wait for confirmation before shutting it down.

---

## Step 8: Extract Learnings

After validation, review the completed work for reusable knowledge. Skip this step if the task was purely mechanical with no new patterns.

1. **Scan completed work** — Read `log.md`, `todos.md`, and `plan.md` for:
   - New patterns or conventions established by this task
   - Quantitative findings (benchmarks, measurements)
   - Decisions that should generalize beyond this task
   - Corrections to existing documented guidelines

2. **Find a home for learnings** — Look for an existing guidelines file in the repo (`CLAUDE.md`, `AGENTS.md`, `docs/guidelines.md`, etc.). If none exists, ask the user where learnings should live (or skip).

3. **Propose additions** — Present draft entries to the user before writing. Each entry should include:
   - A concise title
   - The context (what problem it solves)
   - The pattern or rule (what to do)
   - Evidence (task number and observation)

4. **Log it** — Append to `log.md`:

   ```
   ## Learnings Extracted
   - **Updated:** <file path>
   - **New entries:** <count> patterns added
   ```

---

## Resumption

If execution was interrupted (context lost, user paused, error):

1. Run `/implement-task NNN` again
2. The skill reads existing state from todos.md
3. `in_progress` items are reset to `pending` (the agents are gone)
4. `completed` items are preserved
5. Execution continues from where it left off

This means todos.md is the single source of truth for execution state.

---

## Example Session: Fresh Start

**User:** `/implement-task 001`

**Claude:**

```
Reading task 001-add-rate-limiting...

Plan: reviewed
Todos: 5 items, 3 parallel groups, max concurrency 2

Spawning team (2 agents)...

Assigning Group 1:
  Agent 1 → T01: Add rate limiter middleware skeleton
  Agent 2 → T02: Add rate limit configuration struct
```

*(agents work)*

```
T01 completed — middleware skeleton in api/middleware.go
T02 completed — config struct in config/rate_limit.go

Gate passed. Assigning Group 2:
  Agent 1 → T03: Wire middleware into router
  Agent 2 → T04: Add rate limit tests
```

*(agents work)*

```
T03 completed — middleware wired in api/routes.go
T04 completed — tests in api/middleware_test.go

Gate passed. Assigning Group 3:
  Agent 1 → T05: Add rate limit documentation

T05 completed — docs updated

All items complete. Shutting down team.

Validating...
  Build: passed (go build ./...)
  Tests: passed (go test ./...)

How would you like to verify?
  A. I'll verify manually
  B. Start the application for me
  C. Skip
```

**User:** `A — looks good`

**Claude:**

```
Task 001-add-rate-limiting complete.
  5/5 items done
  6 files modified
  Log: .tasks/001-add-rate-limiting/log.md

Next steps:
  - Review changes with git diff
  - Commit when satisfied
```

---

## Example Session: Resumption

**User:** `/implement-task 001`

**Claude:**

```
Resuming 001-add-rate-limiting:
  3 of 5 items completed
  1 item reset from in_progress → pending (T04)
  1 item pending (T05)

T04 is in Group 2 (no gate — Group 1 already passed)
T05 is in Group 3 (blocked by T04)

Spawning team (1 agent)...
Assigning: Agent 1 → T04: Add rate limit tests
```

*(continues from where it left off)*

---

## Checklist

Before spawning the team:

- [ ] plan.md has status: reviewed
- [ ] todos.md exists and has items
- [ ] Stale in_progress items reset to pending
- [ ] Max agents capped at 4
- [ ] Agent instructions include plan context + patterns

During execution:

- [ ] todos.md updated after each completion
- [ ] log.md appended after each completion
- [ ] Gates respected between parallel groups
- [ ] Failures escalated to user promptly

After completion:

- [ ] todos.md status updated (completed or in_progress)
- [ ] log.md has completion summary
- [ ] Team shut down cleanly
- [ ] Build passes (if a build command applies)
- [ ] Tests pass (if any todo required tests)
- [ ] Manual verification offered to user
- [ ] User informed of next steps
- [ ] Learnings extracted (if any new patterns emerged)
