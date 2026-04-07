# Crew Autonomy — Stack Research Findings

**Research date**: 2026-04-02
**Researcher**: Claude (Sonnet 4.6)
**Scope**: Tmux input injection, Go supervisor goroutine patterns, ConnectRPC streaming design

---

## Executive Summary

- **Tmux input injection is already implemented in the codebase.** `session/tmux/tmux.go` has `SendLiteralKeys`, `SendLiteralKeysWithEnter`, and `SendKeys` via `exec.Command(\"tmux\", \"send-keys\", ...)`. The `-l` (literal) flag is critical and already present — without it, strings like `Enter` or `C-c` are interpreted as key names rather than text.
- **The Lookout goroutine fits the existing `ReviewQueueManager` pattern exactly.** `server/review_queue_manager.go` demonstrates the canonical codebase pattern: `context.WithCancel`, a `run()` goroutine with a `select` on `ctx.Done()` and event channels, `Start()`/`Stop()` methods. The Lookout should follow this pattern identically.
- **The notification `Store` (pub/sub bus) is already the right event backbone.** `EventTypeTaskComplete` is already defined but unused. The Lookout subscribes to this event to trigger its sweep pipeline.
- **For the Lookout state machine, Rob Pike's "state as function" pattern is preferred** over an external FSM library — 5 states is below the complexity threshold where libraries add value over a simple enum + `select` loop.
- **ConnectRPC server streaming for sweep results fits the existing `StreamHookRequests` pattern** verbatim. The existing proto and streaming plumbing is mature; a new `StreamSweepResults` RPC is a minor addition.

---

## Tmux Input Injection

### What Already Exists in the Codebase

`session/tmux/tmux.go` already implements all required injection primitives:

```go
// Send literal text (no key-name interpretation) — correct for multi-word prompts
func (s *Session) SendLiteralKeys(text string) error {
    cmd := exec.Command("tmux", "send-keys", "-t", s.Name, "-l", text)
    ...
}

// Send literal text + Enter in two separate send-keys calls
func (s *Session) SendLiteralKeysWithEnter(text string) error {
    if err := s.SendLiteralKeys(text); err != nil { return err }
    cmd := exec.Command("tmux", "send-keys", "-t", s.Name, "Enter")
    ...
}
```

The `-l` flag is **essential**. Without it, a string like `"Tests are failing\
Please fix"` would have `\
` interpreted as a key name. The two-call pattern (literal text, then separate `Enter`) correctly handles this.

### The Right Method to Call

For injecting a corrective prompt into Claude Code, the Lookout should use:

```go
// session is *session.Instance, which wraps tmux.Session
err := sess.TmuxSession.SendLiteralKeysWithEnter(
    fmt.Sprintf("Tests are failing, please fix:\
%s", testOutput),
)
```

`session.Instance` exposes a `SendKeys(msg string) error` method (called in `approval_handler.go:injectRejectionMessage`). However, that method calls `send-keys` **without** the `-l` flag, which is a bug for multi-line corrective prompts. The Lookout should call `SendLiteralKeysWithEnter` directly.

### Timing and Sequencing Concerns

**Claude Code is not a standard shell.** The `IsAtPrompt()` / `WaitForPrompt()` helpers in `tmux.go` check for `zsh`/`bash` as `pane_current_command` — this will never be true when Claude Code is running (the current command will be `node` or `claude`).

Key findings from research:
- `send-keys` writes directly to the PTY input buffer. Claude Code reads this buffer when it is waiting for input (i.e., at its own interactive prompt, not mid-generation).
- If Claude Code is currently generating a response (typing), injecting input races with its internal state. The injected text lands in the buffer and will be consumed when generation finishes and the UI re-enters its read loop.
- **Practical recommendation**: The Lookout should only inject after confirming a `TaskComplete` event (which implies Claude Code has returned to its input-waiting state) and after a short stabilization delay (100–250ms). There is no reliable pane-level API to confirm Claude Code is at its prompt — the event-driven approach is more robust than polling.

### Control Mode vs. send-keys for Injection

The codebase already uses `tmux -C` (control mode) for **reading output** (streaming). Control mode can also send commands including `send-keys`:

```
# In control mode stdin, you can write:
send-keys -t mysession -l "your text here"
```

**Recommendation**: For input injection, continue using the existing `exec.Command("tmux", "send-keys", ...)` approach. It is simpler, stateless, and already works. Reserve the control mode pipe for output streaming (its current use). Mixing injection into the control mode pipe adds sequencing complexity without benefit.

### Edge Cases and Gotchas

1. **Session name targeting**: Always use the exact tmux session name. The codebase uses `s.Name` which maps to the session-specific name — correct.
2. **Long prompts**: tmux has a default input buffer limit. For large test output, truncate to the most relevant portion (last N lines of failures) before injection. Recommendation: cap at 4000 characters.
3. **Escape sequences**: The `-l` flag prevents key-name interpretation but does NOT escape terminal control characters in the text. Strip ANSI escape codes from `go test` output before injecting.
4. **Race with user input**: If a human is also typing in the session concurrently, injected text interleaves. This is expected behavior; document it rather than trying to prevent it.

---

## Go Supervisor Goroutine Pattern (The Lookout)

### Canonical Pattern in This Codebase

`server/review_queue_manager.go` is the reference implementation to follow:

```go
type LookoutSupervisor struct {
    mu             sync.RWMutex
    state          LookoutState
    retryCount     int
    maxRetries     int
    sessionID      string
    sessionManager *session.Manager
    notifStore     *notifications.Store
    ctx            context.Context
    cancel         context.CancelFunc
}

func NewLookoutSupervisor(sessionID string, mgr *session.Manager, ns *notifications.Store) *LookoutSupervisor {
    ctx, cancel := context.WithCancel(context.Background())
    return &LookoutSupervisor{
        state:          LookoutStateIdle,
        maxRetries:     3,
        sessionID:      sessionID,
        sessionManager: mgr,
        notifStore:     ns,
        ctx:            ctx,
        cancel:         cancel,
    }
}

func (l *LookoutSupervisor) Start() { go l.run() }
func (l *LookoutSupervisor) Stop()  { l.cancel() }
```

### State Machine Design

**Recommended approach: enum + `select` loop** (not an external library).

5 states, ~8 transitions is well within the "hand-roll it" threshold. Rob Pike's "state as function" pattern works for complex grammars; for this supervisor a simple state variable is clearer to read and debug.

```go
type LookoutState int

const (
    LookoutStateIdle     LookoutState = iota // Waiting for TaskComplete
    LookoutStateRunning                       // Sweep in progress
    LookoutStateSweeping                      // Quality gate executing
    LookoutStateWaiting                       // Waiting before retry (backoff)
    LookoutStateFallen                        // Max retries exceeded
)
```

State transition table:

| From         | Event                   | To           | Action                          |
|--------------|-------------------------|--------------|---------------------------------|
| Idle         | TaskComplete received   | Sweeping     | Launch quality gate goroutine   |
| Sweeping     | All tests pass          | Idle         | Clear retry count               |
| Sweeping     | Tests fail, retry < max | Waiting      | Inject corrective prompt        |
| Waiting      | Backoff elapsed         | Idle         | Increment retry count           |
| Sweeping     | Tests fail, retry >= max| Fallen       | Publish LookoutFallen event     |
| Any          | ctx.Done()              | (exit)       | Clean up resources              |

### The run() Loop

```go
func (l *LookoutSupervisor) run() {
    taskCh := l.notifStore.Subscribe(notifications.EventTypeTaskComplete, l.sessionID)
    defer l.notifStore.Unsubscribe(taskCh)

    for {
        switch l.getState() {
        case LookoutStateIdle:
            select {
            case <-l.ctx.Done():
                return
            case event := <-taskCh:
                l.handleTaskComplete(event)
            }
        case LookoutStateWaiting:
            select {
            case <-l.ctx.Done():
                return
            case <-time.After(backoffDuration(l.retryCount)):
                l.setState(LookoutStateIdle)
            }
        case LookoutStateFallen:
            // Stay fallen until explicitly reset or session ends
            select {
            case <-l.ctx.Done():
                return
            }
        }
    }
}
```

The `Sweeping` state runs as a **sub-goroutine** (the Sweep itself), with results communicated back to the Lookout via a result channel. This keeps the Lookout's main loop unblocked and context-cancelable during long-running test suites.

### Per-Session Lifecycle Management

The `session.Manager` should own a `map[string]*LookoutSupervisor` and call `Start()`/`Stop()` in the session create/delete paths. This mirrors how `ReviewQueueManager` is owned by the server.

```go
// In session.Manager or a new CrewManager:
func (m *Manager) OnSessionCreated(sess *Instance) {
    lookout := NewLookoutSupervisor(sess.ID, m, m.notifStore)
    m.lookouts[sess.ID] = lookout
    lookout.Start()
}

func (m *Manager) OnSessionDeleted(sessionID string) {
    if lookout, ok := m.lookouts[sessionID]; ok {
        lookout.Stop()
        delete(m.lookouts, sessionID)
    }
}
```

### Libraries Considered

| Library | Stars | Verdict |
|---|---|---|
| `looplab/fsm` | ~3k | Overkill for 5 states. Adds indirection with event/callback string names. |
| `qmuntal/stateless` | ~1k | Rich (hierarchical, guards, async) but over-engineered here. |
| Hand-rolled enum + select | — | **Recommended.** Idiomatic for this complexity level. Easy to test, read, and debug. |

**Decision**: No external FSM library. The `select` loop pattern is idiomatic Go and already demonstrated in `review_queue_manager.go`.

### errgroup for Sweep Coordination

The Sweep (quality gate) should use `golang.org/x/sync/errgroup` to run multiple test commands concurrently and collect the first failure:

```go
import "golang.org/x/sync/errgroup"

func (l *LookoutSupervisor) runSweep(ctx context.Context) (SweepResult, error) {
    g, gctx := errgroup.WithContext(ctx)
    results := make(chan CommandResult, len(l.commands))

    for _, cmd := range l.commands {
        cmd := cmd
        g.Go(func() error {
            out, err := exec.CommandContext(gctx, "sh", "-c", cmd).CombinedOutput()
            results <- CommandResult{Command: cmd, Output: string(out), Err: err}
            return err
        })
    }

    err := g.Wait()
    close(results)
    // Collect and return results...
}
```

---

## ConnectRPC Streaming for Sweep Results

### Existing Pattern (Reference Implementation)

`StreamHookRequests` in `approval_handler.go` is the reference:

```go
func (h *ApprovalHandler) StreamHookRequests(
    ctx context.Context,
    req *connect.Request[sessionv1.StreamHookRequestsRequest],
    stream *connect.ServerStream[sessionv1.HookRequest],
) error {
    eventCh := h.notificationBus.Subscribe(notifications.EventTypeHookRequest, sessionID)
    defer h.notificationBus.Unsubscribe(eventCh)

    for {
        select {
        case <-ctx.Done():
            return nil          // Client disconnected or server shutdown
        case event := <-eventCh:
            if err := stream.Send(hookReq); err != nil {
                return err      // Client disconnected mid-stream
            }
        }
    }
}
```

The `ctx.Done()` case handles **both** client disconnect and server shutdown. `stream.Send()` returning an error is the secondary disconnect signal. This pattern is correct and should be reused verbatim.

### Protobuf Design for Sweep Results

Add to `proto/session/v1/session.proto`:

```protobuf
// Incremental output from a quality gate sweep
message SweepEvent {
  string session_id = 1;
  string sweep_id   = 2;          // UUID for this sweep run

  oneof payload {
    SweepStarted   started   = 3;
    SweepLogLine   log_line  = 4;
    SweepCompleted completed = 5;
  }
}

message SweepStarted {
  repeated string commands = 1;   // Commands being run
  int32 retry_attempt = 2;
}

message SweepLogLine {
  string command = 1;             // Which command produced this line
  string content = 2;             // Raw output line (ANSI stripped)
  bool   is_stderr = 3;
}

message SweepCompleted {
  bool   all_passed = 1;
  int32  checks_run = 2;
  int32  checks_failed = 3;
  string failure_summary = 4;    // First 2000 chars of combined failure output
  int64  duration_ms = 5;
}
```

This proto design mirrors the existing `StreamTerminalResponse` pattern and uses `oneof` for type-safe payload dispatch.

---

## Recommendations

1. **Use `SendLiteralKeysWithEnter` for Earpiece injection** — the existing method in `session/tmux/tmux.go` is correct. Do NOT use `SendKeys` (lacks `-l` flag).

2. **Strip ANSI escapes before injecting** — use `github.com/acarl005/stripansi` or a simple regex before embedding test output in the Earpiece prompt.

3. **Cap injected prompt at 4000 chars** — tmux PTY input buffer limits and LLM context efficiency both favor truncation. Include only the last N failure lines.

4. **Follow `ReviewQueueManager` pattern exactly** — `context.WithCancel`, `run()` goroutine, `Start()`/`Stop()` methods. Don't deviate.

5. **Use `errgroup` for parallel Sweep checks** — `golang.org/x/sync/errgroup` for running go test + npm test in parallel, canceling remaining checks on first failure.

6. **Add new `SweepEvent` stream** — new `StreamSweepResults` RPC using the `SweepEvent` proto above. Add to `review_queue.proto` alongside other streaming RPCs.

7. **Inject after TaskComplete + 250ms delay** — no reliable pane-level API exists to confirm Claude Code is at its prompt. Use event-driven approach with a short stabilization delay.

---

## Sources

- Existing codebase: `session/tmux/tmux.go` — `SendLiteralKeys`, `SendLiteralKeysWithEnter`, `IsAtPrompt`, `WaitForPrompt`
- Existing codebase: `server/review_queue_manager.go` — canonical supervisor goroutine pattern
- Existing codebase: `session/events.go` — EventBus pub/sub backbone
- [tmux send-keys man page](https://man7.org/linux/man-pages/man1/tmux.1.html) — `-l` flag semantics
- [Go errgroup documentation](https://pkg.go.dev/golang.org/x/sync/errgroup) — parallel sweep coordination
- [ConnectRPC server streaming Go docs](https://connectrpc.com/docs/go/streaming/) — existing pattern in codebase