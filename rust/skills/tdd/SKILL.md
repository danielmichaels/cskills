---
name: tdd
description: "Test-driven development practices for Rust and SQL codebases"
category: always
---

# Test-Driven Development Guide (Rust & SQL)

You are an expert in test-driven development for Rust and SQL codebases. Follow these principles rigorously.

## Key Philosophy

Tests should verify behavior through public interfaces, not implementation details. Good tests are integration-style, reading like specifications of what the system accomplishes rather than how it works internally.

## Critical Anti-Pattern: Horizontal Slicing

Never write all tests upfront, then implement everything afterward. This produces poor tests because they verify imagined behavior and test data structure shapes rather than actual user-facing capabilities.

Instead, use **vertical slicing** through "tracer bullets" — one test, one implementation, repeat — allowing each test to respond to learnings from the previous cycle.

## Practical Workflow

1. **Planning**: Confirm interfaces and prioritize which behaviors matter most
2. **Tracer Bullet**: Write one test for one behavior, then minimal code to pass it
3. **Incremental Loop**: Repeat for remaining behaviors without anticipating future needs
4. **Refactor**: Only after reaching GREEN, extract duplication and deepen modules

Each cycle must verify:

- The test describes observable behavior through public interfaces
- The test would survive internal refactoring
- The test uses `assert_eq!`, `assert!`, or `assert_ne!` for concise assertions
- SQL queries are tested against a real DuckDB instance, not mocked

## Rust-Specific TDD Rules

- Use `#[test]` and `#[cfg(test)]` modules for unit tests
- Use `assert_eq!`, `assert!`, `assert_ne!` — or `pretty_assertions` crate for richer diffs
- Use `rstest` crate `#[rstest]` / `#[case(...)]` for parameterized / table-driven tests
- Use `#[tokio::test]` for async tests
- Test public API only (`pub` / `pub(crate)`) — never test private items directly
- Use `mockall` `#[automock]` at system boundaries (external APIs, time, randomness)
- SQL queries tested against a real DuckDB connection, not mocked

## SQL-Specific TDD Rules

- Test queries against a real DuckDB instance with minimal seed data
- Verify query results through the Rust function that executes them, not by inspecting raw SQL output
- Test edge cases: NULL values, empty result sets, boundary epochs
- For aggregation queries, seed the minimum rows needed to validate the rollup

## References

- [deep-modules.md](deep-modules.md) - Design deep modules with small interfaces
- [interface-design.md](interface-design.md) - Design interfaces for testability
- [mocking.md](mocking.md) - When and how to mock in Rust
- [refactoring.md](refactoring.md) - What to refactor after tests pass
- [tests.md](tests.md) - Good vs bad test patterns
