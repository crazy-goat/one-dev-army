# Implementation Plan for Issue #173

**Created:** 2026-03-23T12:46:24+01:00
**Updated:** 2026-03-23T12:46:24+01:00

## Analysis

1. **Core requirements**: The `TechnicalPlanningPromptTemplate` (lines 71-118) currently generates system architecture descriptions instead of actionable solution approaches. Need to restructure sections to focus on "how to solve" rather than "how the system works".

2. **Files that need changes**:
   - `/internal/dashboard/prompts.go:71-118` - Main template modification
   - `/internal/dashboard/prompts_test.go:15-34` - Update tests to verify new sections

3. **Implementation approach**: Replace "Architecture Overview" and "Component Dependencies" with "Suggested Approach" and "Key Considerations". Update CRITICAL RULES to emphasize actionable guidance over system description.

4. **Testing strategy**: Update existing test assertions to check for new section names instead of removed ones.

## Implementation Steps

### Step 1: **Modify `/internal/dashboard/prompts.go:71-118`**:

- Replace "Architecture Overview" section with "Suggested Approach" section that asks for the best approach based on codebase analysis
- Replace "Component Dependencies" section with "Key Considerations" section for performance, security, etc.
- Rename "Files Requiring Changes" to "Files to Modify" and enhance to require explanations of WHY each file needs changes
- Update CRITICAL RULES (lines 106-112) to emphasize approach over description

### Step 2: **Update `/internal/dashboard/prompts_test.go:15-34`**:

- Change test assertions from checking "Architecture Overview" and "Component Dependencies" to checking "Suggested Approach" and "Key Considerations"

### Step 3: **Order of operations**:

- First, modify the prompt template in prompts.go
- Then update the test assertions in prompts_test.go
- Run tests to verify changes work correctly

