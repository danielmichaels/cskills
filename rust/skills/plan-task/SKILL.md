---
name: plan-task
description: "Design an implementation plan for a researched task. Reads research.md, writes plan.md and todos.md."
argument-hint: "<task-name-or-number>"
disable-model-invocation: true
category: always
---

# plan-task — Implementation Planning Workflow

Design an implementation plan from research findings. Operates in two modes depending on current state.

---

## Directory Scope

Reads from and writes to `.tasks/NNN-slug/` relative to the current working directory.

```
.tasks/
  NNN-slug/
    research.md     # Input (required)
    plan.md         # Output (Mode A)
    todos.md        # Output (Mode B)
```

---

## Task Resolution

Locate the task by matching `$ARGUMENTS` against:
1. Exact number match (e.g. `001`)
2. Slug match (e.g. `add-rate-limiting`)
3. Partial match on number or slug

If ambiguous, present matches and ask the user to choose. If no match, list available tasks.

---

## Mode Detection

**Worktree awareness check:** Check if the current working directory is inside a `.claude/worktrees/` path. If not, warn the user: `"Warning: Not inside a worktree. Consider using /research-task to set up proper isolation."` Allow proceeding regardless.

- **Mode A — Plan creation**: `plan.md` does not exist, or user explicitly requests replanning
- **Mode B — Todo generation**: `plan.md` exists with `status: reviewed`

If `plan.md` exists with `status: draft`, remind the user to review and mark it `reviewed` before generating todos.

---

## Mode A: Plan Creation

### The Job

1. Read `research.md` — absorb findings, key files, patterns, constraints
2. Resolve open questions — present each open question from research.md to the user and get answers
3. Design the approach — spawn up to 2 Plan subagents to develop implementation strategy
4. Write `plan.md` — structured implementation plan
5. Prompt for review — ask user to review, annotate, then update status to `reviewed`

### Step 1: Absorb Research

Read `.tasks/NNN-slug/research.md`. If it doesn't exist, tell the user to run `/research-task` first.

Identify:
- Key files that will be modified
- Patterns to follow
- Constraints to respect
- Open questions to resolve

### Step 2: Resolve Open Questions

Present each open question from research.md to the user with suggested options. Questions tagged `[PROTOTYPED]` should reference their prototypes so the user can evaluate interactively:

```
Before planning, let's resolve the open questions from research:

1. [PROTOTYPED] Which visualization type for the timeline?
   Options with prototypes at prototypes/visualization-type/:
   A. Stacked Dots (option-a-stacked-dots.html)
   B. Swimlanes (option-b-swimlanes.html)
   C. Heatmap Grid (option-c-heatmap-grid.html) — recommended

   Open each file in a browser to compare. Which approach?

2. Should live updates push incrementally or full-refresh?
   A. Incremental (lower bandwidth, more complex)
   B. Full-refresh (simpler, works with current epoch pattern)
```

For non-prototyped questions, present options as before:

```
1. Should rate limits be per-user, per-IP, or per-API-key?
   A. Per-user (requires auth context)
   B. Per-IP (simplest, works for unauthenticated endpoints)
   C. Per-API-key (requires API key infrastructure)
   D. Configurable per-endpoint
```

Record answers and incorporate into the plan.

### Step 3: Design the Approach

Spawn up to 2 Plan subagents:

**Agent 1 — Implementation Design**
- Propose the implementation approach given research findings and resolved questions
- Identify phases if the work should be staged
- Map changes to specific files

**Agent 2 — Risk & Alternatives**
- Identify risks and mitigation strategies
- Propose alternative approaches and tradeoffs
- Flag potential blockers

### Step 4: Write plan.md

```markdown
---
status: draft
created: YYYY-MM-DD
research: research.md
---

# Plan: <Task Title>

## Approach

<High-level implementation strategy in 2-3 paragraphs>

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Decision 1 | Choice made | Why |
| Prototyped Decision | Chosen option (Option X) | Rationale. Prototype: `prototypes/decision-slug/option-x-name.ext` |

## Resolved Questions

| Question | Answer |
|----------|--------|
| From research open questions | User's answer |

## Phases

### Phase 1: <Name>

<What this phase accomplishes>

**Changes:**
- `path/to/file.rs` — Description of changes
- `path/to/other.rs` — Description of changes

### Phase 2: <Name>

<What this phase accomplishes>

**Changes:**
- `path/to/file.rs` — Description of changes

## Dependency Graph

<Which phases/changes depend on others, what can be parallelized>

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Risk 1 | Low/Med/High | Low/Med/High | How to mitigate |

## Alternatives Considered

### <Alternative 1>

<Description, pros, cons, why not chosen>

## Assumptions

- Assumption 1
- Assumption 2
```

### Step 5: Prompt for Review

```
Plan written to .tasks/001-add-rate-limiting/plan.md

Summary:
  - 2 phases: middleware implementation, then configuration
  - 4 files modified, 1 new file
  - Key risk: performance impact on hot path (mitigated by benchmarking)

Please review the plan. When satisfied, update the status
frontmatter from "draft" to "reviewed", then run:
  /plan-task 001
to generate the todo items.
```

---

## Mode B: Todo Generation

### The Job

1. Read `plan.md` (must have `status: reviewed`)
2. Decompose into atomic work items optimized for parallel agent execution
3. Write `todos.md` with structured items
4. Prompt user to proceed with `/implement-task`

### Decomposition Principles

- Each todo should be completable by a single agent in one session
- Todos should modify a focused set of files (ideally 1-3)
- Identify which todos can run in parallel (no file conflicts, no dependency)
- Group into parallel groups and sequential gates

### Write todos.md

```markdown
---
status: pending
created: YYYY-MM-DD
plan: plan.md
---

# Todos: <Task Title>

## Parallelism Guide

```text
Group 1 (parallel): T01, T02
  Gate: T01, T02 must complete
Group 2 (parallel): T03, T04
  Gate: T03, T04 must complete
Group 3 (sequential): T05
```

## Items

### T01: <Title>

- **Status:** pending
- **Blocked by:** none
- **Parallel group:** 1
- **Files:** `path/to/file.rs`
- **Description:** <What to do, with enough context for an agent working from scratch>
- **Acceptance criteria:**
    - [ ] Criterion 1
    - [ ] Criterion 2
    - [ ] Tests pass

### T02: <Title>

- **Status:** pending
- **Blocked by:** none
- **Parallel group:** 1
- **Files:** `path/to/other.rs`
- **Description:** <Detailed description>
- **Acceptance criteria:**
    - [ ] Criterion 1
    - [ ] Criterion 2

### T03: <Title>

- **Status:** pending
- **Blocked by:** T01
- **Parallel group:** 2
- **Files:** `path/to/file.rs`, `path/to/test.rs`
- **Description:** <Detailed description>
- **Acceptance criteria:**
    - [ ] Criterion 1
```

### Prompt User

```
Todos generated: .tasks/001-add-rate-limiting/todos.md

5 items across 3 parallel groups
Max concurrency: 2 agents
Estimated sequential gates: 2

Review the todos, then run:
/implement-task 001
```

---

## Example Session: Mode A

**User:** `/plan-task 001`

**Claude:**
```
Reading research for 001-add-rate-limiting...

Before planning, let's resolve the open questions:

1. Should rate limits be per-user, per-IP, or per-API-key?
   A. Per-user
   B. Per-IP
   C. Per-API-key
   D. Configurable per-endpoint
```

**User:** `D`

**Claude:**
```
2. Should limits be configurable at runtime or compile-time?
   A. Runtime (config/env vars)
   B. Compile-time
   C. Runtime with compile-time defaults
```

**User:** `C`

**Claude:**
```
Planning implementation... (2 agents working)

Plan written to .tasks/001-add-rate-limiting/plan.md

Summary:
- 2 phases: middleware + configuration
- Configurable per-endpoint with compile-time defaults
- 4 files modified, 1 new file

Review the plan, update status to "reviewed", then run /plan-task 001 again.
```

---

## Example Session: Mode B

**User:** `/plan-task 001`

**Claude:**
```
Plan for 001-add-rate-limiting is reviewed. Generating todos...

Todos generated: .tasks/001-add-rate-limiting/todos.md

5 items across 3 parallel groups
Max concurrency: 2 agents

Run /implement-task 001 to begin execution.
```

---

## Checklist

Before finalizing plan.md:

- [ ] All open questions from research resolved with user
- [ ] Approach is clear and actionable
- [ ] Changes mapped to specific files
- [ ] Phases are logically ordered
- [ ] Risks identified with mitigations
- [ ] At least one alternative considered
- [ ] Assumptions stated explicitly

Before finalizing todos.md:

- [ ] Each todo is atomic (one agent, one session)
- [ ] File sets don't conflict within parallel groups
- [ ] Dependencies (blocked_by) are correct
- [ ] Every todo has acceptance criteria
- [ ] Descriptions have enough context for a fresh agent
- [ ] Parallelism guide is accurate
