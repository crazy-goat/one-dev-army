Analyze and create an implementation plan for GitHub issue #%d: %s

Issue body:
%s

Provide a comprehensive technical analysis and implementation plan with the following structure:

## Analysis

1. Core requirements — what exactly needs to be done
2. Files that likely need changes
3. Implementation approach — high-level strategy
4. Testing strategy — what tests to write or update

## Implementation Plan

IMPORTANT: First check if this feature/fix is ALREADY IMPLEMENTED in the codebase.
Read the relevant source files and verify. If and ONLY if the existing code already fully satisfies
all issue requirements with no changes needed, respond with a single line starting with the
exact prefix ALREADY_DONE: followed by your concrete evidence (e.g. "method Foo already exists in bar.go:42").
Do NOT use this if the feature is only partially implemented or needs any modifications.

If changes ARE needed (which is the expected case), create a concrete, actionable plan covering:
1. Which files to create or modify (exact paths)
2. What code changes to make in each file
3. What tests to add or update
4. Order of operations

Be specific and actionable. Do NOT ask questions. Output both sections directly.
