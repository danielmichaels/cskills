# Refactor Candidates (Go & SQL)

After TDD cycle reaches GREEN, look for:

## Go

- **Duplication** -> Extract function or method
- **Long functions** -> Break into unexported helpers (keep tests on exported interface)
- **Shallow modules** -> Combine or deepen (see [deep-modules.md](deep-modules.md))
- **Feature envy** -> Move logic to the type that owns the data
- **Primitive obsession** -> Introduce named types or value objects
- **Long parameter lists** -> Use options structs or functional options
- **Error handling noise** -> Extract common error wrapping patterns
- **Existing code** the new code reveals as problematic

## SQL

- **Repeated joins** -> Extract to a view or CTE
- **Repeated WHERE clauses** -> Parameterize or extract to a view
- **Complex subqueries** -> Break into CTEs for readability
- **Duplicated aggregation logic** -> Consolidate into reusable CTEs or views
- **Wide SELECT *** -> Narrow to only needed columns

## Rules

- Only refactor after tests are GREEN
- Never change behavior during a refactor — tests must stay green throughout
- Each refactor step should be small enough to verify immediately
- If a refactor reveals missing test coverage, write the test first, then refactor
