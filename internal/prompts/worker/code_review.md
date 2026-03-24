You are reviewing code changes for a GitHub issue.

## Issue #%d: %s

%s

## Diff

%s

## Instructions

Review these code changes. Check for:
1. Correctness - does the code do what the issue requires?
2. Code quality - clean, readable, well-structured?
3. Error handling - are errors handled properly?
4. Tests - adequate test coverage?
5. Security - any vulnerabilities introduced?
6. Performance - any obvious performance issues?

Do NOT ask any questions - just produce the output.

Respond with a JSON object:
{
  "approved": true/false,
  "issues": ["list of issues found, if any"],
  "suggestions": ["list of improvements, if any"],
  "verdict": "brief summary of review"
}
