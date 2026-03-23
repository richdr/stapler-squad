# Feature Plan: Permissions Analysis & Automatic Approvals

**Date**: 2026-03-23
**Status**: Planning
**Scope**: Intelligent auto-approval layer that classifies tool use risk, auto-handles low-risk operations, learns from user decisions via analytics, and surfaces rule management in the web UI.

---

## Table of Contents

- [Epic Overview](#epic-overview)
- [Architecture Decisions](#architecture-decisions)
- [Story Breakdown](#story-breakdown)
  - [Story 1: Risk Classifier Engine](#story-1-risk-classifier-engine)
  - [Story 2: Rules Engine & Config Integration](#story-2-rules-engine--config-integration)
  - [Story 3: Analytics Store & Decision Log](#story-3-analytics-store--decision-log)
  - [Story 4: Handler Integration](#story-4-handler-integration)
  - [Story 5: Proto/API Extensions](#story-5-protoapi-extensions)
  - [Story 6: Web UI – Rules & Analytics](#story-6-web-ui--rules--analytics)
- [Known Issues & Bug Risks](#known-issues--bug-risks)
- [Dependency Visualization](#dependency-visualization)
- [Integration Checkpoints](#integration-checkpoints)
- [Context Preparation Guide](#context-preparation-guide)
- [Success Criteria](#success-criteria)

---

## Epic Overview

### User Value

Today, **every** Claude Code tool use request blocks and waits for manual user approval, even for completely benign read-only operations like `find . -name "*.go"`. This creates constant interruption and slows down autonomous agents dramatically.

This feature adds an intelligent classification layer that:
- **Auto-approves** low-risk operations (file reads, glob searches, info-gathering commands) without interrupting the user
- **Auto-denies** high-risk operations (writes to `.env`, system dirs, destructive Bash) with an actionable message suggesting safer alternatives
- **Escalates** medium-risk and borderline operations to the existing manual review queue
- **Records every decision** with full context, enabling the user to tune rules over time based on real patterns

### Success Metrics

| Metric | Target |
|--------|--------|
| Auto-approved rate for read-only tools | ≥ 90% (Glob, Grep, Read, WebFetch) |
| Manual review queue interruptions reduced | ≥ 60% reduction |
| P99 classification latency | < 2ms (pure in-memory, no I/O) |
| Analytics query latency | < 100ms for 30-day window |
| Rule load on startup | < 50ms |
| False positive rate (auto-deny of intended ops) | < 1% measured in first week |

### Scope

**Included:**
- In-process Go classifier with regex-based pattern matching
- Configurable rules stored in `~/.stapler-squad/auto_approve_rules.json`
- Reads existing `~/.claude/settings.json` and project `.claude/settings.json` to avoid re-prompting for already-allowed tools
- Append-only analytics log (`~/.stapler-squad/approval_analytics.jsonl`)
- New ConnectRPC RPCs: `ListApprovalRules`, `UpsertApprovalRule`, `DeleteApprovalRule`, `GetApprovalAnalytics`
- Web UI: Rules management panel with enable/disable toggles, add/edit/delete
- Web UI: Analytics dashboard showing decision distribution, most-triggered rules, recently denied commands
- `systemMessage` injected into Claude context explaining why a denial occurred + safer alternative

**Explicitly Excluded:**
- ML/LLM-based classification (pure rule-based for determinism and performance)
- Per-session rule overrides in this iteration (global + project scope only)
- Audit log exports (CSV/PDF) — not in this iteration
- Multi-user/team rule sharing

### Constraints

- Classification must complete synchronously in `HandlePermissionRequest` before blocking on user decision
- Must not break existing manual-approval flow for escalated items
- Rule files are user-owned JSON — no database migrations required
- Must handle missing/corrupt rule files gracefully (fallback: escalate all to manual)
- The 4-minute HTTP hook timeout is a hard constraint; classification must be near-instant

---

## Architecture Decisions

### ADR-020: Rule-Based Classification over ML

**Context**: Classifying `bash` commands and file paths for risk requires pattern recognition.

**Decision**: Pure regex/glob pattern matching with a tiered rule priority system.

**Rationale**:
- ML inference would add 50-500ms latency, unacceptable in the HTTP hook path
- Rules are deterministic, auditable, and user-editable
- 90% of cases are structurally simple (tool name + command flags + path patterns)
- Users can inspect and tune rules directly; no black-box behavior

**Consequences**: Cannot handle novel adversarial inputs without explicit rules; requires initial rule seed.

**Patterns Applied**: Strategy (classifier implementations per tool type), Chain of Responsibility (rule evaluation order).

---

### ADR-021: Append-Only JSONL Analytics

**Context**: Need to record every decision for rule tuning without blocking the hot path.

**Decision**: Asynchronous append to `~/.stapler-squad/approval_analytics.jsonl`.

**Rationale**:
- Zero-blocking: write dispatched to goroutine, main handler returns immediately
- Human-readable format for offline analysis
- No schema migration risk
- Easy grep/jq queries for debugging
- SQLite migration can happen later (see `docs/tasks/repository-pattern-sqlite-migration.md`)

**Consequences**: No indexed queries; analytics API must do linear scan with caching. Acceptable for < 100K entries / 30 days.

**Patterns Applied**: Event Sourcing (immutable append-only log), CQRS (write path separate from query path).

---

### ADR-022: Claude Settings Integration via Read-Only Parse

**Context**: Claude Code already has `~/.claude/settings.json` with a `permissions.allow` list. We should respect these to avoid double-prompting.

**Decision**: Read `~/.claude/settings.json` and project `.claude/settings.json` at startup and on file change (fsnotify). Cache parsed `allow` regexes in memory. Any tool matching the allow list is auto-approved without creating an analytics entry.

**Rationale**: Respects user's existing configuration as ground truth; avoids UX confusion where stapler-squad asks for something Claude Code already approves.

**Consequences**: If Claude settings file changes, cache must refresh. Stale cache could cause spurious prompts (safe-fail direction).

**Patterns Applied**: Cache-Aside, Observer (file watcher).

---

### ADR-023: Three Decision Outcomes

**Context**: Hook must return one of `allow`, `deny`, or `ask`. `ask` means show terminal prompt.

**Decision**: Classifier returns one of:
- `AutoAllow` — return immediately with `allow`, log analytics async
- `AutoDeny` — return immediately with `deny` + injected `systemMessage` with safer alternative
- `Escalate` — fall through to existing manual review queue (current behavior)

**Rationale**: `ask` is equivalent to `Escalate` in our architecture (the HTTP hook connection blocks until the user responds via the web UI). We keep the existing manual flow intact for escalated cases.

---

## Story Breakdown

### Story 1: Risk Classifier Engine

**User Value**: Developers can define risk classification logic as testable Go code with no external dependencies. This is the foundation all other stories build on.

**Scope**: 1 week

**Acceptance Criteria**:
1. `server/services/classifier.go` exports a `Classifier` interface and a `RuleBasedClassifier` implementation
2. Classification returns `ClassificationResult{Decision, RiskLevel, Reason, Alternative}` in < 1ms
3. Built-in seed rules cover: Read/Glob/Grep/WebFetch (AutoAllow), find without -exec (AutoAllow), rm/git push/curl POST (Escalate), .env writes (AutoDeny)
4. 100% unit test coverage on all seed rules

---

#### Task 1.1 — Define Classifier Interface & Types [2h]

**Objective**: Create the core domain types and `Classifier` interface.

**Context Boundary**:
- Primary: `server/services/classifier.go` (new file, ~150 lines)
- Supporting: `server/services/approval_store.go` (for `PermissionRequestPayload` type reference)

**Prerequisites**: Understanding of existing `PermissionRequestPayload` struct.

**Implementation Approach**:
1. Define `RiskLevel` enum: `RiskLow`, `RiskMedium`, `RiskHigh`, `RiskCritical`
2. Define `ClassificationDecision` enum: `AutoAllow`, `AutoDeny`, `Escalate`
3. Define `ClassificationResult` struct: `Decision`, `RiskLevel`, `Reason string`, `Alternative string`
4. Define `Classifier` interface: `Classify(payload PermissionRequestPayload) ClassificationResult`
5. Define `ClassificationContext` (cwd, git repo type, session path) passed alongside payload

**Validation Strategy**:
- Unit tests: verify zero-value returns Escalate (safe default)
- Unit tests: verify all fields are populated in non-zero results
- `go vet ./...` passes

**INVEST Check**: ✅ Independent (no deps), ✅ Small (~150 lines), ✅ Testable (pure types)

---

#### Task 1.2 — Implement RuleBasedClassifier with Seed Rules [3h]

**Objective**: Implement the `RuleBasedClassifier` with a comprehensive seed rule set.

**Context Boundary**:
- Primary: `server/services/classifier.go` (extend, ~300 lines total)
- Supporting: `server/services/classifier_test.go` (new, ~200 lines)
- Supporting: `server/services/approval_store.go`

**Prerequisites**: Task 1.1 complete.

**Implementation Approach**:
1. Define `Rule` struct: `ToolName string`, `CommandPattern *regexp.Regexp`, `FilePattern *regexp.Regexp`, `Decision ClassificationDecision`, `RiskLevel`, `Reason`, `Alternative`, `Priority int`
2. `RuleBasedClassifier` holds `[]Rule` sorted by priority (higher = checked first)
3. `Classify()` iterates rules, returns first match; default is `Escalate`
4. Seed rules (in priority order):
   - **AutoAllow** (Low risk): `Read`, `Glob`, `Grep`, `WebFetch` (all inputs)
   - **AutoAllow** (Low risk): `Bash` where command matches `^(ls|cat|head|tail|find\s[^|&;]*-name)` and no `-exec`/pipes
   - **AutoDeny** (Critical): `Write`/`Edit` where `file_path` matches `\.env$|\.env\.|/etc/|/System/|/usr/`
   - **AutoDeny** (Critical): `Bash` where command matches `rm\s+-rf\s+/|sudo\s+rm|> /etc/`
   - **Escalate** (High risk): `Bash` where command contains `git push|git reset --hard|curl.*-X POST|wget.*-O /`
   - **Escalate** (Medium): `Write`/`Edit`/`MultiEdit` for any file (user decides)
   - **Escalate** (default): everything else

**Validation Strategy**:
- Unit tests: one test per seed rule, both matching and non-matching cases
- Benchmark: 1000 classifications < 5ms total
- Test: default `Escalate` for unknown tool name

**INVEST Check**: ✅ Independent, ✅ Valuable (immediately functional), ✅ Estimable (3h with tests)

---

#### Task 1.3 — Context-Aware Classification (Git & Path Context) [2h]

**Objective**: Extend classifier to factor in `cwd` and git repository type for context-sensitive decisions.

**Context Boundary**:
- Primary: `server/services/classifier.go`
- Supporting: `server/services/classifier_test.go`
- Supporting: `session/git/` (read only — understand repo detection)

**Prerequisites**: Task 1.2 complete.

**Implementation Approach**:
1. Add `ClassificationContext` param to `Classify()`: `{Cwd string, IsGitRepo bool, RepoRoot string, IsWorktree bool}`
2. Context-sensitive rules:
   - Bash `find` inside `RepoRoot` → AutoAllow (bounded search)
   - `Write` to path within `.git/` → AutoDeny
   - `Write` to path within worktree but outside repo root → Escalate with `Alternative: "Use a path within the worktree"`
3. `BuildContext(cwd string) ClassificationContext` helper that does lightweight `git rev-parse` check (cached 30s)
4. Context cache keyed by `cwd` with 30-second TTL to avoid repeated git subprocess calls

**Validation Strategy**:
- Unit tests with mocked context (IsGitRepo=true/false, IsWorktree=true/false)
- Benchmark: context cache hit path < 0.1ms

**INVEST Check**: ✅ Small, ✅ Testable (injectable context), ✅ Valuable

---

### Story 2: Rules Engine & Config Integration

**User Value**: Users can customize auto-approval behavior via a JSON config file and existing Claude settings are respected automatically.

**Scope**: 1 week

**Acceptance Criteria**:
1. Rules load from `~/.stapler-squad/auto_approve_rules.json` on startup
2. Changes to rules file are picked up within 5 seconds (fsnotify)
3. `~/.claude/settings.json` `permissions.allow` list is parsed and merged as AutoAllow rules
4. Corrupt/missing rules files fail gracefully (log warning, use seed rules only)
5. Rules are additive: user rules take priority over seed rules by default

---

#### Task 2.1 — Rules File Format & Persistence [2h]

**Objective**: Define JSON schema for user rules and implement load/save with atomic writes.

**Context Boundary**:
- Primary: `server/services/rules_store.go` (new, ~200 lines)
- Supporting: `server/services/classifier.go`
- Supporting: `server/services/approval_store.go` (pattern reference for atomic write)

**Prerequisites**: Task 1.1 (types).

**Implementation Approach**:
1. Define `RuleSpec` JSON struct (serializable version of `Rule`): `ID`, `ToolName`, `CommandPattern string`, `FilePattern string`, `Decision`, `RiskLevel`, `Reason`, `Alternative`, `Priority`, `Enabled bool`, `CreatedAt`, `Source` ("user"|"seed"|"claude-settings")
2. Define `RulesFile` struct: `Version int`, `Rules []RuleSpec`
3. `RulesStore.Load()` reads from disk, validates, returns `[]Rule` (compiled regexes)
4. `RulesStore.Save(rules []RuleSpec)` atomic write (write tmp, rename)
5. Versioned format (Version=1) for future migration support

**Validation Strategy**:
- Unit tests: load valid file, load corrupt file (expect seed-only fallback), load missing file (expect seed-only), save and reload roundtrip
- Test: compiled regex is nil-safe (invalid regex in user file → skip rule, log warning)

---

#### Task 2.2 — Claude Settings Parser [2h]

**Objective**: Parse `~/.claude/settings.json` and project `.claude/settings.json` to extract already-allowed tools.

**Context Boundary**:
- Primary: `server/services/claude_settings_parser.go` (new, ~150 lines)
- Supporting: `config/claude.go` (existing `ClaudeConfigManager.GetConfig()`)
- Supporting: `server/services/classifier.go`

**Prerequisites**: Task 1.1.

**Implementation Approach**:
1. Define `ClaudePermissions` struct: `Allow []string` (tool name patterns from settings)
2. `ParseClaudeSettings(path string) (*ClaudePermissions, error)` reads and parses JSON
3. Parse the `permissions.allow` array — patterns are glob-style per Claude Code docs
4. `ToRules(perms *ClaudePermissions) []Rule` converts each allow entry to an AutoAllow rule with `Source="claude-settings"`
5. Merge order: claude-settings rules (highest priority) > user rules > seed rules
6. Parse both `~/.claude/settings.json` and `<cwd>/.claude/settings.json`; project settings take precedence for overlapping tools

**Validation Strategy**:
- Unit tests with fixture JSON files (valid, missing permissions key, empty allow list)
- Test: glob pattern `Bash(find*)` converts to correct regex

---

#### Task 2.3 — File Watcher & Hot Reload [2h]

**Objective**: Pick up rule changes without server restart using fsnotify.

**Context Boundary**:
- Primary: `server/services/rules_store.go`
- Supporting: `server/services/classifier.go`

**Prerequisites**: Task 2.1 complete.

**Implementation Approach**:
1. `RulesStore.WatchAndReload(ctx context.Context)` goroutine using `github.com/fsnotify/fsnotify`
2. Debounce file change events (100ms) to avoid thrashing on rapid saves
3. On reload: `s.mu.Lock()`, recompile rules, replace in-memory slice, `s.mu.Unlock()`
4. Broadcast reload event to `RulesReloadedCh chan struct{}` so classifier can rebuild its sorted slice
5. Log reload with rule count diff

**Validation Strategy**:
- Integration test: write rule file, wait 200ms, verify classifier picks up new rule
- Test: corrupt file mid-watch → keeps previous rules, logs warning

---

### Story 3: Analytics Store & Decision Log

**User Value**: Every classification decision is recorded with full context, enabling users to understand what Claude is trying to do and tune rules based on real patterns.

**Scope**: 1 week

**Acceptance Criteria**:
1. Every classification decision (auto-allow, auto-deny, escalate, manual-allow, manual-deny) is recorded in `~/.stapler-squad/approval_analytics.jsonl`
2. Analytics API returns decision counts, top tools, top denied commands, and rule-triggered counts
3. Analytics query for 30-day window completes in < 500ms
4. Manual decisions (resolved via web UI) are also recorded, correlated by `approval_id`

---

#### Task 3.1 — Analytics Entry Schema & Writer [2h]

**Objective**: Define the analytics event schema and implement async append writer.

**Context Boundary**:
- Primary: `server/services/analytics_store.go` (new, ~200 lines)
- Supporting: `server/services/approval_store.go` (for `PermissionRequestPayload`)
- Supporting: `server/services/classifier.go` (for `ClassificationResult`)

**Prerequisites**: Task 1.1.

**Implementation Approach**:
1. Define `AnalyticsEntry` struct:
   - `ID string` (uuid)
   - `Timestamp time.Time`
   - `SessionID string`
   - `ToolName string`
   - `CommandPreview string` (first 200 chars of command/file_path)
   - `Cwd string`
   - `Decision string` ("auto_allow"|"auto_deny"|"escalate"|"manual_allow"|"manual_deny")
   - `RiskLevel string`
   - `RuleID string` (which rule triggered, empty for manual)
   - `RuleName string`
   - `Reason string`
   - `Alternative string` (if deny)
   - `DurationMs int64` (classification latency)
   - `ApprovalID string` (for correlation with manual decisions)
2. `AnalyticsStore` with buffered channel (capacity 1000) + goroutine that flushes to JSONL file
3. `Record(entry AnalyticsEntry)` non-blocking (drops if buffer full, increments dropped counter)
4. Flush goroutine: drains channel, appends to file, fsync every 10 entries or 5 seconds

**Validation Strategy**:
- Unit tests: write 100 entries, read back file, verify all present
- Test: buffer full → drop + increment counter, no panic
- Benchmark: Record() < 0.1ms (non-blocking)

---

#### Task 3.2 — Manual Decision Recording [1h]

**Objective**: Hook into `ApprovalService.ResolveApproval` to record manual decisions.

**Context Boundary**:
- Primary: `server/services/approval_service.go`
- Supporting: `server/services/analytics_store.go`

**Prerequisites**: Task 3.1 complete.

**Implementation Approach**:
1. Add `analyticsStore *AnalyticsStore` field to `ApprovalService`
2. In `ResolveApproval()`: after resolving, call `as.analyticsStore.Record(...)` with decision `"manual_allow"` or `"manual_deny"`, correlating via `approval_id`
3. Pull `ToolName`, `ToolInput`, `Cwd` from `ApprovalStore.Get(approvalID)` before it's removed
4. Nil-safe: if `analyticsStore` is nil, skip recording (backward compatible)

**Validation Strategy**:
- Unit test: resolve approval → analytics entry written with correct `ApprovalID` and decision
- Test: nil analyticsStore → no panic

---

#### Task 3.3 — Analytics Query Engine [3h]

**Objective**: Implement in-memory analytics aggregations over the JSONL log.

**Context Boundary**:
- Primary: `server/services/analytics_store.go`
- Supporting: `server/services/analytics_store_test.go` (new)

**Prerequisites**: Task 3.1 complete.

**Implementation Approach**:
1. `AnalyticsStore.LoadWindow(since time.Time) ([]AnalyticsEntry, error)` — linear scan of JSONL, filter by timestamp
2. Cache loaded entries in `sync.Map` keyed by date, invalidated when new entries written to that day
3. `AnalyticsSummary` struct:
   - `TotalDecisions int`
   - `DecisionCounts map[string]int` (auto_allow, auto_deny, escalate, manual_allow, manual_deny)
   - `TopTools []ToolStat` (tool name + count, top 10)
   - `TopDeniedCommands []CommandStat` (preview + count, top 10)
   - `TopTriggeredRules []RuleStat` (rule ID + name + count, top 10)
   - `AutoApproveRate float64` (auto_allow / total)
   - `ManualReviewRate float64` (escalate + manual / total)
4. `ComputeSummary(entries []AnalyticsEntry) AnalyticsSummary` pure function, no I/O

**Validation Strategy**:
- Unit tests with fixture JSONL data (100 entries, mixed decisions)
- Test: empty file → zero-value summary, no error
- Benchmark: ComputeSummary(10000 entries) < 50ms

---

### Story 4: Handler Integration

**User Value**: Classification runs automatically on every incoming hook — users experience fewer interruptions without any configuration.

**Scope**: 1 week

**Acceptance Criteria**:
1. `HandlePermissionRequest` invokes classifier before creating `PendingApproval`
2. Auto-allow returns HTTP 200 with `decision.behavior=allow` in < 5ms (no store write, no UI notification)
3. Auto-deny returns HTTP 200 with `decision.behavior=deny` + `systemMessage` with alternative suggestion
4. Escalated items continue through existing manual review flow unchanged
5. Analytics entry recorded asynchronously for all three outcomes
6. Integration test: seed a read-only tool call → auto-allowed, no notification published

---

#### Task 4.1 — Wire Classifier into ApprovalHandler [2h]

**Objective**: Add classifier invocation as the first step in `HandlePermissionRequest`.

**Context Boundary**:
- Primary: `server/services/approval_handler.go`
- Supporting: `server/services/classifier.go`
- Supporting: `server/services/analytics_store.go`

**Prerequisites**: Stories 1, 2, 3 complete.

**Implementation Approach**:
1. Add `classifier Classifier`, `analyticsStore *AnalyticsStore` fields to `ApprovalHandler`
2. At top of `HandlePermissionRequest` (after parsing payload, before `uuid.New()`):
   ```go
   ctx := h.classifier.BuildContext(payload.Cwd)
   result := h.classifier.Classify(payload, ctx)
   h.analyticsStore.Record(newEntry(payload, result, ...))

   switch result.Decision {
   case AutoAllow:
       h.writeDecision(w, "allow", "")
       return
   case AutoDeny:
       h.writeDecision(w, "deny", result.Reason)
       // Optionally inject systemMessage via a separate field (see ADR-024)
       return
   case Escalate:
       // fall through to existing manual flow
   }
   ```
3. `newEntry()` helper builds `AnalyticsEntry` from payload + result + latency
4. Update `NewApprovalHandler()` to accept classifier and analytics store; nil-safe (if classifier nil → always Escalate)

**Validation Strategy**:
- Integration test: `Read` tool → auto-allowed, no store entry, no event bus publish
- Integration test: `.env` write → auto-denied, decision written, no store entry
- Integration test: `Write` to normal file → escalated, store entry created, event bus notified
- Unit test: nil classifier → Escalate (safe default)

---

#### Task 4.2 — systemMessage Alternative Injection [1h]

**Objective**: When auto-denying, inject a helpful `systemMessage` into the hook response so Claude understands why and can use a better approach.

**Context Boundary**:
- Primary: `server/services/approval_handler.go`
- Supporting: `server/services/classifier.go` (`Alternative` field)

**Prerequisites**: Task 4.1.

**Implementation Approach**:
1. The Claude Code hook response format supports a top-level `systemMessage` field (per SDK docs)
2. Extend `hookDecisionResponse` struct to include `SystemMessage string \`json:"systemMessage,omitempty"\``
3. In auto-deny path: populate `SystemMessage` from `result.Alternative` if non-empty
4. Example: `"Instead of 'find / -name *.go', use the Glob tool with pattern '**/*.go' which is faster and safer"`
5. Add `Alternative` strings to all AutoDeny seed rules

**Validation Strategy**:
- Unit test: auto-deny response JSON includes `systemMessage` field
- Unit test: auto-allow response has no `systemMessage`
- Integration test: `.env` write denied → response contains alternative text

---

#### Task 4.3 — Server Wiring & Startup [1h]

**Objective**: Wire new components into `server.go` startup sequence.

**Context Boundary**:
- Primary: `server/server.go`
- Supporting: `server/services/approval_handler.go`
- Supporting: `server/services/rules_store.go`
- Supporting: `server/services/analytics_store.go`

**Prerequisites**: Tasks 4.1, 2.1, 3.1.

**Implementation Approach**:
1. In server startup: `NewRulesStore(rulesPath)`, `NewAnalyticsStore(analyticsPath)`
2. `NewRuleBasedClassifier(rulesStore, claudeSettingsParser)`
3. Start `rulesStore.WatchAndReload(ctx)` goroutine
4. Start analytics flush goroutine
5. Pass classifier + analyticsStore into `NewApprovalHandler(...)`
6. Log startup: "Auto-approval classifier loaded: N rules (M from claude-settings, K user rules, J seed rules)"

**Validation Strategy**:
- Integration test: server starts, rule counts logged correctly
- Test: rules file missing → server starts with seed rules only (no crash)

---

### Story 5: Proto/API Extensions

**User Value**: Web UI can list, create, update, and delete auto-approval rules and query analytics data via ConnectRPC.

**Scope**: 1 week

**Acceptance Criteria**:
1. `ListApprovalRules` RPC returns all rules (user + seed + claude-settings) with source/enabled fields
2. `UpsertApprovalRule` creates or updates a user rule; seed/claude-settings rules are read-only
3. `DeleteApprovalRule` removes a user rule by ID
4. `GetApprovalAnalytics` returns `AnalyticsSummary` for a configurable time window
5. Generated Go + TypeScript code regenerated via `make proto-gen`

---

#### Task 5.1 — Proto Definitions for Rules & Analytics [2h]

**Objective**: Add new messages and RPCs to `session.proto` and `types.proto`.

**Context Boundary**:
- Primary: `proto/session/v1/session.proto`
- Primary: `proto/session/v1/types.proto`
- Supporting: N/A (pure proto, no Go logic)

**Prerequisites**: Story 1 types finalized (RiskLevel, Decision enums).

**Implementation Approach**:

In `types.proto`, add:
```protobuf
enum RiskLevel {
  RISK_LEVEL_UNSPECIFIED = 0;
  RISK_LEVEL_LOW = 1;
  RISK_LEVEL_MEDIUM = 2;
  RISK_LEVEL_HIGH = 3;
  RISK_LEVEL_CRITICAL = 4;
}

enum AutoDecision {
  AUTO_DECISION_UNSPECIFIED = 0;
  AUTO_DECISION_ALLOW = 1;
  AUTO_DECISION_DENY = 2;
  AUTO_DECISION_ESCALATE = 3;
}

message ApprovalRuleProto {
  string id = 1;
  string tool_name = 2;
  string command_pattern = 3;
  string file_pattern = 4;
  AutoDecision decision = 5;
  RiskLevel risk_level = 6;
  string reason = 7;
  string alternative = 8;
  int32 priority = 9;
  bool enabled = 10;
  string source = 11; // "user", "seed", "claude-settings"
  google.protobuf.Timestamp created_at = 12;
}

message AnalyticsSummaryProto {
  int32 total_decisions = 1;
  map<string, int32> decision_counts = 2;
  repeated ToolStatProto top_tools = 3;
  repeated CommandStatProto top_denied_commands = 4;
  repeated RuleStatProto top_triggered_rules = 5;
  double auto_approve_rate = 6;
  double manual_review_rate = 7;
  google.protobuf.Timestamp window_start = 8;
  google.protobuf.Timestamp window_end = 9;
}

message ToolStatProto { string tool_name = 1; int32 count = 2; }
message CommandStatProto { string preview = 1; int32 count = 2; string tool_name = 3; }
message RuleStatProto { string rule_id = 1; string rule_name = 2; int32 count = 3; }
```

In `session.proto`, add RPCs:
```protobuf
rpc ListApprovalRules(ListApprovalRulesRequest) returns (ListApprovalRulesResponse) {}
rpc UpsertApprovalRule(UpsertApprovalRuleRequest) returns (UpsertApprovalRuleResponse) {}
rpc DeleteApprovalRule(DeleteApprovalRuleRequest) returns (DeleteApprovalRuleResponse) {}
rpc GetApprovalAnalytics(GetApprovalAnalyticsRequest) returns (GetApprovalAnalyticsResponse) {}
```

**Validation Strategy**:
- `make proto-gen` succeeds without errors
- Generated Go types compile with the rest of the server

---

#### Task 5.2 — RulesService & AnalyticsService RPC Handlers [3h]

**Objective**: Implement the four RPC handlers.

**Context Boundary**:
- Primary: `server/services/rules_service.go` (new, ~200 lines)
- Supporting: `server/services/rules_store.go`
- Supporting: `server/services/analytics_store.go`
- Supporting: Generated proto Go types

**Prerequisites**: Tasks 5.1, 2.1, 3.3 complete.

**Implementation Approach**:
1. `ListApprovalRules`: return all rules from `RulesStore.All()` mapped to proto
2. `UpsertApprovalRule`: validate (non-empty tool name, valid regex), call `RulesStore.Upsert()`, return updated rule
3. `DeleteApprovalRule`: validate source="user" (refuse to delete seed/claude-settings), call `RulesStore.Delete()`, return success
4. `GetApprovalAnalytics`: parse `window_days` param (default 7, max 90), call `analyticsStore.LoadWindow()` + `ComputeSummary()`, map to proto
5. Register on `SessionService` (same service, not a new one)

**Validation Strategy**:
- Unit tests for all four handlers
- Test: delete seed rule → `CodePermissionDenied` error
- Test: upsert invalid regex → `CodeInvalidArgument` error
- Test: analytics 30-day window with 1000 fixture entries → correct counts

---

### Story 6: Web UI – Rules & Analytics

**User Value**: Users can see what's being auto-approved/denied, understand why, and tune rules without editing JSON files directly.

**Scope**: 1.5 weeks

**Acceptance Criteria**:
1. New "Auto-Approval" section in settings/sidebar with Rules and Analytics tabs
2. Rules list shows all rules with source badge (User/Seed/Claude), enabled toggle, edit/delete for user rules
3. "Add Rule" form with tool picker, pattern fields, decision selector, and live pattern test
4. Analytics tab shows decision pie chart, top tools bar chart, recent denials table, and auto-approval rate trend
5. All charts update when analytics API is queried
6. Mobile-responsive layout

---

#### Task 6.1 — Rules Management UI [4h]

**Objective**: Build the rules list and add/edit form components.

**Context Boundary**:
- Primary: `web-app/src/components/settings/ApprovalRulesPanel.tsx` (new)
- Primary: `web-app/src/lib/hooks/useApprovalRules.ts` (new)
- Supporting: `web-app/src/gen/` (generated TS types from Task 5.1)

**Prerequisites**: Tasks 5.1, 5.2 complete (API available).

**Implementation Approach**:
1. `useApprovalRules` hook: `listRules()`, `upsertRule()`, `deleteRule()` via ConnectRPC client; optimistic updates
2. `ApprovalRulesPanel` component:
   - Table/list of rules with columns: Tool, Pattern, Decision (colored badge), Risk, Source (pill), Priority, Enabled toggle
   - Seed/claude-settings rules: greyed-out, read-only, no delete button
   - User rules: enabled toggle, edit pencil, delete trash
3. `RuleEditorModal` form: tool name dropdown (Bash, Read, Write, Glob, Grep, WebFetch, or custom), command pattern, file pattern, decision radio (Allow/Deny/Escalate), risk level, reason, alternative
4. Live regex test input: enter a sample command, see if the pattern matches
5. Pattern validation: compile regex client-side, show error inline if invalid

**Validation Strategy**:
- Manual test: add user rule → appears in list with User badge
- Manual test: toggle enabled → rule becomes crossed out
- Manual test: invalid regex → inline error, submit disabled

---

#### Task 6.2 — Analytics Dashboard UI [4h]

**Objective**: Build the analytics visualization components.

**Context Boundary**:
- Primary: `web-app/src/components/settings/ApprovalAnalyticsPanel.tsx` (new)
- Primary: `web-app/src/lib/hooks/useApprovalAnalytics.ts` (new)
- Supporting: `web-app/src/gen/` (generated TS types)

**Prerequisites**: Task 5.2 complete.

**Implementation Approach**:
1. `useApprovalAnalytics` hook: `getAnalytics(windowDays: 7|30|90)`, refetch on window change
2. Summary cards row: Total Decisions, Auto-Approve Rate (%), Manual Review Rate (%), Auto-Deny Count
3. Decision distribution: simple donut/pie chart (recharts or CSS-based) with auto_allow/auto_deny/escalate/manual_allow/manual_deny segments
4. Top Tools table: tool name, decision counts, sparkline
5. Recent Denials table: timestamp, tool, command preview, rule triggered, alternative shown
6. Window selector: 7d / 30d / 90d tabs
7. Empty state: "No decisions recorded yet. Start a session to see analytics."

**Note**: If recharts is not already in package.json, use simple CSS-based visualization (percentage bars) to avoid adding a new dependency.

**Validation Strategy**:
- Manual test: switch window from 7d to 30d → data refreshes
- Manual test: empty analytics → shows empty state, no crash
- Lighthouse score: no regression on performance

---

#### Task 6.3 — Navigation Integration [1h]

**Objective**: Add "Auto-Approval" entry to sidebar navigation and integrate panels.

**Context Boundary**:
- Primary: `web-app/src/components/layout/Sidebar.tsx` (or equivalent nav component)
- Primary: `web-app/src/app/` (routing — add auto-approval page or tab)
- Supporting: `web-app/src/components/settings/ApprovalRulesPanel.tsx`
- Supporting: `web-app/src/components/settings/ApprovalAnalyticsPanel.tsx`

**Prerequisites**: Tasks 6.1, 6.2 complete.

**Implementation Approach**:
1. Add "Auto-Approval" link with shield icon to sidebar nav, below existing "Approvals"
2. Route to new page: `/auto-approval` with Rules and Analytics tabs
3. Or: add as tabs within an existing Settings page if that pattern already exists
4. Show rule count badge on nav item (from `useApprovalRules`)

**Validation Strategy**:
- Manual test: navigate to auto-approval page → loads without error
- Manual test: rules count badge updates after adding a rule

---

## Known Issues & Bug Risks

### Bug 001: 🐛 Race Condition: Classifier Reads Stale Rules [SEVERITY: Medium]

**Description**: `WatchAndReload` goroutine swaps `rules` slice under write lock, but `Classify()` reads under read lock. If the rules slice is swapped mid-classification (theoretically mid-sort), results could be inconsistent.

**Mitigation**:
- Use `sync.RWMutex` consistently; never store a pointer to the slice outside the lock
- Classify() acquires RLock for full duration; WatchAndReload acquires full Lock

**Files Affected**: `server/services/classifier.go`, `server/services/rules_store.go`

**Prevention**: Write race detector test (`go test -race`); must pass in CI.

---

### Bug 002: 🐛 Analytics JSONL Corruption on Server Crash [SEVERITY: Low]

**Description**: If the server crashes between a `write()` call and the newline terminator, the last JSONL line may be malformed.

**Mitigation**:
- Write each entry as a complete line atomically via `fmt.Fprintf(f, "%s\n", jsonBytes)`
- On `LoadWindow()`, use `json.Decoder` per-line with error skip for malformed lines (log warning)

**Files Affected**: `server/services/analytics_store.go`

---

### Bug 003: 🐛 False Positive AutoDeny: User Intentionally Writes to .env.example [SEVERITY: Medium]

**Description**: Seed rule denies any write matching `\.env` — but `.env.example` and `.env.template` are safe to modify.

**Mitigation**:
- Tighten pattern to `(^|/)\.env$` (exact filename match, not substring)
- Add explicit AutoAllow rule for `\.env\.(example|template|sample)$`

**Files Affected**: `server/services/classifier.go` (seed rules)

**Prevention**: Unit test for `.env.example`, `.env`, `.env.local` paths.

---

### Bug 004: 🐛 Claude Settings Parser Breaks on Non-Standard JSON [SEVERITY: Low]

**Description**: Claude's settings files may contain comments (JSON5-style) or trailing commas in some versions.

**Mitigation**:
- Use `encoding/json` with `json.Decoder` which rejects non-standard JSON — fail gracefully
- Log warning and skip claude-settings rules rather than crashing
- Test with both strict and slightly malformed files

**Files Affected**: `server/services/claude_settings_parser.go`

---

### Bug 005: 🐛 Priority Inversion: User Rules Can't Override Seed Rules [SEVERITY: Medium]

**Description**: If seed rules have fixed priorities (e.g., AutoDeny `.env` = priority 100), a user cannot add an AutoAllow for `.env.example` at a lower priority.

**Mitigation**:
- Reserve priorities 1-99 for user rules, 100-199 for seed rules, 200+ for claude-settings (highest)
- Document priority ranges clearly
- User rules at priority 50 always checked before seed rules at priority 100

**Files Affected**: `server/services/classifier.go`

---

### Bug 007: 🐛 UI Approval Buttons Not Resolving Requests [SEVERITY: HIGH — Confirmed]

**Description**: Clicking Approve/Deny in the web UI does not visibly resolve the pending approval. The `ResolveApproval` RPC may succeed but the HTTP hook handler is not receiving the decision, or the UI is not correctly correlating approval IDs from notifications to the `useApprovals` hook.

**Likely Root Causes** (to investigate):
1. `approval_id` extracted from notification metadata may not match the ID passed to `ResolveApproval`
2. The `decisionCh` channel may have been garbage-collected or the `PendingApproval` removed from the store before the RPC arrives
3. The `useApprovals` hook may be calling `listPendingApprovals` and `resolveApproval` on different endpoints or with mismatched IDs
4. Race: approval expires (4-minute timeout cleanup) removing it from store before user clicks

**Mitigation**:
- Add server-side logging in `ResolveApproval`: log whether `approvalStore.Resolve()` finds the approval ID
- Add client-side logging in `useApprovals.ts`: log the exact approval ID being sent
- Check that `ApprovalNavBadge` and `ApprovalPanel` both use the same `useApprovals` hook instance (not two separate pollers)

**Files Affected**:
- `server/services/approval_service.go` — `ResolveApproval()` handler
- `server/services/approval_store.go` — `Resolve()` channel send
- `web-app/src/lib/hooks/useApprovals.ts` — `resolveApproval()` call
- `web-app/src/lib/hooks/useSessionNotifications.ts` — `approval_id` extraction

**Integration Test Required** (Task 4.X — add to Story 4 before shipping):
Write a Go integration test that:
1. Creates a real HTTP handler (`httptest.NewServer`)
2. Sends a mock `PermissionRequest` hook payload (POST to `/api/hooks/permission-request`)
3. Verifies the request blocks (goroutine waiting on channel)
4. Calls `ResolveApproval` RPC with the approval ID received from the notification
5. Verifies the blocked HTTP handler returns with `behavior=allow` within 1 second
6. Verifies the approval is removed from the store

This test must pass before the auto-approval feature ships; it validates the existing flow that new classification logic sits in front of.

**Prevention**: Require this integration test to be green in CI before any approval-related PRs merge.

---

### Bug 006: 🐛 Analytics Buffer Full Under Load [SEVERITY: Low]

**Description**: Channel buffer of 1000 may overflow if many sessions generate rapid hook calls.

**Mitigation**:
- Log dropped entry count as a metric
- Make buffer size configurable (`AnalyticsBufferSize` in config)
- Default 1000 is safe for typical < 10 sessions; document limit

**Files Affected**: `server/services/analytics_store.go`

---

## Dependency Visualization

```
Story 1: Risk Classifier Engine
├── Task 1.1: Interface & Types ───────────────────────────────────┐
│   └── Task 1.2: Seed Rules ────────────────────────────────────── │ ─┐
│       └── Task 1.3: Context Awareness ──────────────────────────  │  │
│                                                                    │  │
Story 2: Rules Engine & Config Integration                          │  │
├── Task 2.1: Rules File Format ─────────── (needs 1.1) ───────────┤  │
│   └── Task 2.3: File Watcher ──────────── (needs 2.1)            │  │
└── Task 2.2: Claude Settings Parser ────── (needs 1.1) ───────────┤  │
                                                                    │  │
Story 3: Analytics Store                                            │  │
├── Task 3.1: Schema & Writer ───────────── (needs 1.1) ───────────┤  │
├── Task 3.2: Manual Decision Recording ─── (needs 3.1)            │  │
└── Task 3.3: Query Engine ──────────────── (needs 3.1)            │  │
                                                                    │  │
Story 4: Handler Integration ────────────── (needs 1,2,3) ─────────┘  │
├── Task 4.1: Wire Classifier ──────────────────────────────────────   │
├── Task 4.2: systemMessage Injection ─────── (needs 4.1)             │
└── Task 4.3: Server Wiring ──────────────── (needs 4.1, 2.1, 3.1)   │
                                                                       │
Story 5: Proto/API Extensions ────────────── (needs 1) ────────────────┘
├── Task 5.1: Proto Definitions ───────────────────────────────────────
└── Task 5.2: RPC Handlers ──────────────── (needs 5.1, 2.1, 3.3)

Story 6: Web UI ─────────────────────────── (needs 5.1, 5.2)
├── Task 6.1: Rules Management UI
├── Task 6.2: Analytics Dashboard UI
└── Task 6.3: Navigation Integration ──────── (needs 6.1, 6.2)

Parallel execution opportunities:
- Stories 1, 2, 3 can be developed in parallel once Task 1.1 is done
- Story 5 (proto) can be drafted in parallel with Stories 2/3
- Story 6 (UI) can start with mock data while Story 5 handlers are in progress
```

---

## Integration Checkpoints

**After Story 1** (Risk Classifier Engine):
- `go test ./server/services/... -run TestClassifier` passes with 100% of seed rules tested
- Benchmark shows < 1ms classification for 100 rules

**After Story 2** (Rules Engine):
- Rules load from disk correctly; watch-and-reload works end-to-end
- Claude settings parser correctly extracts allow list from a real `~/.claude/settings.json`

**After Story 3** (Analytics):
- Analytics entries appear in `~/.stapler-squad/approval_analytics.jsonl` after a test hook call
- Manual resolution via existing web UI is recorded with correct `approval_id` correlation

**After Story 4** (Handler Integration):
- Full integration test: send a `Read` hook payload → HTTP 200 with `allow` in < 5ms, no UI notification
- Full integration test: send `.env` Write payload → HTTP 200 with `deny` + systemMessage
- Existing manual approval flow for `Write` to normal file unchanged

**After Story 5** (API Extensions):
- `make proto-gen` succeeds, generated Go + TypeScript code compiles
- RPCs return correct data for all four endpoints

**Final** (Story 6 complete):
- All acceptance criteria met for all 6 stories
- Manual E2E test: start session, trigger Read → auto-approved, check analytics page shows entry
- Manual E2E test: add user rule, trigger matching command → respects user rule
- `go test -race ./...` passes (no race conditions)
- Lighthouse performance score ≥ existing baseline

---

## Context Preparation Guide

### For Task 1.1 (Classifier Interface)
**Files to load**:
- `server/services/approval_store.go` — `PermissionRequestPayload` struct (the input to classify)
- `server/services/approval_handler.go` — `HandlePermissionRequest()` to understand where classification fits

**Concepts**: Go interfaces, enum-style const blocks, value types vs pointer types in structs.

---

### For Task 2.2 (Claude Settings Parser)
**Files to load**:
- `config/claude.go` — existing `ClaudeConfigManager` and `GetConfig()`
- `server/services/approval_handler.go` — `InjectHookConfig()` to understand how settings files are already written

**Concepts**: Claude Code's `settings.json` format — specifically the `permissions.allow` array which contains glob patterns matching tool names + arguments.

---

### For Task 4.1 (Wire Classifier)
**Files to load**:
- `server/services/approval_handler.go` — full file (this is the primary modification target)
- `server/services/classifier.go` — interface + result types
- `server/services/analytics_store.go` — `Record()` method signature

**Concepts**: The HTTP hook flow — parse payload → classify → either return immediately (auto) OR create PendingApproval → broadcast → block → write decision.

---

### For Task 5.1 (Proto Definitions)
**Files to load**:
- `proto/session/v1/types.proto` — existing message conventions
- `proto/session/v1/session.proto` — existing RPC patterns (request/response pairs)
- `proto/session/v1/events.proto` — for reference on event types

**Concepts**: proto3 field numbering (must not reuse), `optional` fields, `map<string, int32>` for aggregates.

---

### For Task 6.1 (Rules UI)
**Files to load**:
- `web-app/src/components/sessions/ApprovalCard.tsx` — existing approval component pattern
- `web-app/src/lib/hooks/useApprovals.ts` — hook pattern for ConnectRPC calls
- `web-app/src/gen/` — generated TypeScript types (after Task 5.1 + proto-gen)

**Concepts**: React hooks with optimistic updates, ConnectRPC TypeScript client usage patterns.

---

## Success Criteria

- [ ] All 6 stories complete with acceptance criteria met
- [ ] `go test -race ./...` passes (no race conditions in classifier, rules store, analytics)
- [ ] `go test -cover ./server/services/...` reports ≥ 80% coverage for new files
- [ ] Classifier benchmarks: classification < 1ms P99
- [ ] Analytics query benchmarks: 30-day summary < 500ms for 50K entries
- [ ] Full integration test: Read/Glob/Grep auto-approved, .env Write auto-denied, Write auto-escalated
- [ ] `make proto-gen` succeeds; all generated code compiles
- [ ] Web UI: rules panel renders without error; analytics panel renders without error
- [ ] Lighthouse performance score: no regression from baseline
- [ ] Documentation: this file linked from `docs/tasks/TODO.md`
