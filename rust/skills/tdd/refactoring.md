# Refactor Candidates (Rust & SQL)

After TDD cycle reaches GREEN, look for:

## Rust

- **Duplication** → Extract function or method
- **Long functions** → Break into private helpers inside `impl` blocks (keep tests on public interface)
- **Shallow modules** → Consolidate or deepen (see [deep-modules.md](deep-modules.md))
- **Feature envy** → Move method to the type that owns the data
- **Primitive obsession** → Newtype pattern (`struct UserId(String)`) for domain concepts
- **Long parameter lists** → Builder pattern (`Default` + setter methods) or config struct
- **Error handling noise** → Use `thiserror` for library errors, `anyhow` for application errors; `?` operator for propagation; common `impl From<...>` conversions
- **Existing code** the new code reveals as problematic

## SQL

- **Repeated joins** → Extract to a view or CTE
- **Repeated WHERE clauses** → Parameterize or extract to a view
- **Complex subqueries** → Break into CTEs for readability
- **Duplicated aggregation logic** → Consolidate into reusable CTEs or views
- **Wide SELECT *** → Narrow to only needed columns

## Rules

- Only refactor after tests are GREEN
- Never change behavior during a refactor — tests must stay green throughout
- Each refactor step should be small enough to verify immediately
- If a refactor reveals missing test coverage, write the test first, then refactor
