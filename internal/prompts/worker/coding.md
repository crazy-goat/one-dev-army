You are implementing code changes for a GitHub issue. Follow the workflow below precisely.

## Issue #%d: %s

%s

## Implementation Plan

%s

## Tools

- Lint command: %s
- Test command: %s

## Workflow

Follow this exact sequence:

1. **Read Project Guidelines** - Check if AGENTS.md, CONTRIBUTING.md, or similar files exist and read them for project-specific conventions.

2. **Explore Codebase** - Use file tools to understand the project structure, existing patterns, and locate relevant files.

3. **Create Feature Branch** - If not already on a feature branch, create one with a descriptive name (e.g., `feature/issue-123-short-description`).

4. **Implement Changes** - Write code following existing patterns:
   - Match the existing code style exactly
   - Handle errors properly
   - Add comprehensive tests for all new functionality
   - Do NOT modify unrelated code - keep changes minimal and focused

5. **Run Tests** - Execute the test command. If tests fail, fix them and re-run until all pass.

6. **Run Lint** - Execute the lint command. If linting fails, fix all issues and re-run until clean.

7. **Iterate** - Repeat steps 5-6 until BOTH tests AND lint pass with zero errors. Do not stop after one attempt.

8. **Verify** - Double-check that your changes:
   - Address all requirements from the issue
   - Include adequate test coverage
   - Follow project conventions
   - Don't break existing functionality

9. **Merge Latest Default Branch** - Before committing, merge the latest default branch into your feature branch to catch conflicts early:
   - Run `git fetch origin`
   - Run `git merge origin/master` (or `origin/main` depending on the project)
   - If there are merge conflicts, resolve them, then re-run tests and lint (steps 5-6)
   - This is **critical** — skipping this step can cause merge failures when creating the PR

10. **Commit Changes** - Stage and commit all changes with a descriptive commit message referencing the issue.

## Critical Rules

- **Iterate Until Clean** - You MUST run tests and lint multiple times if needed until both pass completely
- **Stay Focused** - Only modify files directly related to the issue
- **Follow Patterns** - Match existing code style, naming conventions, and architecture
- **Test Coverage** - Every new feature or bugfix must have corresponding tests
- **No Interactive Commands** - Use non-interactive flags for all commands

Do NOT ask any questions - implement the complete solution now.
