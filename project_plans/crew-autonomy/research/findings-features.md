# Crew Autonomy — Features Research

**Dimension**: Comparable implementations, quality gate design, test runner detection
**Date**: 2026-04-02
**Researcher**: Claude (claude-sonnet-4-6)

---

## Executive Summary

- **Aider is the closest prior art**: it has a mature `--test-cmd` / `--lint-cmd` loop that auto-runs after each AI edit, feeds failures back as a corrective message, and retries up to `--auto-test` / `--lint` retry limits. The implementation is a direct template for The Sweep + Earpiece.
- **Claude Code's Stop hook is architecturally ideal for triggering The Sweep** — it fires after the agent has finished a turn, receives exit code + stdout/stderr, and can inject a blocking message before the session resumes. This may replace a custom Lookout entirely.
- **SWE-bench's harness reveals battle-tested patterns**: containerized test runs, timeout enforcement (typically 300s), structured pass/fail capture, and environment reset between attempts.
- **The highest-signal quality checks** are (in order): test suite pass/fail → type check (tsc/mypy) → build compilation → diff size guard. Pure lint is low signal for blocking.
- **Test runner detection** is solved by checking for lockfiles/manifests in order: `go.mod` → `Cargo.toml` → `pyproject.toml`/`setup.py` → `package.json` — the same heuristic used by VS Code's test discovery and several open-source tools.

---

## Comparable Implementations

### 1. Aider — `--test-cmd` / `--lint-cmd` loop

**What it does:**
Aider implements an explicit "lint-test-fix" loop that runs after every set of AI-generated edits:

1. After the AI applies file changes, Aider runs `--lint-cmd` (default: none, must be configured).
2. If lint exits non-zero, the output is injected back to the AI as a new user message: `"I ran the linter and got these errors, please fix them"`.
3. Same pattern for `--test-cmd`. Aider runs the command, captures stdout/stderr and exit code, and constructs a corrective message.
4. The loop retries up to `--auto-lint` / `--auto-test` limits (default: 3 retries).
5. If still failing after N retries, Aider abandons the loop and reports to the user.

**Key design choices:**
- Commands are arbitrary shell strings — no test runner detection logic; the user configures them.
- Aider passes the **full combined stdout+stderr** of the failing command as context. No summarization.
- In `--yes` (non-interactive) mode, the retry loop runs fully autonomously.
- The `--test-cmd` can be `pytest`, `go test ./...`, `npm test` — whatever the project needs.

**What we can adopt:**
- The "capture output → inject as corrective user message" pattern is proven. Use it verbatim for the Earpiece.
- The N-retry ceiling with escalation to human review is the right failure mode.
- Passing raw command output (not a summary) gives the model the best chance to self-correct.

**Source**: https://aider.chat/docs/usage/lint-test.html

---

### 2. SWE-bench — Evaluation Harness

**What it does:**
SWE-bench is the standard benchmark for AI software engineering agents. Its evaluation harness:

1. Takes a GitHub issue + a patch (diff) produced by an agent.
2. Applies the patch to the repository in a **Docker container** (clean environment).
3. Runs the relevant test files for that issue using the project's own test runner (pytest for Python repos).
4. Captures per-test pass/fail results and compares to a "golden" expected outcome.
5. Reports `resolved` (all expected tests pass) vs `unresolved`.

**Key design choices:**
- Tests run with a **300-second timeout** per instance (configurable). This is the community-accepted upper bound for a single agent run.
- The harness uses **project-specific test commands** from a manifest — no universal detection.
- Environment isolation (Docker) prevents cross-contamination between attempts.
- The pass/fail signal is binary per test case, aggregated to a resolved/not-resolved result.

**What we can adopt:**
- The 300s timeout ceiling is a good starting point for The Sweep's `--timeout` default.
- Per-attempt environment state matters. In our context: ensure git state is clean before running tests (no stale build artifacts from a previous failed run bleeding into the current check).
- Structured result capture (pass count, fail count, which tests failed) is more useful than just an exit code for constructing the Earpiece message.

**Source**: https://www.swebench.com/ and https://github.com/SWE-bench/SWE-bench

---

### 3. OpenHands (OpenDevin) — Autonomous Loop

**What it does:**
OpenHands implements a more general agent loop where the agent can execute bash commands, run tests, read output, and iterate. Its relevant pattern:

1. The agent decides to run tests as a tool call (bash execution).
2. The framework captures output and returns it as an observation.
3. The agent decides whether to continue editing or declare completion.
4. A "verifier" component can be configured to gate task completion.

**Key design choices:**
- Test execution is **inside** the agent loop, not external to it. The agent itself chooses when to run tests.
- This is more autonomous but less controllable — the agent can choose to skip tests.
- The verifier pattern (external check that must pass before a task is marked complete) is closer to what Crew Autonomy needs.

**What we can adopt:**
- The distinction between "agent-initiated tests" (inside the loop) and "supervisor-initiated tests" (The Sweep, outside and after the loop) is important. Crew Autonomy is doing the latter, which gives stronger guarantees.
- OpenHands' verifier pattern validates our approach: external quality gates are the right architectural boundary.

**Source**: https://github.com/All-Hands-AI/OpenHands

---

### 4. Devin / Claude Code CLI — Agent-Initiated Quality Checks

**What they do:**
Devin (Cognition) and Claude Code (Anthropic CLI) both encourage the agent to run tests itself during its work loop. Claude Code's `CLAUDE.md` can instruct the agent to run tests before reporting completion.

**Relevant to us:**
- Claude Code agents already often run `go test ./...` or `npm test` themselves before completing.
- However, this is unreliable — the agent may forget, time out, or decide tests aren't relevant.
- The Sweep provides a **mandatory, external, non-skippable** quality gate, which is the key differentiator.

---

## Claude Code Hook System

Claude Code supports lifecycle hooks configured in `settings.json` under `"hooks"`. Hooks run shell commands at specific points in the agent's lifecycle and can influence agent behavior via exit codes and output.

### Hook Types

| Hook | Trigger | Can Block | Output Sent To Agent |
|------|---------|-----------|---------------------|
| `PreToolUse` | Before any tool call | Yes (exit 2 = block tool) | Yes (via stderr) |
| `PostToolUse` | After any tool call | No | Yes (via stdout, injected as tool result context) |
| `Notification` | Agent sends a notification | No | No |
| `Stop` | Agent finishes a turn / calls the Stop tool | Yes (exit 2 = prevent stop) | Yes (via stdout) |

### The Stop Hook — Critical for Crew Autonomy

The `Stop` hook is architecturally the most relevant:

- **Fires when**: The Claude agent calls `Stop` or its turn naturally ends.
- **Can block**: If the hook exits with code `2`, Claude does NOT stop — it continues the conversation.
- **Can inject**: The hook's stdout is fed back to the agent as a new message before it continues.
- **Use case for us**: Run The Sweep inside the Stop hook. If tests fail, exit 2 with the failure output → Claude gets the failure message and retries. If tests pass, exit 0 → Claude stops and the session goes to the review queue.

**This is a near-complete implementation of The Sweep + Earpiece using only the Stop hook.** The hook approach has several advantages:
- No separate process needed to watch for session completion (the hook is the trigger).
- Retry logic is implicit — if the hook exits 2, Claude automatically retries.
- The N-retry limit can be implemented with a counter in a temp file (per session PID).

**Limitations of the Stop hook approach:**
- Hook is configured per-user in `~/.claude/settings.json`, not per-session or per-project by default. For Crew Autonomy, we need per-session hooks (injected when we spawn the Operative). Claude Code supports `--settings` flag to override settings file per invocation — this solves it.
- The hook runs in the same process context as the agent. If the test suite is long-running, the agent is blocked. Acceptable for our use case.
- Hook stdout length limits are not explicitly documented — very large test output may need truncation before injection.

### PreToolUse Hook — Useful for Guardrails

Could be used to:
- Block the agent from running dangerous commands (`rm -rf`, `git push --force`).
- Log all tool calls for the review queue audit trail.
- Not directly relevant to the correction loop, but valuable for safety.

### PostToolUse Hook — Useful for Incremental Feedback

Could be used to:
- Run a quick lint check after each file write.
- Inject immediate feedback without waiting for the Stop hook.
- Higher noise, but catches errors earlier in the loop.

### Verdict on Hooks

**Recommendation**: Use the `Stop` hook as the primary Sweep trigger. This replaces the need for a custom Lookout that polls tmux output. The Stop hook is the right abstraction — it fires at exactly the moment we care about (agent declares completion), and it gives us injection + blocking capability natively.

**Source**: https://docs.anthropic.com/en/docs/claude-code/hooks

---

## Recommended Sweep Checks

Ranked by signal value for the "did the AI agent do good work" question:

### Tier 1 — Blocking (must pass)

| Check | Signal | Rationale |
|-------|--------|-----------|
| **Test suite pass/fail** | Highest | Direct functional validation. A failing test means the implementation is broken. Raw output gives the agent everything it needs to fix the issue. |
| **Build/compilation** | High | Catches syntax errors, missing imports, type mismatches at the compiler level. Fast, cheap, and unambiguous. For Go: `go build ./...`. For TS: `tsc --noEmit`. |
| **Type check (if separate from build)** | High | For TypeScript (`tsc --noEmit`) and Python (`mypy`), this catches a class of bugs that tests might not cover. |

### Tier 2 — Informational (report but don't block by default)

| Check | Signal | Rationale |
|-------|--------|-----------|
| **Diff size guard** | Medium | If the agent modified > N files or > M lines beyond what was expected, flag for human review. Catches "agent went too wide" without false-positive blocking. |
| **Lint** | Low-Medium | Lint failures are rarely functional bugs. Running lint is valuable for the review queue display, but using it as a blocking gate creates too many false positives. Exception: `--fail-on-lint` flag for teams that want it. |
| **No new TODO/FIXME** | Low | Catches the agent punting on hard parts. Easy to check, useful signal. |

### Tier 3 — Optional / Future

| Check | Signal | Rationale |
|-------|--------|-----------|
| **Security scan (gosec, semgrep)** | Situational | High false positive rate. Better as an async check after the review queue, not a blocking gate. |
| **Coverage delta** | Low | Coverage decrease isn't always bad (e.g., deleted dead code). Too noisy for autonomous blocking. |

### Ordering Recommendation

Run checks in this order for fast-fail behavior:
1. Build/compile (fastest, catches compile errors immediately)
2. Type check (fast, catches type errors)
3. Test suite (slowest, but most signal — run last so compile errors don't hide test errors)
4. Diff size guard (instant, pure metadata)

---

## Test Runner Detection

### Recommended Heuristic (Priority-Ordered)

Check for the following files in the project directory (and parent directories up to git root), stopping at first match:

```
Priority 1: go.mod           → go test ./...
Priority 2: Cargo.toml       → cargo test
Priority 3: pyproject.toml   → pytest (or: uv run pytest / python -m pytest)
            setup.py         → pytest / python -m pytest
            requirements.txt → pytest (weaker signal, fall through to check for pytest)
Priority 4: package.json     → inspect "scripts.test" field
            → if scripts.test exists: npm test / yarn test / pnpm test
            → detect package manager: bun.lockb → bun test, pnpm-lock.yaml → pnpm test,
              yarn.lock → yarn test, else npm test
Priority 5: Makefile         → check for "test:" target → make test
Priority 6: .github/workflows/*.yml → grep for test commands as fallback hint
```

### Multiple Manifests (Monorepos)

If multiple manifests are found (e.g., Go backend + TS frontend in same repo):
- Run all detected test suites.
- OR: Detect which files were modified by the agent (via `git diff --name-only HEAD~1`) and only run the test suites covering those paths.
- The "run all" approach is safer for correctness; the "targeted" approach is faster for iteration.

### TypeScript Specifics

For TypeScript projects, separate the type check from the test run:
- Type check: `tsc --noEmit` (or `npx tsc --noEmit`)
- Tests: look at `package.json` scripts — `"test"`, `"test:ci"`, `"vitest"`, `"jest"` are common keys.

### Timeout Recommendations

| Ecosystem | Suggested Default | Rationale |
|-----------|-----------------|-----------|
| Go | 120s | `go test ./...` compiles and runs; large codebases can take 60-90s |
| Node/TS | 120s | Jest/Vitest startup + test execution |
| Python | 180s | pytest can be slow with heavy test suites |
| Rust | 300s | `cargo test` includes compilation which is very slow |
| Make | 180s | Unknown, use conservative default |

**Global max timeout**: 300s (matches SWE-bench community standard). If tests exceed this, the Sweep reports timeout as a failure and injects "tests timed out" as the Earpiece message.

### Implementation Notes

- Check `CLAUDE.md` / project-level config first: if the project specifies a test command, use it. This overrides auto-detection.
- Cache the detected runner per session to avoid re-detection overhead on every retry.
- Log the detected runner prominently so users can verify and override.

---

## Recommendations

1. **Use Claude Code's Stop hook as the Sweep trigger.** It provides the injection and blocking capability needed without a custom Lookout process. Configure per-session via `--settings` flag when spawning the Operative.

2. **Copy Aider's correction message format.** Aider's pattern of "I ran [command] and got this output: [raw output]" is battle-tested. Keep it simple — no summarization, pass the raw failure output.

3. **Implement the retry counter via a temp file keyed on session ID.** On each Stop hook invocation: read counter from `/tmp/crew-autonomy-retries-{session-id}`, increment, compare to N, write back. Clean up on session completion.

4. **Default Sweep configuration**: run `build → typecheck → test` in order, stop at first failure, inject the failure output. Report lint separately in the review queue UI but don't block on it.

5. **Implement test runner detection as a simple file-priority lookup** (no external library needed). Check `go.mod`, `Cargo.toml`, `pyproject.toml`, `package.json` in that order. Honor project-level config override in `CLAUDE.md` or a `.crew-autonomy.json`.

6. **Set default timeouts** at 120s for Go/Node, 180s for Python, 300s for Rust. Make them configurable per-session.

7. **Diff size guard**: flag (but don't block by default) if the agent modified more than 20 files or more than 500 lines net. Surface this prominently in the review queue.

8. **For the review queue**: always include the Sweep's full output (pass/fail, which tests failed, diff stat) so human reviewers have full context.

---

## Sources

| Title | URL | Accessed |
|-------|-----|---------|
| Aider — Lint and test docs | https://aider.chat/docs/usage/lint-test.html | 2026-04-02 |
| Aider — Full options reference | https://aider.chat/docs/config/options.html | 2026-04-02 |
| SWE-bench official site | https://www.swebench.com/ | 2026-04-02 |
| SWE-bench GitHub repository | https://github.com/SWE-bench/SWE-bench | 2026-04-02 |
| Claude Code Hooks documentation | https://docs.anthropic.com/en/docs/claude-code/hooks | 2026-04-02 |
| OpenHands (OpenDevin) GitHub | https://github.com/All-Hands-AI/OpenHands | 2026-04-02 |
| Brave Search: "aider auto-test loop autonomous correction agent coding" | — | 2026-04-02 |
| Brave Search: "SWE-bench evaluation harness test runner agent patch" | — | 2026-04-02 |
| Brave Search: "claude code hooks Stop PostToolUse autonomous agent loop" | — | 2026-04-02 |
| Brave Search: "AI coding agent quality gate automated validation loop" | — | 2026-04-02 |
| Brave Search: "test runner detection multi-language project type go python node heuristic" | — | 2026-04-02 |
