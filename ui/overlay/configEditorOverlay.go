package overlay

import (
	"claude-squad/config"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ConfigEditorOverlay represents a config file editor overlay for Claude config files
type ConfigEditorOverlay struct {
	BaseOverlay

	// Config manager
	configMgr *config.ClaudeConfigManager

	// State
	mode           string // "list", "edit"
	availableFiles []config.ConfigFile
	selectedIndex  int
	currentFile    *config.ConfigFile
	originalContent string

	// Edit mode
	textarea     textarea.Model
	editFocused  bool
	hasUnsavedChanges bool

	// Status
	statusMessage string
	errorMessage  string

	// Callbacks
	OnComplete func()
	OnCancel   func()
}

// NewConfigEditorOverlay creates a new config editor overlay
func NewConfigEditorOverlay() (*ConfigEditorOverlay, error) {
	mgr, err := config.NewClaudeConfigManager()
	if err != nil {
		return nil, fmt.Errorf("failed to create config manager: %w", err)
	}

	files, err := mgr.ListConfigs()
	if err != nil {
		return nil, fmt.Errorf("failed to list configs: %w", err)
	}

	ta := textarea.New()
	ta.ShowLineNumbers = true
	ta.CharLimit = 0
	ta.MaxHeight = 0

	overlay := &ConfigEditorOverlay{
		configMgr:      mgr,
		mode:           "list",
		availableFiles: files,
		selectedIndex:  0,
		textarea:       ta,
		editFocused:    true,
	}

	overlay.BaseOverlay.SetSize(80, 30)
	overlay.BaseOverlay.Focus()

	return overlay, nil
}

// SetSize updates the overlay dimensions
func (c *ConfigEditorOverlay) SetSize(width, height int) {
	c.BaseOverlay.SetSize(width, height)
	c.textarea.SetWidth(width - 6)
	c.textarea.SetHeight(height - 10)
}

// HandleKeyPress processes keyboard input
func (c *ConfigEditorOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	// Handle common keys (Esc)
	if handled, shouldClose := c.BaseOverlay.HandleCommonKeys(msg); handled {
		if shouldClose {
			if c.hasUnsavedChanges {
				c.statusMessage = "Unsaved changes! Press ctrl+s to save or ctrl+q to quit without saving"
				return false
			}
			if c.OnCancel != nil {
				c.OnCancel()
			}
			return true
		}
	}

	switch c.mode {
	case "list":
		return c.handleListMode(msg)
	case "edit":
		return c.handleEditMode(msg)
	}

	return false
}

// handleListMode handles keys in list selection mode
func (c *ConfigEditorOverlay) handleListMode(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "up", "k":
		if c.selectedIndex > 0 {
			c.selectedIndex--
		}
	case "down", "j":
		if c.selectedIndex < len(c.availableFiles)-1 {
			c.selectedIndex++
		}
	case "enter", " ":
		return c.openSelectedFile()
	case "q", "esc":
		if c.OnCancel != nil {
			c.OnCancel()
		}
		return true
	}
	return false
}

// handleEditMode handles keys in edit mode
func (c *ConfigEditorOverlay) handleEditMode(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "ctrl+s":
		return c.saveCurrentFile()
	case "ctrl+q":
		if c.hasUnsavedChanges {
			c.statusMessage = "Unsaved changes! Press ctrl+s to save first"
			return false
		}
		c.mode = "list"
		c.currentFile = nil
		c.statusMessage = ""
		c.errorMessage = ""
		return false
	case "esc":
		if c.hasUnsavedChanges {
			c.statusMessage = "Unsaved changes! Press ctrl+q to go back or ctrl+s to save"
			return false
		}
		c.mode = "list"
		c.currentFile = nil
		return false
	default:
		var cmd tea.Cmd
		c.textarea, cmd = c.textarea.Update(msg)
		_ = cmd

		// Check if content changed
		if c.textarea.Value() != c.originalContent {
			c.hasUnsavedChanges = true
		} else {
			c.hasUnsavedChanges = false
		}
	}
	return false
}

// openSelectedFile loads the selected file for editing
func (c *ConfigEditorOverlay) openSelectedFile() bool {
	if c.selectedIndex >= len(c.availableFiles) {
		return false
	}

	configFile := c.availableFiles[c.selectedIndex]

	c.currentFile = &configFile
	c.originalContent = configFile.Content
	c.textarea.SetValue(configFile.Content)
	c.textarea.Focus()
	c.mode = "edit"
	c.hasUnsavedChanges = false
	c.statusMessage = ""
	c.errorMessage = ""

	return false
}

// saveCurrentFile saves the current file being edited
func (c *ConfigEditorOverlay) saveCurrentFile() bool {
	if c.currentFile == nil {
		return false
	}

	newContent := c.textarea.Value()

	// Validate if it's settings.json
	if strings.HasSuffix(c.currentFile.Name, "settings.json") {
		if err := c.configMgr.ValidateJSON(c.currentFile.Name, newContent); err != nil {
			c.errorMessage = fmt.Sprintf("Validation error: %v", err)
			return false
		}
	}

	// Save the file
	if err := c.configMgr.UpdateConfig(c.currentFile.Name, newContent); err != nil {
		c.errorMessage = fmt.Sprintf("Error saving: %v", err)
		return false
	}

	c.originalContent = newContent
	c.hasUnsavedChanges = false
	c.statusMessage = fmt.Sprintf("✓ Saved %s", c.currentFile.Name)
	c.errorMessage = ""

	return false
}

// View renders the overlay
func (c *ConfigEditorOverlay) View() string {
	switch c.mode {
	case "list":
		return c.renderListMode()
	case "edit":
		return c.renderEditMode()
	}
	return ""
}

// renderListMode renders the file selection list
func (c *ConfigEditorOverlay) renderListMode() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	itemStyle := lipgloss.NewStyle().PaddingLeft(2)
	selectedStyle := lipgloss.NewStyle().PaddingLeft(2).Background(lipgloss.Color("8")).Bold(true)

	b.WriteString(titleStyle.Render("📝 Claude Config Editor"))
	b.WriteString("\n\n")

	if c.errorMessage != "" {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
		b.WriteString(errorStyle.Render(c.errorMessage))
		b.WriteString("\n\n")
	}

	b.WriteString("Select a config file to edit:\n\n")

	for i, file := range c.availableFiles {
		var line string
		if i == c.selectedIndex {
			line = selectedStyle.Render("▶ " + file.Name)
		} else {
			line = itemStyle.Render("  " + file.Name)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	b.WriteString(helpStyle.Render("↑↓: Navigate • Enter: Edit • Esc/q: Close"))

	return b.String()
}

// renderEditMode renders the file editor
func (c *ConfigEditorOverlay) renderEditMode() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	// Title with file name
	fileName := "(no file)"
	if c.currentFile != nil {
		fileName = c.currentFile.Name
	}
	title := fmt.Sprintf("📝 Editing: %s", fileName)
	if c.hasUnsavedChanges {
		title += " [modified]"
	}
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n\n")

	// Status messages
	if c.statusMessage != "" {
		b.WriteString(statusStyle.Render(c.statusMessage))
		b.WriteString("\n\n")
	}
	if c.errorMessage != "" {
		b.WriteString(errorStyle.Render(c.errorMessage))
		b.WriteString("\n\n")
	}

	// Text editor
	b.WriteString(c.textarea.View())
	b.WriteString("\n\n")

	// Help text
	help := "Ctrl+S: Save • Ctrl+Q: Back to list • Esc: Cancel"
	if c.hasUnsavedChanges {
		help = "⚠️  Unsaved changes! • " + help
	}
	b.WriteString(helpStyle.Render(help))

	return b.String()
}

// Init initializes the overlay
func (c *ConfigEditorOverlay) Init() tea.Cmd {
	return nil
}
