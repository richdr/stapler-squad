# Emergency Test Timeout Resolution

## Critical Issue Analysis

**ROOT CAUSE**: Tests are hanging in `config.GetClaudeCommand()` which calls external shell commands (`which claude`) during test setup.

**Impact**:
- Complete test suite failure (30s timeout)
- App package unusable for testing
- CI/CD pipeline blocked

**Evidence**:
- Stack trace shows hanging in `syscall.Wait4()`
- Process stuck in `config.GetClaudeCommand()` -> `globalCommandExecutor.Output(cmd)`
- Test setup calls `config.DefaultConfig()` which triggers command execution

---

## Emergency Atomic Tasks

### Task EMERGENCY-T1: Mock External Command Dependencies (45 minutes)

**Scope**: Replace real command execution with mocks in test environment
**Files**:
- `app/test_helpers.go:269` (test setup calling config.DefaultConfig)
- `config/config.go:162` (GetClaudeCommand implementation)
- `config/config_test.go` (add mocking infrastructure)

**Context**: Test isolation, external dependency mocking, command executor abstraction

**Root Cause**: `config.GetClaudeCommand()` runs real shell commands (`which claude`) during test setup, causing indefinite hangs when commands don't exist or hang.

**Success Criteria**:
- Tests complete without timeouts
- No external command execution during tests
- Mocked command executor provides predictable responses
- Test setup completes in <1 second

**Implementation**:
1. Create mock command executor for tests
2. Modify test helpers to use mocked config
3. Provide default mock responses for GetClaudeCommand
4. Ensure test isolation from system environment

**Testing**: `go test -v ./app -timeout=10s` passes without hangs

### Task EMERGENCY-T2: Fix UI Test Snapshot Mismatches (30 minutes)

**Scope**: Update or regenerate failing UI snapshot tests
**Files**:
- `test/ui/session_ui_test.go` (failing assertions)
- `ui/overlay/session_setup_overlay_test.go` (snapshot mismatches)
- Snapshot files in `test/ui/snapshots/` (if they exist)

**Context**: UI layout changes, test snapshot maintenance, visual regression testing

**Root Cause**: UI layout has changed but test snapshots haven't been updated to match current rendering.

**Success Criteria**:
- All UI tests pass with current implementation
- Snapshots match actual rendered output
- No false positive test failures

**Implementation**:
1. Run tests to see current vs expected output
2. Update snapshots or assertions to match current behavior
3. Verify changes are intentional improvements, not regressions
4. Document any significant UI changes

**Testing**: `go test -v ./test/ui ./ui/overlay` passes completely

---

## Context Preparation Guide

### For Task EMERGENCY-T1:
**Required Understanding**:
- How `config.GetClaudeCommand()` executes shell commands
- Test setup flow in `app/test_helpers.go`
- Command executor interface pattern for mocking

**Files to Review**:
- Stack trace showing hang location
- Current mocking patterns in existing tests
- Command execution implementation

### For Task EMERGENCY-T2:
**Required Understanding**:
- UI snapshot testing patterns
- Current vs expected output differences
- Whether changes represent improvements or regressions

**Diagnostic Commands**:
```bash
# See specific UI test failures
go test -v ./test/ui -run TestSessionInstanceListRendering
go test -v ./ui/overlay -run TestSessionSetupOverlay
```

---

## INVEST Validation

| Task | Independent | Negotiable | Valuable | Estimable | Small | Testable |
|------|-------------|------------|----------|-----------|-------|----------|
| T1   | ✅ Config pkg focused | ✅ Mock approach flexible | ✅ Unblocks testing | ✅ Command mocking bounded | ✅ 45min scope | ✅ Timeout elimination |
| T2   | ✅ UI tests only | ✅ Snapshot vs assertion | ✅ Clean test suite | ✅ Visual comparison bounded | ✅ 30min focused | ✅ Pass/fail clear |

---

## Dependency Visualization

```
Task EMERGENCY-T1 (Command Mocking)
    ↓
Task EMERGENCY-T2 (UI Snapshots)
    ↓
Full Test Suite Restoration
```

**Sequential Execution Required**: T1 must complete first to enable T2 execution without timeouts.

---

## Success Criteria Summary

**Technical Success**:
- ✅ No test timeouts (all tests complete in <30s total)
- ✅ No external command dependencies in tests
- ✅ All UI tests pass with current implementation
- ✅ Clean test execution: `go test ./... -timeout=30s`

**Quality Success**:
- ✅ Tests provide reliable feedback for development
- ✅ CI/CD pipeline unblocked
- ✅ Foundation established for teatest integration

**Business Success**:
- ✅ Development workflow restored
- ✅ Production deployment pipeline enabled
- ✅ Test-driven development possible

**Total Estimated Effort**: 75 minutes across 2 focused atomic tasks
**Risk**: LOW - Isolated mocking and snapshot updates only
**Impact**: HIGH - Restores entire testing infrastructure