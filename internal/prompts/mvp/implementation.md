Implement the following plan for GitHub issue #%d: %s

Implementation plan:
%s

Working directory: %s

Test command: %s

Instructions:
- Read existing files before modifying them
- Make all necessary code changes
- Create new files as needed
- After implementing, run the test command and fix any failures until all tests pass
- Do NOT proceed until tests pass — iterate on the code until they do
- Run the lint command for this project and fix ALL lint errors before finishing (see AGENTS.md in the working directory for the exact lint command)
- Commit ALL your changes with a descriptive message
- After committing, push the branch: git push
- Before finishing, run "git status" and verify the working tree is clean — if there are any uncommitted changes, commit and push them
- You are in a fully automated pipeline. NEVER ask questions or wait for input.
- Make your best judgment and proceed immediately.
- CRITICAL: Do NOT use git worktrees. Work directly in the provided working directory. Do NOT run "git worktree" commands.
