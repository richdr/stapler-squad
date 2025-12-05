# BUG-011: UI Category Rendering Test Failure [SEVERITY: High]

**Status**: 🐛 Open
**Discovered**: 2025-12-05 during test stabilization work
**Impact**: UI rendering tests failing, unknown production impact

## Problem Description

UI tests for category rendering are failing. This is **distinct from BUG-008** which is specifically about sessions not rendering in categories. This bug covers broader UI rendering issues.

**Failing tests** (suspected):
1. Category header rendering
2. Category expand/collapse UI
3. Category styling and borders
4. Category navigation visual feedback
5. Additional UI rendering tests (TBD)

**Unknown details**:
- Exact failing tests not captured
- Specific error messages not recorded
- Whether this affects production UI

## Reproduction

```bash
# Run UI tests
go test ./ui -v

# Likely failures in category rendering tests
# Exact tests and errors TBD
```

**Expected**: All UI rendering tests pass
**Actual**: Multiple failures (exact list TBD)

## Root Cause Analysis

**Investigation required** - Potential causes:

### 1. Terminal Size Assumptions

```go
// Tests may assume specific terminal dimensions
// Rendering output depends on width/height
// Test expectations may be outdated
```

**Example issue**:
```go
// Test expects 80x24 terminal
list := NewList(80, 24)

// But rendering assumes 100x30
// Output doesn't match expectations
```

### 2. Styling Changes

```go
// Recent lipgloss style updates
// Color scheme changes
// Border style modifications
// Test expectations not updated
```

**Example**:
```go
// Test expects old border style
expected := "┌─────┐"

// New style uses different characters
actual := "╭─────╮"
```

### 3. Category Count Display

```go
// Category header format changes
// "(1)" vs "1" vs "[1]" formatting
// Test regex doesn't match new format
```

**Example**:
```go
// Old format: "All (1)"
// New format: "All [1]"
// Test regex: "All \\(\\d+\\)" ❌ Doesn't match
```

### 4. Snapshot Staleness

```go
// Tests use golden files/snapshots
// Snapshots not updated after UI changes
// Every test fails due to format mismatch
```

### 5. BubbleTea Rendering Differences

```go
// BubbleTea view rendering may have changed
// Whitespace handling differences
// Line ending inconsistencies (LF vs CRLF)
```

## Files Affected (Unknown)

Investigation needed to determine affected files:
- `ui/list_test.go` - Main UI tests
- `ui/list.go` - Rendering logic
- `app/app_test.go` - App-level rendering tests (possibly)
- Test fixtures/snapshots (possibly)

**Context boundary**: ⚠️ Unknown (requires investigation)

## Investigation Steps

### Phase 1: Identify Failing Tests (15 minutes)

```bash
# Run UI tests with verbose output
go test ./ui -v > ui_test_output.txt 2>&1

# Find all failures
grep "FAIL:" ui_test_output.txt

# Capture error details
grep -B 5 -A 10 "FAIL:" ui_test_output.txt > ui_failures_detail.txt
```

### Phase 2: Categorize Failures (30 minutes)

For each failing test:
1. Read test code to understand expectations
2. Read error message to find mismatch
3. Categorize: Outdated expectation vs real bug
4. Check if related to BUG-008 or distinct issue

### Phase 3: Update Test Expectations (1-2 hours)

**If outdated snapshots**:
```bash
# Regenerate golden files
go test ./ui -update-golden

# Or manually update expected strings in tests
```

**If styling changes**:
```go
// Update expected output to match new styles
expected := strings.TrimSpace(`
╭─────────────────────╮
│ Sessions            │
├─────────────────────┤
│ > ▼ All [1]         │
│   - Test Session    │
╰─────────────────────╯
`)
```

### Phase 4: Fix Real Bugs (1-3 hours)

**If production rendering bugs found**:
1. Fix rendering logic in `ui/list.go`
2. Verify fix in manual TUI testing
3. Update tests to match correct behavior
4. Add regression tests

### Phase 5: Test Stabilization (30 minutes)

```bash
# Run tests multiple times
for i in {1..10}; do
    go test ./ui -run TestCategoryRendering || echo "Run $i failed"
done

# Check for flakiness
# Fix any intermittent failures
```

## Expected Fix Outcomes

After investigation and fixes:
- All UI rendering tests pass ✅
- Test expectations match current UI ✅
- Real rendering bugs identified and fixed ✅
- Tests are deterministic (not flaky) ✅
- Manual TUI verification confirms correctness ✅

## Impact Assessment

**Severity**: **High**
- **User-Facing**: Unknown (depends on if bugs are in tests or production)
- **Data Loss**: No
- **Workaround**: Unknown
- **Frequency**: Every test run
- **Scope**: UI rendering, potentially user-visible

**Priority**: P2 - Important for test suite health and UI quality

**Timeline**:
- Phase 1 (Identify): 15 minutes
- Phase 2 (Categorize): 30 minutes
- Phase 3 (Update): 1-2 hours
- Phase 4 (Fix bugs): 1-3 hours (if needed)
- Phase 5 (Stabilize): 30 minutes
- **Total**: 3-6 hours

## Prevention Strategy

**Test maintenance**:
1. Update tests when UI changes
2. Use flexible assertions (not brittle snapshots)
3. Test behavior, not exact rendering
4. Document why tests expect specific output

**UI development**:
1. Run UI tests before committing changes
2. Update test expectations with UI changes
3. Add visual regression testing
4. Manual TUI verification for all UI changes

**CI/CD**:
1. Block merges on UI test failures
2. Generate visual diffs for failures
3. Require test updates in UI PRs
4. Track test flakiness

## Related Issues

- **BUG-008**: Category rendering in tests - sessions don't render (CRITICAL, open)
- **BUG-009**: Session package test failures (high, open)
- **Test Stabilization Epic**: See `docs/tasks/test-stabilization-and-teatest-integration.md`

## Additional Notes

**Relationship to BUG-008**:

BUG-008 is specifically about **sessions not rendering despite existing**. This bug (BUG-011) covers **other UI rendering test failures**:

- Category header format mismatches
- Styling inconsistencies
- Layout calculation errors
- Snapshot staleness
- etc.

**These may overlap** or be the same root cause, but investigation will clarify.

**Recommendation**:

1. **Investigate this bug first** (3-6 hours)
   - Faster to identify and categorize all UI test failures
   - May reveal that BUG-008 is part of a larger pattern
   - Updates to test expectations may fix multiple bugs at once

2. **Then focus on BUG-008** if still failing after updates
   - If all other UI tests pass after expectations updated
   - But session rendering still fails
   - Then BUG-008 is a distinct logic bug (not test staleness)

**Don't duplicate work** - Start with broad UI test investigation, then narrow to specific rendering logic bugs.

---

**Bug Tracking ID**: BUG-011
**Related Feature**: UI Rendering (ui/ package)
**Fix Complexity**: Unknown (investigation required)
**Fix Risk**: Low-Medium (mostly test updates expected)
**Blocked By**: Investigation needed (Phase 1-2)
**Blocks**: UI development, test suite reliability
**Related To**: BUG-008 (may overlap or share root cause)
