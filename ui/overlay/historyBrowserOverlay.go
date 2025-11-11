package overlay

import (
	"claude-squad/session"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// HistoryBrowserOverlay represents a history browser overlay for Claude session history
type HistoryBrowserOverlay struct {
	BaseOverlay

	// History data
	history *session.ClaudeSessionHistory
	entries []session.ClaudeHistoryEntry

	// State
	mode           string // "list", "detail"
	selectedIndex  int
	scrollOffset   int
	searchQuery    string
	searchMode     bool

	// Detail view
	currentEntry *session.ClaudeHistoryEntry

	// Status
	statusMessage string
	errorMessage  string

	// Callbacks
	OnSelectEntry func(entry session.ClaudeHistoryEntry)
	OnCancel      func()
}

// NewHistoryBrowserOverlay creates a new history browser overlay
func NewHistoryBrowserOverlay() (*HistoryBrowserOverlay, error) {
	hist, err := session.NewClaudeSessionHistoryFromClaudeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to load history: %w", err)
	}

	entries := hist.GetAll()

	overlay := &HistoryBrowserOverlay{
		history:       hist,
		entries:       entries,
		mode:          "list",
		selectedIndex: 0,
		scrollOffset:  0,
		searchMode:    false,
	}

	overlay.BaseOverlay.SetSize(80, 30)
	overlay.BaseOverlay.Focus()

	return overlay, nil
}

// SetSize updates the overlay dimensions
func (h *HistoryBrowserOverlay) SetSize(width, height int) {
	h.BaseOverlay.SetSize(width, height)
}

// HandleKeyPress processes keyboard input
func (h *HistoryBrowserOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	// Handle common keys (Esc)
	if handled, shouldClose := h.BaseOverlay.HandleCommonKeys(msg); handled {
		if shouldClose {
			if h.searchMode {
				h.searchMode = false
				h.searchQuery = ""
				h.entries = h.history.GetAll()
				h.selectedIndex = 0
				h.scrollOffset = 0
				return false
			}
			if h.mode == "detail" {
				h.mode = "list"
				h.currentEntry = nil
				return false
			}
			if h.OnCancel != nil {
				h.OnCancel()
			}
			return true
		}
	}

	if h.searchMode {
		return h.handleSearchMode(msg)
	}

	switch h.mode {
	case "list":
		return h.handleListMode(msg)
	case "detail":
		return h.handleDetailMode(msg)
	}

	return false
}

// handleSearchMode handles keys in search mode
func (h *HistoryBrowserOverlay) handleSearchMode(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "enter":
		// Perform search
		if h.searchQuery != "" {
			h.entries = h.history.Search(h.searchQuery)
			h.selectedIndex = 0
			h.scrollOffset = 0
			h.statusMessage = fmt.Sprintf("Found %d entries", len(h.entries))
		}
		h.searchMode = false
	case "esc":
		// Cancel search
		h.searchMode = false
		h.searchQuery = ""
	case "backspace":
		if len(h.searchQuery) > 0 {
			h.searchQuery = h.searchQuery[:len(h.searchQuery)-1]
		}
	default:
		// Add character to search query
		if len(msg.String()) == 1 {
			h.searchQuery += msg.String()
		}
	}
	return false
}

// handleListMode handles keys in list mode
func (h *HistoryBrowserOverlay) handleListMode(msg tea.KeyMsg) bool {
	visibleHeight := h.GetResponsiveHeight() - 6

	switch msg.String() {
	case "up", "k":
		if h.selectedIndex > 0 {
			h.selectedIndex--
			if h.selectedIndex < h.scrollOffset {
				h.scrollOffset = h.selectedIndex
			}
		}
	case "down", "j":
		if h.selectedIndex < len(h.entries)-1 {
			h.selectedIndex++
			if h.selectedIndex >= h.scrollOffset+visibleHeight {
				h.scrollOffset = h.selectedIndex - visibleHeight + 1
			}
		}
	case "enter", " ":
		if h.selectedIndex < len(h.entries) {
			h.currentEntry = &h.entries[h.selectedIndex]
			h.mode = "detail"
		}
	case "/", "s":
		h.searchMode = true
		h.searchQuery = ""
		h.statusMessage = ""
	case "r":
		// Reset/refresh
		h.entries = h.history.GetAll()
		h.selectedIndex = 0
		h.scrollOffset = 0
		h.searchQuery = ""
		h.statusMessage = "Refreshed history"
	case "q", "esc":
		if h.OnCancel != nil {
			h.OnCancel()
		}
		return true
	}
	return false
}

// handleDetailMode handles keys in detail view mode
func (h *HistoryBrowserOverlay) handleDetailMode(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "o":
		// Open/select entry
		if h.currentEntry != nil && h.OnSelectEntry != nil {
			h.OnSelectEntry(*h.currentEntry)
			return true
		}
	case "q", "esc":
		h.mode = "list"
		h.currentEntry = nil
	}
	return false
}

// View renders the overlay
func (h *HistoryBrowserOverlay) View() string {
	if h.searchMode {
		return h.renderSearchMode()
	}

	switch h.mode {
	case "list":
		return h.renderListMode()
	case "detail":
		return h.renderDetailMode()
	}
	return ""
}

// renderSearchMode renders the search input
func (h *HistoryBrowserOverlay) renderSearchMode() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	b.WriteString(titleStyle.Render("🔍 Search Claude History"))
	b.WriteString("\n\n")
	b.WriteString("Search: " + h.searchQuery + "█")
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("Enter: Search • Esc: Cancel"))

	return b.String()
}

// renderListMode renders the history entry list
func (h *HistoryBrowserOverlay) renderListMode() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	itemStyle := lipgloss.NewStyle().PaddingLeft(2)
	selectedStyle := lipgloss.NewStyle().PaddingLeft(2).Background(lipgloss.Color("8")).Bold(true)
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))

	title := fmt.Sprintf("📚 Claude History (%d entries)", len(h.entries))
	if h.searchQuery != "" {
		title += fmt.Sprintf(" - Search: %q", h.searchQuery)
	}
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n\n")

	if h.statusMessage != "" {
		b.WriteString(statusStyle.Render(h.statusMessage))
		b.WriteString("\n\n")
	}

	if len(h.entries) == 0 {
		b.WriteString("No history entries found.\n")
	} else {
		visibleHeight := h.GetResponsiveHeight() - 6
		endIdx := h.scrollOffset + visibleHeight
		if endIdx > len(h.entries) {
			endIdx = len(h.entries)
		}

		for i := h.scrollOffset; i < endIdx; i++ {
			entry := h.entries[i]
			timestamp := entry.UpdatedAt.Format("2006-01-02 15:04")

			line := fmt.Sprintf("%s - %s (%s) - %d msgs",
				timestamp,
				truncateString(entry.Name, 30),
				entry.Model,
				entry.MessageCount)

			if i == h.selectedIndex {
				line = selectedStyle.Render("▶ " + line)
			} else {
				line = itemStyle.Render("  " + line)
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	help := "↑↓: Navigate • Enter: Details • /: Search • r: Refresh • Esc/q: Close"
	b.WriteString(helpStyle.Render(help))

	return b.String()
}

// renderDetailMode renders the detail view for a selected entry
func (h *HistoryBrowserOverlay) renderDetailMode() string {
	if h.currentEntry == nil {
		return "No entry selected"
	}

	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("7"))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	b.WriteString(titleStyle.Render("📄 History Entry Details"))
	b.WriteString("\n\n")

	b.WriteString(labelStyle.Render("Name: "))
	b.WriteString(valueStyle.Render(h.currentEntry.Name))
	b.WriteString("\n\n")

	b.WriteString(labelStyle.Render("ID: "))
	b.WriteString(valueStyle.Render(h.currentEntry.ID))
	b.WriteString("\n\n")

	b.WriteString(labelStyle.Render("Project: "))
	project := h.currentEntry.Project
	if project == "" {
		project = "(none)"
	}
	b.WriteString(valueStyle.Render(project))
	b.WriteString("\n\n")

	b.WriteString(labelStyle.Render("Model: "))
	b.WriteString(valueStyle.Render(h.currentEntry.Model))
	b.WriteString("\n\n")

	b.WriteString(labelStyle.Render("Message Count: "))
	b.WriteString(valueStyle.Render(fmt.Sprintf("%d", h.currentEntry.MessageCount)))
	b.WriteString("\n\n")

	b.WriteString(labelStyle.Render("Created: "))
	b.WriteString(valueStyle.Render(h.currentEntry.CreatedAt.Format(time.RFC1123)))
	b.WriteString("\n\n")

	b.WriteString(labelStyle.Render("Last Updated: "))
	b.WriteString(valueStyle.Render(h.currentEntry.UpdatedAt.Format(time.RFC1123)))
	b.WriteString("\n\n")

	b.WriteString(helpStyle.Render("o: Open Project • Esc/q: Back to list"))

	return b.String()
}

// Init initializes the overlay
func (h *HistoryBrowserOverlay) Init() tea.Cmd {
	return nil
}

// truncateString truncates a string to a maximum length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
