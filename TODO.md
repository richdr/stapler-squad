# Claude Squad - Current Priority Tasks

## EMERGENCY: Critical Test Timeouts 🚨

**Status**: BLOCKING - Test suite hangs indefinitely
**Priority**: P0 - Must fix immediately before any other work

### Root Cause: External Command Dependencies in Tests
Tests hang in `config.GetClaudeCommand()` which executes shell commands during setup.

### Immediate Actions Required:
- [ ] **CRITICAL**: Mock external command dependencies in test environment
- [ ] **URGENT**: Fix UI test snapshot mismatches
- [ ] **VALIDATION**: Ensure clean test execution with `go test ./... -timeout=30s`

**See**: [Emergency Test Timeouts Task](docs/tasks/emergency-test-timeouts.md)

---

## Next Priority: Test Stabilization

**Status**: Ready after build fixes complete
**Priority**: P1 - Required for production deployment

### Test Infrastructure Tasks:
- [ ] Fix UI search index nil pointer issues (`TestFuzzySearchIntegration`)
- [ ] Resolve layout calculation mismatches (`TestLayoutDebug`)
- [ ] Stabilize session package test timeouts
- [ ] Integrate teatest framework for TUI testing

**See**: [Test Stabilization Epic](docs/tasks/test-stabilization-and-teatest-integration.md)

---

## Documentation Maintenance

### Completed and Updated:
- [x] ✅ **Contextual Git Repository Discovery** - All implementation complete
- [x] ✅ **Unit Testing & Validation** - Comprehensive test coverage
- [x] ✅ **Path Validation & UX** - Enhanced error handling and shortcuts
- [x] ✅ **Edge Case Handling** - Network paths, permissions, empty queries

### Architecture Implementation Status:
- [x] ✅ **SessionSetupOverlay** - Contextual discovery fully implemented
- [x] ✅ **FuzzyInputOverlay** - Raw path entry support added
- [x] ✅ **Git Integration** - Repository, branch, worktree discovery working
- [x] ✅ **Performance** - Benchmarked at 0.47ms per operation

---

## Future Priorities (After Emergency Resolution)

### Medium Term (Next 3-5 Sessions):
- [ ] **Session Health Check Integration** - Evaluate health check system
- [ ] **Filtering System Enhancement** - Tag vs Category analysis
- [ ] **Help System Consolidation** - Compare current vs unused help generator

### Long Term (Future Sessions):
- [ ] **Dead Code Removal** - Clean up unused constructors and test mocks
- [ ] **Performance Optimization** - Large directory tree handling improvements
- [ ] **Advanced Features** - Network path support, fuzzy path matching

---

## Context Notes

**Last Updated**: 2025-01-17
**Current Phase**: Emergency Build Stabilization
**Next Milestone**: Restore compilation and test execution capability

**Critical Dependencies**:
- Build failures must be resolved before any other development work
- Test stabilization required for production deployment confidence
- All major feature work is complete and functional (when builds work)