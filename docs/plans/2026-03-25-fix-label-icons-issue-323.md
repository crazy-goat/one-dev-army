# Fix Label Icons - Wrong Emojis Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix wrong emoji mappings in the `labelIcon` template function so size labels use animal emojis and priority labels use colored circles per the #317 spec.

**Architecture:** Direct emoji replacement in the `labelIcon` switch statement in `server.go` and corresponding test updates in `handlers_test.go`. The `labelTooltip` function and template wiring are already correct — no changes needed there.

**Tech Stack:** Go, html/template

---

### Task 1: Update `labelIcon` emojis in server.go

**Files:**
- Modify: `internal/dashboard/server.go:153-180`

**Step 1: Fix the emoji mappings**

Replace the `labelIcon` function body (lines 153-180) with corrected emojis:

```go
"labelIcon": func(label string) string {
    switch label {
    case "type:feature":
        return "✨"
    case "type:bug":
        return "🐛"
    case "type:docs":
        return "📚"
    case "type:refactor":
        return "🔧"
    case "size:S":
        return "🐜"
    case "size:M":
        return "🐕"
    case "size:L":
        return "🐘"
    case "size:XL":
        return "🦕"
    case "priority:high":
        return "🔴"
    case "priority:medium":
        return "🟡"
    case "priority:low":
        return "🟢"
    default:
        return ""
    }
},
```

Changes:
| Label | Old | New |
|-------|-----|-----|
| size:S | 🟢 | 🐜 |
| size:M | 🟡 | 🐕 |
| size:L | 🟠 | 🐘 |
| size:XL | 🔴 | 🦕 |
| priority:high | 🔥 | 🔴 |
| priority:medium | ⚡ | 🟡 |
| priority:low | 🌱 | 🟢 |

**Step 2: Verify it compiles**

Run: `go build ./internal/dashboard/...`
Expected: Success, no errors

**Step 3: Commit**

```bash
git add internal/dashboard/server.go
git commit -m "fix: correct label icon emojis for size and priority labels (#323)"
```

---

### Task 2: Update test helper funcMap in handlers_test.go

**Files:**
- Modify: `internal/dashboard/handlers_test.go:51-77`

**Step 1: Fix the helper funcMap emojis**

Update the `labelIcon` function in the test helper funcMap (around lines 51-77) to match the corrected emojis. Same 7 replacements as Task 1:

```go
"labelIcon": func(label string) string {
    switch label {
    case "type:feature":
        return "✨"
    case "type:bug":
        return "🐛"
    case "type:docs":
        return "📚"
    case "type:refactor":
        return "🔧"
    case "size:S":
        return "🐜"
    case "size:M":
        return "🐕"
    case "size:L":
        return "🐘"
    case "size:XL":
        return "🦕"
    case "priority:high":
        return "🔴"
    case "priority:medium":
        return "🟡"
    case "priority:low":
        return "🟢"
    default:
        return ""
    }
},
```

**Step 2: Verify it compiles**

Run: `go build ./internal/dashboard/...`
Expected: Success

---

### Task 3: Update TestLabelIcon test expectations and inline funcMap

**Files:**
- Modify: `internal/dashboard/handlers_test.go:4726-4796`

**Step 1: Fix test case expectations (lines 4736-4742)**

```go
{"size:S", "size:S", "🐜"},
{"size:M", "size:M", "🐕"},
{"size:L", "size:L", "🐘"},
{"size:XL", "size:XL", "🦕"},
{"priority:high", "priority:high", "🔴"},
{"priority:medium", "priority:medium", "🟡"},
{"priority:low", "priority:low", "🟢"},
```

**Step 2: Fix inline funcMap in TestLabelIcon (lines 4752-4776)**

Same 7 emoji replacements as the main function:
- `size:S` → `🐜`, `size:M` → `🐕`, `size:L` → `🐘`, `size:XL` → `🦕`
- `priority:high` → `🔴`, `priority:medium` → `🟡`, `priority:low` → `🟢`

**Step 3: Run TestLabelIcon**

Run: `go test -race -run TestLabelIcon ./internal/dashboard/`
Expected: PASS

---

### Task 4: Update TestBoardTemplate_LabelIcons assertions

**Files:**
- Modify: `internal/dashboard/handlers_test.go:4914-4922`

**Step 1: Fix icon assertions**

Line 4918-4919: Change `🟡` to `🐕` and update error message:
```go
if !strings.Contains(output, "🐕") {
    t.Error("template should contain size M icon (🐕)")
}
```

Lines 4921-4922: Change `🔥` to `🔴` and update error message:
```go
if !strings.Contains(output, "🔴") {
    t.Error("template should contain high priority icon (🔴)")
}
```

**Step 2: Run TestBoardTemplate_LabelIcons**

Run: `go test -race -run TestBoardTemplate_LabelIcons ./internal/dashboard/`
Expected: PASS

**Step 3: Run all dashboard tests**

Run: `go test -race ./internal/dashboard/...`
Expected: All PASS

**Step 4: Run linter**

Run: `golangci-lint run ./internal/dashboard/...`
Expected: No errors

**Step 5: Commit**

```bash
git add internal/dashboard/handlers_test.go
git commit -m "test: update label icon tests to match corrected emojis (#323)"
```
