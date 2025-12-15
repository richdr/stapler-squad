# SQLite Schema Normalization Feature Plan

## Executive Summary

This document outlines a comprehensive plan to normalize the SQLite schema for claude-squad, extracting optional metadata into separate tables to eliminate NULL handling issues and improve database design. The current schema has nullable TEXT columns for GitHub integration, worktree detection, and other optional features that cause scanning errors when reading into Go string types.

## Problem Statement

### Current Issues
1. **Nullable Column Scanning Errors**: SQLite nullable TEXT columns cause errors when scanned into Go string types
2. **Temporary COALESCE Workaround**: Current queries use `COALESCE(column, '')` as a band-aid solution
3. **Schema Denormalization**: Optional metadata mixed with core session data violates normalization principles
4. **Performance Impact**: Large sessions table with many nullable columns affects query performance
5. **Maintenance Burden**: Adding new optional features requires modifying the core sessions table

### Affected Columns
The following nullable columns in the sessions table need extraction:
- **GitHub Integration**: github_pr_number, github_pr_url, github_owner, github_repo, github_source_ref, cloned_repo_path
- **Worktree Detection**: main_repo_path, is_worktree
- **Optional Paths**: working_dir, existing_worktree
- **Optional Metadata**: prompt, category, session_type, tmux_prefix, last_output_signature
- **Timestamp Fields**: last_terminal_update, last_meaningful_output, last_added_to_queue, last_viewed, last_acknowledged

## Requirements

### Functional Requirements
1. **FR1**: Extract GitHub-related columns into a separate `github_sessions` table
2. **FR2**: Extract worktree detection fields into the existing `worktrees` table or new table
3. **FR3**: Extract optional metadata into appropriate normalized tables
4. **FR4**: Maintain backward compatibility with existing data
5. **FR5**: Support atomic migrations with rollback capability
6. **FR6**: Preserve all existing functionality without breaking changes

### Non-Functional Requirements
1. **NFR1**: Query performance must not degrade (use appropriate indexes)
2. **NFR2**: Migration must complete within 30 seconds for 10,000 sessions
3. **NFR3**: Zero data loss during migration
4. **NFR4**: Support concurrent reads during migration
5. **NFR5**: Maintain referential integrity with foreign keys

## Architecture & Design

### Normalized Schema Design

#### Core Tables (Modified)

**sessions** (core data only)
```sql
CREATE TABLE sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT UNIQUE NOT NULL,
    path TEXT NOT NULL,
    branch TEXT NOT NULL DEFAULT '',
    status INTEGER NOT NULL,
    height INTEGER NOT NULL DEFAULT 24,
    width INTEGER NOT NULL DEFAULT 80,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    auto_yes INTEGER NOT NULL DEFAULT 0,
    program TEXT NOT NULL,
    is_expanded INTEGER NOT NULL DEFAULT 1
);
```

#### New Tables for Optional Data

**github_sessions** (GitHub PR integration)
```sql
CREATE TABLE github_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL UNIQUE,
    pr_number INTEGER,
    pr_url TEXT,
    owner TEXT NOT NULL,
    repo TEXT NOT NULL,
    source_ref TEXT,
    cloned_repo_path TEXT,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);
```

**session_metadata** (optional configuration)
```sql
CREATE TABLE session_metadata (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL UNIQUE,
    working_dir TEXT,
    existing_worktree TEXT,
    prompt TEXT,
    category TEXT,
    session_type TEXT,
    tmux_prefix TEXT,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);
```

**session_timestamps** (activity tracking)
```sql
CREATE TABLE session_timestamps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL UNIQUE,
    last_terminal_update DATETIME,
    last_meaningful_output DATETIME,
    last_output_signature TEXT,
    last_added_to_queue DATETIME,
    last_viewed DATETIME, 
    last_acknowledged DATETIME,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);
```

**worktree_detection** (git worktree info)
```sql
CREATE TABLE worktree_detection (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL UNIQUE,
    main_repo_path TEXT NOT NULL,
    is_worktree INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);
```

### Migration Strategy

#### Phase 1: Schema Version 3 - Add New Tables
1. Create new normalized tables without dropping old columns
2. Add indexes for foreign keys and common queries
3. Implement dual-write mode in repository layer

#### Phase 2: Data Migration
1. Copy existing data from sessions table to new tables
2. Verify data integrity with checksums
3. Run in transaction with savepoint for rollback

#### Phase 3: Schema Version 4 - Drop Old Columns
1. After verification period, drop deprecated columns
2. Update all queries to use JOINs
3. Remove dual-write code

### Query Patterns

#### Loading a Complete Session
```sql
SELECT 
    s.*,
    g.pr_number, g.pr_url, g.owner, g.repo, g.source_ref, g.cloned_repo_path,
    m.working_dir, m.existing_worktree, m.prompt, m.category, m.session_type, m.tmux_prefix,
    t.last_terminal_update, t.last_meaningful_output, t.last_output_signature,
    t.last_added_to_queue, t.last_viewed, t.last_acknowledged,
    w.main_repo_path, w.is_worktree
FROM sessions s
LEFT JOIN github_sessions g ON s.id = g.session_id
LEFT JOIN session_metadata m ON s.id = m.session_id
LEFT JOIN session_timestamps t ON s.id = t.session_id
LEFT JOIN worktree_detection w ON s.id = w.session_id
WHERE s.title = ?
```

#### Optimized List Query (minimal joins)
```sql
SELECT s.*, m.category
FROM sessions s
LEFT JOIN session_metadata m ON s.id = m.session_id
ORDER BY s.created_at DESC
```

## Implementation Plan

### Phase 1: Preparation (Week 1)

#### Task 1.1: Create Migration Infrastructure
- [ ] Add migration version tracking
- [ ] Implement rollback mechanism
- [ ] Create migration test framework
- [ ] Add migration benchmarks

#### Task 1.2: Design Repository Abstraction
- [ ] Create `RepositoryV3` interface with new methods
- [ ] Implement query builders for JOIN operations
- [ ] Add connection pooling configuration
- [ ] Design batch insert/update methods

### Phase 2: Schema Implementation (Week 2)

#### Task 2.1: Create New Tables
- [ ] Write CREATE TABLE statements for all new tables
- [ ] Add appropriate indexes
- [ ] Configure foreign key constraints
- [ ] Test referential integrity

#### Task 2.2: Implement Dual-Write Mode
- [ ] Modify Create() to write to both old and new schema
- [ ] Modify Update() to write to both schemas
- [ ] Add feature flag for dual-write mode
- [ ] Implement consistency checking

### Phase 3: Data Migration (Week 3)

#### Task 3.1: Migration Script
- [ ] Write migration function with transaction support
- [ ] Implement batch processing for large datasets
- [ ] Add progress logging
- [ ] Create rollback savepoints

#### Task 3.2: Data Verification
- [ ] Implement checksum verification
- [ ] Create data comparison tool
- [ ] Add migration dry-run mode
- [ ] Write migration report generator

### Phase 4: Query Updates (Week 4)

#### Task 4.1: Update Repository Methods
- [ ] Rewrite GetWithOptions() to use JOINs
- [ ] Update ListWithOptions() with selective JOINs
- [ ] Optimize queries with EXPLAIN QUERY PLAN
- [ ] Add query performance metrics

#### Task 4.2: Testing & Validation
- [ ] Write comprehensive integration tests
- [ ] Perform load testing with 10K+ sessions
- [ ] Validate backward compatibility
- [ ] Test concurrent access patterns

### Phase 5: Cleanup (Week 5)

#### Task 5.1: Remove Old Schema
- [ ] Drop deprecated columns from sessions table
- [ ] Remove COALESCE workarounds
- [ ] Clean up dual-write code
- [ ] Update documentation

#### Task 5.2: Performance Optimization
- [ ] Analyze query patterns with production data
- [ ] Add missing indexes based on usage
- [ ] Optimize connection pool settings
- [ ] Implement query result caching

## Known Issues & Mitigation

### Issue 1: Migration Performance
**Risk**: Large databases may experience slow migration
**Mitigation**: 
- Batch processing with configurable batch size
- Progress indicators for user feedback
- Option to run migration in background
- Support for resumable migrations

### Issue 2: Concurrent Access During Migration
**Risk**: Active sessions may fail during schema changes
**Mitigation**:
- Use SQLite WAL mode for concurrent reads
- Implement retry logic with exponential backoff
- Dual-write mode ensures data availability
- Graceful degradation to read-only mode

### Issue 3: Rollback Complexity
**Risk**: Partial migration may leave inconsistent state
**Mitigation**:
- Transaction-based migration with savepoints
- Pre-migration backup creation
- Verification checksums at each step
- Automated rollback on failure

### Issue 4: Query Performance Regression
**Risk**: JOINs may be slower than denormalized queries
**Mitigation**:
- Selective JOINs based on LoadOptions
- Materialized views for common queries
- Query result caching layer
- Connection pooling optimization

## Testing Strategy

### Unit Tests
- [ ] Test each migration step in isolation
- [ ] Verify data integrity constraints
- [ ] Test rollback scenarios
- [ ] Validate NULL handling

### Integration Tests
- [ ] End-to-end migration testing
- [ ] Concurrent access testing
- [ ] Performance regression testing
- [ ] Backward compatibility testing

### Load Tests
- [ ] Migration with 10K sessions
- [ ] Query performance with normalized schema
- [ ] Concurrent read/write operations
- [ ] Memory usage profiling

### Chaos Testing
- [ ] Interrupt migration at various points
- [ ] Simulate disk space issues
- [ ] Test with corrupted data
- [ ] Network interruption scenarios

## Success Metrics

1. **Migration Success Rate**: 100% successful migrations without data loss
2. **Query Performance**: No more than 10% degradation in query latency
3. **Migration Duration**: Complete within 30 seconds for 10K sessions
4. **Zero Downtime**: Application remains available during migration
5. **Code Quality**: Eliminate all COALESCE workarounds

## Rollback Plan

### Immediate Rollback (During Migration)
1. Transaction rollback to savepoint
2. Restore from pre-migration backup
3. Revert to previous schema version
4. Clear migration status flags

### Post-Migration Rollback
1. Re-create dropped columns
2. Copy data back from normalized tables
3. Update repository to use old schema
4. Deprecate new tables (keep for reference)

## Documentation Updates

### Developer Documentation
- [ ] Update schema documentation
- [ ] Document new repository methods
- [ ] Add migration guide
- [ ] Update contribution guidelines

### Operational Documentation
- [ ] Migration runbook
- [ ] Troubleshooting guide
- [ ] Performance tuning guide
- [ ] Backup/restore procedures

## Appendix A: Migration SQL Scripts

### Create New Tables
```sql
-- Schema Version 3: Add normalized tables
BEGIN TRANSACTION;

-- GitHub integration table
CREATE TABLE IF NOT EXISTS github_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL UNIQUE,
    pr_number INTEGER,
    pr_url TEXT,
    owner TEXT NOT NULL,
    repo TEXT NOT NULL,
    source_ref TEXT,
    cloned_repo_path TEXT,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE INDEX idx_github_sessions_session_id ON github_sessions(session_id);
CREATE INDEX idx_github_sessions_owner_repo ON github_sessions(owner, repo);

-- Session metadata table
CREATE TABLE IF NOT EXISTS session_metadata (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL UNIQUE,
    working_dir TEXT,
    existing_worktree TEXT,
    prompt TEXT,
    category TEXT,
    session_type TEXT,
    tmux_prefix TEXT,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE INDEX idx_session_metadata_session_id ON session_metadata(session_id);
CREATE INDEX idx_session_metadata_category ON session_metadata(category);

-- Timestamps table
CREATE TABLE IF NOT EXISTS session_timestamps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL UNIQUE,
    last_terminal_update DATETIME,
    last_meaningful_output DATETIME,
    last_output_signature TEXT,
    last_added_to_queue DATETIME,
    last_viewed DATETIME,
    last_acknowledged DATETIME,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE INDEX idx_session_timestamps_session_id ON session_timestamps(session_id);
CREATE INDEX idx_session_timestamps_meaningful_output ON session_timestamps(last_meaningful_output);

-- Worktree detection table
CREATE TABLE IF NOT EXISTS worktree_detection (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL UNIQUE,
    main_repo_path TEXT NOT NULL,
    is_worktree INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE INDEX idx_worktree_detection_session_id ON worktree_detection(session_id);

-- Update schema version
INSERT INTO schema_version (version, applied_at) VALUES (3, datetime('now'));

COMMIT;
```

### Migrate Data
```sql
-- Migrate existing data to new tables
BEGIN TRANSACTION;

-- Migrate GitHub data
INSERT INTO github_sessions (session_id, pr_number, pr_url, owner, repo, source_ref, cloned_repo_path)
SELECT id, github_pr_number, github_pr_url, 
       COALESCE(github_owner, ''), COALESCE(github_repo, ''),
       github_source_ref, cloned_repo_path
FROM sessions
WHERE github_owner IS NOT NULL OR github_repo IS NOT NULL;

-- Migrate metadata
INSERT INTO session_metadata (session_id, working_dir, existing_worktree, prompt, category, session_type, tmux_prefix)
SELECT id, working_dir, existing_worktree, prompt, category, session_type, tmux_prefix
FROM sessions
WHERE working_dir IS NOT NULL 
   OR existing_worktree IS NOT NULL 
   OR prompt IS NOT NULL 
   OR category IS NOT NULL 
   OR session_type IS NOT NULL 
   OR tmux_prefix IS NOT NULL;

-- Migrate timestamps
INSERT INTO session_timestamps (session_id, last_terminal_update, last_meaningful_output, 
                                last_output_signature, last_added_to_queue, last_viewed, last_acknowledged)
SELECT id, last_terminal_update, last_meaningful_output, last_output_signature,
       last_added_to_queue, last_viewed, last_acknowledged
FROM sessions
WHERE last_terminal_update IS NOT NULL 
   OR last_meaningful_output IS NOT NULL 
   OR last_output_signature IS NOT NULL
   OR last_added_to_queue IS NOT NULL
   OR last_viewed IS NOT NULL
   OR last_acknowledged IS NOT NULL;

-- Migrate worktree detection
INSERT INTO worktree_detection (session_id, main_repo_path, is_worktree)
SELECT id, COALESCE(main_repo_path, ''), is_worktree
FROM sessions
WHERE main_repo_path IS NOT NULL OR is_worktree = 1;

COMMIT;
```

## Appendix B: Performance Benchmarks

### Current Schema (Baseline)
- List all sessions: ~50ms for 1000 sessions
- Get single session: ~5ms
- Update timestamps: ~10ms

### Expected Performance (Normalized)
- List all sessions (no joins): ~45ms for 1000 sessions
- List with metadata (1 join): ~55ms for 1000 sessions  
- Get single session (4 joins): ~8ms
- Update timestamps: ~8ms (single table update)

## Appendix C: Risk Matrix

| Risk | Probability | Impact | Mitigation |
|------|------------|--------|------------|
| Data loss during migration | Low | Critical | Transactions, backups, verification |
| Performance degradation | Medium | High | Selective JOINs, caching, indexes |
| Migration failure | Low | High | Rollback mechanism, dry-run mode |
| Concurrent access issues | Medium | Medium | WAL mode, retry logic |
| Schema version conflicts | Low | Medium | Version tracking, compatibility checks |

## References

- [SQLite Schema Design Best Practices](https://www.sqlite.org/lang.html)
- [Database Normalization Forms](https://en.wikipedia.org/wiki/Database_normalization)
- [Go database/sql Package](https://golang.org/pkg/database/sql/)
- [SQLite Performance Tuning](https://www.sqlite.org/pragma.html)
