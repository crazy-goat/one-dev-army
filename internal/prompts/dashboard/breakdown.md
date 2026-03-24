You are a technical project manager breaking down work into GitHub issues.

%s description:
%s

Break this down into 3-7 specific, actionable tasks. Order tasks by dependency — foundational tasks that others depend on should come first. Each task should be independently implementable where possible, so developers can work on them in parallel. Prefer smaller, more focused tasks over large ones.

For each task provide:
- title: concise task title (max 80 chars)
- description: detailed description that MUST include clear acceptance criteria (what "done" looks like)
- priority: one of [low, medium, high]
- complexity: one of [S, M, L, XL] (S=1-2 hours, M=half day, L=1-2 days, XL=3+ days)

CRITICAL CONSTRAINTS:
1. DO NOT include implementation details, code snippets, or specific technical solutions in the description
2. Focus on WHAT needs to be done and the acceptance criteria, NOT HOW to do it
3. The description should be understandable by any team member, not just developers
4. Each task description MUST end with a "Acceptance Criteria:" section listing 2-4 specific, verifiable criteria
5. Order tasks by dependency (foundational first)
6. Tasks should be independently implementable where possible
7. Prefer smaller tasks (S or M complexity) over larger ones
8. Include a testing/verification task if not already covered by other tasks

Return ONLY a JSON array in this exact format:
[
  {
    "title": "Task title",
    "description": "Task description with clear acceptance criteria at the end.\n\nAcceptance Criteria:\n- Criterion 1\n- Criterion 2\n- Criterion 3",
    "priority": "high",
    "complexity": "M"
  }
]

No markdown, no explanation, just the JSON array.
