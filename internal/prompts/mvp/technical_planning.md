Analyze and create an implementation plan for GitHub issue #%d: %s

Issue body:
%s

CRITICAL FIRST STEP — Check if already implemented:
Before creating any plan, READ the relevant source files in the codebase using available file tools. Verify if this feature/fix is ALREADY IMPLEMENTED. If and ONLY if the existing code already fully satisfies all issue requirements with no changes needed, respond with a single line starting with the exact prefix ALREADY_DONE: followed by your concrete evidence (e.g. "method Foo already exists in bar.go:42"). Do NOT use this if the feature is only partially implemented or needs any modifications.

If changes ARE needed (which is the expected case), provide a comprehensive technical analysis and implementation plan with the following structure:

## Analysis

1. Core requirements — what exactly needs to be done
2. Files that likely need changes (based on reading the actual codebase)
3. Implementation approach — high-level strategy
4. Testing strategy — what tests to write or update
5. Complexity estimate — small (hours), medium (days), or large (weeks)
6. Potential breaking changes — any API changes, database migrations, or backward-incompatible modifications

## Implementation Plan

Create a concrete, actionable plan covering:
1. Which files to create or modify (exact paths) — based on actually reading the codebase
2. What code changes to make in each file
3. What tests to add or update — check existing test files and patterns first
4. Coding conventions to follow — identify the project's style from existing code
5. Order of operations — step-by-step sequence

REQUIREMENTS:
- Use file tools to READ the codebase before planning — understand the existing structure, patterns, and conventions
- Check for existing tests and test patterns in the project
- Identify the project's coding conventions by examining existing code
- Be specific and actionable — exact file paths, function names, etc.
- Do NOT ask questions. Output both sections directly.
