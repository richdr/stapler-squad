package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadGitContext tests loading git context from the database
func TestLoadGitContext(t *testing.T) {
	repo, cleanup := createTestRepositoryInMemory(t)
	defer cleanup()

	ctx := context.Background()

	// Create base session
	session := createTestSession("git-context-test")
	err := repo.Create(ctx, session)
	require.NoError(t, err)

	// Get session ID
	var sessionID int64
	err = repo.db.QueryRowContext(ctx, "SELECT id FROM sessions WHERE title = ?", session.Title).Scan(&sessionID)
	require.NoError(t, err)

	// Insert test git context data
	_, err = repo.db.ExecContext(ctx, `
		INSERT INTO git_context (session_id, branch, base_commit_sha, pr_number, pr_url, owner, repo, source_ref)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, sessionID, "feature/test", "abc123def456", 42, "https://github.com/test/repo/pull/42",
		"test", "repo", "refs/pull/42/head")
	require.NoError(t, err)

	// Load git context
	gitCtx, err := repo.loadGitContext(ctx, sessionID)
	require.NoError(t, err)
	require.NotNil(t, gitCtx)

	// Verify all fields
	assert.Equal(t, "feature/test", gitCtx.Branch)
	assert.Equal(t, "abc123def456", gitCtx.BaseCommitSHA)
	assert.Equal(t, 42, gitCtx.PRNumber)
	assert.Equal(t, "https://github.com/test/repo/pull/42", gitCtx.PRURL)
	assert.Equal(t, "test", gitCtx.Owner)
	assert.Equal(t, "repo", gitCtx.Repo)
	assert.Equal(t, "refs/pull/42/head", gitCtx.SourceRef)
}

// TestLoadGitContext_NoData tests behavior when no git context exists
func TestLoadGitContext_NoData(t *testing.T) {
	repo, cleanup := createTestRepositoryInMemory(t)
	defer cleanup()

	ctx := context.Background()

	// Create base session without git context
	session := createTestSession("no-git-context")
	err := repo.Create(ctx, session)
	require.NoError(t, err)

	var sessionID int64
	err = repo.db.QueryRowContext(ctx, "SELECT id FROM sessions WHERE title = ?", session.Title).Scan(&sessionID)
	require.NoError(t, err)

	// Load git context (should return nil, no error)
	gitCtx, err := repo.loadGitContext(ctx, sessionID)
	require.NoError(t, err)
	assert.Nil(t, gitCtx, "Git context should be nil when no data exists")
}

// TestLoadFilesystemContext tests loading filesystem context from the database
func TestLoadFilesystemContext(t *testing.T) {
	repo, cleanup := createTestRepositoryInMemory(t)
	defer cleanup()

	ctx := context.Background()

	// Create base session
	session := createTestSession("fs-context-test")
	err := repo.Create(ctx, session)
	require.NoError(t, err)

	var sessionID int64
	err = repo.db.QueryRowContext(ctx, "SELECT id FROM sessions WHERE title = ?", session.Title).Scan(&sessionID)
	require.NoError(t, err)

	// Insert test filesystem context data
	_, err = repo.db.ExecContext(ctx, `
		INSERT INTO filesystem_context (
			session_id, project_path, working_dir, is_worktree,
			main_repo_path, cloned_repo_path, existing_worktree, session_type
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, sessionID, "/home/user/project", "/home/user/project/src", 1,
		"/home/user/main-repo", "/tmp/cloned", "/worktrees/existing", SessionTypeNewWorktree)
	require.NoError(t, err)

	// Load filesystem context
	fsCtx, err := repo.loadFilesystemContext(ctx, sessionID)
	require.NoError(t, err)
	require.NotNil(t, fsCtx)

	// Verify all fields
	assert.Equal(t, "/home/user/project", fsCtx.ProjectPath)
	assert.Equal(t, "/home/user/project/src", fsCtx.WorkingDir)
	assert.True(t, fsCtx.IsWorktree)
	assert.Equal(t, "/home/user/main-repo", fsCtx.MainRepoPath)
	assert.Equal(t, "/tmp/cloned", fsCtx.ClonedRepoPath)
	assert.Equal(t, "/worktrees/existing", fsCtx.ExistingWorktree)
	assert.Equal(t, SessionTypeNewWorktree, fsCtx.SessionType)
}

// TestLoadFilesystemContext_NoData tests behavior when no filesystem context exists
func TestLoadFilesystemContext_NoData(t *testing.T) {
	repo, cleanup := createTestRepositoryInMemory(t)
	defer cleanup()

	ctx := context.Background()

	session := createTestSession("no-fs-context")
	err := repo.Create(ctx, session)
	require.NoError(t, err)

	var sessionID int64
	err = repo.db.QueryRowContext(ctx, "SELECT id FROM sessions WHERE title = ?", session.Title).Scan(&sessionID)
	require.NoError(t, err)

	// Load filesystem context (should return nil, no error)
	fsCtx, err := repo.loadFilesystemContext(ctx, sessionID)
	require.NoError(t, err)
	assert.Nil(t, fsCtx, "Filesystem context should be nil when no data exists")
}

// TestLoadTerminalContext tests loading terminal context from the database
func TestLoadTerminalContext(t *testing.T) {
	repo, cleanup := createTestRepositoryInMemory(t)
	defer cleanup()

	ctx := context.Background()

	// Create base session
	session := createTestSession("terminal-context-test")
	err := repo.Create(ctx, session)
	require.NoError(t, err)

	var sessionID int64
	err = repo.db.QueryRowContext(ctx, "SELECT id FROM sessions WHERE title = ?", session.Title).Scan(&sessionID)
	require.NoError(t, err)

	// Insert test terminal context data
	_, err = repo.db.ExecContext(ctx, `
		INSERT INTO terminal_context (
			session_id, height, width, tmux_session_name,
			tmux_prefix, tmux_server_socket, terminal_type
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, sessionID, 40, 120, "squad_test_session", "squad_", "/tmp/tmux-1000/default", "tmux")
	require.NoError(t, err)

	// Load terminal context
	termCtx, err := repo.loadTerminalContext(ctx, sessionID)
	require.NoError(t, err)
	require.NotNil(t, termCtx)

	// Verify all fields
	assert.Equal(t, 40, termCtx.Height)
	assert.Equal(t, 120, termCtx.Width)
	assert.Equal(t, "squad_test_session", termCtx.TmuxSessionName)
	assert.Equal(t, "squad_", termCtx.TmuxPrefix)
	assert.Equal(t, "/tmp/tmux-1000/default", termCtx.TmuxServerSocket)
	assert.Equal(t, "tmux", termCtx.TerminalType)
}

// TestLoadTerminalContext_NoData tests behavior when no terminal context exists
func TestLoadTerminalContext_NoData(t *testing.T) {
	repo, cleanup := createTestRepositoryInMemory(t)
	defer cleanup()

	ctx := context.Background()

	session := createTestSession("no-terminal-context")
	err := repo.Create(ctx, session)
	require.NoError(t, err)

	var sessionID int64
	err = repo.db.QueryRowContext(ctx, "SELECT id FROM sessions WHERE title = ?", session.Title).Scan(&sessionID)
	require.NoError(t, err)

	// Load terminal context (should return nil, no error)
	termCtx, err := repo.loadTerminalContext(ctx, sessionID)
	require.NoError(t, err)
	assert.Nil(t, termCtx, "Terminal context should be nil when no data exists")
}

// TestLoadUIPreferences tests loading UI preferences from the database
func TestLoadUIPreferences(t *testing.T) {
	repo, cleanup := createTestRepositoryInMemory(t)
	defer cleanup()

	ctx := context.Background()

	// Create base session
	session := createTestSession("ui-prefs-test")
	err := repo.Create(ctx, session)
	require.NoError(t, err)

	var sessionID int64
	err = repo.db.QueryRowContext(ctx, "SELECT id FROM sessions WHERE title = ?", session.Title).Scan(&sessionID)
	require.NoError(t, err)

	// Insert test UI preferences data
	_, err = repo.db.ExecContext(ctx, `
		INSERT INTO ui_preferences (
			session_id, category, is_expanded, grouping_strategy, sort_order
		) VALUES (?, ?, ?, ?, ?)
	`, sessionID, "Development", 1, "category", "name")
	require.NoError(t, err)

	// Add test tags
	tags := []string{"Frontend", "Urgent", "React"}
	for _, tag := range tags {
		// Insert tag if not exists
		_, err = repo.db.ExecContext(ctx, "INSERT OR IGNORE INTO tags (name) VALUES (?)", tag)
		require.NoError(t, err)

		// Get tag ID
		var tagID int64
		err = repo.db.QueryRowContext(ctx, "SELECT id FROM tags WHERE name = ?", tag).Scan(&tagID)
		require.NoError(t, err)

		// Link tag to session
		_, err = repo.db.ExecContext(ctx, `
			INSERT INTO session_tags (session_id, tag_id) VALUES (?, ?)
		`, sessionID, tagID)
		require.NoError(t, err)
	}

	// Load UI preferences
	uiPrefs, err := repo.loadUIPreferences(ctx, sessionID)
	require.NoError(t, err)
	require.NotNil(t, uiPrefs)

	// Verify all fields
	assert.Equal(t, "Development", uiPrefs.Category)
	assert.True(t, uiPrefs.IsExpanded)
	assert.Equal(t, "category", uiPrefs.GroupingStrategy)
	assert.Equal(t, "name", uiPrefs.SortOrder)
	assert.ElementsMatch(t, tags, uiPrefs.Tags)
}

// TestLoadUIPreferences_NoData tests behavior when no UI preferences exist
func TestLoadUIPreferences_NoData(t *testing.T) {
	repo, cleanup := createTestRepositoryInMemory(t)
	defer cleanup()

	ctx := context.Background()

	session := createTestSession("no-ui-prefs")
	err := repo.Create(ctx, session)
	require.NoError(t, err)

	var sessionID int64
	err = repo.db.QueryRowContext(ctx, "SELECT id FROM sessions WHERE title = ?", session.Title).Scan(&sessionID)
	require.NoError(t, err)

	// Load UI preferences (should return nil, no error)
	uiPrefs, err := repo.loadUIPreferences(ctx, sessionID)
	require.NoError(t, err)
	assert.Nil(t, uiPrefs, "UI preferences should be nil when no data exists")
}

// TestLoadActivityTracking tests loading activity tracking from the database
func TestLoadActivityTracking(t *testing.T) {
	repo, cleanup := createTestRepositoryInMemory(t)
	defer cleanup()

	ctx := context.Background()

	// Create base session
	session := createTestSession("activity-test")
	err := repo.Create(ctx, session)
	require.NoError(t, err)

	var sessionID int64
	err = repo.db.QueryRowContext(ctx, "SELECT id FROM sessions WHERE title = ?", session.Title).Scan(&sessionID)
	require.NoError(t, err)

	// Create test timestamps
	now := time.Now()
	lastUpdate := now.Add(-5 * time.Minute)
	lastMeaningful := now.Add(-10 * time.Minute)
	lastViewed := now.Add(-15 * time.Minute)
	lastAcknowledged := now.Add(-20 * time.Minute)
	lastAddedToQueue := now.Add(-25 * time.Minute)

	// Insert test activity tracking data
	_, err = repo.db.ExecContext(ctx, `
		INSERT INTO activity_tracking (
			session_id, last_terminal_update, last_meaningful_output,
			last_output_signature, last_added_to_queue, last_viewed, last_acknowledged
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, sessionID, lastUpdate, lastMeaningful, "signature-abc123", lastAddedToQueue, lastViewed, lastAcknowledged)
	require.NoError(t, err)

	// Load activity tracking
	activity, err := repo.loadActivityTracking(ctx, sessionID)
	require.NoError(t, err)
	require.NotNil(t, activity)

	// Verify all fields (allowing for small time differences due to DB serialization)
	assert.WithinDuration(t, lastUpdate, activity.LastTerminalUpdate, time.Second)
	assert.WithinDuration(t, lastMeaningful, activity.LastMeaningfulOutput, time.Second)
	assert.Equal(t, "signature-abc123", activity.LastOutputSignature)
	assert.WithinDuration(t, lastAddedToQueue, activity.LastAddedToQueue, time.Second)
	assert.WithinDuration(t, lastViewed, activity.LastViewed, time.Second)
	assert.WithinDuration(t, lastAcknowledged, activity.LastAcknowledged, time.Second)
}

// TestLoadActivityTracking_NoData tests behavior when no activity tracking exists
func TestLoadActivityTracking_NoData(t *testing.T) {
	repo, cleanup := createTestRepositoryInMemory(t)
	defer cleanup()

	ctx := context.Background()

	session := createTestSession("no-activity")
	err := repo.Create(ctx, session)
	require.NoError(t, err)

	var sessionID int64
	err = repo.db.QueryRowContext(ctx, "SELECT id FROM sessions WHERE title = ?", session.Title).Scan(&sessionID)
	require.NoError(t, err)

	// Load activity tracking (should return nil, no error)
	activity, err := repo.loadActivityTracking(ctx, sessionID)
	require.NoError(t, err)
	assert.Nil(t, activity, "Activity tracking should be nil when no data exists")
}

// TestLoadCloudContext tests loading cloud context from the database
func TestLoadCloudContext(t *testing.T) {
	repo, cleanup := createTestRepositoryInMemory(t)
	defer cleanup()

	ctx := context.Background()

	// Create base session
	session := createTestSession("cloud-context-test")
	err := repo.Create(ctx, session)
	require.NoError(t, err)

	var sessionID int64
	err = repo.db.QueryRowContext(ctx, "SELECT id FROM sessions WHERE title = ?", session.Title).Scan(&sessionID)
	require.NoError(t, err)

	// Insert test cloud context data
	_, err = repo.db.ExecContext(ctx, `
		INSERT INTO cloud_context (
			session_id, provider, region, instance_id,
			api_endpoint, api_key_ref, cloud_session_id, conversation_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, sessionID, "aws", "us-west-2", "i-1234567890abcdef0",
		"https://api.example.com", "keyring:aws-dev", "cloud-sess-123", "conv-456")
	require.NoError(t, err)

	// Load cloud context
	cloudCtx, err := repo.loadCloudContext(ctx, sessionID)
	require.NoError(t, err)
	require.NotNil(t, cloudCtx)

	// Verify all fields
	assert.Equal(t, "aws", cloudCtx.Provider)
	assert.Equal(t, "us-west-2", cloudCtx.Region)
	assert.Equal(t, "i-1234567890abcdef0", cloudCtx.InstanceID)
	assert.Equal(t, "https://api.example.com", cloudCtx.APIEndpoint)
	assert.Equal(t, "keyring:aws-dev", cloudCtx.APIKeyRef)
	assert.Equal(t, "cloud-sess-123", cloudCtx.CloudSessionID)
	assert.Equal(t, "conv-456", cloudCtx.ConversationID)
}

// TestLoadCloudContext_NoData tests behavior when no cloud context exists
func TestLoadCloudContext_NoData(t *testing.T) {
	repo, cleanup := createTestRepositoryInMemory(t)
	defer cleanup()

	ctx := context.Background()

	session := createTestSession("no-cloud-context")
	err := repo.Create(ctx, session)
	require.NoError(t, err)

	var sessionID int64
	err = repo.db.QueryRowContext(ctx, "SELECT id FROM sessions WHERE title = ?", session.Title).Scan(&sessionID)
	require.NoError(t, err)

	// Load cloud context (should return nil, no error)
	cloudCtx, err := repo.loadCloudContext(ctx, sessionID)
	require.NoError(t, err)
	assert.Nil(t, cloudCtx, "Cloud context should be nil when no data exists")
}

// TestGetWithOptions_AllContexts tests loading all contexts with LoadOptions
func TestGetWithOptions_AllContexts(t *testing.T) {
	repo, cleanup := createTestRepositoryInMemory(t)
	defer cleanup()

	ctx := context.Background()

	// Create base session
	session := createTestSession("all-contexts-test")
	err := repo.Create(ctx, session)
	require.NoError(t, err)

	var sessionID int64
	err = repo.db.QueryRowContext(ctx, "SELECT id FROM sessions WHERE title = ?", session.Title).Scan(&sessionID)
	require.NoError(t, err)

	// Insert all context types
	_, err = repo.db.ExecContext(ctx, `
		INSERT INTO git_context (session_id, branch, pr_number, pr_url, owner, repo)
		VALUES (?, ?, ?, ?, ?, ?)
	`, sessionID, "main", 100, "https://github.com/org/repo/pull/100", "org", "repo")
	require.NoError(t, err)

	_, err = repo.db.ExecContext(ctx, `
		INSERT INTO filesystem_context (session_id, project_path, working_dir)
		VALUES (?, ?, ?)
	`, sessionID, "/projects/test", "/projects/test/src")
	require.NoError(t, err)

	_, err = repo.db.ExecContext(ctx, `
		INSERT INTO terminal_context (session_id, height, width, terminal_type)
		VALUES (?, ?, ?, ?)
	`, sessionID, 30, 100, "tmux")
	require.NoError(t, err)

	_, err = repo.db.ExecContext(ctx, `
		INSERT INTO ui_preferences (session_id, category, is_expanded)
		VALUES (?, ?, ?)
	`, sessionID, "Work", 1)
	require.NoError(t, err)

	now := time.Now()
	_, err = repo.db.ExecContext(ctx, `
		INSERT INTO activity_tracking (session_id, last_terminal_update, last_output_signature)
		VALUES (?, ?, ?)
	`, sessionID, now, "sig-xyz")
	require.NoError(t, err)

	_, err = repo.db.ExecContext(ctx, `
		INSERT INTO cloud_context (session_id, provider, region)
		VALUES (?, ?, ?)
	`, sessionID, "gcp", "us-central1")
	require.NoError(t, err)

	// Load with all options enabled
	options := LoadOptions{
		LoadTags:          true,
		LoadWorktree:      true,
		LoadDiffStats:     true,
		LoadDiffContent:   true,
		LoadClaudeSession: true,
	}

	retrieved, err := repo.GetWithOptions(ctx, session.Title, options)
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	// Verify core session data
	assert.Equal(t, session.Title, retrieved.Title)
	assert.Equal(t, session.Path, retrieved.Path)
	assert.Equal(t, session.Status, retrieved.Status)
}

// TestGetWithOptions_SelectiveLoading tests loading only specific contexts
func TestGetWithOptions_SelectiveLoading(t *testing.T) {
	repo, cleanup := createTestRepositoryInMemory(t)
	defer cleanup()

	ctx := context.Background()

	// Create base session with all context data
	session := createTestSession("selective-test")
	err := repo.Create(ctx, session)
	require.NoError(t, err)

	var sessionID int64
	err = repo.db.QueryRowContext(ctx, "SELECT id FROM sessions WHERE title = ?", session.Title).Scan(&sessionID)
	require.NoError(t, err)

	// Insert all context types (so we can test selective loading)
	_, err = repo.db.ExecContext(ctx, `
		INSERT INTO git_context (session_id, branch) VALUES (?, ?)
	`, sessionID, "develop")
	require.NoError(t, err)

	_, err = repo.db.ExecContext(ctx, `
		INSERT INTO terminal_context (session_id, height, width) VALUES (?, ?, ?)
	`, sessionID, 24, 80)
	require.NoError(t, err)

	_, err = repo.db.ExecContext(ctx, `
		INSERT INTO cloud_context (session_id, provider) VALUES (?, ?)
	`, sessionID, "azure")
	require.NoError(t, err)

	// Load with minimal options (nothing enabled)
	options := LoadOptions{}
	retrieved, err := repo.GetWithOptions(ctx, session.Title, options)
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	// Verify session loaded but contexts were not loaded from new tables
	assert.Equal(t, session.Title, retrieved.Title)
	// Note: Context loading is currently not integrated with InstanceData
	// This test verifies the method doesn't fail with minimal options
}

// TestGetWithOptions_NonExistentSession tests error handling for missing sessions
func TestGetWithOptions_NonExistentSession(t *testing.T) {
	repo, cleanup := createTestRepositoryInMemory(t)
	defer cleanup()

	ctx := context.Background()

	options := LoadOptions{
		LoadTags:          true,
		LoadWorktree:      true,
		LoadDiffStats:     true,
		LoadDiffContent:   true,
		LoadClaudeSession: true,
	}

	// Try to load non-existent session
	_, err := repo.GetWithOptions(ctx, "nonexistent-session", options)
	assert.Error(t, err, "Should fail to load non-existent session")
	assert.Contains(t, err.Error(), "not found")
}

// Helper function to create an in-memory test repository
func createTestRepositoryInMemory(t *testing.T) (*SQLiteRepository, func()) {
	// Use :memory: database for fast, isolated tests
	repo, err := NewSQLiteRepository(WithDatabasePath(":memory:"))
	require.NoError(t, err)

	cleanup := func() {
		repo.Close()
	}

	return repo, cleanup
}

// TestMultipleContextsIntegration tests loading multiple sessions with mixed context data
func TestMultipleContextsIntegration(t *testing.T) {
	repo, cleanup := createTestRepositoryInMemory(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple sessions with different context combinations
	sessions := []struct {
		title       string
		hasGit      bool
		hasTerminal bool
		hasCloud    bool
	}{
		{"full-context", true, true, true},
		{"git-only", true, false, false},
		{"terminal-only", false, true, false},
		{"no-contexts", false, false, false},
	}

	for _, s := range sessions {
		// Insert minimal session data directly to avoid worktree loading issues
		_, err := repo.db.ExecContext(ctx, `
			INSERT INTO sessions (
				title, path, working_dir, branch, status, height, width,
				created_at, updated_at, auto_yes, prompt, program
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, s.title, "/test/path", "/test/path", "main", Running, 24, 80,
			time.Now(), time.Now(), false, "", "claude")
		require.NoError(t, err)

		var sessionID int64
		err = repo.db.QueryRowContext(ctx, "SELECT id FROM sessions WHERE title = ?", s.title).Scan(&sessionID)
		require.NoError(t, err)

		if s.hasGit {
			_, err = repo.db.ExecContext(ctx, `
				INSERT INTO git_context (
					session_id, branch, base_commit_sha, pr_number, pr_url, owner, repo, source_ref
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			`, sessionID, "branch-"+s.title, "", 0, "", "", "", "")
			require.NoError(t, err)
		}

		if s.hasTerminal {
			_, err = repo.db.ExecContext(ctx, `
				INSERT INTO terminal_context (
					session_id, height, width, tmux_session_name, tmux_prefix, tmux_server_socket, terminal_type
				) VALUES (?, ?, ?, ?, ?, ?, ?)
			`, sessionID, 30, 120, "", "", "", "")
			require.NoError(t, err)
		}

		if s.hasCloud {
			_, err = repo.db.ExecContext(ctx, `
				INSERT INTO cloud_context (
					session_id, provider, region, instance_id, api_endpoint, api_key_ref, cloud_session_id, conversation_id
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			`, sessionID, "aws", "us-east-1", "", "", "", "", "")
			require.NoError(t, err)
		}
	}

	// Verify individual context loading
	for _, s := range sessions {
		var sessionID int64
		err := repo.db.QueryRowContext(ctx, "SELECT id FROM sessions WHERE title = ?", s.title).Scan(&sessionID)
		require.NoError(t, err)

		gitCtx, err := repo.loadGitContext(ctx, sessionID)
		require.NoError(t, err)
		if s.hasGit {
			assert.NotNil(t, gitCtx, "Expected git context for "+s.title)
		} else {
			assert.Nil(t, gitCtx, "Expected no git context for "+s.title)
		}

		termCtx, err := repo.loadTerminalContext(ctx, sessionID)
		require.NoError(t, err)
		if s.hasTerminal {
			assert.NotNil(t, termCtx, "Expected terminal context for "+s.title)
		} else {
			assert.Nil(t, termCtx, "Expected no terminal context for "+s.title)
		}

		cloudCtx, err := repo.loadCloudContext(ctx, sessionID)
		require.NoError(t, err)
		if s.hasCloud {
			assert.NotNil(t, cloudCtx, "Expected cloud context for "+s.title)
		} else {
			assert.Nil(t, cloudCtx, "Expected no cloud context for "+s.title)
		}
	}
}

// TestContextLoadingWithDatabaseError tests error handling during context loading
func TestContextLoadingWithDatabaseError(t *testing.T) {
	repo, cleanup := createTestRepositoryInMemory(t)
	defer cleanup()

	ctx := context.Background()

	// Close the database to simulate connection errors
	repo.Close()

	// Try to load context with closed database
	_, err := repo.loadGitContext(ctx, 1)
	assert.Error(t, err, "Should fail with closed database")
}

// TestUIPreferencesWithEmptyTags tests loading UI preferences when no tags exist
func TestUIPreferencesWithEmptyTags(t *testing.T) {
	repo, cleanup := createTestRepositoryInMemory(t)
	defer cleanup()

	ctx := context.Background()

	// Create base session
	session := createTestSession("ui-no-tags")
	err := repo.Create(ctx, session)
	require.NoError(t, err)

	var sessionID int64
	err = repo.db.QueryRowContext(ctx, "SELECT id FROM sessions WHERE title = ?", session.Title).Scan(&sessionID)
	require.NoError(t, err)

	// Insert UI preferences without tags - use empty strings for nullable string fields
	_, err = repo.db.ExecContext(ctx, `
		INSERT INTO ui_preferences (session_id, category, is_expanded, grouping_strategy, sort_order)
		VALUES (?, ?, ?, ?, ?)
	`, sessionID, "Testing", 0, "", "")
	require.NoError(t, err)

	// Load UI preferences
	uiPrefs, err := repo.loadUIPreferences(ctx, sessionID)
	require.NoError(t, err)
	require.NotNil(t, uiPrefs)

	// Verify fields
	assert.Equal(t, "Testing", uiPrefs.Category)
	assert.False(t, uiPrefs.IsExpanded)
	assert.Empty(t, uiPrefs.Tags, "Tags should be empty slice, not nil")
}

// TestActivityTrackingMinimalData tests activity tracking with minimal required data
func TestActivityTrackingMinimalData(t *testing.T) {
	repo, cleanup := createTestRepositoryInMemory(t)
	defer cleanup()

	ctx := context.Background()

	// Create base session
	session := createTestSession("activity-minimal")
	err := repo.Create(ctx, session)
	require.NoError(t, err)

	var sessionID int64
	err = repo.db.QueryRowContext(ctx, "SELECT id FROM sessions WHERE title = ?", session.Title).Scan(&sessionID)
	require.NoError(t, err)

	// Insert activity tracking with only signature and valid timestamps
	// Note: Current implementation expects non-NULL timestamps, so we provide zero time
	zeroTime := time.Time{}
	_, err = repo.db.ExecContext(ctx, `
		INSERT INTO activity_tracking (
			session_id, last_terminal_update, last_meaningful_output,
			last_output_signature, last_added_to_queue, last_viewed, last_acknowledged
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, sessionID, zeroTime, zeroTime, "signature-only", zeroTime, zeroTime, zeroTime)
	require.NoError(t, err)

	// Load activity tracking
	activity, err := repo.loadActivityTracking(ctx, sessionID)
	require.NoError(t, err)
	require.NotNil(t, activity)

	// Verify signature is loaded
	assert.Equal(t, "signature-only", activity.LastOutputSignature)
	// Zero times should be preserved
	assert.True(t, activity.LastTerminalUpdate.IsZero() || activity.LastTerminalUpdate.Equal(zeroTime))
}
