Implement the following plan for GitHub issue #%d: %s

Implementation plan:
%s

Working directory: %s

Test command: %s

ARTIFACT — Save coding notes to file:
Before starting implementation, READ the planning artifact at: .oda/artifacts/%d/01-planning.md
Save your coding notes, decisions, and progress to: .oda/artifacts/%d/02-coding.md

CRITICAL: Save coding notes to the artifact file using file write tools BEFORE returning your response.

PIPELINE FAILURE RECOVERY:
Check if the file .oda/artifacts/%d/pipeline-fail.log exists.
If the file exists:
- CRITICAL: Read the pipeline failure logs from this file
- CRITICAL: Fix ALL errors described in the logs BEFORE proceeding with any other work
- The pipeline previously failed — your primary goal is to fix those failures
If the file does not exist, proceed normally with the implementation plan.

STEP-BY-STEP WORKFLOW:

0. CHECK FOR PIPELINE FAILURE LOGS
   - Check if .oda/artifacts/%d/pipeline-fail.log exists
   - If the file exists:
     - CRITICAL: Read the pipeline failure logs from this file
     - CRITICAL: Fix ALL errors described in the logs BEFORE proceeding
     - The pipeline previously failed — your primary goal is to fix those failures
   - If the file does not exist, skip this step

1. READ AGENTS.md FIRST
   - Read the AGENTS.md file in the working directory for project-specific rules
   - Note the exact lint command and any other requirements

2. READ PLANNING ARTIFACT
   - Read .oda/artifacts/%d/01-planning.md to understand the technical plan

3. ESTABLISH BASELINE
   - Run the test command BEFORE making any changes
   - Verify tests pass in the current state (baseline)

4. IMPLEMENT CHANGES
   - Read existing files before modifying them
   - Make all necessary code changes according to the plan
   - Create new files as needed
   - Verify your implementation matches the plan

5. TEST-FIX-LINT-FIX LOOP (REPEAT UNTIL CLEAN)
   - Run the test command
   - If tests fail, fix the code and re-run tests
   - Run the lint command (from AGENTS.md)
   - If lint errors exist, fix them and re-run lint
   - Do NOT proceed until both tests AND lint pass

6. ATOMIC COMMITS
   - Make focused, atomic commits (one logical change per commit)
   - Use descriptive commit messages referencing the issue number
   - Example: "feat: add user authentication for #123"

7. PUSH CHANGES
   - Push the branch: git push
   - Verify the push succeeded

8. FINAL VERIFICATION
   - Run "git status" and verify the working tree is clean
   - If there are any uncommitted changes, commit and push them
   - Run tests one final time to confirm everything passes

CRITICAL RULES:
- You are in a fully automated pipeline. NEVER ask questions or wait for input.
- Make your best judgment and proceed immediately.
- Do NOT use git worktrees. Work directly in the provided working directory. Do NOT run "git worktree" commands.
- The branch is already created and checked out — do not create new branches.
