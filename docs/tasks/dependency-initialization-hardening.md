# Harden Server Dependency Initialization

**Date**: 2026-03-18
**Status**: Draft
**Scope**: Replace the fragile sequential initialization in `server/dependencies.go` with compile-time-enforced dependency ordering using Go's type system
**Related**: [Architecture Refactor](architecture-refactor.md) (Story 1, Task 1.4 created the current `BuildDependencies` function)

---

## Table of Contents

- [Problem Statement](#problem-statement)
- [ADR-011: Named Constructor Chain vs Builder Pattern vs DI Framework](#adr-011-named-constructor-chain-vs-builder-pattern-vs-di-framework)
- [ADR-012: Phase Struct Granularity](#adr-012-phase-struct-granularity)
- [Dependency Visualization](#dependency-visualization)
- [Epic: Dependency Initialization Hardening](#epic-dependency-initialization-hardening)
  - [Story 1: Introduce Typed Intermediate Structs](#story-1-introduce-typed-intermediate-structs)
  - [Story 2: Refactor BuildDependencies to Use Typed Phase Structs](#story-2-refactor-builddependencies-to-use-typed-phase-structs)
  - [Story 3: Integration Test for Server Startup](#story-3-integration-test-for-server-startup)
- [Known Issues](#known-issues)
- [Verification Checklist](#verification-checklist)

---

## Problem Statement

`server/dependencies.go` contains `BuildDependencies()`, a 180-line function that constructs 11+ components in a strict sequential order. The current implementation is a procedural script where:

1. **Ordering is enforced only by comments and code position** -- reordering two lines can cause a nil pointer panic at runtime, not a compilation error.
2. **Late wiring via setter injection** -- `SessionService` is constructed first, then mutated by 5 different `Set*` calls (`SetStatusManager`, `SetReviewQueuePoller`, `SetReactiveQueueManager`, `SetExternalDiscovery`, `SetNotificationStore`). A developer who forgets one of these calls gets a nil dereference in production, not a build failure.
3. **Implicit dependency flow** -- Components like `ReviewQueuePoller` depend on `ReviewQueue`, `StatusManager`, and `Storage`, but this is only visible by reading the constructor signature. There is no structural guarantee that these were constructed before the poller.

The goal is to make the dependency ordering self-documenting and compiler-verified: if you construct dependencies in the wrong order, the code does not compile.

### Current Initialization Flow (12 Steps)

From `server/dependencies.go`:

```
Step 1:  SessionService (creates Storage, EventBus, ReviewQueue internally)
Step 2:  StatusManager (no dependencies)
Step 3:  ReviewQueuePoller (depends on ReviewQueue, StatusManager, Storage)
Step 4:  Wire StatusManager + ReviewQueuePoller into SessionService
Step 5:  LoadInstances from Storage
Step 6:  Wire ReviewQueue + StatusManager on each Instance; wire Instances into Poller
Step 7:  Start tmux sessions for loaded instances
Step 8:  Start controllers for running instances
Step 7.5: Startup scan + orphaned approval sync
Step 9:  ReactiveQueueManager (depends on ReviewQueue, Poller, EventBus, StatusManager, Storage)
Step 10: ScrollbackManager (independent)
Step 11: TmuxStreamerManager (independent)
Step 12: ExternalDiscovery + ExternalApprovalMonitor (depend on Storage, ReviewQueue, StatusManager, Poller)
```

### Files Affected

| File | Role |
|------|------|
| `server/dependencies.go` | Primary target -- contains `BuildDependencies()` and `ServerDependencies` struct |
| `server/server.go` | Calls `BuildDependencies()`, wires HTTP handlers using the result |
| `server/review_queue_manager.go` | `ReactiveQueueManager` constructor and lifecycle |
| `server/services/session_service.go` | `SessionService` with 5 `Set*` methods for late-wired dependencies |

---

## ADR-011: Named Constructor Chain vs Builder Pattern vs DI Framework

**Status**: Proposed

**Context**: Three approaches can enforce dependency ordering at compile time in Go.

### Option A: Named Constructor Chain (Recommended)

Each initialization phase produces a typed struct. The next phase's constructor requires the previous phase's struct as a parameter. Reordering causes a type error.

```go
// Phase 1 produces CoreDeps
type CoreDeps struct {
    SessionService *services.SessionService
    Storage        *session.Storage
    EventBus       *events.EventBus
    ReviewQueue    *session.ReviewQueue
}
func BuildCoreDeps() (*CoreDeps, error) { ... }

// Phase 2 requires CoreDeps, produces ServiceDeps
type ServiceDeps struct {
    CoreDeps
    StatusManager     *session.InstanceStatusManager
    ReviewQueuePoller *session.ReviewQueuePoller
}
func BuildServiceDeps(core *CoreDeps) (*ServiceDeps, error) { ... }

// Phase 3 requires ServiceDeps, produces RuntimeDeps
type RuntimeDeps struct {
    ServiceDeps
    Instances            []*session.Instance
    ReactiveQueueMgr     *ReactiveQueueManager
    ScrollbackManager    *scrollback.ScrollbackManager
    TmuxStreamerManager  *session.ExternalTmuxStreamerManager
    ExternalDiscovery    *session.ExternalSessionDiscovery
    ExternalApprovalMon  *session.ExternalApprovalMonitor
}
func BuildRuntimeDeps(svc *ServiceDeps) (*RuntimeDeps, error) { ... }
```

**Advantages**:
- Ordering enforced by function signatures: `BuildRuntimeDeps` cannot be called without a `*ServiceDeps`, which cannot exist without calling `BuildServiceDeps`.
- No framework dependency. Pure Go types and functions.
- Incremental: easy to add a new phase or split an existing one.
- Each phase function is testable in isolation with mock inputs.
- IDE support: "go to definition" on the parameter type shows exactly what was constructed.

**Disadvantages**:
- More types to maintain (3 phase structs + the final `ServerDependencies`).
- Embedding means the final struct carries all intermediate fields -- consumers can access "internal" phase outputs.

### Option B: Builder Pattern with Validation

A `DependencyBuilder` struct accumulates state. Each step sets fields and returns `*DependencyBuilder` for method chaining. A `Build()` method at the end validates all prerequisites.

```go
type DependencyBuilder struct {
    sessionService    *services.SessionService
    storage           *session.Storage
    statusManager     *session.InstanceStatusManager
    // ... all fields
    errors            []error
}

func NewDependencyBuilder() *DependencyBuilder { ... }
func (b *DependencyBuilder) WithCoreDeps() *DependencyBuilder { ... }
func (b *DependencyBuilder) WithServiceDeps() *DependencyBuilder { ... }
func (b *DependencyBuilder) WithRuntimeDeps() *DependencyBuilder { ... }
func (b *DependencyBuilder) Build() (*ServerDependencies, error) { ... }
```

**Advantages**:
- Familiar pattern. Fluent API reads well.
- Validation in `Build()` catches missing dependencies at startup.

**Disadvantages**:
- Ordering is still runtime-enforced, not compile-time. `Build()` returns an error, not a type error. A developer can call `WithRuntimeDeps()` before `WithCoreDeps()` and only discover the issue when the program runs.
- The builder accumulates mutable state, making it harder to reason about what has been initialized at any point.
- Each `With*` method must check prerequisites internally, duplicating the ordering knowledge.

### Option C: DI Framework (wire, fx, dig)

Use a compile-time DI generator like Google Wire or a runtime DI container like Uber fx/dig.

**Google Wire**:
- Generates initialization code from provider functions at compile time.
- Enforces that all dependencies are satisfied.
- Reordering providers has no effect (the generator sorts them).

**Uber fx**:
- Runtime DI container. Providers are registered and resolved automatically.
- Lifecycle hooks for start/stop.

**Advantages**:
- Automatic dependency resolution. No manual ordering at all.
- Wire catches missing providers at code generation time (compile-time equivalent).
- fx provides lifecycle management (start/stop hooks) which could replace manual `Start()` calls.

**Disadvantages**:
- **External dependency**: Adds `go.uber.org/fx` or `github.com/google/wire` to the module. The project currently has no DI framework and adding one is a significant architectural decision.
- **Go-specific constraints**: Unlike Java Spring, Go's DI frameworks lack generics-based type resolution (no constructor parameter injection by type matching). Wire uses code generation, which adds a build step. fx uses reflection, which loses type safety.
- **Debugging opacity**: When a dependency is missing, Wire gives a code generation error and fx gives a runtime error with a dependency graph dump. Neither is as clear as a compilation error from a missing function parameter.
- **Overkill for this scope**: The dependency graph has ~12 nodes and ~20 edges. A DI framework is designed for hundreds of components. The overhead (learning curve, toolchain integration, debugging complexity) exceeds the benefit.
- **Team unfamiliarity**: The existing codebase uses no frameworks beyond ConnectRPC and Ent. Introducing a DI framework requires team buy-in and documentation.

### Decision

**Option A: Named Constructor Chain**. The compile-time guarantees from Go's type system are stronger than runtime validation (Option B) and do not require external dependencies (Option C). The three phase structs align naturally with the three logical initialization stages already visible in `BuildDependencies()`:

1. **Core**: Create the foundational services (SessionService with its internal Storage/EventBus/ReviewQueue).
2. **Service**: Create management components that depend on core services (StatusManager, ReviewQueuePoller).
3. **Runtime**: Load instances, start processes, wire callbacks, create reactive/streaming infrastructure.

### Rationale (from literature)

- "Dependency Inversion Principle" (Robert C. Martin, "Clean Architecture") -- depend on abstractions. The phase structs are concrete types that serve as "milestones" guaranteeing a minimum set of initialized dependencies.
- "Make illegal states unrepresentable" (Yaron Minsky) -- if you cannot construct a `ServiceDeps` without a `CoreDeps`, then a `ServiceDeps` with a nil `Storage` is unrepresentable.
- "A Philosophy of Software Design" (John Ousterhout) -- deep modules with narrow interfaces. Each phase constructor has a narrow interface (one input struct, one output struct) that hides the complexity of wiring.

---

## ADR-012: Phase Struct Granularity

**Status**: Proposed

**Context**: The initialization has 12 logical steps. How many typed phase structs should we introduce?

### Options

| Granularity | Phase Structs | Trade-off |
|------------|---------------|-----------|
| Fine (1 per step) | 12 structs | Maximum compile-time safety but excessive type proliferation |
| Medium (1 per logical phase) | 3 structs | Good safety-to-complexity ratio |
| Coarse (single struct) | 1 struct (status quo) | No compile-time ordering; current problem |

### Decision

**3 phase structs** aligned with natural dependency boundaries:

1. **`CoreDeps`** -- Step 1: SessionService and its internally created dependencies (Storage, EventBus, ReviewQueue). These have no external dependencies and are the foundation for everything else.

2. **`ServiceDeps`** -- Steps 2-4: StatusManager, ReviewQueuePoller, and wiring them into SessionService. These depend on CoreDeps outputs and must be established before instances are loaded.

3. **`RuntimeDeps`** -- Steps 5-12: Instance loading, tmux startup, controller startup, startup scan, ReactiveQueueManager, ScrollbackManager, TmuxStreamerManager, ExternalDiscovery, ExternalApprovalMonitor. These depend on both CoreDeps and ServiceDeps and involve side effects (process creation, filesystem I/O).

### Rationale

The three phases correspond to distinct failure modes:
- **CoreDeps failure**: Unrecoverable. Cannot start the server at all (database open failure, config error).
- **ServiceDeps failure**: Unrecoverable. Management infrastructure required for any session operations.
- **RuntimeDeps failure**: Partially recoverable. Individual instance failures are logged and skipped. The server can start with zero instances and still serve the web UI.

This distinction also maps cleanly to testability:
- **CoreDeps**: Requires database setup (integration test).
- **ServiceDeps**: Can be tested with mock CoreDeps.
- **RuntimeDeps**: Requires tmux/filesystem (integration test) or can be tested with mock ServiceDeps.

---

## Dependency Visualization

```
                    BuildCoreDeps()
                         |
                         v
    +--------------------------------------------+
    |              CoreDeps                       |
    |                                            |
    |  SessionService ----+                      |
    |       |             |                      |
    |       +-> Storage   +-> EventBus           |
    |       +-> ReviewQueue                      |
    |       +-> ApprovalStore                    |
    +--------------------------------------------+
                         |
                         v
                  BuildServiceDeps(core)
                         |
                         v
    +--------------------------------------------+
    |              ServiceDeps                    |
    |                                            |
    |  CoreDeps (embedded)                       |
    |  StatusManager                             |
    |  ReviewQueuePoller ----+                   |
    |       |                |                   |
    |       +-> ReviewQueue  +-> StatusManager   |
    |       +-> Storage      +-> ApprovalStore   |
    |                                            |
    |  [SessionService wired with StatusManager  |
    |   and ReviewQueuePoller]                   |
    +--------------------------------------------+
                         |
                         v
                  BuildRuntimeDeps(svc)
                         |
                         v
    +--------------------------------------------+
    |              RuntimeDeps                    |
    |                                            |
    |  ServiceDeps (embedded)                    |
    |                                            |
    |  Instances []*Instance ----+               |
    |       |                    |               |
    |       +-> ReviewQueue      +-> StatusMgr   |
    |       (wired per-instance)                 |
    |                                            |
    |  ReactiveQueueMgr ----+                    |
    |       |               |                    |
    |       +-> Poller      +-> EventBus         |
    |       +-> StatusMgr   +-> Storage          |
    |                                            |
    |  ScrollbackManager (independent)           |
    |  TmuxStreamerManager (independent)          |
    |  ExternalDiscovery ----+                   |
    |       |                |                   |
    |       +-> Storage      +-> ReviewQueue     |
    |       +-> StatusMgr    +-> Poller          |
    |  ExternalApprovalMonitor (independent)      |
    +--------------------------------------------+
                         |
                         v
               ServerDependencies
              (flattened view for
               server.go consumers)
```

### Setter Injection Elimination Map

Current `SessionService` has 5 `Set*` methods called post-construction. The hardened design eliminates each:

| Current Setter | Called In | Phase Where Wired | Elimination Strategy |
|---------------|----------|-------------------|---------------------|
| `SetStatusManager` | `BuildDependencies` step 4 | ServiceDeps | Pass to `BuildServiceDeps`; wire inside that function |
| `SetReviewQueuePoller` | `BuildDependencies` step 4 | ServiceDeps | Pass to `BuildServiceDeps`; wire inside that function |
| `SetReactiveQueueManager` | `BuildDependencies` step 9 | RuntimeDeps | Pass to `BuildRuntimeDeps`; wire inside that function |
| `SetExternalDiscovery` | `server.go` line 97 | RuntimeDeps (or server.go) | Wire in `BuildRuntimeDeps` or `NewServer` |
| `SetNotificationStore` | `server.go` line 92 | server.go (post-deps) | Wire in `NewServer` after `BuildDependencies` returns |

The `Set*` methods are retained on `SessionService` for backward compatibility but are no longer required for correct initialization -- the phase constructors handle the wiring.

---

## Epic: Dependency Initialization Hardening

### Story 1: Introduce Typed Intermediate Structs

**Goal**: Define the three phase structs (`CoreDeps`, `ServiceDeps`, `RuntimeDeps`) and their constructor functions, without yet changing `BuildDependencies`.

**Acceptance Criteria**:
- Three new exported types exist in `server/dependencies.go`
- Each type has a constructor function with a typed input parameter
- `go build ./...` passes
- Existing `BuildDependencies` is unchanged (new code is additive)

#### Task 1.1: Define `CoreDeps` struct and `BuildCoreDeps()` constructor

**File**: `server/dependencies.go`

**Change**: Add the `CoreDeps` struct and its constructor. The constructor encapsulates Step 1 from the current `BuildDependencies`: calling `services.NewSessionServiceFromConfig()` and extracting Storage, EventBus, ReviewQueue.

```go
// CoreDeps holds the foundational dependencies created during Phase 1.
// These have no external prerequisites and form the base for all other components.
type CoreDeps struct {
    SessionService *services.SessionService
    Storage        *session.Storage
    EventBus       *events.EventBus
    ReviewQueue    *session.ReviewQueue
    ApprovalStore  *services.ApprovalStore
}

// BuildCoreDeps constructs Phase 1 dependencies: SessionService and its internal
// components (Storage, EventBus, ReviewQueue, ApprovalStore).
// Returns an error only for unrecoverable failures (database open, config parse).
func BuildCoreDeps() (*CoreDeps, error) {
    sessionService, err := services.NewSessionServiceFromConfig()
    if err != nil {
        return nil, fmt.Errorf("initialize SessionService: %w", err)
    }
    return &CoreDeps{
        SessionService: sessionService,
        Storage:        sessionService.GetStorage(),
        EventBus:       sessionService.GetEventBus(),
        ReviewQueue:    sessionService.GetReviewQueueInstance(),
        ApprovalStore:  sessionService.GetApprovalStore(),
    }, nil
}
```

**Tests**: Unit test that `BuildCoreDeps()` returns non-nil fields (requires database setup, so this is an integration test -- see Story 3).

**Risk**: None. Additive change. Existing code is untouched.

#### Task 1.2: Define `ServiceDeps` struct and `BuildServiceDeps()` constructor

**File**: `server/dependencies.go`

**Change**: Add `ServiceDeps` embedding `CoreDeps`, with `StatusManager` and `ReviewQueuePoller`. The constructor takes `*CoreDeps` as input -- this is where compile-time ordering enforcement begins.

```go
// ServiceDeps holds Phase 2 dependencies: management components that depend on CoreDeps.
// Embedding CoreDeps ensures Phase 1 was completed before Phase 2 can begin.
type ServiceDeps struct {
    *CoreDeps
    StatusManager     *session.InstanceStatusManager
    ReviewQueuePoller *session.ReviewQueuePoller
}

// BuildServiceDeps constructs Phase 2 dependencies using Phase 1 outputs.
// Creates StatusManager and ReviewQueuePoller, then wires them into SessionService.
// Compile-time guarantee: cannot be called without a *CoreDeps.
func BuildServiceDeps(core *CoreDeps) (*ServiceDeps, error) {
    if core == nil {
        return nil, fmt.Errorf("BuildServiceDeps: CoreDeps is nil (Phase 1 not completed)")
    }
    if core.Storage == nil || core.EventBus == nil || core.ReviewQueue == nil {
        return nil, fmt.Errorf("BuildServiceDeps: CoreDeps has nil fields")
    }

    statusManager := session.NewInstanceStatusManager()
    reviewQueuePoller := session.NewReviewQueuePoller(
        core.ReviewQueue, statusManager, core.Storage,
    )
    reviewQueuePoller.SetApprovalProvider(core.ApprovalStore)

    // Wire into SessionService (eliminates two Set* calls from server.go)
    core.SessionService.SetStatusManager(statusManager)
    core.SessionService.SetReviewQueuePoller(reviewQueuePoller)

    return &ServiceDeps{
        CoreDeps:          core,
        StatusManager:     statusManager,
        ReviewQueuePoller: reviewQueuePoller,
    }, nil
}
```

**Tests**: Verify `BuildServiceDeps(nil)` returns error. Verify with valid `CoreDeps` that all fields are non-nil.

**Risk**: Low. The wiring logic (`SetStatusManager`, `SetReviewQueuePoller`) moves from `BuildDependencies` into `BuildServiceDeps` but the operations are identical.

#### Task 1.3: Define `RuntimeDeps` struct and `BuildRuntimeDeps()` constructor

**File**: `server/dependencies.go`

**Change**: Add `RuntimeDeps` embedding `ServiceDeps`. This is the largest constructor, covering Steps 5-12: instance loading, tmux startup, controller startup, startup scan, ReactiveQueueManager, independent managers, and external discovery.

```go
// RuntimeDeps holds Phase 3 dependencies: runtime components that involve
// process creation, filesystem I/O, and callback wiring.
// Embedding ServiceDeps ensures Phases 1 and 2 were completed.
type RuntimeDeps struct {
    *ServiceDeps
    Instances               []*session.Instance
    ReactiveQueueMgr        *ReactiveQueueManager
    ScrollbackManager       *scrollback.ScrollbackManager
    TmuxStreamerManager     *session.ExternalTmuxStreamerManager
    ExternalDiscovery       *session.ExternalSessionDiscovery
    ExternalApprovalMonitor *session.ExternalApprovalMonitor
}

// BuildRuntimeDeps constructs Phase 3 dependencies using Phase 2 outputs.
// Loads instances, starts tmux sessions, wires callbacks, creates streaming infrastructure.
// Individual instance failures are logged and skipped (non-fatal).
// Compile-time guarantee: cannot be called without a *ServiceDeps.
func BuildRuntimeDeps(svc *ServiceDeps) (*RuntimeDeps, error) {
    if svc == nil {
        return nil, fmt.Errorf("BuildRuntimeDeps: ServiceDeps is nil (Phase 2 not completed)")
    }

    // Steps 5-7: Load instances, wire dependencies, start tmux sessions
    instances, err := svc.Storage.LoadInstances()
    if err != nil {
        return nil, fmt.Errorf("load instances: %w", err)
    }
    for _, inst := range instances {
        inst.SetReviewQueue(svc.ReviewQueue)
        inst.SetStatusManager(svc.StatusManager)
    }
    svc.ReviewQueuePoller.SetInstances(instances)

    // Start tmux sessions (non-fatal failures logged)
    for _, inst := range instances {
        if !inst.Started() {
            if err := inst.Start(false); err != nil {
                log.ErrorLog.Printf("Failed to start instance '%s': %v", inst.Title, err)
            }
        }
    }

    // Persist migrated data, start controllers, startup scan
    // ... (full implementation extracted from current BuildDependencies)

    // ReactiveQueueManager
    reactiveQueueMgr := NewReactiveQueueManager(
        svc.ReviewQueue, svc.ReviewQueuePoller,
        svc.EventBus, svc.StatusManager, svc.Storage,
    )
    svc.SessionService.SetReactiveQueueManager(reactiveQueueMgr)

    // Independent managers + external discovery
    // ... (full implementation extracted from current BuildDependencies)

    return &RuntimeDeps{
        ServiceDeps:             svc,
        Instances:               instances,
        ReactiveQueueMgr:        reactiveQueueMgr,
        ScrollbackManager:       scrollbackManager,
        TmuxStreamerManager:     tmuxStreamerManager,
        ExternalDiscovery:       externalDiscovery,
        ExternalApprovalMonitor: externalApprovalMonitor,
    }, nil
}
```

**Tests**: See Story 3 for integration test.

**Risk**: Medium. This is the largest function and contains the most side effects (tmux process creation, filesystem I/O). Must be implemented as an exact behavioral copy of the current Steps 5-12 in `BuildDependencies`.

#### Task 1.4: Add nil-guard validation to each phase constructor

**File**: `server/dependencies.go`

**Change**: Each `Build*Deps` function validates its input is non-nil and that critical fields are populated. This provides a defense-in-depth layer on top of the compile-time ordering guarantee.

```go
func BuildServiceDeps(core *CoreDeps) (*ServiceDeps, error) {
    if core == nil {
        return nil, fmt.Errorf("BuildServiceDeps: CoreDeps is nil (Phase 1 not completed)")
    }
    if core.Storage == nil || core.EventBus == nil || core.ReviewQueue == nil {
        return nil, fmt.Errorf("BuildServiceDeps: CoreDeps has nil fields: "+
            "Storage=%v EventBus=%v ReviewQueue=%v",
            core.Storage != nil, core.EventBus != nil, core.ReviewQueue != nil)
    }
    // ... rest of constructor
}
```

**Tests**: Table-driven tests with nil CoreDeps, CoreDeps with nil Storage, etc.

**Risk**: None. Additive validation.

---

### Story 2: Refactor BuildDependencies to Use Typed Phase Structs

**Goal**: Replace the body of `BuildDependencies()` with calls to `BuildCoreDeps`, `BuildServiceDeps`, `BuildRuntimeDeps`. The existing `ServerDependencies` struct remains as the public return type but is now populated from the phase structs.

**Acceptance Criteria**:
- `BuildDependencies()` is reduced to ~20 lines: three phase calls and a mapping to `ServerDependencies`
- All `Set*` calls on `SessionService` happen inside phase constructors, not in `server.go`
- `go build ./...` and `go test ./...` pass
- `make restart-web` starts correctly and all session operations work

#### Task 2.1: Rewrite `BuildDependencies()` to delegate to phase constructors

**File**: `server/dependencies.go`

**Change**: Replace the 180-line function body with phase delegation.

```go
func BuildDependencies() (*ServerDependencies, error) {
    core, err := BuildCoreDeps()
    if err != nil {
        return nil, fmt.Errorf("phase 1 (core): %w", err)
    }

    svc, err := BuildServiceDeps(core)
    if err != nil {
        return nil, fmt.Errorf("phase 2 (services): %w", err)
    }

    rt, err := BuildRuntimeDeps(svc)
    if err != nil {
        return nil, fmt.Errorf("phase 3 (runtime): %w", err)
    }

    return &ServerDependencies{
        SessionService:          rt.SessionService,
        Storage:                 rt.Storage,
        EventBus:                rt.EventBus,
        StatusManager:           rt.StatusManager,
        ReviewQueue:             rt.ReviewQueue,
        ReviewQueuePoller:       rt.ReviewQueuePoller,
        ReactiveQueueMgr:        rt.ReactiveQueueMgr,
        ScrollbackManager:       rt.ScrollbackManager,
        TmuxStreamerManager:     rt.TmuxStreamerManager,
        ExternalDiscovery:       rt.ExternalDiscovery,
        ExternalApprovalMonitor: rt.ExternalApprovalMonitor,
    }, nil
}
```

**Tests**: Existing tests must continue to pass. Manual verification with `make restart-web`.

**Risk**: High. This is the critical refactoring step. Behavioral equivalence must be verified line-by-line against the current implementation. The most likely failure mode is a subtle ordering change where a side effect (like `reviewQueuePoller.SetApprovalProvider`) is moved to a different position relative to other wiring steps.

**Mitigation**: Before refactoring, copy the current `BuildDependencies` body into comments inside each `Build*Deps` function to serve as a reference. Remove comments only after tests pass.

#### Task 2.2: Move `SetExternalDiscovery` and `SetNotificationStore` wiring from `server.go` into phase constructors or `BuildDependencies`

**File**: `server/server.go`, `server/dependencies.go`

**Change**: Currently, `server.go` calls `deps.SessionService.SetExternalDiscovery(deps.ExternalDiscovery)` on line 97 and `deps.SessionService.SetNotificationStore(notifStore)` on line 92. These should move:

- `SetExternalDiscovery` moves into `BuildRuntimeDeps` (ExternalDiscovery is created there).
- `SetNotificationStore` stays in `server.go` because the notification store depends on config directory resolution that is a server-level concern. However, it should be wrapped in a helper or explicitly documented.

**Tests**: Verify that `ListSessions` includes external sessions (proves SetExternalDiscovery was called).

**Risk**: Low. Moving the call site does not change behavior.

#### Task 2.3: Update `ServerDependencies` comments to reference phase structs

**File**: `server/dependencies.go`

**Change**: Update the doc comments on `ServerDependencies` and each `Build*` function to form a self-documenting chain:

```go
// ServerDependencies holds all wired service components for the HTTP server.
// Constructed via BuildDependencies(), which executes three initialization phases:
//
//   Phase 1 (BuildCoreDeps):    SessionService, Storage, EventBus, ReviewQueue
//   Phase 2 (BuildServiceDeps): StatusManager, ReviewQueuePoller (wired into SessionService)
//   Phase 3 (BuildRuntimeDeps): Instance loading, process startup, reactive infrastructure
//
// Phase ordering is enforced at compile time: each Build*Deps function requires
// the previous phase's output struct as a parameter. Reordering causes a type error.
```

Also update the large comment block in `server.go` `NewServer` to reference the new phase structure instead of listing 12 individual steps.

**Risk**: None. Documentation only.

#### Task 2.4: Remove stale helper functions if any become unreachable

**File**: `server/dependencies.go`

**Change**: After the refactor, scan for helper functions (`scanSessionsOnStartup`, `syncOrphanedApprovalsToQueue`, `mapDetectedStatus`, `addStartupItem`) and verify they are still reachable from the new phase constructors. These should remain in `dependencies.go` but may need to be called from `BuildRuntimeDeps` instead of the old `BuildDependencies`.

**Risk**: Low. These are internal functions with no external callers.

---

### Story 3: Integration Test for Server Startup

**Goal**: Add an integration test that verifies the server starts correctly with all dependencies wired, proving the phase chain produces a functional `ServerDependencies`.

**Acceptance Criteria**:
- Test creates a temporary database, builds dependencies, and verifies all `ServerDependencies` fields are non-nil
- Test verifies that an instance can be created, paused, and resumed through the `SessionService`
- Test verifies the `ReviewQueuePoller` has the correct number of instances
- Test runs in CI (`go test ./server/...`)

#### Task 3.1: Create `server/dependencies_test.go` with phase constructor unit tests

**File**: `server/dependencies_test.go` (new)

**Change**: Add tests for each phase constructor in isolation.

```go
func TestBuildCoreDeps_ReturnsNonNilFields(t *testing.T) {
    // Requires temp database -- skip in short mode
    if testing.Short() {
        t.Skip("requires database setup")
    }
    core, err := BuildCoreDeps()
    require.NoError(t, err)
    assert.NotNil(t, core.SessionService)
    assert.NotNil(t, core.Storage)
    assert.NotNil(t, core.EventBus)
    assert.NotNil(t, core.ReviewQueue)
    assert.NotNil(t, core.ApprovalStore)
}

func TestBuildServiceDeps_RejectsNilCore(t *testing.T) {
    _, err := BuildServiceDeps(nil)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "CoreDeps is nil")
}

func TestBuildServiceDeps_RejectsIncompleteCore(t *testing.T) {
    _, err := BuildServiceDeps(&CoreDeps{}) // All nil fields
    assert.Error(t, err)
}
```

**Risk**: Test isolation. `BuildCoreDeps` creates a real database. Use `t.TempDir()` for the database path and set a test-specific config directory via environment variable or constructor parameter.

#### Task 3.2: Create integration test for full `BuildDependencies()` chain

**File**: `server/dependencies_test.go`

**Change**: Add an integration test that runs the full chain and verifies the returned `ServerDependencies` is functional.

```go
func TestBuildDependencies_FullChain(t *testing.T) {
    if testing.Short() {
        t.Skip("integration test -- requires tmux and filesystem")
    }

    deps, err := BuildDependencies()
    require.NoError(t, err)

    // Verify all fields are non-nil
    assert.NotNil(t, deps.SessionService, "SessionService")
    assert.NotNil(t, deps.Storage, "Storage")
    assert.NotNil(t, deps.EventBus, "EventBus")
    assert.NotNil(t, deps.StatusManager, "StatusManager")
    assert.NotNil(t, deps.ReviewQueue, "ReviewQueue")
    assert.NotNil(t, deps.ReviewQueuePoller, "ReviewQueuePoller")
    assert.NotNil(t, deps.ReactiveQueueMgr, "ReactiveQueueMgr")
    assert.NotNil(t, deps.ScrollbackManager, "ScrollbackManager")
    assert.NotNil(t, deps.TmuxStreamerManager, "TmuxStreamerManager")
    assert.NotNil(t, deps.ExternalDiscovery, "ExternalDiscovery")
    assert.NotNil(t, deps.ExternalApprovalMonitor, "ExternalApprovalMonitor")
}
```

**Risk**: This test requires tmux to be installed on the CI runner. Gate with a build tag or environment variable check.

#### Task 3.3: Add regression test for dependency ordering

**File**: `server/dependencies_test.go`

**Change**: Add a test that verifies the compile-time ordering guarantee works as expected. This is a "documentation test" that demonstrates the API to future developers.

```go
func TestPhaseOrdering_CompileTimeGuarantee(t *testing.T) {
    // This test documents the compile-time ordering guarantee.
    // The following would NOT compile if uncommented:
    //
    //   svc, _ := BuildServiceDeps(nil)  // nil is *CoreDeps but will return error
    //   rt, _ := BuildRuntimeDeps(nil)   // nil is *ServiceDeps but will return error
    //
    // The following is a type error at compile time:
    //   core, _ := BuildCoreDeps()
    //   rt, _ := BuildRuntimeDeps(core)  // compile error: *CoreDeps != *ServiceDeps
    //
    // The correct usage is:
    //   core, _ := BuildCoreDeps()
    //   svc, _ := BuildServiceDeps(core)
    //   rt, _ := BuildRuntimeDeps(svc)

    t.Log("Phase ordering is enforced by Go's type system. See comments for examples.")
}
```

**Risk**: None. Documentation-only test.

---

## Known Issues

### Phase struct embedding may create "god object" access patterns [SEVERITY: Medium]

**Description**: `RuntimeDeps` embeds `ServiceDeps` which embeds `CoreDeps`. This means any code with a `*RuntimeDeps` can access `rt.SessionService`, `rt.StatusManager`, `rt.Storage`, etc. directly. While convenient, this erodes the boundary between phases -- a consumer might access `rt.Storage` when it should only know about `rt.ScrollbackManager`.

**Mitigation**:
- Document that phase structs are construction-time artifacts, not runtime dependencies. Once `BuildDependencies` returns a `ServerDependencies`, the phase structs are discarded.
- The `ServerDependencies` struct (the public API) contains only the fields that `server.go` and handler registrations need. It does not embed phase structs.
- If phase struct leakage becomes a problem in practice, replace embedding with explicit field copying in each `Build*Deps` function.

**Files Likely Affected**:
- `server/dependencies.go` (struct definitions)

**Prevention Strategy**:
- Keep `CoreDeps`, `ServiceDeps`, `RuntimeDeps` unexported if possible (but this limits testability from `server_test.go` which is in the same package -- since both are `package server`, unexported types are accessible in tests).
- Add a code review checklist item: "Do not pass phase structs beyond `BuildDependencies`."

### Circular dependency risk if two services need each other [SEVERITY: Medium]

**Description**: Currently, `SessionService` is created in Phase 1, then mutated in Phase 2 (`SetStatusManager`, `SetReviewQueuePoller`) and Phase 3 (`SetReactiveQueueManager`). This is a form of circular dependency: `SessionService` needs `StatusManager`, and `StatusManager` conceptually operates on sessions managed by `SessionService`. The phase struct approach surfaces this circularity more explicitly.

If a future change requires Phase 2 components to call back into Phase 1 components during construction (e.g., `ReviewQueuePoller` needs to call `SessionService.CreateSession` during initialization), the phase chain breaks because Phase 1 is not fully wired at Phase 2 construction time.

**Mitigation**:
- The phase constructors only perform construction and wiring, not runtime operations. No component should call business logic methods on another component during initialization.
- If a circular dependency arises, introduce an interface at the boundary. For example, `ReviewQueuePoller` depends on a `SessionLister` interface (with a `ListInstances` method), not on `*SessionService` directly.
- Document in each phase constructor: "This function must not trigger runtime operations on the components it wires."

**Files Likely Affected**:
- `server/services/session_service.go` (setter methods)
- `server/dependencies.go` (phase constructors)
- `session/review_queue_poller.go` (if future changes add SessionService dependency)

**Prevention Strategy**:
- Enforce a rule: phase constructors call only `New*` constructors and `Set*` wiring methods. They never call business logic methods like `ListSessions`, `CreateSession`, `Start`, or `Poll`.
- The sole exception is `BuildRuntimeDeps`, which calls `LoadInstances`, `Start`, and `StartController` -- these are explicitly documented as side-effectful and are the last phase.

### `SessionService` still requires post-construction setter injection [SEVERITY: Low]

**Description**: Even after the refactor, `SessionService` is created in Phase 1 with an incomplete dependency set, then wired in Phases 2 and 3. The `Set*` methods remain on `SessionService` and could still be called in the wrong order or not at all.

This is a deliberate design trade-off: `SessionService` creates Storage and EventBus internally (via `NewSessionServiceFromConfig`), so it must exist before the components that depend on those outputs can be created. The alternative -- passing Storage and EventBus as constructor parameters to `SessionService` -- would require refactoring `NewSessionServiceFromConfig`, which is out of scope for this feature.

**Mitigation**:
- The `Set*` calls are encapsulated inside `BuildServiceDeps` and `BuildRuntimeDeps`. As long as callers use `BuildDependencies()` (or the phase chain), the setters are always called in the correct order.
- Add a `Validate()` method to `SessionService` that checks all required dependencies are non-nil. Call it at the end of `BuildRuntimeDeps` as a defense-in-depth check.

**Files Likely Affected**:
- `server/services/session_service.go`
- `server/dependencies.go`

**Prevention Strategy**:
- In a future refactor (tracked in architecture-refactor.md), extract Storage and EventBus creation from `SessionService` so they can be passed as constructor parameters, eliminating the need for setter injection entirely.

### `time.Sleep(500ms)` in startup scan creates timing sensitivity [SEVERITY: Low]

**Description**: `BuildDependencies` (and by extension `BuildRuntimeDeps`) contains a `time.Sleep(500 * time.Millisecond)` between controller startup and the startup scan. This delay exists to let controllers initialize their terminal readers. Moving this sleep into `BuildRuntimeDeps` preserves the behavior but the timing sensitivity remains -- if controllers take longer than 500ms to initialize, the startup scan misses their state.

**Mitigation**:
- This is a pre-existing issue, not introduced by the refactor.
- A proper fix would replace the sleep with a readiness signal from controllers (e.g., a channel or sync.WaitGroup). This is out of scope for this feature but noted for future improvement.

**Files Likely Affected**:
- `server/dependencies.go` (startup scan section of `BuildRuntimeDeps`)

### Potential for stale embedded `CoreDeps` reference after mutation [SEVERITY: Low]

**Description**: `BuildServiceDeps` receives `*CoreDeps` and calls `core.SessionService.SetStatusManager(statusManager)`. This mutates the `SessionService` that is also referenced from `CoreDeps.SessionService`. Since `ServiceDeps` embeds `*CoreDeps` (pointer), the mutation is visible through both `svc.SessionService` and `svc.CoreDeps.SessionService`. However, if someone mistakenly copies `CoreDeps` by value (embedding `CoreDeps` instead of `*CoreDeps`), the mutation would be lost.

**Mitigation**:
- Use pointer embedding (`*CoreDeps`, `*ServiceDeps`) consistently. The plan already specifies this.
- Add a comment on the struct definitions: "Always embed as pointer to ensure mutations in phase constructors are visible to subsequent phases."

**Files Likely Affected**:
- `server/dependencies.go` (struct definitions)

---

## Verification Checklist

After completing each story, verify:

- [ ] `go build ./...` succeeds with no errors
- [ ] `go test ./...` passes
- [ ] `go vet ./...` reports no new issues
- [ ] `make restart-web` starts the server and web UI loads at localhost:8543
- [ ] Session create, pause, resume, delete work via web UI
- [ ] Terminal streaming works for at least one session
- [ ] Review queue populates when a session reaches approval state
- [ ] External session discovery works (if mux sessions are available)
- [ ] Verify no nil pointer panics in logs during startup with existing sessions
- [ ] Verify no nil pointer panics in logs during startup with zero sessions (fresh database)

### Story-Specific Verification

**Story 1**: New types compile. `BuildCoreDeps`, `BuildServiceDeps`, `BuildRuntimeDeps` exist but are not yet called by `BuildDependencies`. No behavioral change.

**Story 2**: `BuildDependencies` delegates to phase constructors. Behavioral equivalence confirmed by full manual test of session lifecycle (create -> attach terminal -> view diff -> pause -> resume -> delete).

**Story 3**: `go test ./server/... -run TestBuildDependencies` passes in CI.

---

## Task Summary

| Story | Task | Description | Risk | Dependencies |
|-------|------|-------------|------|--------------|
| 1 | 1.1 | Define `CoreDeps` struct and `BuildCoreDeps()` | Low | None |
| 1 | 1.2 | Define `ServiceDeps` struct and `BuildServiceDeps()` | Low | 1.1 |
| 1 | 1.3 | Define `RuntimeDeps` struct and `BuildRuntimeDeps()` | Medium | 1.2 |
| 1 | 1.4 | Add nil-guard validation to phase constructors | Low | 1.1, 1.2, 1.3 |
| 2 | 2.1 | Rewrite `BuildDependencies()` to delegate to phases | High | Story 1 |
| 2 | 2.2 | Move `Set*` calls from `server.go` into phases | Low | 2.1 |
| 2 | 2.3 | Update documentation comments | Low | 2.1 |
| 2 | 2.4 | Remove stale helper functions if unreachable | Low | 2.1 |
| 3 | 3.1 | Phase constructor unit tests | Low | Story 1 |
| 3 | 3.2 | Full chain integration test | Medium | Story 2 |
| 3 | 3.3 | Ordering guarantee documentation test | Low | Story 1 |
