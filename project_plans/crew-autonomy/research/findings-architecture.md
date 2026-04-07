# Crew Autonomy — Architecture Research Findings

Accessed: 2026-04-02

---

## Executive Summary

Three architectural decisions dominate the design:

1. **State machine approach for Lookout**: A manual `switch/select` pattern inside a single goroutine's `run()` loop is the best fit for this codebase — not looplab/fsm and not goroutine-per-state. The manual pattern mirrors `ReviewQueueManager` exactly and allows synchronous state queries via `RWMutex`.

2. **Score proto extension**: Add an optional `Score` message field (field number 10+) to `ReviewItem`. Proto3 message-type fields are inherently optional (nil when absent), so old React clients silently ignore it. No breaking change. The `Score` message lives in `review_queue.proto`.

3. **Fixer placement**: The Fixer should be a **separate struct** that also subscribes to the EventBus — not crammed into `ReviewQueueManager`. It needs different concerns (lifecycle management of Lookout goroutines, capacity limits, escalation logic) that don't belong in the review queue router.

---

## Lookout State Machine

### Recommended States

```
Idle → Active → Sweeping → AwaitingRetry → Fallen → Stopped
                         ↘ Stopped (sweep passed)
```

| State | Meaning |
|---|---|
| `Idle` | Session exists but hasn't started working yet |
| `Active` | Session is actively doing work (first tool use seen) |
| `Sweeping` | `ReasonTaskComplete` fired; Sweep is running |
| `AwaitingRetry` | Sweep failed; Earpiece sent; waiting for session to resume |
| `Fallen` | maxRetries exhausted; escalated to Mastermind |
| `Stopped` | Session terminated or Lookout shut down |

Transitions driven by EventBus events and Sweep results:
- `Idle → Active`: `EventSessionNeedsAttention` (any non-complete reason)
- `Active → Sweeping`: `EventSessionNeedsAttention` with `ReasonTaskComplete`
- `Sweeping → AwaitingRetry`: Sweep failed + Going Dark mode
- `Sweeping → Stopped`: Sweep passed (Score assembled, dropped to review queue)
- `Sweeping → Fallen`: Sweep failed + Supervised mode (no Earpiece)
- `AwaitingRetry → Active`: `EventSessionResumed`
- `AwaitingRetry → Fallen`: retry count >= maxRetries

### Library Decision: Manual > looplab/fsm

**looplab/fsm** (2.4k stars, Apache 2.0) is well-regarded but adds friction:
- Callbacks are stringly-typed (`"enter_sweeping"`, `"after_task_complete"`)
- Thread safety requires wrapping in `ThreadSafeFSM` or a manual `sync.RWMutex`
- The codebase doesn't use it; adding a dependency for one 6-state machine is over-engineering

**Goroutine-per-state** is explicitly discouraged (see [embargoed.co](https://embargoed.co/posts/dont-implement-state-machines-with-goroutines/)): you can't synchronously query state, race conditions arise between state goroutines, and goroutine leaks are hard to avoid.

**Recommended: manual `switch/select` in a single goroutine**, matching the `ReviewQueueManager` pattern exactly:

```go
type LookoutState int

const (
    LookoutIdle         LookoutState = iota
    LookoutActive
    LookoutSweeping
    LookoutAwaitingRetry
    LookoutFallen
    LookoutStopped
)

type Lookout struct {
    sessionID    string
    mu           sync.RWMutex
    state        LookoutState
    retryCount   int
    maxRetries   int
    goingDark    bool

    eventCh      chan session.Event
    ctx          context.Context
    cancel       context.CancelFunc
    doneCh       chan<- LookoutResult  // reports back to Fixer
}

func (l *Lookout) State() LookoutState {
    l.mu.RLock()
    defer l.mu.RUnlock()
    return l.state
}

func (l *Lookout) run() {
    defer l.cancel()
    for {
        select {
        case event := <-l.eventCh:
            l.mu.Lock()
            l.handleEvent(event)
            l.mu.Unlock()
        case <-l.ctx.Done():
            return
        }
    }
}
```

### Locking Strategy: Hybrid (Channels + RWMutex)

- **Channels** for incoming events (async, non-blocking delivery from EventBus)
- **`sync.RWMutex`** for `state` field (allows concurrent reads, rare writes)
- State is written only inside `run()` goroutine after lock acquisition
- External callers (web UI, Fixer) query state via `State()` method — synchronous, no deadlock risk

**Critical**: Never call a method that acquires the mutex from within a callback that already holds the mutex.

---

## Score — Extending ReviewItem Proto

### Recommended Design

Add an optional `Score` message field to `ReviewItem` in `review_queue.proto`:

```protobuf
// In review_queue.proto — safe to add, old clients ignore unknown fields

message ReviewItem {
  string session_id = 1;
  string title = 2;
  AttentionReason reason = 3;
  string message = 4;
  AutoApprovalStatus auto_approval_status = 5;
  // Score is populated only when reason = TASK_COMPLETE and Sweep ran.
  // Old clients (pre-crew-autonomy) will see score == nil and render normally.
  Score score = 10;
}

message Score {
  TestResults test_results = 1;
  DiffSummary diff_summary = 2;
  RetryHistory retry_history = 3;
  repeated SweepCheck sweep_checks = 4;
}

message TestResults {
  bool passed = 1;
  string output_excerpt = 2;   // first 2000 chars of test output
  int64 duration_ms = 3;
  int32 tests_run = 4;
  int32 tests_failed = 5;
}

message DiffSummary {
  int32 files_changed = 1;
  int32 lines_added = 2;
  int32 lines_deleted = 3;
  repeated string changed_files = 4;
}

message RetryHistory {
  int32 attempt_count = 1;
  int32 max_retries = 2;
  repeated RetryAttempt attempts = 3;
}

message RetryAttempt {
  int32 number = 1;
  string failure_reason = 2;  // first 500 chars of test output
  int64 timestamp_ms = 3;
}

message SweepCheck {
  string name = 1;          // "go test", "npm test", "lint"
  bool passed = 2;
  string output_excerpt = 3;
}
```

### Why This Works

**Proto3 backward compatibility rules:**
- Adding new fields with higher field numbers: **always safe**
- Old clients reading a `ReviewItem` with `score` set: **silently ignore** (proto3 unknown fields are preserved but not decoded)
- New clients reading an old `ReviewItem`: see `score == nil`, render the same as today
- ConnectRPC with `protojson`: unknown JSON fields silently dropped by React (no error)

**Why not wrapping ReviewItem in a new ScoreItem message?**
- Would require a new RPC endpoint and streaming handler
- Existing `StreamReviewQueue` streaming would stop delivering Score-enriched items
- Optional field on existing message is cheaper and backward-compatible

**Field number 10**: Skips over the existing 1–5 range to leave room. High numbers have no wire cost.

---

## Fixer Architecture

### Placement Decision: Separate struct, same EventBus subscription

The Fixer should **not** live inside `ReviewQueueManager`. Reasons:
- `ReviewQueueManager` routes events to the review queue for human review — a well-defined single responsibility
- The Fixer manages Lookout goroutine lifecycle, enforces capacity limits, and escalates to the Mastermind — orthogonal concerns
- Mixing them would create a "god object" that handles both human review routing and autonomous correction supervision

**Recommended structure:**

```go
type Fixer struct {
    mu         sync.RWMutex
    lookouts   map[string]*Lookout  // keyed by session ID
    maxDark    int                  // max concurrent Going Dark sessions

    ctx        context.Context
    cancel     context.CancelFunc
    wg         sync.WaitGroup
    doneCh     chan LookoutResult   // Lookouts report completion here

    // Dependencies
    queueMgr   *ReviewQueueManager  // for escalating to Mastermind
    sessionSvc SessionService
    injector   TmuxInjector
}

func (f *Fixer) Start(ctx context.Context, bus *session.EventBus) {
    ctx, f.cancel = context.WithCancel(ctx)
    sub := bus.Subscribe()
    go f.run(ctx, bus, sub)
    go f.reapLookouts()  // processes doneCh
}

func (f *Fixer) SpawnLookout(sessionID string, goingDark bool) error {
    f.mu.Lock()
    defer f.mu.Unlock()

    if goingDark {
        darkCount := f.countDarkLocked()
        if darkCount >= f.maxDark {
            return fmt.Errorf("at capacity: %d/%d Going Dark sessions", darkCount, f.maxDark)
        }
    }

    ctx, cancel := context.WithCancel(f.ctx)
    lookout := NewLookout(sessionID, goingDark, f.doneCh, f.injector)
    lookout.ctx = ctx
    lookout.cancel = cancel
    f.lookouts[sessionID] = lookout
    f.wg.Add(1)
    go func() {
        defer f.wg.Done()
        lookout.run()
    }()
    return nil
}
```

### Supervisor Pattern (Erlang-inspired, Go-idiomatic)

Based on [Erlang Supervisors in Go](https://medium.com/@teivah/erlang-supervisors-in-go-b0c77b1b3291):

- **Strategy**: `one_for_one` — if a Lookout terminates (normal or error), only that Lookout is affected
- **Lifecycle**: Fixer spawns Lookouts via `SpawnLookout()`, each Lookout runs its own goroutine
- **Panic recovery**: Wrap each Lookout's goroutine body in a `recover()` defer, report via `doneCh`
- **Graceful shutdown**: `f.wg.Wait()` after `f.cancel()` in `Stop()`
- **Capacity limit**: `countDarkLocked()` checks `len(goingDarkLookouts)` before spawning

### Fixer ↔ ReviewQueueManager Integration

The Fixer receives a reference to `ReviewQueueManager` to:
1. Call `queueMgr.NotifyStreamers()` when a Score is ready (task complete + sweep passed)
2. Drop a `ReasonTestsFailing` item when `Fallen` (maxRetries exhausted)

The Fixer does NOT replace `ReviewQueueManager` — it uses it as a downstream sink.

---

## Earpiece Concurrency Safety

### Identified Risks

| Risk | Likelihood | Severity |
|---|---|---|
| Two Lookouts for same session | Very low | High |
| Earpiece fires mid-turn (Operative still outputting) | Medium | Medium |
| Earpiece and human input racing | Low | Low |
| Earpiece injection while session is at Y/n prompt | Medium | High |

### Guards and Invariants

**1. Single Lookout per session (Fixer enforces)**
```go
// In Fixer.SpawnLookout():
if _, exists := f.lookouts[sessionID]; exists {
    return fmt.Errorf("lookout already exists for session %s", sessionID)
}
```
The `lookouts` map is the source of truth. Checked under `f.mu.Lock()` before every spawn.

**2. Earpiece only fires from `AwaitingRetry` state**
The Lookout sends the Earpiece only when transitioning `Sweeping → AwaitingRetry`. This transition is protected by `l.mu.Lock()` and can only happen once per retry cycle. A second Earpiece cannot fire until `AwaitingRetry → Active → Sweeping` completes.

**3. Injection timing: wait for prompt before injecting**
The tmux `IsAtPrompt()` / `WaitForPrompt()` methods check for shell prompts — these WON'T work for Claude Code (which runs as a Node process, not bash/zsh). Instead:

- Wait for `EventSessionResumed` before sending the next Earpiece (the session is definitionally at a prompt after resuming)
- Use a `time.Sleep(500ms)` guard after `ReasonTaskComplete` before injecting — gives Claude Code's output flush time to complete
- Check `tmux pane-current-command` via `tmux display-message -p '#{pane_current_command}'` — if it returns `node` (Claude Code) rather than an interactive tool, the session is likely at a prompt

**4. One injection at a time per session**
The `LookoutState` machine ensures the Earpiece can only be sent in state `Sweeping → AwaitingRetry`. Once in `AwaitingRetry`, no further injections happen until the cycle completes. This is structural, not just convention.

**5. No Earpiece in Supervised mode**
`goingDark bool` is set at Lookout creation time and is immutable. The Earpiece code path is behind `if l.goingDark` and cannot fire for Supervised sessions.

---

## Recommendations

1. **Use manual switch/select state machine** — skip looplab/fsm. The `ReviewQueueManager` pattern is the right template; copy its `run()` + `handleEvent()` structure verbatim for `Lookout`.

2. **Add `Score score = 10` to `ReviewItem` proto** — optional message field, safe for all existing clients. Write it in `review_queue.proto` alongside the other `ReviewItem`-adjacent messages.

3. **Fixer as a separate struct** — create `server/crew/fixer.go` and `server/crew/lookout.go`. Don't extend `ReviewQueueManager`. Wire them both to the same EventBus in `server.go`.

4. **Hybrid concurrency: channels for events, RWMutex for state** — Lookout's `run()` loop reads from an `eventCh`, transitions state under `mu.Lock()`. External callers read `State()` via `mu.RLock()`.

5. **`doneCh` from Lookout to Fixer** — each Lookout holds a `chan<- LookoutResult` reference to report normal exit, sweep pass/fail, and Fall events back to the Fixer's event loop.

6. **Earpiece timing guard** — don't inject immediately on `ReasonTaskComplete`. Wait for `EventSessionResumed` (the session's own signal that it's ready) before sending the Earpiece.

7. **`countDarkLocked()` before every spawn** — enforce capacity limit atomically under `f.mu.Lock()` in `SpawnLookout()`.

---

## Sources

- [looplab/fsm — Finite State Machine for Go](https://github.com/looplab/fsm) — 2.4k stars, Apache 2.0
- [Don't implement state machines with goroutines](https://embargoed.co/posts/dont-implement-state-machines-with-goroutines/) — Andrei, Aug 2023
- [Backward Compatibility for Protocol Buffers](https://medium.com/@shlomi.noach/backward-compatibility-for-protocol-buffers-9504f5ea11d4) — Shlomi Noach, Oct 2021
- [Best practices for proto3 backward-compatible schema evolution](https://stackoverflow.com/questions/62748009/best-practices-for-proto3-backward-compatible-schema-evolution) — Stack Overflow
- [Erlang Supervisors in Go](https://medium.com/@teivah/erlang-supervisors-in-go-b0c77b1b3291) — Teiva Harsanyi, Jul 2020
- [Mutex vs Channels: Choosing the Right Concurrency Primitive in Go](https://matthewkorthauer.com/posts/golang-mutex-vs-channel/) — Jul 2024
- Existing codebase: `server/review_queue_manager.go`, `session/events.go`, `proto/session/v1/review_queue.proto`
