---
name: research-task
description: "Research a codebase task by exploring code, patterns, and dependencies. Creates a .tasks/ directory with detailed findings."
argument-hint: "<task-description>"
disable-model-invocation: true
category: always
---

# research-task — Codebase Research Workflow

Explore and document a task's codebase landscape before planning or implementation begins.

---

## Directory Scope

All task state lives in `.tasks/` relative to the current working directory. This directory is intended to be committed to git.

```
.tasks/
  NNN-slug/
    research.md
    prototypes/
      decision-slug/
        README.md
        option-a-name.ext
        option-b-name.ext
```

---

## The Job

1. **Scan for overlaps** — Read `.tasks/*/research.md` summaries to detect related existing tasks
2. **Resolve overlaps** — If found, prompt the user to merge, reference, proceed independently, or cancel
3. **Name the task** — Derive a brief hyphenated slug, auto-increment the number prefix (e.g. `004-add-rate-limiting`), confirm with user
4. **Research the codebase** — Spawn up to 3 Explore subagents in parallel to investigate architecture, patterns, and dependencies
5. **Classify open questions** — Categorize each question and identify which need prototyping
6. **Prototype decisions** — For experiential-tradeoff questions, create standalone runnable prototypes
7. **Synthesize findings** — Write `research.md` with structured sections (including prototypes)
8. **Review** — Present a summary, prompt user to identify gaps or approve

---

## Step 1: Scan for Overlaps

Read all existing `.tasks/*/research.md` files. Extract the `summary` section from each. Compare against the new task description for thematic overlap (shared files, same subsystem, related goals).

If overlaps are found, present them to the user:

```
Found potentially related tasks:

  003-refactor-auth — Refactoring the auth middleware and session handling
  005-add-rbac — Adding role-based access control

How would you like to proceed?
  A. Merge into an existing task
  B. Reference the related task(s) and proceed independently
  C. Cancel
```

If no overlaps, proceed directly to naming.

---

## Step 2: Name the Task

1. Scan `.tasks/` for existing directories to determine the next number (zero-padded to 3 digits)
2. Derive a short hyphenated slug from the task description (3-5 words max)
3. Present the proposed name to the user for confirmation:

```
Proposed task: 004-add-rate-limiting

Confirm? (y/n or suggest alternative)
```

4. Create the directory `.tasks/NNN-slug/`

---

## Step 3: Research the Codebase

Spawn up to 3 Explore subagents in parallel. Each focuses on a different research dimension:

**Agent 1 — Architecture & Patterns**
- How is the relevant subsystem structured?
- What patterns does the codebase use for similar functionality?
- What abstractions and interfaces exist?

**Agent 2 — Dependencies & Integration**
- What files and packages are involved?
- What are the upstream/downstream dependencies?
- What external libraries or services interact with this area?

**Agent 3 — Tests & Constraints**
- What test coverage exists for the affected area?
- What invariants or constraints does the code enforce?
- Are there linters, CI checks, or configuration that constrain changes?

Each agent should return structured findings, not raw file dumps.

---

## Step 4: Classify Open Questions

After the Explore subagents return, collect all open questions and classify each as:

- **Policy/requirements** — Needs user input, no prototype (e.g., "per-user or per-IP rate limiting?")
- **Configuration** — Has a clear best answer from research, no prototype (e.g., "which config format?")
- **Experiential tradeoff** — Multiple valid approaches where seeing options side-by-side changes the decision → **prototype this**

All three conditions must be true for a question to warrant prototyping:
1. Multiple valid approaches exist
2. Tradeoffs are experiential (need to see/run to compare)
3. Decision has high impact on core implementation

**Skip condition:** If no open questions are classified as experiential tradeoffs, Step 5 (Prototype Decisions) is skipped entirely. This keeps the common case fast — classification is quick, and most tasks won't have experiential tradeoffs.

---

## Step 5: Prototype Decisions

For each experiential-tradeoff question, spawn one subagent per **decision** (not per option). The agent creates all options for a single decision to ensure they're comparable — same synthetic data, same scale, only the differentiating factor changes.

**Prototype format by decision type:**

| Decision Type | Format | Example |
|---|---|---|
| Visual/UI | Standalone HTML with CDN deps | Layout, chart type, component design |
| Data model/query | `.sql` with setup + query | Schema design, query strategy |
| Go architecture | `_test.go` or `//go:build ignore` | Interface design, API shape |

**Scope constraints:**
- Max 4 option files per decision
- Each file under 200 lines
- No project imports — standalone only
- Synthetic data only — no real DB/service connections
- Single-file per option (if it needs multiple files, it's too complex)

**Agent instructions include:**
- Decision context and options to prototype
- Format (HTML/SQL/Go)
- Requirement to create a `README.md` in `prototypes/decision-slug/` with a comparison table
- For HTML prototypes: project theme tokens (dark bg: `#1d232a`, text: `#a6adba`, DaisyUI CDN)

Each `prototypes/decision-slug/README.md` follows this format:

```markdown
# Decision: <Title>

## Context
<Why this decision matters>

## Options
| Option | File | Key Differentiator | Tradeoff |
|--------|------|--------------------|----------|
| A: Name | `option-a-name.ext` | Unique aspect | Pro/con |
| B: Name | `option-b-name.ext` | Unique aspect | Pro/con |

## How to Evaluate
<Instructions for running/viewing>

## Recommendation
<If research agent has a strong lean>
```

---

## Step 6: Write research.md

Synthesize all findings into `.tasks/NNN-slug/research.md` with the following structure:

```markdown
---
status: draft
created: YYYY-MM-DD
description: "<original task description>"
related_tasks: []
---

# Research: <Task Title>

## Summary

<2-3 sentence overview of what was found>

## Codebase Findings

### Architecture

<How the relevant subsystem is structured, key abstractions, data flow>

### Existing Patterns

<Patterns the codebase uses for similar functionality — include file references>

### Key Files

| File | Role |
|------|------|
| `path/to/file.go` | Brief description |

### Dependency Map

<Upstream/downstream dependencies, external services, library usage>

## Test Coverage

<What tests exist, what's covered, what gaps are apparent>

## Constraints

<Invariants, linter rules, CI requirements, performance budgets>

## Prototypes

> Only include this section if experiential-tradeoff questions were identified in Step 5.

### <Decision Title>

<Brief context>

| Option | File | Key Differentiator |
|--------|------|--------------------|
| A: Name | `prototypes/slug/option-a-name.html` | What makes it unique |
| B: Name | `prototypes/slug/option-b-name.html` | What makes it unique |

See `prototypes/slug/README.md` for comparison and evaluation instructions.

**Recommendation:** <lean if any>

## Open Questions

- [ ] [PROTOTYPED] Which visualization type? → See prototypes/visualization-type/
- [ ] Question that needs user input or further investigation
- [ ] Another unresolved question
```

---

## Step 7: Review with User

Present a concise summary of findings and ask. Include prototype information when present:

```
Research complete for 004-add-rate-limiting.

Key findings:
  - The API layer uses middleware chaining in api/middleware.go
  - Rate limiting would fit as a new middleware
  - No existing rate limiting — the closest pattern is the auth middleware

1 decision prototyped:
  - Visualization type: 3 options in prototypes/visualization-type/
    Open each .html file in a browser to compare

2 open questions remain (1 prototyped, 1 needs user input)

Review the full research at .tasks/004-add-rate-limiting/research.md

Are there any gaps to investigate, or is this ready for /plan-task?
```

If no prototypes were created, omit the prototypes section and show the simpler format:

```
Research complete for 004-add-rate-limiting.

Key findings:
  - ...

3 open questions need resolution before planning

Review the full research at .tasks/004-add-rate-limiting/research.md

Are there any gaps to investigate, or is this ready for /plan-task?
```

If the user identifies gaps, spawn additional Explore subagents to fill them and update research.md.

---

## Example Session (without prototypes)

**User:** `/research-task Add rate limiting to the API endpoints`

**Claude:**
```
Scanning .tasks/ for related work...
No existing tasks found.

Proposed task: 001-add-rate-limiting
Confirm? (y/n or suggest alternative)
```

**User:** `y`

**Claude:**
```
Creating .tasks/001-add-rate-limiting/
Researching the codebase... (3 parallel agents)
```

*(agents complete, no experiential tradeoffs identified — skipping prototypes)*

**Claude:**
```
Research complete for 001-add-rate-limiting.

Key findings:
  - API routes defined in api/routes.go, middleware in api/middleware.go
  - Request handling follows handler → middleware → service pattern
  - Auth middleware is the closest existing pattern to reference
  - No rate limiting exists anywhere in the codebase
  - External dependency: stdlib net/http is sufficient, no extra libs needed

2 open questions:
  - Should rate limits be per-user, per-IP, or per-API-key?
  - Should limits be configurable at runtime or compile-time?

Full research: .tasks/001-add-rate-limiting/research.md

Any gaps to fill, or ready for /plan-task 001?
```

---

## Example Session (with prototypes)

**User:** `/research-task Add entity timeline visualization to detail pages`

**Claude:**
```
Scanning .tasks/ for related work...
No existing tasks found.

Proposed task: 002-entity-timeline-view
Confirm? (y/n or suggest alternative)
```

**User:** `y`

**Claude:**
```
Creating .tasks/002-entity-timeline-view/
Researching the codebase... (3 parallel agents)
```

*(agents complete, 1 experiential tradeoff identified)*

```
Classifying open questions...
  - "Which visualization type?" → experiential tradeoff (prototyping)
  - "Should live updates push incrementally?" → policy/requirements (no prototype)

Prototyping 1 decision... (1 agent)
```

*(prototype agent complete)*

**Claude:**
```
Research complete for 002-entity-timeline-view.

Key findings:
  - Detail pages use ECharts for all visualizations
  - Epoch-based push updates feed chart data via signals
  - Timeline data available from _stats tables with event type column

1 decision prototyped:
  - Visualization type: 3 options in prototypes/visualization-type/
    Open each .html file in a browser to compare

2 open questions (1 prototyped, 1 needs user input)

Full research: .tasks/002-entity-timeline-view/research.md

Any gaps to fill, or ready for /plan-task 002?
```

---

## Checklist

Before finalizing research.md:

- [ ] Scanned existing tasks for overlaps
- [ ] User confirmed the task name
- [ ] All three research dimensions covered (architecture, dependencies, tests)
- [ ] Key files identified with roles
- [ ] Existing patterns documented with file references
- [ ] Open questions classified (policy / configuration / experiential)
- [ ] Experiential questions have prototypes (or none identified — skip is fine)
- [ ] Each prototype is standalone and runnable
- [ ] Prototype README has comparison table
- [ ] research.md references prototypes with `[PROTOTYPED]` tags
- [ ] Open questions captured as actionable items
- [ ] Summary is concise and accurate
- [ ] Status is `draft`