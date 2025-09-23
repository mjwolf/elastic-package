// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package tui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TextComponentMode determines if the component is read-only or editable
type TextComponentMode int

const (
	ViewMode TextComponentMode = iota
	EditMode
)

// TextComponent represents a unified text display/input component that can be read-only or editable
type TextComponent struct {
	title        string
	content      string
	mode         TextComponentMode
	message      string
	defaultValue string

	// View mode fields
	lines    []string
	viewport int
	offset   int
	hoffset  int // horizontal offset for wide content
	width    int
	height   int
	maxLines int
	maxWidth int

	// Edit mode fields
	textarea textarea.Model
	focused  bool
	error    string

	// Common fields
	submitted bool
	cancelled bool
	finished  bool
}

// NewTextComponent creates a new text component in the specified mode
func NewTextComponent(mode TextComponentMode, title, content string) *TextComponent {
	tc := &TextComponent{
		title:   title,
		content: content,
		mode:    mode,
		width:   80,
		height:  24,
	}

	if mode == ViewMode {
		tc.initViewMode()
	} else {
		tc.initEditMode()
	}

	return tc
}

// NewTextComponentForEdit creates a new text component for editing with a message and default value
func NewTextComponentForEdit(message, defaultValue string) *TextComponent {
	tc := &TextComponent{
		mode:         EditMode,
		message:      message,
		defaultValue: defaultValue,
		content:      defaultValue,
		focused:      true,
		width:        80,
		height:       24,
	}

	tc.initEditMode()
	return tc
}

func (tc *TextComponent) initViewMode() {
	tc.lines = strings.Split(tc.content, "\n")
	tc.maxLines = len(tc.lines)
	tc.viewport = 18 // Leave space for header and footer

	// Calculate maximum line width for horizontal scrolling
	tc.maxWidth = 0
	for _, line := range tc.lines {
		if len(line) > tc.maxWidth {
			tc.maxWidth = len(line)
		}
	}
}

func (tc *TextComponent) initEditMode() {
	ta := textarea.New()
	ta.Placeholder = "Enter your text here... (ESC to cancel, Ctrl+D to submit)"
	ta.SetWidth(80)
	ta.SetHeight(8)
	ta.Focus()
	ta.SetValue(tc.content)

	// Custom key bindings - disable the default submit on enter
	ta.KeyMap.InsertNewline.SetEnabled(true)

	tc.textarea = ta
}

// TextComponentModel is the bubbletea model for the unified text component
type TextComponentModel struct {
	component *TextComponent
}

// NewTextComponentModel creates a new model for the text component
func NewTextComponentModel(component *TextComponent) *TextComponentModel {
	return &TextComponentModel{component: component}
}

func (m *TextComponentModel) Init() tea.Cmd {
	if m.component.mode == EditMode {
		return textarea.Blink
	}
	return tea.EnterAltScreen
}

func (m *TextComponentModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.component.width = msg.Width
		m.component.height = msg.Height
		if m.component.mode == ViewMode {
			// Leave more space for header, content borders, footer, and instructions
			m.component.viewport = msg.Height - 8
			if m.component.viewport < 1 {
				m.component.viewport = 1
			}
		}
		return m, nil

	case tea.KeyMsg:
		if m.component.mode == ViewMode {
			return m.updateViewMode(msg)
		} else {
			return m.updateEditMode(msg)
		}
	}

	// For edit mode, update the textarea
	if m.component.mode == EditMode {
		var cmd tea.Cmd
		m.component.textarea, cmd = m.component.textarea.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *TextComponentModel) updateViewMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "enter":
		m.component.finished = true
		return m, tea.Quit

	// Single line navigation
	case "up", "k":
		if m.component.offset > 0 {
			m.component.offset--
		}

	case "down", "j":
		maxOffset := m.component.maxLines - m.component.viewport
		if maxOffset < 0 {
			maxOffset = 0
		}
		if m.component.offset < maxOffset {
			m.component.offset++
		}

	// Full page navigation (vim/less style)
	case "pgup", "ctrl+b", "b":
		m.component.offset -= m.component.viewport
		if m.component.offset < 0 {
			m.component.offset = 0
		}

	case "pgdown", "ctrl+f", "f", " ":
		maxOffset := m.component.maxLines - m.component.viewport
		if maxOffset < 0 {
			maxOffset = 0
		}
		m.component.offset += m.component.viewport
		if m.component.offset > maxOffset {
			m.component.offset = maxOffset
		}

	// Half page navigation
	case "ctrl+d", "d":
		halfPage := m.component.viewport / 2
		if halfPage < 1 {
			halfPage = 1
		}
		maxOffset := m.component.maxLines - m.component.viewport
		if maxOffset < 0 {
			maxOffset = 0
		}
		m.component.offset += halfPage
		if m.component.offset > maxOffset {
			m.component.offset = maxOffset
		}

	case "ctrl+u", "u":
		halfPage := m.component.viewport / 2
		if halfPage < 1 {
			halfPage = 1
		}
		m.component.offset -= halfPage
		if m.component.offset < 0 {
			m.component.offset = 0
		}

	// Horizontal navigation
	case "left", "h":
		if m.component.hoffset > 0 {
			m.component.hoffset--
		}

	case "right", "l":
		contentWidth := m.component.width - 8 // Account for border and padding
		maxHOffset := m.component.maxWidth - contentWidth
		if maxHOffset < 0 {
			maxHOffset = 0
		}
		if m.component.hoffset < maxHOffset {
			m.component.hoffset++
		}

	// Top/bottom navigation
	case "home", "g":
		m.component.offset = 0

	case "end", "G":
		maxOffset := m.component.maxLines - m.component.viewport
		if maxOffset < 0 {
			maxOffset = 0
		}
		m.component.offset = maxOffset
	}

	return m, nil
}

func (m *TextComponentModel) updateEditMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// User wants to cancel
		m.component.cancelled = true
		return m, tea.Quit
	case "ctrl+d":
		// User wants to submit (alternative to enter since enter adds newlines)
		m.component.submitted = true
		return m, tea.Quit
	case "ctrl+c":
		m.component.cancelled = true
		return m, tea.Quit
	}

	return m, nil
}

func (m *TextComponentModel) View() string {
	if m.component.mode == ViewMode {
		return m.viewModeRender()
	} else {
		return m.editModeRender()
	}
}

func (m *TextComponentModel) viewModeRender() string {
	var b strings.Builder

	// Header with title and scroll position
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")). // Bright white text
		Background(lipgloss.Color("27")). // Blue background
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("86")).
		BorderBottom(true).
		Width(m.component.width-4). // Account for border and padding
		MarginBottom(1).            // Add space after header
		Padding(0, 2).              // Add horizontal padding
		Align(lipgloss.Center)

	scrollInfo := ""
	if m.component.maxLines > m.component.viewport {
		lineStart := m.component.offset + 1
		lineEnd := m.component.offset + m.component.viewport
		if lineEnd > m.component.maxLines {
			lineEnd = m.component.maxLines
		}
		scrollInfo = fmt.Sprintf(" | Lines %d-%d of %d", lineStart, lineEnd, m.component.maxLines)
	}

	// Add horizontal position if content is wider than viewport
	contentWidth := m.component.width - 8
	if m.component.maxWidth > contentWidth {
		hPos := m.component.hoffset + 1
		scrollInfo += fmt.Sprintf(" | Col %d", hPos)
	}

	titleText := m.component.title
	if scrollInfo != "" {
		titleText = fmt.Sprintf("%s%s", m.component.title, scrollInfo)
	}

	// Ensure title is not empty
	if titleText == "" {
		titleText = "Content Viewer"
	}

	b.WriteString(headerStyle.Render(titleText))
	b.WriteString("\n")

	// Content area
	contentStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1).
		Width(m.component.width - 4)

	var contentLines []string
	end := m.component.offset + m.component.viewport
	if end > m.component.maxLines {
		end = m.component.maxLines
	}
	for i := m.component.offset; i < end; i++ {
		line := m.component.lines[i]

		// Apply horizontal scrolling
		if m.component.hoffset > 0 && len(line) > m.component.hoffset {
			line = line[m.component.hoffset:]
		} else if m.component.hoffset > 0 {
			line = ""
		}

		// Truncate line if it's too wide
		if len(line) > contentWidth {
			line = line[:contentWidth]
		}

		contentLines = append(contentLines, line)
	}

	// Pad with empty lines if needed
	for len(contentLines) < m.component.viewport {
		contentLines = append(contentLines, "")
	}

	content := strings.Join(contentLines, "\n")
	b.WriteString(contentStyle.Render(content))

	// Footer instructions
	b.WriteString("\n")
	instructionsStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Italic(true)

	instructions := "↑↓/jk: line | ←→/hl: scroll | PgUp/PgDn/Ctrl+B/Ctrl+F/b/f/Space: page | d/u: half page | Home/End/g/G: top/bottom | Enter/q/Esc: close"
	b.WriteString(instructionsStyle.Render(instructions))

	return b.String()
}

func (m *TextComponentModel) editModeRender() string {
	var b strings.Builder

	// Question message
	style := blurredStyle
	if m.component.focused {
		style = focusedStyle
	}
	b.WriteString(style.Render(m.component.message))
	b.WriteString("\n")

	// Instructions
	if m.component.focused {
		b.WriteString(helpStyle.Render("  Use Ctrl+D to submit, ESC to cancel"))
		b.WriteString("\n\n")
	}

	// TextArea
	b.WriteString(m.component.textarea.View())

	// Error message
	if m.component.error != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render("✗ " + m.component.error))
	}

	return b.String()
}

// Value returns the current value for edit mode
func (tc *TextComponent) Value() interface{} {
	if tc.cancelled {
		return nil
	}
	if tc.mode == EditMode {
		return strings.TrimSpace(tc.textarea.Value())
	}
	return tc.content
}

// IsCancelled returns true if the user pressed ESC
func (tc *TextComponent) IsCancelled() bool {
	return tc.cancelled
}

// IsSubmitted returns true if the user submitted in edit mode
func (tc *TextComponent) IsSubmitted() bool {
	return tc.submitted
}

// ShowContent displays content in a scrollable viewer and waits for user to close it
func ShowContent(title, content string) error {
	component := NewTextComponent(ViewMode, title, content)
	model := NewTextComponentModel(component)

	// Enable mouse support and alternate screen for better display
	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	_, err := program.Run()
	if err != nil {
		return err
	}

	return nil
}

// AskTextArea runs a text area dialog for multi-line input
func AskTextArea(message string) (string, error) {
	component := NewTextComponentForEdit(message, "")
	model := NewTextComponentModel(component)
	program := tea.NewProgram(model)

	finalModel, err := program.Run()
	if err != nil {
		return "", err
	}

	result := finalModel.(*TextComponentModel).component
	if result.cancelled {
		return "", ErrCancelled
	}

	if result.submitted {
		return strings.TrimSpace(result.textarea.Value()), nil
	}

	return "", ErrCancelled
}

// ErrCancelled is returned when user cancels the dialog
var ErrCancelled = errors.New("cancelled by user")

// TextArea represents a multiline text input prompt that implements the Prompt interface
// This is a wrapper around TextComponent for compatibility with the questionnaire system
type TextArea struct {
	component *TextComponent
}

// NewTextArea creates a new textarea prompt for the questionnaire system
func NewTextArea(message, defaultValue string) *TextArea {
	component := NewTextComponentForEdit(message, defaultValue)
	return &TextArea{
		component: component,
	}
}

func (t *TextArea) Message() string      { return t.component.message }
func (t *TextArea) Default() interface{} { return t.component.defaultValue }
func (t *TextArea) SetError(err string)  { t.component.error = err }
func (t *TextArea) SetFocused(focused bool) {
	t.component.focused = focused
	if focused {
		t.component.textarea.Focus()
	} else {
		t.component.textarea.Blur()
	}
}

// Value returns the current value
func (t *TextArea) Value() interface{} {
	if t.component.cancelled {
		return nil
	}
	return strings.TrimSpace(t.component.textarea.Value())
}

// IsCancelled returns true if the user pressed ESC
func (t *TextArea) IsCancelled() bool {
	return t.component.cancelled
}

func (t *TextArea) Update(msg tea.Msg) (Prompt, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			// User wants to cancel
			t.component.cancelled = true
			return t, nil
		case "ctrl+d":
			// User wants to submit (alternative to enter since enter adds newlines)
			return t, nil
		}
	}

	var cmd tea.Cmd
	t.component.textarea, cmd = t.component.textarea.Update(msg)
	return t, cmd
}

func (t *TextArea) Render() string {
	var b strings.Builder

	// Question message
	style := blurredStyle
	if t.component.focused {
		style = focusedStyle
	}
	b.WriteString(style.Render(t.component.message))
	b.WriteString("\n")

	// Instructions
	if t.component.focused {
		b.WriteString(helpStyle.Render("  Use Ctrl+D to submit, ESC to cancel"))
		b.WriteString("\n")
	}

	// TextArea
	b.WriteString(t.component.textarea.View())

	// Error message
	if t.component.error != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render("✗ " + t.component.error))
	}

	return b.String()
}
