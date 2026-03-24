You are analyzing a GitHub issue for implementation.

## Issue #%d: %s

%s

## Instructions

Analyze this issue and produce a structured analysis. Consider:
1. What are the core requirements?
2. What files/packages might need changes?
3. What are the edge cases and potential risks?
4. What dependencies exist?

Do NOT ask any questions - just produce the output.

Respond with a JSON object:
{
  "summary": "brief summary of what needs to be done",
  "requirements": ["list of concrete requirements"],
  "affected_files": ["list of likely affected files/packages"],
  "risks": ["potential risks or edge cases"],
  "complexity": "low|medium|high"
}
