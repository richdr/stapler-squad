# Research Plan: Crew Autonomy

**Created**: 2026-04-02
**Input**: project_plans/crew-autonomy/requirements.md

## Subtopics & Search Strategy

### 1. Stack
**Output**: `research/findings-stack.md`
**Focus**: tmux programmatic input injection mechanics, Go supervisor goroutine patterns, ConnectRPC streaming for async results
**Searches** (cap: 5):
1. `tmux send-keys programmatic input injection golang`
2. `tmux control mode input injection non-interactive`
3. `golang supervisor goroutine per-worker lifecycle pattern`
4. `connectrpc server streaming response golang`
5. `golang context cancellation goroutine cleanup pattern`

### 2. Features
**Output**: `research/findings-features.md`
**Focus**: Comparable autonomous agent loop implementations, valuable quality gate checks, test runner auto-detection
**Searches** (cap: 5):
1. `autonomous AI coding agent correction loop quality gate`
2. `SWE-bench test harness agent evaluation pipeline`
3. `aider auto-test run agent loop implementation`
4. `test runner detection multi-language go python node project`
5. `claude code hooks PreToolUse PostToolUse notification stop`

### 3. Architecture
**Output**: `research/findings-architecture.md`
**Focus**: Per-session supervisor state machine, protobuf extension without breaking clients, concurrency safety for input injection
**Searches** (cap: 4):
1. `golang state machine per-goroutine supervisor pattern`
2. `protobuf backward compatible field addition optional`
3. `concurrent safe goroutine input injection tmux race condition`
4. `golang worker pool supervisor tree erlang OTP pattern`

### 4. Pitfalls
**Output**: `research/findings-pitfalls.md`
**Focus**: Infinite retry loops, injection timing, tmux pane state detection, test runner false positives
**Searches** (cap: 4):
1. `autonomous agent infinite loop prevention retry circuit breaker`
2. `tmux send-keys timing prompt detection wait for shell`
3. `tmux pane current command detection is-at-prompt`
4. `AI agent correction loop oscillation failure mode`
