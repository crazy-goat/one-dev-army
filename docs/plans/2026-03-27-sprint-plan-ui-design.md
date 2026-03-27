# Design: Sprint Plan UI

**Date:** 2026-03-27  
**Status:** Approved  
**Approach:** React Component (Option C)

## Overview

Nowa strona "Plan Sprint" w dashboardzie ODA, umożliwiająca automatyczny dobór ticketów z puli GitHub Issues (bez przypisanego milestone) do aktualnego sprintu przy użyciu AI.

## Kluczowe Decyzje (z iteracji)

1. **Aktualny sprint** - najstarszy otwarty milestone (funkcja `GetCurrentSprint()` już istnieje)
2. **Kontekst dla AI** - pobieramy ostatni tag (release) dla historii kontekstowej
3. **Target** - liczba ticketów (nie złożoność), zakładamy ~1h per ticket
4. **Overcommit** - soft limit 20%, AI może zaproponować do 120% targetu
5. **Zależności** - GitHub Linked Issues API (pełna integracja)
6. **Drzewko zależności** - "wszystko albo nic" z gałęzi (transitive dependencies)
7. **Draft/Undo/Refresh** - nie implementujemy (F5 wystarczy, użytkownik sam usuwa milestone)
8. **Przycisk Plan Sprint** - widoczny zawsze gdy sprint ma 0 ticketów

## User Flow

1. **Przycisk na Board** - Użytkownik klika "Plan Sprint" (widoczny gdy aktualny sprint ma 0 ticketów)
2. **Przekierowanie** - Przejście na stronę `/sprint/plan`
3. **Konfiguracja** - Użytkownik ustawia target ticket count (domyślnie z configu)
4. **Generowanie** - Kliknięcie "Generate Proposal" uruchamia AI w tle
   - AI pobiera ostatni tag dla kontekstu
   - AI pobiera GitHub Linked Issues dla zależności
5. **Review** - Drzewko zależności z checkboxami
   - Gałąź jest zaznaczona/odznaczona jako całość
   - Odznaczenie elementu = odznaczenie całej gałęzi
6. **Zatwierdzenie** - Kliknięcie "Assign to Sprint" z progressem przypisywania
7. **Sukces** - Redirect na board z komunikatem

## UI Layout

```
┌─────────────────────────────────────────────────────────────┐
│  ← Back to Board                                            │
│                                                             │
│  Sprint: [Nazwa Aktualnego Sprintu]    Dates: [start-end]   │
│                                                             │
│  ┌─────────────────────────────────────────────────────────┐ │
│  │  Target ticket count: [10]  [Generate Proposal]       │ │
│  └─────────────────────────────────────────────────────────┘ │
│                                                             │
│  [Loading spinner / Drzewko zależności]                     │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  ☑ Epic: User Authentication                         │  │
│  │    ├─ ☑ #123 Fix login bug (priority:high)          │  │
│  │    │   └─ ☑ #456 Add password reset (priority:med)  │  │
│  │    └─ ☑ #789 Update docs (priority:low)               │  │
│  ├──────────────────────────────────────────────────────┤  │
│  │  ☑ Feature: Dashboard                                │  │
│  │    └─ ☑ #234 Add charts (priority:medium)            │  │
│  └──────────────────────────────────────────────────────┘  │
│                                                             │
│  Selected: 12/10 tickets (120%)  [Assign to Sprint]       │
└─────────────────────────────────────────────────────────────┘
```

**Assign Progress:**
```
┌─────────────────────────────────────────────────────────────┐
│  Assigning tickets to sprint...                             │
│                                                             │
│  [████████████░░░░░░░░]  6/10 tickets assigned              │
│                                                             │
│  ✓ Epic: User Authentication (3 tickets)                    │
│  ✓ #234 Add charts                                         │
│  → Processing Feature: Dashboard...                        │
└─────────────────────────────────────────────────────────────┘
```

## Component Architecture

```
PlanSprintPage (główny)
├── SprintHeader
│   ├── BackButton
│   └── SprintInfo (nazwa, daty, liczba ticketów)
├── CapacityConfig
│   ├── TargetCountInput
│   └── GenerateButton
├── ProposalSection
│   ├── LoadingState (spinner + logs)
│   └── DependencyTree
│       └── TreeNode (checkbox + recursive children)
└── ActionBar
    ├── Stats (selected / target + %)
    └── AssignButton
```

## API Endpoints

### GET /api/v2/sprint/current
Zwraca dane aktualnego otwartego sprintu (najstarszy otwarty milestone).

```json
{
  "number": 5,
  "title": "Sprint 12",
  "state": "open",
  "startDate": "2026-03-27",
  "endDate": "2026-04-10",
  "ticketCount": 0
}
```

### GET /api/v2/issues/unassigned
Zwraca listę ticketów bez przypisanego milestone (kandydaci).

```json
[
  {
    "number": 123,
    "title": "Fix login bug",
    "labels": ["priority:high", "type:bug"],
    "complexity": 3
  }
]
```

### GET /api/v2/sprint/last-tag
Zwraca ostatni tag (release) dla kontekstu AI.

```json
{
  "tag": "v1.2.3",
  "date": "2026-03-20",
  "issues": [100, 101, 102]
}
```

### GET /api/v2/issues/:number/linked
Zwraca powiązane issue (GitHub Linked Issues) dla budowania drzewka.

```json
{
  "number": 123,
  "linked_issues": [
    {"number": 456, "relationship": "blocked_by"},
    {"number": 789, "relationship": "blocks"}
  ]
}
```

### POST /api/v2/sprint/propose
Uruchamia AI do doboru ticketów (async).

**Request:**
```json
{
  "targetCount": 10,
  "lastTag": "v1.2.3",
  "includeDependencies": true
}
```

**Response:**
```json
{
  "jobId": "abc-123"
}
```

### GET /api/v2/sprint/propose/:jobId
Pobiera status i wynik propozycji AI.

**Response:**
```json
{
  "status": "completed",
  "proposal": [
    {
      "number": 123,
      "title": "Fix login bug",
      "reason": "High priority bug",
      "complexity": 3,
      "dependencies": [456, 789],
      "branch": "auth-epic"
    }
  ],
  "branches": [
    {
      "id": "auth-epic",
      "name": "Epic: User Authentication",
      "root_issue": 123,
      "issues": [123, 456, 789],
      "total_complexity": 12
    }
  ]
}
```

### POST /api/v2/sprint/assign
Przypisuje wybrane tickety do aktualnego sprintu (streaming).

**Request:**
```json
{
  "issueNumbers": [123, 456, 789],
  "branches": ["auth-epic", "dashboard-feature"]
}
```

**Response:** SSE stream
```
data: {"type": "progress", "current": 1, "total": 10, "issue": 123, "branch": "auth-epic"}
data: {"type": "progress", "current": 4, "total": 10, "issue": 789, "branch": "auth-epic"}
data: {"type": "completed"}
```

## Data Structures

```typescript
// Sprint info
interface Sprint {
  number: number;
  title: string;
  state: 'open' | 'closed';
  startDate: string;
  endDate: string;
  ticketCount: number;
}

// Issue candidate
interface IssueCandidate {
  number: number;
  title: string;
  labels: string[];
  complexity?: number;
  priority?: 'high' | 'medium' | 'low';
  type?: 'bug' | 'feature' | 'docs' | 'other';
}

// Linked issue (GitHub Linked Issues)
interface LinkedIssue {
  number: number;
  relationship: 'blocked_by' | 'blocks' | 'relates_to';
}

// Branch (gałąź zależności)
interface Branch {
  id: string;
  name: string;
  root_issue: number;
  issues: number[];
  total_complexity: number;
}

// AI Proposal
interface Proposal {
  issues: ProposedIssue[];
  branches: Branch[];
}

interface ProposedIssue extends IssueCandidate {
  reason: string;
  dependencies: number[];
  branch: string;
}

// Assignment progress
interface AssignmentProgress {
  type: 'progress' | 'completed' | 'error';
  current: number;
  total: number;
  issue?: number;
  branch?: string;
  error?: string;
}

// Tree node for UI
interface TreeNode {
  issue: ProposedIssue;
  children: TreeNode[];
  branch: string;
  selected: boolean;
}
```

## Drzewko Zależności - Logika "Wszystko albo Nic"

### Zasady:
1. Każda gałąź ma root issue (najwyższy w hierarchii)
2. Zaznaczenie roota = zaznaczenie całej gałęzi
3. Odznaczenie dowolnego elementu = odznaczenie całej gałęzi
4. Gałąź jest albo w sprincie w całości, albo wcale

### Przykład:
```
Epic: Auth (root)
├── #123 Login bug (blokuje #456)
│   └── #456 Password reset (zależy od #123)
└── #789 Docs update (niezależne)
```

- Zaznaczam Epic → wszystkie 3 tickety zaznaczone
- Odznaczam #456 → cała gałąź Auth się odznacza (Epic, #123, #456, #789)
- Zaznaczam tylko #789 → nie działa, bo #789 jest częścią gałęzi Auth

## Error Handling

### Scenariusze błędów:

1. **Brak aktualnego sprintu**
   - Komunikat: "No active sprint. Create one first."
   - Przycisk redirect do tworzenia sprintu

2. **Brak ticketów bez milestone**
   - Komunikat: "No unassigned issues available."
   - Info jak dodać nowe issues

3. **AI timeout/error**
   - Retry button
   - Logi błędu do debugowania

4. **GitHub API error przy assign**
   - Partial success screen
   - Lista: zapisane / niezapisane tickety
   - Retry dla niezapisanych

5. **Rate limiting**
   - Exponential backoff
   - Info użytkownikowi: "GitHub API rate limit, retrying in X seconds..."

6. **GitHub Linked Issues API niedostępne**
   - Fallback: parsowanie opisów ticketów (#123)
   - Warning: "Using fallback dependency detection"

### UI Error States:
- Toast notifications dla błędów
- Retry buttons przy każdym etapie
- "Partial success" view gdy część ticketów się nie zapisała
- Loading states z cancel option (tylko dla generowania AI)

## AI Selection Criteria

AI dobiera tickety na podstawie:
1. **Priority** (priority:high > medium > low)
2. **Type** (bug > feature > docs > other)
3. **Zależności** - wybiera kompletne gałęzie (wszystko albo nic)
4. **Kontekst z ostatniego tagu** - co było robione w poprzednim releasie
5. **Target count** - soft limit, może przekroczyć o 20%

**Prompt do LLM:**
```
Given:
1. List of unassigned GitHub issues (with labels, complexity)
2. Last release tag: {lastTag} (for context)
3. Target count: {targetCount} (soft limit, can exceed by 20%)
4. GitHub Linked Issues data (dependency graph)

Select the best issues to include in the current sprint.

Rules:
- Group issues into branches based on dependencies
- Select complete branches only (all or nothing)
- Prioritize: priority:high > medium > low
- Consider what was done in last release for context
- Can exceed target by up to 20% if it completes important branches

Return JSON with:
1. Selected issues with reasoning
2. Branches structure (root issue + all dependencies)
3. Total count (can be 100-120% of target)

Format:
{
  "issues": [...],
  "branches": [...]
}
```

## Configuration

W `.oda/config.yaml`:
```yaml
sprint:
  planning:
    default_target_count: 10
    max_overcommit_percentage: 20  # can exceed target by 20%
```

## Success Criteria

- [ ] Użytkownik może wygenerować propozycję AI jednym kliknięciem
- [ ] AI pobiera ostatni tag dla kontekstu
- [ ] AI pobiera GitHub Linked Issues dla zależności
- [ ] Propozycja pokazuje drzewko zależności z gałęziami
- [ ] Zaznaczanie działa jako "wszystko albo nic" z gałęzi
- [ ] Progres bar pokazuje postęp przypisywania do GitHub (z podziałem na gałęzie)
- [ ] Po zakończeniu redirect na board z komunikatem sukcesu
- [ ] Wszystkie błędy są obsłużone z retry options
- [ ] Fallback dla GitHub Linked Issues API (parsowanie opisów)
