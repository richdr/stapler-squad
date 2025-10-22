package overlay

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TagEditorOverlay provides an interactive interface for managing session tags.
// It supports adding, removing, and viewing tags with real-time feedback.
type TagEditorOverlay struct {
	BaseOverlay // Embed base for common overlay functionality

	// Session identification
	sessionTitle string

	// Tag state
	tags          []string      // Current tags for the session
	selectedIndex int           // Currently selected tag index (-1 for none)
	inputActive   bool          // Whether the input field is active
	input         textinput.Model // Input for new tags

	// UI state
	message     string // Status message (success/error feedback)
	messageType string // "success", "error", or ""

	// Callbacks
	OnComplete func(tags []string) // Called when editor is closed with changes
	OnCancel   func()              // Called when editor is cancelled
}

// NewTagEditorOverlay creates a new tag editor overlay.
func NewTagEditorOverlay(sessionTitle string, initialTags []string) *TagEditorOverlay {
	ti := textinput.New()
	ti.Placeholder = "Enter tag name..."
	ti.CharLimit = 50
	ti.Width = 40

	// Make a copy of tags to avoid modifying the original
	tagsCopy := make([]string, len(initialTags))
	copy(tagsCopy, initialTags)

	overlay := &TagEditorOverlay{
		sessionTitle:  sessionTitle,
		tags:          tagsCopy,
		selectedIndex: -1,
		inputActive:   false,
		input:         ti,
		message:       "",
		messageType:   "",
	}

	// Initialize BaseOverlay with default size
	overlay.BaseOverlay.SetSize(60, 20)
	overlay.BaseOverlay.Focus()

	return overlay
}

// SetSize updates the overlay dimensions.
func (t *TagEditorOverlay) SetSize(width, height int) {
	t.BaseOverlay.SetSize(width, height)
}

// HandleKeyPress processes keyboard input.
// Returns true if the overlay should be closed.
func (t *TagEditorOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	// Clear message on any key press
	t.message = ""
	t.messageType = ""

	// Handle input field first if active
	if t.inputActive {
		return t.handleInputKeys(msg)
	}

	// Use BaseOverlay for common keys (Esc)
	if handled, shouldClose := t.BaseOverlay.HandleCommonKeys(msg); handled {
		if shouldClose {
			if t.OnCancel != nil {
				t.OnCancel()
			}
			return true
		}
	}

	// Handle navigation and actions
	switch msg.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		if t.selectedIndex > 0 {
			t.selectedIndex--
		}
		return false

	case tea.KeyDown, tea.KeyCtrlN:
		if t.selectedIndex < len(t.tags)-1 {
			t.selectedIndex++
		}
		return false

	case tea.KeyEnter:
		// Save and close
		if t.OnComplete != nil {
			t.OnComplete(t.tags)
		}
		return true

	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "a", "A":
			// Activate input for adding tag
			t.inputActive = true
			t.input.Focus()
			return false

		case "d", "D":
			// Delete selected tag
			if t.selectedIndex >= 0 && t.selectedIndex < len(t.tags) {
				removedTag := t.tags[t.selectedIndex]
				t.tags = append(t.tags[:t.selectedIndex], t.tags[t.selectedIndex+1:]...)
				if t.selectedIndex >= len(t.tags) {
					t.selectedIndex = len(t.tags) - 1
				}
				t.message = "Removed tag: " + removedTag
				t.messageType = "success"
			} else {
				t.message = "No tag selected"
				t.messageType = "error"
			}
			return false
		}
	}

	return false
}

// handleInputKeys processes keyboard input when the input field is active.
func (t *TagEditorOverlay) handleInputKeys(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyEsc:
		// Cancel input
		t.inputActive = false
		t.input.Blur()
		t.input.SetValue("")
		return false

	case tea.KeyEnter:
		// Add new tag
		newTag := strings.TrimSpace(t.input.Value())
		if newTag == "" {
			t.message = "Tag name cannot be empty"
			t.messageType = "error"
			return false
		}

		// Check for duplicates
		for _, tag := range t.tags {
			if tag == newTag {
				t.message = "Tag already exists: " + newTag
				t.messageType = "error"
				t.input.SetValue("")
				return false
			}
		}

		// Add the tag
		t.tags = append(t.tags, newTag)
		t.selectedIndex = len(t.tags) - 1
		t.message = "Added tag: " + newTag
		t.messageType = "success"

		// Reset input
		t.inputActive = false
		t.input.Blur()
		t.input.SetValue("")
		return false

	default:
		// Pass key to input field
		var cmd tea.Cmd
		t.input, cmd = t.input.Update(msg)
		_ = cmd // We don't use the command in this simple overlay
		return false
	}
}

// Render renders the tag editor overlay.
func (t *TagEditorOverlay) Render() string {
	// Use responsive sizing from BaseOverlay
	responsiveWidth := t.GetResponsiveWidth()
	hPadding, vPadding := t.GetResponsivePadding()

	// Create styles with responsive sizing
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(vPadding, hPadding).
		Width(responsiveWidth)

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("62")).
		Bold(true).
		MarginBottom(1)

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Italic(true).
		MarginTop(1)

	selectedTagStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("62")).
		Foreground(lipgloss.Color("0")).
		Padding(0, 1)

	normalTagStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("7")).
		Padding(0, 1)

	messageSuccessStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("10")).
		Bold(true)

	messageErrorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("9")).
		Bold(true)

	// Build the view
	var content strings.Builder

	// Title
	content.WriteString(titleStyle.Render("Edit Tags: " + t.sessionTitle))
	content.WriteString("\n\n")

	// Tags list
	if len(t.tags) == 0 {
		content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("No tags"))
		content.WriteString("\n")
	} else {
		for i, tag := range t.tags {
			var tagText string
			if i == t.selectedIndex {
				tagText = selectedTagStyle.Render("• " + tag)
			} else {
				tagText = normalTagStyle.Render("  " + tag)
			}
			content.WriteString(tagText)
			content.WriteString("\n")
		}
	}

	// Input field (if active)
	if t.inputActive {
		content.WriteString("\n")
		content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Render("New tag:"))
		content.WriteString("\n")
		content.WriteString(t.input.View())
		content.WriteString("\n")
	}

	// Status message
	if t.message != "" {
		content.WriteString("\n")
		if t.messageType == "success" {
			content.WriteString(messageSuccessStyle.Render("✓ " + t.message))
		} else if t.messageType == "error" {
			content.WriteString(messageErrorStyle.Render("✗ " + t.message))
		} else {
			content.WriteString(t.message)
		}
		content.WriteString("\n")
	}

	// Help text
	content.WriteString("\n")
	if t.inputActive {
		content.WriteString(helpStyle.Render("Enter: add tag • Esc: cancel"))
	} else {
		content.WriteString(helpStyle.Render("a: add • d: delete • ↑↓: navigate • Enter: save • Esc: cancel"))
	}

	return style.Render(content.String())
}
