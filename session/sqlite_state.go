package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"claude-squad/config"
	"claude-squad/log"

	"github.com/gofrs/flock"
)

// SQLiteState combines SQLite-backed instance storage with JSON-backed app state.
// This allows gradual migration where sessions are in SQLite but UI preferences remain in JSON.
type SQLiteState struct {
	// Instance storage via SQLite (direct repository access)
	repo *SQLiteRepository
	mu   sync.RWMutex

	// App state (help screens, UI preferences) still in JSON
	appStateFile string
	lockFile     *flock.Flock

	// In-memory app state
	helpScreensSeen uint32
	ui              config.UIState
}

// LoadSQLiteState creates a new SQLiteState with SQLite for instances and JSON for app state.
func LoadSQLiteState() (*SQLiteState, error) {
	// Use workspace-aware or instance-aware path
	baseDir, err := config.GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config directory: %w", err)
	}
	dbPath := filepath.Join(baseDir, "sessions.db")
	appStateFile := filepath.Join(baseDir, "app_state.json")

	// Ensure directory exists
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create state directory: %w", err)
	}

	// Create SQLite repository for instances
	repo, err := NewSQLiteRepository(WithDatabasePath(dbPath))
	if err != nil {
		return nil, fmt.Errorf("failed to create SQLite repository: %w", err)
	}

	// Create file lock for app state JSON
	lockFilePath := appStateFile + ".lock"
	fileLock := flock.New(lockFilePath)

	state := &SQLiteState{
		repo:            repo,
		appStateFile:    appStateFile,
		lockFile:        fileLock,
		helpScreensSeen: 0,
		ui: config.UIState{
			HidePaused:       false,
			CategoryExpanded: make(map[string]bool),
			SearchMode:       false,
			SearchQuery:      "",
			SelectedIdx:      0,
		},
	}

	// Load app state from JSON (if exists)
	if err := state.loadAppState(); err != nil {
		log.WarningLog.Printf("Failed to load app state from JSON: %v", err)
		// Continue with defaults
	}

	log.InfoLog.Printf("Loaded SQLiteState: db=%s, app_state=%s", dbPath, appStateFile)

	return state, nil
}

// SaveInstances saves instances to SQLite
func (s *SQLiteState) SaveInstances(instancesJSON json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Unmarshal JSON to instance data
	var instances []InstanceData
	if err := json.Unmarshal(instancesJSON, &instances); err != nil {
		return fmt.Errorf("failed to unmarshal instances: %w", err)
	}

	ctx := context.Background()

	// Get existing sessions to determine create vs update
	existingSessions, err := s.repo.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list existing sessions: %w", err)
	}

	// Create a map of existing session titles for quick lookup
	existingTitles := make(map[string]bool)
	for _, sess := range existingSessions {
		existingTitles[sess.Title] = true
	}

	// Create or update each instance
	for _, instance := range instances {
		if existingTitles[instance.Title] {
			if err := s.repo.Update(ctx, instance); err != nil {
				log.ErrorLog.Printf("Failed to update session '%s': %v", instance.Title, err)
				return fmt.Errorf("failed to update session '%s': %w", instance.Title, err)
			}
		} else {
			if err := s.repo.Create(ctx, instance); err != nil {
				log.ErrorLog.Printf("Failed to create session '%s': %v", instance.Title, err)
				return fmt.Errorf("failed to create session '%s': %w", instance.Title, err)
			}
		}
	}

	// Delete sessions that are no longer in the input (synchronization)
	incomingTitles := make(map[string]bool)
	for _, instance := range instances {
		incomingTitles[instance.Title] = true
	}

	for _, existing := range existingSessions {
		if !incomingTitles[existing.Title] {
			if err := s.repo.Delete(ctx, existing.Title); err != nil {
				log.WarningLog.Printf("Failed to delete removed session '%s': %v", existing.Title, err)
			}
		}
	}

	return nil
}

// GetInstances returns instances from SQLite as JSON
func (s *SQLiteState) GetInstances() json.RawMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ctx := context.Background()

	instances, err := s.repo.List(ctx)
	if err != nil {
		log.ErrorLog.Printf("Failed to list instances from SQLite: %v", err)
		return json.RawMessage("[]")
	}

	data, err := json.Marshal(instances)
	if err != nil {
		log.ErrorLog.Printf("Failed to marshal instances to JSON: %v", err)
		return json.RawMessage("[]")
	}

	return data
}

// DeleteAllInstances removes all instances from SQLite
func (s *SQLiteState) DeleteAllInstances() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := context.Background()

	instances, err := s.repo.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list instances: %w", err)
	}

	for _, instance := range instances {
		if err := s.repo.Delete(ctx, instance.Title); err != nil {
			return fmt.Errorf("failed to delete instance '%s': %w", instance.Title, err)
		}
	}

	return nil
}

// GetHelpScreensSeen returns the bitmask of seen help screens
func (s *SQLiteState) GetHelpScreensSeen() uint32 {
	return s.helpScreensSeen
}

// SetHelpScreensSeen updates the bitmask of seen help screens
func (s *SQLiteState) SetHelpScreensSeen(seen uint32) error {
	s.helpScreensSeen = seen
	return s.saveAppState()
}

// UI State Management Methods (UIStateAccess interface implementation)

// GetUIState returns a copy of the current UI state
func (s *SQLiteState) GetUIState() config.UIState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ui
}

// SetHidePaused updates the hide paused filter state
func (s *SQLiteState) SetHidePaused(hidePaused bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ui.HidePaused = hidePaused
	return s.saveAppState()
}

// SetCategoryExpanded updates the expanded state for a category
func (s *SQLiteState) SetCategoryExpanded(category string, expanded bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ui.CategoryExpanded == nil {
		s.ui.CategoryExpanded = make(map[string]bool)
	}
	s.ui.CategoryExpanded[category] = expanded
	return s.saveAppState()
}

// GetCategoryExpanded returns whether a category is expanded (defaults to true for new categories)
func (s *SQLiteState) GetCategoryExpanded(category string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.ui.CategoryExpanded == nil {
		return true // Default to expanded for new categories
	}
	expanded, exists := s.ui.CategoryExpanded[category]
	if !exists {
		return true // Default to expanded for new categories
	}
	return expanded
}

// SetSearchMode updates the search mode state
func (s *SQLiteState) SetSearchMode(searchMode bool, query string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ui.SearchMode = searchMode
	s.ui.SearchQuery = query
	return s.saveAppState()
}

// GetSearchState returns the current search mode and query
func (s *SQLiteState) GetSearchState() (bool, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ui.SearchMode, s.ui.SearchQuery
}

// SetSelectedIndex updates the selected session index
func (s *SQLiteState) SetSelectedIndex(index int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ui.SelectedIdx = index
	return s.saveAppState()
}

// GetSelectedIndex returns the last selected session index
func (s *SQLiteState) GetSelectedIndex() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ui.SelectedIdx
}

// RefreshState reloads state (currently only app state, instances are always fresh from SQLite)
func (s *SQLiteState) RefreshState() error {
	return s.loadAppState()
}

// Close releases resources (SQLite connection and file locks)
func (s *SQLiteState) Close() error {
	var err error

	// Close SQLite repository
	if s.repo != nil {
		if closeErr := s.repo.Close(); closeErr != nil {
			err = fmt.Errorf("failed to close SQLite repository: %w", closeErr)
		}
	}

	// Unlock file lock
	if s.lockFile != nil && s.lockFile.Locked() {
		if unlockErr := s.lockFile.Unlock(); unlockErr != nil {
			if err != nil {
				err = fmt.Errorf("%w; also failed to unlock: %w", err, unlockErr)
			} else {
				err = fmt.Errorf("failed to unlock: %w", unlockErr)
			}
		}
	}

	return err
}

// appStateData is the JSON structure for app state file
type appStateData struct {
	HelpScreensSeen uint32           `json:"help_screens_seen"`
	UI              config.UIState `json:"ui"`
}

// loadAppState loads app state from JSON file
func (s *SQLiteState) loadAppState() error {
	// Check if file exists
	if _, err := os.Stat(s.appStateFile); os.IsNotExist(err) {
		// File doesn't exist, use defaults
		return nil
	}

	// Acquire shared lock for reading
	ctx, cancel := context.WithTimeout(context.Background(), config.DefaultLockTimeout)
	defer cancel()

	locked, err := s.lockFile.TryLockContext(ctx, 50*time.Millisecond)
	if err != nil || !locked {
		return fmt.Errorf("failed to acquire lock for reading app state: %w", err)
	}
	defer s.lockFile.Unlock()

	// Read file
	data, err := os.ReadFile(s.appStateFile)
	if err != nil {
		return fmt.Errorf("failed to read app state file: %w", err)
	}

	// Unmarshal
	var appState appStateData
	if err := json.Unmarshal(data, &appState); err != nil {
		return fmt.Errorf("failed to unmarshal app state: %w", err)
	}

	// Update in-memory state
	s.helpScreensSeen = appState.HelpScreensSeen
	s.ui = appState.UI

	// Ensure map is initialized
	if s.ui.CategoryExpanded == nil {
		s.ui.CategoryExpanded = make(map[string]bool)
	}

	return nil
}

// saveAppState saves app state to JSON file
func (s *SQLiteState) saveAppState() error {
	// Ensure directory exists
	dir := filepath.Dir(s.appStateFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Acquire exclusive lock for writing
	ctx, cancel := context.WithTimeout(context.Background(), config.DefaultLockTimeout)
	defer cancel()

	locked, err := s.lockFile.TryLockContext(ctx, 50*time.Millisecond)
	if err != nil || !locked {
		return fmt.Errorf("failed to acquire lock for writing app state: %w", err)
	}
	defer s.lockFile.Unlock()

	// Marshal app state
	appState := appStateData{
		HelpScreensSeen: s.helpScreensSeen,
		UI:              s.ui,
	}

	data, err := json.MarshalIndent(appState, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal app state: %w", err)
	}

	// Write to file
	if err := os.WriteFile(s.appStateFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write app state file: %w", err)
	}

	return nil
}
