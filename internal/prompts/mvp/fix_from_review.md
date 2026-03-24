Fix the issues found during code review for GitHub issue #%d: %s

Working directory: %s

Test command: %s

Code review feedback:
%s

STEP-BY-STEP WORKFLOW:

1. UNDERSTAND THE REVIEW
   - Read and understand each review comment carefully
   - Identify which files are mentioned in the review
   - Note the specific issues raised and their severity

2. FIX ISSUES SYSTEMATICALLY
   - Read the files mentioned in the review
   - Fix ALL issues raised by the reviewer
   - For each fix, verify it specifically addresses the review comment
   - Do NOT introduce new issues while fixing existing ones
   - Do NOT change code unrelated to the review feedback

3. TEST-FIX-LINT-FIX LOOP (REPEAT UNTIL CLEAN)
   - Run the test command
   - If tests fail, fix the code and re-run tests
   - Run the lint command for this project (see AGENTS.md)
   - If lint errors exist, fix them and re-run lint
   - Do NOT proceed until both tests AND lint pass

4. VERIFY FIXES
   - Review each fix to ensure it directly addresses the corresponding review comment
   - Check that no new issues were introduced
   - Run tests one final time to confirm everything passes

5. COMMIT AND PUSH
   - Commit your fixes with a descriptive message referencing the review
   - Example: "fix: address code review feedback for #123"
   - Push the branch: git push
   - Verify the push succeeded

6. FINAL CHECK
   - Run "git status" and verify the working tree is clean
   - If there are uncommitted changes, commit and push them

CRITICAL RULES:
- You are in a fully automated pipeline. NEVER ask questions or wait for input.
- Make your best judgment and proceed immediately.
- Do NOT use git worktrees. Work directly in the provided working directory. Do NOT run "git worktree" commands.
