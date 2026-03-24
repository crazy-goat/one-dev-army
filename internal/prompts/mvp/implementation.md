Implement the following plan for GitHub issue #%d: %s

Implementation plan:
%s

Working directory: %s

Test command: %s

STEP-BY-STEP WORKFLOW:

1. READ AGENTS.md FIRST
   - Read the AGENTS.md file in the working directory for project-specific rules
   - Note the exact lint command and any other requirements

2. ESTABLISH BASELINE
   - Run the test command BEFORE making any changes
   - Verify tests pass in the current state (baseline)

3. IMPLEMENT CHANGES
   - Read existing files before modifying them
   - Make all necessary code changes according to the plan
   - Create new files as needed
   - Verify your implementation matches the plan

4. TEST-FIX-LINT-FIX LOOP (REPEAT UNTIL CLEAN)
   - Run the test command
   - If tests fail, fix the code and re-run tests
   - Run the lint command (from AGENTS.md)
   - If lint errors exist, fix them and re-run lint
   - Do NOT proceed until both tests AND lint pass

5. ATOMIC COMMITS
   - Make focused, atomic commits (one logical change per commit)
   - Use descriptive commit messages referencing the issue number
   - Example: "feat: add user authentication for #123"

6. PUSH CHANGES
   - Push the branch: git push
   - Verify the push succeeded

7. FINAL VERIFICATION
   - Run "git status" and verify the working tree is clean
   - If there are any uncommitted changes, commit and push them
   - Run tests one final time to confirm everything passes

CRITICAL RULES:
- You are in a fully automated pipeline. NEVER ask questions or wait for input.
- Make your best judgment and proceed immediately.
- Do NOT use git worktrees. Work directly in the provided working directory. Do NOT run "git worktree" commands.
- The branch is already created and checked out — do not create new branches.
