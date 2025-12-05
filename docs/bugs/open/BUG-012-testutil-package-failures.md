# BUG-012: Testutil Package Failures [SEVERITY: Medium]

**Status**: 🔍 Investigating
**Discovered**: 2025-12-05 during test stabilization work
**Impact**: Test infrastructure broken, blocks test development

## Problem Description

The `testutil` package, which provides test utilities and helpers, has failing tests. This is **infrastructure failure** - the tools used to test other code are themselves broken.

**Implications**:
- Test helpers may not work correctly
- New tests may use broken utilities
- Test isolation may be compromised
- Mock/stub behavior may be incorrect

**Unknown details**:
- Specific failing tests not captured
- Root cause not investigated
- Impact on other test packages unknown

## Reproduction

```bash
# Run testutil package tests
go test ./testutil -v

# Expected: Infrastructure tests pass
# Actual: Multiple failures (exact errors TBD)
```

**Expected**: All testutil tests pass (meta-test stability)
**Actual**: Failures in test infrastructure

## Root Cause Analysis

**Investigation required** - Potential causes:

### 1. Outdated Test Mocks

```go
// Mocks may not match current interfaces
// Interface changes broke mock implementations
// Mock methods missing or have wrong signatures
```

**Example**:
```go
// Interface changed
type SessionManager interface {
    Create(opts CreateOptions) error  // Added opts parameter
}

// Mock not updated
type MockSessionManager struct{}
func (m *MockSessionManager) Create() error { // ❌ Wrong signature
    return nil
}
```

### 2. Fixture Data Staleness

```go
// Test fixtures (JSON, config files) outdated
// Schema changes not reflected in fixtures
// Expected field values no longer valid
```

**Example**:
```json
// Fixture: test_session.json
{
    "title": "Test",
    "category": "All"  // ❌ Missing new required fields
}
```

### 3. Helper Function Changes

```go
// Helper functions modified without updating tests
// Return types changed
// Error handling changed
// Side effects added
```

### 4. Dependency Injection Issues

```go
// Test utilities may use global state
// Singleton patterns broken
// Initialization order issues
```

### 5. Resource Cleanup Issues

```go
// Test utilities not cleaning up properly
// Temp files/directories left behind
// tmux sessions not killed
// State leaking between tests
```

## Files Affected (Unknown)

Investigation needed to determine affected files:
- `testutil/*.go` - Test utility implementations
- `testutil/*_test.go` - Failing tests
- Possibly affects all packages using testutil

**Context boundary**: ⚠️ Unknown (requires investigation)

## Investigation Steps

### Phase 1: Capture Test Output (15 minutes)

```bash
# Run testutil tests verbosely
go test ./testutil -v > testutil_output.txt 2>&1

# Identify failures
grep "FAIL:" testutil_output.txt

# Capture full error context
grep -B 10 -A 10 "FAIL:" testutil_output.txt > testutil_failures.txt
```

### Phase 2: Identify Broken Utilities (30 minutes)

```bash
# Find which utilities are tested
ls testutil/*_test.go

# Check which utilities are broken
for test in testutil/*_test.go; do
    echo "Testing $test"
    go test "$test" -v
done
```

### Phase 3: Check Usage Impact (30 minutes)

```bash
# Find which packages import testutil
grep -r "testutil" . --include="*_test.go"

# Check if other test failures are due to broken testutil
# Compare test failure patterns with testutil issues
```

### Phase 4: Fix Broken Utilities (2-4 hours)

Based on findings:

**Update mocks**:
```go
// Regenerate mocks if using mockgen
go generate ./testutil

// Or manually update mock implementations
```

**Update fixtures**:
```bash
# Regenerate fixture data with current schema
go run ./scripts/generate_fixtures.go

# Or manually update JSON/config files
```

**Fix helper functions**:
```go
// Update helper signatures
// Fix return types
// Update error handling
```

**Add cleanup logic**:
```go
// Add proper teardown
func (h *TestHelper) Cleanup() {
    os.RemoveAll(h.tempDir)
    h.killTmuxSessions()
    h.resetState()
}

// Use in tests
defer helper.Cleanup()
```

### Phase 5: Verify No Regressions (1 hour)

```bash
# Run all tests that use testutil
go test ./ui -v
go test ./session -v
go test ./app -v

# Verify no new failures introduced
# Check that fixes don't break other tests
```

## Expected Fix Outcomes

After investigation and fixes:
- All testutil tests pass ✅
- Test helpers work correctly ✅
- Mocks match current interfaces ✅
- Fixtures have valid data ✅
- No resource leaks in test utilities ✅
- Other test packages can use utilities safely ✅

## Impact Assessment

**Severity**: **Medium**
- **User-Facing**: No (test infrastructure only)
- **Data Loss**: No
- **Workaround**: Don't use broken utilities (hard to know which)
- **Frequency**: Every test run using testutil
- **Scope**: Test infrastructure, affects all test development

**Priority**: P2 - Important for test development, doesn't affect production

**Timeline**:
- Phase 1 (Capture output): 15 minutes
- Phase 2 (Identify broken): 30 minutes
- Phase 3 (Check impact): 30 minutes
- Phase 4 (Fix): 2-4 hours
- Phase 5 (Verify): 1 hour
- **Total**: 4-6 hours

## Prevention Strategy

**Test infrastructure maintenance**:
1. Test the test utilities (meta-testing)
2. Run testutil tests in CI
3. Update mocks when interfaces change
4. Regenerate fixtures when schemas change
5. Document utility usage and assumptions

**Code generation**:
```bash
# Use go generate for mocks
//go:generate mockgen -source=session.go -destination=testutil/mock_session.go

# Run before tests in CI
go generate ./...
go test ./...
```

**Test isolation**:
```go
// Use t.Cleanup() for proper teardown
func TestSomething(t *testing.T) {
    helper := testutil.NewHelper(t)
    t.Cleanup(func() {
        helper.Cleanup()
    })
    // Test code
}
```

**Fixture validation**:
```go
// Add schema validation for fixtures
func LoadFixture(path string) (*Fixture, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }

    var f Fixture
    if err := json.Unmarshal(data, &f); err != nil {
        return nil, err
    }

    // Validate against current schema
    if err := f.Validate(); err != nil {
        return nil, fmt.Errorf("fixture %s invalid: %w", path, err)
    }

    return &f, nil
}
```

## Related Issues

- **BUG-008**: Category rendering in tests (CRITICAL, open) - May be using broken testutil
- **BUG-009**: Session package test failures (high, open) - May be due to broken mocks
- **BUG-010**: tmux banner detection (high, open) - May be in testutil/tmux helpers
- **Test Stabilization Epic**: See `docs/tasks/test-stabilization-and-teatest-integration.md`

## Additional Notes

**Why this matters**:

Broken test infrastructure is **more dangerous than broken tests**:

1. **False confidence**: Tests pass but are using broken utilities
2. **Hidden bugs**: Utilities mask real issues in production code
3. **Cascading failures**: One broken utility breaks many tests
4. **Test debt accumulation**: Developers work around broken utilities

**Priority justification**:

This is **P2 (not P1)** because:
- Doesn't directly affect production code
- Other tests may not depend on broken utilities
- Can work around by not using testutil

But should be **fixed before adding new tests** to avoid building on broken foundation.

**Recommendation**:

1. **Investigate quickly** (1 hour) to assess impact
2. **Fix critical utilities first** (those blocking other tests)
3. **File separate bugs** for each broken utility found
4. **Update fixtures** as part of fix (don't defer)
5. **Add meta-tests** to catch future breakage

**Don't ignore test infrastructure problems** - They compound exponentially.

---

**Bug Tracking ID**: BUG-012
**Related Feature**: Test Infrastructure (testutil/ package)
**Fix Complexity**: Medium (multiple utilities, mocks, fixtures)
**Fix Risk**: Low-Medium (test code only, but affects all tests)
**Blocked By**: Investigation needed (Phase 1-3)
**Blocks**: Test development, may contribute to other test failures
**Related To**: All test-related bugs (BUG-008, BUG-009, BUG-010, BUG-011)
