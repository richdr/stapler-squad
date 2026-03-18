# Split SessionService into Domain-Focused Services

## Epic Overview

**User Value**: `server/services/session_service.go` is a 2,054-line God Object implementing 47 RPC methods plus inline business logic. Developers modifying one area (e.g., notifications) must load the entire file's context. Splitting into domain-focused services reduces cognitive load, enables independent testing, and makes the boundary between concerns explicit.

**Success Metrics**:
- `session_service.go` reduced to < 500 lines (pure session CRUD + delegation)
- Each domain service has its own file with ≤ 300 lines
- Zero regression in existing API behavior (all 47 RPCs still functional)
- Each domain service independently unit-testable

**Scope**: Extract 5 domain areas from `SessionService` while keeping the ConnectRPC-generated interface satisfied via a Facade pattern. The proto service definition remains a single `service SessionService` — no breaking changes to the wire protocol.

**Constraints**:
- The generated `SessionServiceHandler` interface (47 methods) must still be implemented by one struct
- No changes to proto definitions or generated TypeScript client
- `BuildDependencies()` in `server/dependencies.go` must wire all new services
- All existing Go tests must pass after each story

---

## Current State Analysis

### Already-Extracted Services (patterns to follow)

| File | Struct | RPCs Handled |
|------|--------|-------------|
| `server/services/review_queue_service.go` | `ReviewQueueService` | GetReviewQueue, AcknowledgeSession, WatchReviewQueue, LogUserInteraction |
| `server/services/search_service.go` | `SearchService` | ListClaudeHistory, GetClaudeHistoryDetail, GetClaudeHistoryMessages, SearchClaudeHistory |
| `server/services/github_service.go` | `GitHubService` | GetPRInfo, GetPRComments, PostPRComment, MergePR, ClosePR |
| `server/services/workspace_service.go` | `WorkspaceService` | GetVCSStatus, GetWorkspaceInfo, ListWorkspaceTargets, SwitchWorkspace |

### Still Implemented Inline in `session_service.go`

- Lines 1409-1505: `GetClaudeConfig`, `ListClaudeConfigs`, `UpdateClaudeConfig` (3 methods)
- Lines 1122-1298: `GetLogs` (1 method)
- Lines 1582-1689: `SendNotification` (1 method)
- Lines 1692-1816: `FocusWindow` (1 method)
- Lines 1989-2054: `CreateDebugSnapshot` (1 method)

### In separate files but still on `*SessionService`

- `notification_history_service.go`: `GetNotificationHistory`, `MarkNotificationRead`, `ClearNotificationHistory`
- `approval_handler.go`: `ResolveApproval`, `ListPendingApprovals`

### ReviewQueue helpers misplaced in `session_service.go`

- Lines 1300-1399: `WatchReviewQueueFilters` struct, `convertProtoPriorities`, `convertProtoReasons`, `formatDuration`

---

## Architecture Decision Records

### ADR-P2-2-01: Single Proto Service with Go-Side Facade

**Context**: The generated `SessionServiceHandler` interface has 47 methods. All must be on one struct.

**Decision**: Keep one proto `service SessionService`. On the Go side, `SessionService` becomes a thin Facade delegating each RPC to the appropriate domain service.

**Rationale**: Proto splitting would require frontend client changes. The Facade pattern achieves logical separation with zero wire-protocol changes — the pattern already established for review queue, search, GitHub, and workspace RPCs.

**Consequences**: `SessionService` keeps a receiver method for every RPC. Non-CRUD RPCs become 1-line delegations.

---

### ADR-P2-2-02: Dependency Injection via Constructor Parameters

**Decision**: Each domain service receives required dependencies through its constructor. `BuildDependencies()` constructs services in the correct order. Late-wiring `Set*` methods preserved only where startup ordering demands it.

---

### ADR-P2-2-03: Delete `notification_history_service.go` After Extraction

**Decision**: Move methods from `notification_history_service.go` to `NotificationService` during Story 2. Delete the original file.

**Consequences**: Clean file structure — each service has exactly one file.

---

## Story Breakdown

### Story 1: Extract ConfigService [1 week]

**User Value**: Config RPCs are isolated from session management. A developer working on config validation no longer loads 2,000 lines of context.

**Acceptance Criteria**:
- `server/services/config_service.go` contains 3 config RPCs
- `ConfigService` has zero dependencies (creates `ClaudeConfigManager` per call)
- `session_service.go` config methods are 1-line delegations
- Unit tests for all 3 config RPCs pass independently

#### Task 1.1: Create ConfigService and wire into SessionService [2h]

**Context Boundary**:
- Primary: `server/services/config_service.go` (new, ~100 lines)
- Supporting: `server/services/session_service.go` (lines 1409-1505)
- Supporting: `server/dependencies.go`
- ~300 lines total

**Implementation**:
1. Create `config_service.go` with `ConfigService` struct and `NewConfigService() *ConfigService`
2. Move `GetClaudeConfig`, `ListClaudeConfigs`, `UpdateClaudeConfig` from `SessionService`
3. Add `configSvc *ConfigService` field to `SessionService`
4. Replace 3 method bodies with `return s.configSvc.MethodName(ctx, req)`
5. Wire in `BuildDependencies()`

**Validation**:
- `go build ./...` passes
- `go test ./server/...` passes
- GET `/config` page in web UI shows current config

---

### Story 2: Extract NotificationService [1 week]

**User Value**: Notification sending, history, and rate limiting are in one service. Developers can reason about notification behavior without loading session CRUD code.

**Acceptance Criteria**:
- `server/services/notification_service.go` contains all 4 notification RPCs + `SendNotification`
- `notification_history_service.go` is deleted
- `validateLocalhostOrigin` moved to shared `helpers.go`

#### Task 2.1: Create NotificationService [2h]

**Context Boundary**:
- Primary: `server/services/notification_service.go` (new, ~200 lines)
- Supporting: `notification_history_service.go` (delete after, 157 lines)
- Supporting: `session_service.go` (lines 1582-1689)
- Supporting: `server/dependencies.go`

**Implementation**:
1. Create `notification_service.go` with `NotificationService` struct
2. Dependencies: `notificationStore`, `notificationRateLimiter`, `eventBus`, `reviewQueuePoller` (late-wired)
3. Move `SendNotification` from `session_service.go:1582-1689`
4. Move `GetNotificationHistory`, `MarkNotificationRead`, `ClearNotificationHistory` from `notification_history_service.go`
5. Extract `validateLocalhostOrigin`/`isLocalhostIP` to `server/services/helpers.go`
6. Add `notificationSvc *NotificationService` to `SessionService`, wire 4 delegations
7. Delete `notification_history_service.go`
8. Wire in `BuildDependencies()`

**Validation**:
- `go build ./...` passes
- Notification panel in web UI still receives and displays notifications
- Localhost origin check still rejects remote requests

---

### Story 3: Extract ApprovalService [1 week]

**User Value**: Hook approval RPCs (`ResolveApproval`, `ListPendingApprovals`) have their own service with no dependency on session management.

**Acceptance Criteria**:
- `server/services/approval_service.go` contains 2 approval RPCs
- `approval_handler.go` (the HTTP handler) unchanged
- `session_service.go` approval methods are 1-line delegations

#### Task 3.1: Create ApprovalService [2h]

**Context Boundary**:
- Primary: `server/services/approval_service.go` (new, ~80 lines)
- Supporting: `server/services/approval_handler.go` (lines 266-345)
- Supporting: `server/services/session_service.go`
- Supporting: `server/dependencies.go`

**Implementation**:
1. Create `approval_service.go` with `ApprovalService` struct
2. Single dependency: `approvalStore`
3. Move `ResolveApproval` and `ListPendingApprovals` from `approval_handler.go:266-345`
4. Add `approvalSvc *ApprovalService` to `SessionService`, wire 2 delegations
5. Wire in `BuildDependencies()`

**Validation**:
- Hook approval flow still works end-to-end
- `go test ./server/...` passes

---

### Story 4: Extract UtilityService [1 week]

**User Value**: Diagnostic utilities (`GetLogs`, `FocusWindow`, `CreateDebugSnapshot`) are isolated. The boundary between session management and debugging tools is explicit.

**Acceptance Criteria**:
- `server/services/utility_service.go` contains 3 utility RPCs
- `session_service.go` utility methods are 1-line delegations

#### Task 4.1: Create UtilityService [2h]

**Context Boundary**:
- Primary: `server/services/utility_service.go` (new, ~300 lines)
- Supporting: `server/services/session_service.go` (lines 1122-1298, 1692-1816, 1989-2054)
- Supporting: `server/dependencies.go`

**Implementation**:
1. Create `utility_service.go` with `UtilityService` struct
2. Dependencies: `reviewQueuePoller`, `approvalStore`
3. Move `GetLogs` (lines 1122-1298), `FocusWindow` (lines 1692-1816), `CreateDebugSnapshot` (lines 1989-2054)
4. Add `utilitySvc *UtilityService` to `SessionService`, wire 3 delegations
5. Wire in `BuildDependencies()`

**Validation**:
- Logs page still displays log entries
- Debug snapshot endpoint returns JSON
- `go test ./server/...` passes

---

### Story 5: Complete ReviewQueueService Delegation and Final Cleanup [1 week]

**User Value**: No dead code or misplaced helpers remain in `session_service.go`. The file size is verifiably < 500 lines.

**Acceptance Criteria**:
- `WatchReviewQueueFilters`, `convertProtoPriorities`, `convertProtoReasons`, `formatDuration` moved to `review_queue_service.go`
- `session_service.go` < 500 lines
- Compile-time interface check: `var _ sessionv1connect.SessionServiceHandler = (*SessionService)(nil)`

#### Task 5.1: Move misplaced helpers and add compile-time check [2h]

**Context Boundary**:
- Primary: `server/services/session_service.go` (lines 1300-1399)
- Primary: `server/services/review_queue_service.go` (append ~100 lines)
- ~500 lines total

**Implementation**:
1. Move `WatchReviewQueueFilters` struct and its methods (lines 1300-1328) to `review_queue_service.go`
2. Move `convertProtoPriorities` (lines 1340-1355)
3. Move `convertProtoReasons` (lines 1357-1375)
4. Move `formatDuration` (lines 1377-1399)
5. Add `var _ sessionv1connect.SessionServiceHandler = (*SessionService)(nil)` to `session_service.go`
6. Verify `wc -l session_service.go` < 500

**Validation**:
- `go build ./...` passes (compile-time check catches interface gaps)
- `wc -l server/services/session_service.go` outputs < 500
- Review queue UI still functional

---

## Known Issues

### Shared `validateLocalhostOrigin` Duplication [SEVERITY: Low]

Both `NotificationService` and `UtilityService` need localhost validation. **Mitigation**: Extract to `server/services/helpers.go` in Story 2.

---

### Late Wiring of ReviewQueuePoller [SEVERITY: Medium]

`NotificationService` needs `reviewQueuePoller` but the poller is created after `SessionService`. **Mitigation**: `NotificationService.SetReviewQueuePoller()` called in `BuildDependencies()` after poller creation, with nil guard in `SendNotification`.

---

### ApprovalStore Shared Across 4 Consumers [SEVERITY: Medium]

`ApprovalService`, `ApprovalHandler` (HTTP), `ReviewQueueService`, and `UtilityService` all share one `ApprovalStore`. **Mitigation**: Create once in `BuildDependencies()`, pass pointer to all consumers. `ApprovalStore` uses `sync.RWMutex` internally.

---

## Dependency Visualization

```
Story 1 (ConfigService)       --\
Story 2 (NotificationService)  --+---> Story 5 (Cleanup)
Story 3 (ApprovalService)     --+
Story 4 (UtilityService)      --/
```

Stories 1-4 are fully independent and can proceed in parallel. Story 5 is cleanup after all others.

---

## Integration Checkpoints

**After Story 1**: Config RPCs isolated. `session_service.go` reduced by ~100 lines.

**After Story 2**: Notification domain fully isolated. `notification_history_service.go` deleted.

**After Story 3**: Approval domain isolated. `approval_handler.go` cleaned up.

**After Story 4**: Utility RPCs isolated. `session_service.go` now only contains session CRUD.

**After Story 5**: `session_service.go` < 500 lines. Compile-time interface check prevents future regressions.

---

## Context Preparation Guide

**Story 1**: Read `session_service.go:1409-1505`, `dependencies.go`, and `search_service.go` (reference pattern).

**Story 2**: Read `notification_history_service.go` (157 lines), `session_service.go:1582-1689`, and `review_queue_service.go` (late-wiring pattern).

**Story 3**: Read `approval_handler.go:266-345`, `approval_store.go`.

**Story 4**: Read `session_service.go:1122-1298, 1692-1816, 1989-2054`.

**Story 5**: Read `session_service.go:1300-1399` and `review_queue_service.go`.

---

## Success Criteria

- [ ] `session_service.go` < 500 lines
- [ ] 5 new domain service files in `server/services/`
- [ ] `notification_history_service.go` deleted
- [ ] Compile-time interface check present
- [ ] `go build ./... && go test ./...` passes
- [ ] All web UI features functional after each story
