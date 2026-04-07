# Requirements: Crew Autonomy

**Status**: Draft | **Phase**: 1 — Ideation complete
**Created**: 2026-04-02
**Vocabulary**: This project uses the Heist metaphor established in conversation. See glossary below.

---

## Problem Statement

Today, stapler-squad is a **passive supervisor** — it observes Claude agent sessions, surfaces
them for human review when they need attention, and enforces permission rules via the
`ApprovalHandler` + `RuleBasedClassifier`. The human is in the loop for every tool decision
and every completed task.

This creates two friction points:

1. **Review queue noise** — routine, safe operations escalate to human review unnecessarily,
   even though the `RuleBasedClassifier` already has the signals to auto-classify them.

2. **No correction loop** — when a session completes work (test failures, partial implementation,
   wrong direction), a human must manually type a correction into the terminal. There is no
   mechanism for the system to detect completion, evaluate quality, and inject a corrective
   prompt automatically.

The result: running 3–5 concurrent sessions requires near-constant human attention. The goal
is to allow a developer to **launch sessions and walk away**, returning only when genuine
decisions are needed.

---

## Success Criteria

**Day 1 after launch:**
- A developer can start an Operative on a feature, go do other work, and return to a
  validated Score in the review queue — test output attached, diff summarized, ready to
  approve.
- Routine operations (read, lint, test runs) no longer appear in the review queue.
- A session that produces failing tests automatically receives an Earpiece correction and
  retries up to a configured limit before escalating to the Mastermind.

**Progressive trust model:**
- Start in **Supervised mode** (reduced noise, Sweep runs but Earpiece is off — human still
  sends corrections manually). This is the safe default.
- Promote sessions to **Going Dark mode** (full autonomous loop with Earpiece enabled) once
  the operator is comfortable with a session's behavior.
- Trust level is configurable; the system never silently enters full autonomy.

---

## Scope

### Must Have (MoSCoW)

- **Session Input Injection** (`SendInput` API) — programmatic write to a session's tmux pane.
  This is the unlock primitive. Without it, the Earpiece cannot exist.
- **Lookout** — per-session supervisor goroutine that:
  - Detects `ReasonTaskComplete` from the existing review queue signals
  - Triggers The Sweep on completion
  - On Sweep failure: sends an Earpiece correction (if Going Dark mode) or flags for human
  - On Sweep pass: assembles The Score and drops it in the review queue
  - Tracks retry count; escalates to Mastermind after `maxRetries`
- **The Sweep** — pluggable quality gate pipeline run after session completion:
  - Detect and run the project's test suite (`go test`, `npm test`, `pytest`, etc.)
  - Configurable: additional checks (lint, diff size guard, custom script hook) are opt-in
- **The Score** — enriched review queue item that carries: test results, diff summary,
  retry history, and risk level. Human sees validated output, not raw mid-flight session state.
- **Trust level config** — per-session toggle (Supervised vs Going Dark) surfaced in the
  web UI session card. Defaults to Supervised.
- **The Fixer** — lightweight cross-session patrol that monitors all active Lookouts,
  routes escalations to the Mastermind (review queue), and enforces capacity limits
  (max concurrent Going Dark sessions).

### Out of Scope (v1)

- Multi-session coordination (Gastown Convoys) — single-session loop only
- Auto-commit or auto-PR creation after a clean Sweep
- LLM-powered Sweep analysis (scoring output quality semantically)
- Dead Drop / Inside Man (session-to-session context handoff) — future phase
- Scheduler / capacity governance beyond a simple max-concurrent limit
- Custom per-repo crew.json configuration (v1 uses global config)

---

## Constraints

- **Tech stack**: Go backend, ConnectRPC/protobuf, React/Next.js frontend, tmux for session
  management. All new components must fit this stack.
- **Existing primitives to reuse**:
  - `ReactiveQueueManager` — event-driven routing (subscribe to `EventTaskComplete`)
  - `ApprovalHandler` + `RuleBasedClassifier` — Standing Orders (already classifies and
    auto-allows/denies tool use; Lookout builds on top of this, not beside it)
  - `EventBus` — pub/sub backbone (Lookout subscribes to session events here)
  - `session.ReviewQueue` + `session.AttentionReason` — `ReasonTaskComplete`,
    `ReasonTestsFailing` are already defined signal types
- **No regressions**: Supervised mode must be the default. Existing review queue behavior
  is unchanged unless the session opts into Going Dark.
- **Safety invariant**: The Earpiece can only send input to sessions in Going Dark mode.
  Supervised sessions never receive autonomous injections.
- **Solo developer project** — no team coordination overhead; implementation sequenced for
  one developer working in focused sprints.

---

## Context

### Existing Work

From conversation analysis of the codebase:

| Component | File | Relevance |
|---|---|---|
| `ReactiveQueueManager` | `server/review_queue_manager.go` | Lookout subscribes to its event stream |
| `ApprovalHandler` | `server/services/approval_handler.go` | Pattern for the Earpiece (writes back to session) |
| `RuleBasedClassifier` | `server/services/classifier.go` | Already classifies tool use; Sweep uses same pattern |
| `ReviewQueuePoller` | `session/` | Detects `ReasonTaskComplete` — the Lookout's trigger |
| `EventBus` | `server/events/` | Pub/sub backbone for all reactive coordination |
| `AttentionReason` enum | `session/` | `ReasonTaskComplete`, `ReasonTestsFailing` already exist |

The `ApprovalHandler.writeDecision()` method is the closest existing analog to the Earpiece —
it writes a structured response back through an open HTTP connection. The Earpiece requires
a different mechanism (tmux `send-keys`) because it's injecting a free-form prompt, not
responding to a specific hook call.

### Prior Art Reviewed

- **Gastown** (gastownhall/gastown) — multi-agent orchestration system. Vocabulary rejected
  in favor of the Heist metaphor (Operative, Lookout, Fixer, Sweep, Score, Earpiece).
  Architecture patterns adopted: Witness→Lookout, Deacon→Fixer, Refinery→Sweep.

### Stakeholders

- Solo developer / project owner: Tyler Stapler
- Users: developers running multiple concurrent Claude Code sessions

---

## Glossary (Heist Metaphor)

| Term | Technical concept |
|---|---|
| **Operative** | A Claude agent session doing the work |
| **Lookout** | Per-session supervisor goroutine |
| **Fixer** | Cross-session patrol / coordinator |
| **The Sweep** | Quality gate pipeline (tests, lint, diff) |
| **The Score** | Validated output package for human review |
| **The Earpiece** | Corrective prompt injected back into the session |
| **Going Dark** | Full autonomous mode (Earpiece enabled) |
| **Supervised** | Default mode (Sweep runs, no Earpiece) |
| **The Mastermind** | The human reviewer |
| **Standing Orders** | `RuleBasedClassifier` rules |
| **Heat** | Risk level from classifier |
| **Clean** | Auto-allow |
| **Burned** | Auto-deny |
| **The Fall** | Retry limit exhausted, escalate to Mastermind |
| **The Job** | Work assignment / task spec |
| **The Floor Plan** | Requirements document |
| **Dead Drop** | Handoff notes for a successor session |

---

## Research Dimensions Needed

- [ ] **Stack** — how other tools implement programmatic tmux input injection; ConnectRPC
      streaming patterns for Sweep result delivery; existing Go session supervision libraries
- [ ] **Features** — survey of comparable autonomous agent loop implementations (Gastown,
      Aider, SWE-bench harnesses); what Sweep checks are most valuable in practice
- [ ] **Architecture** — Lookout state machine design; Sweep pipeline abstraction;
      how to attach Score metadata to the existing `ReviewItem` proto without breaking
      current clients; Earpiece concurrency safety
- [ ] **Pitfalls** — infinite correction loops; Earpiece injection timing (session may not
      be at a prompt); tmux send-keys vs Claude Code's `/input` hook; test runner detection
      reliability across project types
