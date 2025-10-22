package ui

import (
	"claude-squad/session"
	"testing"
)

// TestGetGroupKeys_Category tests category-based grouping (single-membership)
func TestGetGroupKeys_Category(t *testing.T) {
	tests := []struct {
		name     string
		instance *session.Instance
		want     []string
	}{
		{
			name: "with category",
			instance: &session.Instance{
				Category: "Work",
			},
			want: []string{"Work"},
		},
		{
			name: "with nested category",
			instance: &session.Instance{
				Category: "Work/Frontend",
			},
			want: []string{"Work/Frontend"},
		},
		{
			name:     "without category",
			instance: &session.Instance{},
			want:     []string{"Uncategorized"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getGroupKeys(tt.instance, GroupByCategory)
			if len(got) != len(tt.want) {
				t.Errorf("getGroupKeys() returned %d keys, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("getGroupKeys()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestGetGroupKeys_Tag tests tag-based grouping (multi-membership)
func TestGetGroupKeys_Tag(t *testing.T) {
	tests := []struct {
		name     string
		instance *session.Instance
		want     []string
	}{
		{
			name: "single tag",
			instance: &session.Instance{
				Tags: []string{"Frontend"},
			},
			want: []string{"Frontend"},
		},
		{
			name: "multiple tags - multi-membership",
			instance: &session.Instance{
				Tags: []string{"Frontend", "React", "TypeScript"},
			},
			want: []string{"Frontend", "React", "TypeScript"},
		},
		{
			name:     "no tags",
			instance: &session.Instance{},
			want:     []string{"Untagged"},
		},
		{
			name: "empty tags array",
			instance: &session.Instance{
				Tags: []string{},
			},
			want: []string{"Untagged"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getGroupKeys(tt.instance, GroupByTag)
			if len(got) != len(tt.want) {
				t.Errorf("getGroupKeys() returned %d keys, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("getGroupKeys()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestGetGroupKeys_Branch tests branch-based grouping (single-membership)
func TestGetGroupKeys_Branch(t *testing.T) {
	tests := []struct {
		name     string
		instance *session.Instance
		want     []string
	}{
		{
			name: "with branch",
			instance: &session.Instance{
				Branch: "main",
			},
			want: []string{"main"},
		},
		{
			name: "feature branch",
			instance: &session.Instance{
				Branch: "feature/authentication",
			},
			want: []string{"feature/authentication"},
		},
		{
			name:     "without branch",
			instance: &session.Instance{},
			want:     []string{"No Branch"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getGroupKeys(tt.instance, GroupByBranch)
			if len(got) != len(tt.want) {
				t.Errorf("getGroupKeys() returned %d keys, want %d", len(got), len(tt.want))
				return
			}
			if got[0] != tt.want[0] {
				t.Errorf("getGroupKeys() = %v, want %v", got[0], tt.want[0])
			}
		})
	}
}

// TestGetGroupKeys_Program tests program-based grouping with argument parsing
func TestGetGroupKeys_Program(t *testing.T) {
	tests := []struct {
		name     string
		instance *session.Instance
		want     []string
	}{
		{
			name: "simple program",
			instance: &session.Instance{
				Program: "claude",
			},
			want: []string{"claude"},
		},
		{
			name: "program with arguments",
			instance: &session.Instance{
				Program: "claude --model gpt-4",
			},
			want: []string{"claude"},
		},
		{
			name: "program with complex arguments",
			instance: &session.Instance{
				Program: "aider --model claude-3-opus --auto-commits",
			},
			want: []string{"aider"},
		},
		{
			name:     "without program",
			instance: &session.Instance{},
			want:     []string{"Unknown Program"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getGroupKeys(tt.instance, GroupByProgram)
			if len(got) != len(tt.want) {
				t.Errorf("getGroupKeys() returned %d keys, want %d", len(got), len(tt.want))
				return
			}
			if got[0] != tt.want[0] {
				t.Errorf("getGroupKeys() = %v, want %v", got[0], tt.want[0])
			}
		})
	}
}

// TestGetGroupKeys_Status tests status-based grouping
func TestGetGroupKeys_Status(t *testing.T) {
	tests := []struct {
		name     string
		instance *session.Instance
		want     []string
	}{
		{
			name: "running status",
			instance: &session.Instance{
				Status: session.Running,
			},
			want: []string{"Running"},
		},
		{
			name: "ready status",
			instance: &session.Instance{
				Status: session.Ready,
			},
			want: []string{"Ready"},
		},
		{
			name: "paused status",
			instance: &session.Instance{
				Status: session.Paused,
			},
			want: []string{"Paused"},
		},
		{
			name: "needs approval status",
			instance: &session.Instance{
				Status: session.NeedsApproval,
			},
			want: []string{"Needs Approval"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getGroupKeys(tt.instance, GroupByStatus)
			if len(got) != len(tt.want) {
				t.Errorf("getGroupKeys() returned %d keys, want %d", len(got), len(tt.want))
				return
			}
			if got[0] != tt.want[0] {
				t.Errorf("getGroupKeys() = %v, want %v", got[0], tt.want[0])
			}
		})
	}
}

// TestGetGroupKeys_Path tests path-based grouping with repository extraction
func TestGetGroupKeys_Path(t *testing.T) {
	tests := []struct {
		name     string
		instance *session.Instance
		want     []string
	}{
		{
			name: "unix path",
			instance: &session.Instance{
				Path: "/Users/foo/repos/myproject",
			},
			want: []string{"myproject"},
		},
		{
			name: "nested path",
			instance: &session.Instance{
				Path: "/home/user/workspace/deep/nested/project",
			},
			want: []string{"project"},
		},
		{
			name:     "without path",
			instance: &session.Instance{},
			want:     []string{"Unknown Path"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getGroupKeys(tt.instance, GroupByPath)
			if len(got) != len(tt.want) {
				t.Errorf("getGroupKeys() returned %d keys, want %d", len(got), len(tt.want))
				return
			}
			if got[0] != tt.want[0] {
				t.Errorf("getGroupKeys() = %v, want %v", got[0], tt.want[0])
			}
		})
	}
}

// TestGetGroupKeys_None tests no grouping strategy
func TestGetGroupKeys_None(t *testing.T) {
	instance := &session.Instance{
		Category: "Work",
		Tags:     []string{"Frontend", "React"},
	}

	got := getGroupKeys(instance, GroupByNone)
	want := []string{"All Sessions"}

	if len(got) != len(want) {
		t.Errorf("getGroupKeys() returned %d keys, want %d", len(got), len(want))
		return
	}
	if got[0] != want[0] {
		t.Errorf("getGroupKeys() = %v, want %v", got[0], want[0])
	}
}

// TestOrganizeByStrategy_MultiMembership tests multi-membership with tags
func TestOrganizeByStrategy_MultiMembership(t *testing.T) {
	// Create test instances with tags
	instance1 := &session.Instance{
		Title: "Frontend Project",
		Tags:  []string{"Frontend", "React"},
	}
	instance2 := &session.Instance{
		Title: "Backend Project",
		Tags:  []string{"Backend", "Go"},
	}
	instance3 := &session.Instance{
		Title: "Full Stack Project",
		Tags:  []string{"Frontend", "Backend"},
	}

	// Create list and add instances
	list := &List{
		items:                []*session.Instance{instance1, instance2, instance3},
		categoryGroups:       make(map[string][]*session.Instance),
		groupExpanded:        make(map[string]bool),
		groupingStrategy:     GroupByTag,
		categoriesNeedUpdate: true,
	}

	// Organize by tag strategy
	list.OrganizeByStrategy()

	// Verify multi-membership: instance1 should be in both Frontend and React groups
	frontendGroup := list.categoryGroups["Frontend"]
	if len(frontendGroup) != 2 {
		t.Errorf("Frontend group has %d instances, want 2 (instance1 and instance3)", len(frontendGroup))
	}

	reactGroup := list.categoryGroups["React"]
	if len(reactGroup) != 1 {
		t.Errorf("React group has %d instances, want 1", len(reactGroup))
	}

	backendGroup := list.categoryGroups["Backend"]
	if len(backendGroup) != 2 {
		t.Errorf("Backend group has %d instances, want 2 (instance2 and instance3)", len(backendGroup))
	}

	goGroup := list.categoryGroups["Go"]
	if len(goGroup) != 1 {
		t.Errorf("Go group has %d instances, want 1", len(goGroup))
	}

	// Verify that instance3 appears in both Frontend and Backend groups
	foundInFrontend := false
	foundInBackend := false
	for _, inst := range frontendGroup {
		if inst.Title == "Full Stack Project" {
			foundInFrontend = true
		}
	}
	for _, inst := range backendGroup {
		if inst.Title == "Full Stack Project" {
			foundInBackend = true
		}
	}
	if !foundInFrontend {
		t.Error("Full Stack Project not found in Frontend group")
	}
	if !foundInBackend {
		t.Error("Full Stack Project not found in Backend group")
	}
}

// TestOrganizeByStrategy_SingleMembership tests single-membership with category
func TestOrganizeByStrategy_SingleMembership(t *testing.T) {
	instance1 := &session.Instance{
		Title:     "Work Project 1",
		Category:  "Work",
		IsManaged: true,
	}
	instance2 := &session.Instance{
		Title:     "Work Project 2",
		Category:  "Work",
		IsManaged: true,
	}
	instance3 := &session.Instance{
		Title:     "Personal Project",
		Category:  "Personal",
		IsManaged: true,
	}

	list := &List{
		items:                []*session.Instance{instance1, instance2, instance3},
		categoryGroups:       make(map[string][]*session.Instance),
		groupExpanded:        make(map[string]bool),
		groupingStrategy:     GroupByCategory,
		categoriesNeedUpdate: true,
	}

	list.OrganizeByStrategy()

	// Verify single-membership: each instance appears in only one group
	workGroup := list.categoryGroups["Squad Sessions/Work"]
	if len(workGroup) != 2 {
		t.Errorf("Work group has %d instances, want 2", len(workGroup))
	}

	personalGroup := list.categoryGroups["Squad Sessions/Personal"]
	if len(personalGroup) != 1 {
		t.Errorf("Personal group has %d instances, want 1", len(personalGroup))
	}
}

// TestOrganizeByStrategy_HidePausedFilter tests filtering during organization
func TestOrganizeByStrategy_HidePausedFilter(t *testing.T) {
	instance1 := &session.Instance{
		Title:  "Active Session",
		Status: session.Running,
		Tags:   []string{"Active"},
	}
	instance2 := &session.Instance{
		Title:  "Paused Session",
		Status: session.Paused,
		Tags:   []string{"Active"},
	}

	list := &List{
		items:                []*session.Instance{instance1, instance2},
		categoryGroups:       make(map[string][]*session.Instance),
		groupExpanded:        make(map[string]bool),
		groupingStrategy:     GroupByTag,
		hidePaused:           true,
		categoriesNeedUpdate: true,
	}

	list.OrganizeByStrategy()

	// Verify that paused instance is filtered out
	activeGroup := list.categoryGroups["Active"]
	if len(activeGroup) != 1 {
		t.Errorf("Active group has %d instances, want 1 (paused should be filtered)", len(activeGroup))
	}
	if activeGroup[0].Title != "Active Session" {
		t.Errorf("Active group contains wrong instance: %s", activeGroup[0].Title)
	}
}

// TestOrganizeByStrategy_CategoryHierarchy tests two-tier hierarchy for categories
func TestOrganizeByStrategy_CategoryHierarchy(t *testing.T) {
	managedInstance := &session.Instance{
		Title:     "Managed Session",
		Category:  "Work",
		IsManaged: true,
	}
	externalInstance := &session.Instance{
		Title:     "External Session",
		Category:  "Personal",
		IsManaged: false,
	}
	uncategorizedManaged := &session.Instance{
		Title:     "Uncategorized Managed",
		IsManaged: true,
	}
	uncategorizedExternal := &session.Instance{
		Title:     "Uncategorized External",
		IsManaged: false,
	}

	list := &List{
		items: []*session.Instance{
			managedInstance,
			externalInstance,
			uncategorizedManaged,
			uncategorizedExternal,
		},
		categoryGroups:       make(map[string][]*session.Instance),
		groupExpanded:        make(map[string]bool),
		groupingStrategy:     GroupByCategory,
		categoriesNeedUpdate: true,
	}

	list.OrganizeByStrategy()

	// Verify two-tier hierarchy
	if _, exists := list.categoryGroups["Squad Sessions/Work"]; !exists {
		t.Error("Squad Sessions/Work group not created")
	}
	if _, exists := list.categoryGroups["External Claude/Personal"]; !exists {
		t.Error("External Claude/Personal group not created")
	}
	if _, exists := list.categoryGroups["Squad Sessions"]; !exists {
		t.Error("Squad Sessions (uncategorized) group not created")
	}
	if _, exists := list.categoryGroups["External Claude"]; !exists {
		t.Error("External Claude (uncategorized) group not created")
	}

	// Verify group contents
	squadWork := list.categoryGroups["Squad Sessions/Work"]
	if len(squadWork) != 1 || squadWork[0].Title != "Managed Session" {
		t.Error("Squad Sessions/Work group has incorrect content")
	}

	externalPersonal := list.categoryGroups["External Claude/Personal"]
	if len(externalPersonal) != 1 || externalPersonal[0].Title != "External Session" {
		t.Error("External Claude/Personal group has incorrect content")
	}
}

// TestOrganizeByStrategy_PerformanceOptimization tests skip behavior
func TestOrganizeByStrategy_PerformanceOptimization(t *testing.T) {
	instance := &session.Instance{
		Title:    "Test Session",
		Category: "Work",
	}

	list := &List{
		items:                []*session.Instance{instance},
		categoryGroups:       make(map[string][]*session.Instance),
		groupExpanded:        make(map[string]bool),
		groupingStrategy:     GroupByCategory,
		categoriesNeedUpdate: false, // Should skip reorganization
	}

	// Manually populate groups
	list.categoryGroups["OldGroup"] = []*session.Instance{instance}

	// Call OrganizeByStrategy - should skip due to !categoriesNeedUpdate
	list.OrganizeByStrategy()

	// Verify that old groups are preserved (not reorganized)
	if _, exists := list.categoryGroups["OldGroup"]; !exists {
		t.Error("Organization ran when it should have been skipped (performance optimization failed)")
	}
	if _, exists := list.categoryGroups["Squad Sessions/Work"]; exists {
		t.Error("Organization ran when categoriesNeedUpdate was false")
	}
}

// TestOrganizeByStrategy_AllStrategies tests all 8 grouping strategies
func TestOrganizeByStrategy_AllStrategies(t *testing.T) {
	instance := &session.Instance{
		Title:       "Test Session",
		Category:    "Work",
		Branch:      "main",
		Path:        "/repos/project",
		Program:     "claude",
		Status:      session.Running,
		SessionType: session.SessionTypeDirectory,
		Tags:        []string{"Frontend", "React"},
		IsManaged:   true,
	}

	strategies := []struct {
		strategy  GroupingStrategy
		expectKey string // Expected group key
	}{
		{GroupByCategory, "Squad Sessions/Work"},
		{GroupByBranch, "main"},
		{GroupByPath, "project"},
		{GroupByProgram, "claude"},
		{GroupByStatus, "Running"},
		{GroupBySessionType, "Directory Session"},
		{GroupByTag, "Frontend"}, // Multi-membership: also creates "React" group
		{GroupByNone, "All Sessions"},
	}

	for _, tt := range strategies {
		t.Run(tt.strategy.String(), func(t *testing.T) {
			list := &List{
				items:                []*session.Instance{instance},
				categoryGroups:       make(map[string][]*session.Instance),
				groupExpanded:        make(map[string]bool),
				groupingStrategy:     tt.strategy,
				categoriesNeedUpdate: true,
			}

			list.OrganizeByStrategy()

			if _, exists := list.categoryGroups[tt.expectKey]; !exists {
				t.Errorf("Expected group key %s not found in categoryGroups", tt.expectKey)
			}

			// For tag strategy, verify multi-membership
			if tt.strategy == GroupByTag {
				if _, exists := list.categoryGroups["React"]; !exists {
					t.Error("Tag strategy should create React group for multi-membership")
				}
			}
		})
	}
}
