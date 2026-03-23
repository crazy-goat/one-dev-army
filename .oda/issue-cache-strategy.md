## Problem: Nieaktualne dane z GitHub API nadpisują lokalny cache

### Opis problemu
Obecnie mechanizm synchronizacji z GitHubem (`SyncService`) używa strategii **zawsze nadpisuj** (`INSERT OR REPLACE`). Gdy GitHub API zwraca nieaktualne dane (cache CDN po ich stronie), lokalny cache w SQLite jest nadpisywany starszymi informacjami.

**Przykład scenariusza błędu:**
1. Ticket #123 ma labelkę `stage:in-progress` (zapisana w lokalnym cache)
2. Użytkownik przesuwa ticket na boardzie → labelka zmienia się na `stage:code-review`
3. Przychodzi auto-sync z GitHuba (co 30s)
4. GitHub zwraca jeszcze starą wersję (cache CDN)
5. Lokalny cache jest nadpisywany starą labelką `stage:in-progress`
6. Dashboard pokazuje nieaktualny stan

### Proponowane rozwiązania

#### Opcja 1: Konserwatywny (timestamp-based)
**Mechanizm:** Przed zapisem porównuj `updated_at` z GitHub API z `cached_at` w SQLite. Zapisz tylko jeśli dane z GitHuba są nowsze.

**Zalety:**
- Chroni przed nadpisaniem nowszych danych starszymi
- Prosta implementacja

**Wady:**
- Jeśli ktoś zmieni labelkę ręcznie w UI ODA (bezpośrednio w SQLite), a potem przyjdzie sync ze starszym timestampem ze strony GitHuba - zmiana zostanie cofnięta
- Nie rozróżnia źródła zmiany (auto-sync vs ręczna akcja)

#### Opcja 2: Hybrydowy (kontekstowy)
**Mechanizm:** 
- Dla **auto-sync** (co 30s): Konserwatywny - ignoruj starsze dane z GitHuba
- Dla **ręcznych akcji** (przycisk w dashboard, zmiana stage przez API): Agresywny - zawsze zapisz + aktualizuj timestamp

**Zalety:**
- Chroni przed bugiem cache CDN
- Ręczne zmiany w dashboardzie działają natychmiast i nie są cofane
- Przewidywalne zachowanie dla użytkownika

**Wady:**
- Nieco bardziej złożona implementacja (trzeba dodać flagę `force` do `SaveIssueCache`)
- Wymaga zmian w handlerach dashboardu

#### Opcja 3: Pesymistyczny (zawsze czyść przed sync)
**Mechanizm:** Przed każdym synciem czyść cały cache i buduj od nowa. Nie ma problemu nadpisywania, bo zawsze zaczynamy od zera.

**Zalety:**
- Najprostsza implementacja
- Gwarancja spójności

**Wady:**
- Utrata historii zmian w cache (np. `merged_at` może się różnić między syncami)
- Krótkie "miganie" danych na dashboardzie podczas sync
- Większe obciążenie SQLite (DELETE + INSERT zamiast UPDATE)

---

### Rekomendacja
**Opcja 2 (Hybrydowy)** - najlepszy balans między bezpieczeństwem a funkcjonalnością.

### Zadania do wykonania
- [ ] Dodać pole `updated_at` (timestamp z GitHub API) do struktury `github.Issue`
- [ ] Zmodyfikować `SaveIssueCache` aby przyjmował flagę `force bool`
- [ ] Zmienić logikę w `sync.go` (auto-sync) na konserwatywną
- [ ] Zmienić logikę w `dashboard/handlers.go` (ręczne akcje) na agresywną
- [ ] Dodać logowanie gdy dane są pomijane z powodu starego timestampa

---

**Którą opcję wybieramy?**
