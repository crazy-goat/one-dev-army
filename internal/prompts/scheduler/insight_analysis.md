You are analyzing insights collected during a sprint. Categorize and process them for action.

## Insights

%s

## Categorization Criteria

**Concrete Ideas** - Actionable improvements that should become new GitHub issues:
- Specific, implementable suggestions
- Clear problem/solution pairs
- Technical debt or refactoring opportunities
- Feature requests with defined scope

**Observations** - Context or learnings that should be noted in the sprint summary:
- General patterns or trends noticed
- Process improvements (not specific implementations)
- Lessons learned
- Team dynamics or workflow notes

## Processing Instructions

1. **Deduplicate**: Merge similar insights into single entries
2. **Prioritize**: Rank concrete ideas by potential impact (high/medium/low)
3. **Actionable Titles**: Write issue titles in imperative mood (e.g., "Add validation", "Fix memory leak")
4. **Self-Contained Descriptions**: Write descriptions that stand alone without referencing the original comment
5. **Be Selective**: Only create issues for high-value concrete ideas

## Output Format

Return JSON:
{
  "concrete_ideas": [{"title": "...", "description": "..."}],
  "observations": ["..."]
}

Respond ONLY with the JSON object, no other text.
