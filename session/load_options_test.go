package session

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSelectiveLoading verifies that LoadOptions correctly control what data is loaded
func TestSelectiveLoading(t *testing.T) {
	// Create temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	repo, err := NewSQLiteRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	defer repo.Close()

	ctx := context.Background()

	// Create a test session with all data
	testData := InstanceData{
		Title:      "test-session",
		Path:       "/tmp/test",
		WorkingDir: "/tmp/test",
		Branch:     "main",
		Status:     Ready,
		Worktree: GitWorktreeData{
			RepoPath:     "/tmp/repo",
			WorktreePath: "/tmp/worktree",
			BranchName:   "feature",
		},
		DiffStats: DiffStatsData{
			Added:   100,
			Removed: 50,
			Content: "This is a very large diff content that should not be loaded with LoadMinimal or LoadSummary",
		},
		Tags: []string{"Frontend", "Urgent"},
		ClaudeSession: ClaudeSessionData{
			SessionID:      "claude-123",
			ConversationID: "conv-456",
			ProjectName:    "my-project",
		},
	}

	err = repo.Create(ctx, testData)
	require.NoError(t, err)

	// Test LoadMinimal - should load only core fields
	t.Run("LoadMinimal", func(t *testing.T) {
		session, err := repo.GetWithOptions(ctx, "test-session", LoadMinimal)
		require.NoError(t, err)

		assert.Equal(t, "test-session", session.Title)
		assert.Equal(t, "/tmp/test", session.Path)
		assert.Equal(t, Ready, session.Status)

		// Should NOT load child data
		assert.Empty(t, session.Worktree.RepoPath, "LoadMinimal should not load worktree")
		assert.Zero(t, session.DiffStats.Added, "LoadMinimal should not load diff stats")
		assert.Empty(t, session.DiffStats.Content, "LoadMinimal should not load diff content")
		assert.Empty(t, session.Tags, "LoadMinimal should not load tags")
		assert.Empty(t, session.ClaudeSession.SessionID, "LoadMinimal should not load Claude session")
	})

	// Test LoadSummary - should load everything except diff content
	t.Run("LoadSummary", func(t *testing.T) {
		session, err := repo.GetWithOptions(ctx, "test-session", LoadSummary)
		require.NoError(t, err)

		assert.Equal(t, "test-session", session.Title)

		// Should load worktree
		assert.Equal(t, "/tmp/repo", session.Worktree.RepoPath)
		assert.Equal(t, "feature", session.Worktree.BranchName)

		// Should load diff stats (counts only)
		assert.Equal(t, 100, session.DiffStats.Added)
		assert.Equal(t, 50, session.DiffStats.Removed)

		// Should NOT load heavy diff content
		assert.Empty(t, session.DiffStats.Content, "LoadSummary should not load diff content")

		// Should load tags
		assert.Equal(t, []string{"Frontend", "Urgent"}, session.Tags)

		// Should load Claude session
		assert.Equal(t, "claude-123", session.ClaudeSession.SessionID)
	})

	// Test LoadFull - should load everything including diff content
	t.Run("LoadFull", func(t *testing.T) {
		session, err := repo.GetWithOptions(ctx, "test-session", LoadFull)
		require.NoError(t, err)

		assert.Equal(t, "test-session", session.Title)

		// Should load all child data including diff content
		assert.Equal(t, "/tmp/repo", session.Worktree.RepoPath)
		assert.Equal(t, 100, session.DiffStats.Added)
		assert.Contains(t, session.DiffStats.Content, "very large diff content")
		assert.Equal(t, []string{"Frontend", "Urgent"}, session.Tags)
		assert.Equal(t, "claude-123", session.ClaudeSession.SessionID)
	})

	// Test LoadDiffOnly - should load only diff-related data
	t.Run("LoadDiffOnly", func(t *testing.T) {
		session, err := repo.GetWithOptions(ctx, "test-session", LoadDiffOnly)
		require.NoError(t, err)

		// Should load worktree (needed for diff context)
		assert.Equal(t, "/tmp/repo", session.Worktree.RepoPath)

		// Should load diff stats and content
		assert.Equal(t, 100, session.DiffStats.Added)
		assert.Contains(t, session.DiffStats.Content, "very large diff content")

		// Should NOT load tags or Claude session
		assert.Empty(t, session.Tags, "LoadDiffOnly should not load tags")
		assert.Empty(t, session.ClaudeSession.SessionID, "LoadDiffOnly should not load Claude session")
	})

	// Test default List() uses LoadSummary
	t.Run("List uses LoadSummary by default", func(t *testing.T) {
		sessions, err := repo.List(ctx)
		require.NoError(t, err)
		require.Len(t, sessions, 1)

		session := sessions[0]

		// Should have summary data
		assert.Equal(t, 100, session.DiffStats.Added)
		assert.Equal(t, []string{"Frontend", "Urgent"}, session.Tags)

		// Should NOT have diff content
		assert.Empty(t, session.DiffStats.Content, "List() should use LoadSummary which excludes diff content")
	})

	// Test default Get() uses LoadFull
	t.Run("Get uses LoadFull by default", func(t *testing.T) {
		session, err := repo.Get(ctx, "test-session")
		require.NoError(t, err)

		// Should have all data including diff content
		assert.Contains(t, session.DiffStats.Content, "very large diff content", "Get() should use LoadFull which includes diff content")
	})
}

// TestBuilderMethods verifies the fluent builder methods work correctly
func TestBuilderMethods(t *testing.T) {
	// Start with LoadMinimal and add what we need
	options := LoadMinimal.WithTags().WithDiffContent()

	assert.True(t, options.LoadTags, "WithTags should enable tag loading")
	assert.True(t, options.LoadDiffContent, "WithDiffContent should enable diff content loading")
	assert.True(t, options.LoadDiffStats, "WithDiffContent should imply LoadDiffStats")
	assert.False(t, options.LoadWorktree, "Should not enable worktree unless explicitly set")
	assert.False(t, options.LoadClaudeSession, "Should not enable Claude session unless explicitly set")

	// Remove what we don't need
	options = LoadFull.WithoutDiffContent()

	assert.True(t, options.LoadWorktree, "Should preserve worktree setting")
	assert.True(t, options.LoadTags, "Should preserve tags setting")
	assert.False(t, options.LoadDiffContent, "WithoutDiffContent should disable diff content")
}

// BenchmarkSelectiveLoading measures performance impact of selective loading
func BenchmarkSelectiveLoading(b *testing.B) {
	// Create temporary database
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")

	repo, err := NewSQLiteRepository(WithDatabasePath(dbPath))
	if err != nil {
		b.Fatal(err)
	}
	defer repo.Close()

	ctx := context.Background()

	// Create a session with large diff content (simulate 1MB diff)
	largeDiff := make([]byte, 1024*1024) // 1 MB
	for i := range largeDiff {
		largeDiff[i] = 'x'
	}

	testData := InstanceData{
		Title:      "bench-session",
		Path:       "/tmp/test",
		WorkingDir: "/tmp/test",
		Status:     Ready,
		DiffStats: DiffStatsData{
			Added:   5000,
			Removed: 3000,
			Content: string(largeDiff),
		},
		Tags: []string{"Frontend", "Backend", "DevOps"},
	}

	err = repo.Create(ctx, testData)
	if err != nil {
		b.Fatal(err)
	}

	b.Run("LoadMinimal", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := repo.GetWithOptions(ctx, "bench-session", LoadMinimal)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("LoadSummary", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := repo.GetWithOptions(ctx, "bench-session", LoadSummary)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("LoadFull", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := repo.GetWithOptions(ctx, "bench-session", LoadFull)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
