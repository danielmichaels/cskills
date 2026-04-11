# cskills

A CLI tool for installing [Claude Code](https://docs.anthropic.com/en/docs/claude-code) skills into any repository.

Skills are markdown files that live in `.claude/skills/` and guide Claude Code's behaviour
when working in your codebase. `cskills` ships a curated set of these, embedded in the binary,
so you can bootstrap a new repo in seconds.

## Install

```bash
go install github.com/danielmichaels/cskills/cmd/cskills@latest
```

## Usage

### List available skills

```bash
# All languages
cskills list

# Filter by language
cskills list --lang go
```

### Install skills

```bash
# Install "always" skills and interactively pick custom ones
cskills install --lang go

# Install everything (always + custom)
cskills install --lang go --all

# Install specific skills by name
cskills install --lang go --skill tdd,datastar

# Overwrite existing skill files
cskills install --lang rust --all --force
```

Invalid skill names produce an error with the list of valid options:

```
error: unknown skill "foo" for go (available: datastar, plan-task, research-task, tdd)
```

## Available skills

| Skill | Languages | Category | Description |
|-------|-----------|----------|-------------|
| tdd | go, rust | always | Test-driven development practices |
| research-task | go, rust | always | Research a codebase task by exploring code, patterns, and dependencies |
| plan-task | go, rust | always | Design an implementation plan for a researched task |
| implement-task | go, rust | always | Execute a planned task by spawning an agent team from todos.md |
| datastar | go | custom | Best practices for building web apps with the Datastar hypermedia framework |

### Skill categories

- **always** — installed by default with `cskills install --lang <lang>`
- **custom** — only installed when explicitly selected (`--skill`, `--all`, or interactive prompt)

## Adding skills

Create a new directory under `<lang>/skills/<name>/` with a `SKILL.md` file containing frontmatter:

```markdown
---
name: my-skill
description: What this skill teaches Claude Code
category: always
---

Skill content goes here...
```

Any additional files in the skill directory are installed alongside `SKILL.md`.
