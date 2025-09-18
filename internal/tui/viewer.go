// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Viewer represents a scrollable text viewer
type Viewer struct {
	title    string
	content  string
	lines    []string
	viewport int
	offset   int
	hoffset  int // horizontal offset for wide content
	width    int
	height   int
	maxLines int
	maxWidth int
	finished bool
}

// NewViewer creates a new scrollable text viewer
func NewViewer(title, content string) *Viewer {
	lines := strings.Split(content, "\n")

	// Calculate maximum line width for horizontal scrolling
	maxWidth := 0
	for _, line := range lines {
		if len(line) > maxWidth {
			maxWidth = len(line)
		}
	}

	return &Viewer{
		title:    title,
		content:  content,
		lines:    lines,
		maxLines: len(lines),
		maxWidth: maxWidth,
		width:    80,
		height:   20,
		viewport: 15, // Leave space for header and footer
	}
}

// ViewerModel is the bubbletea model for the viewer
type ViewerModel struct {
	viewer *Viewer
}

func NewViewerModel(viewer *Viewer) *ViewerModel {
	return &ViewerModel{viewer: viewer}
}

func (m *ViewerModel) Init() tea.Cmd {
	return nil
}

func (m *ViewerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.viewer.width = msg.Width
		m.viewer.height = msg.Height
		m.viewer.viewport = msg.Height - 6 // Leave space for header, footer, and instructions
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "enter":
			m.viewer.finished = true
			return m, tea.Quit

		// Single line navigation
		case "up", "k":
			if m.viewer.offset > 0 {
				m.viewer.offset--
			}

		case "down", "j":
			maxOffset := m.viewer.maxLines - m.viewer.viewport
			if maxOffset < 0 {
				maxOffset = 0
			}
			if m.viewer.offset < maxOffset {
				m.viewer.offset++
			}

		// Full page navigation (vim/less style)
		case "pgup", "ctrl+b", "b":
			m.viewer.offset -= m.viewer.viewport
			if m.viewer.offset < 0 {
				m.viewer.offset = 0
			}

		case "pgdown", "ctrl+f", "f", " ":
			maxOffset := m.viewer.maxLines - m.viewer.viewport
			if maxOffset < 0 {
				maxOffset = 0
			}
			m.viewer.offset += m.viewer.viewport
			if m.viewer.offset > maxOffset {
				m.viewer.offset = maxOffset
			}

		// Half page navigation
		case "ctrl+d", "d":
			halfPage := m.viewer.viewport / 2
			if halfPage < 1 {
				halfPage = 1
			}
			maxOffset := m.viewer.maxLines - m.viewer.viewport
			if maxOffset < 0 {
				maxOffset = 0
			}
			m.viewer.offset += halfPage
			if m.viewer.offset > maxOffset {
				m.viewer.offset = maxOffset
			}

		case "ctrl+u", "u":
			halfPage := m.viewer.viewport / 2
			if halfPage < 1 {
				halfPage = 1
			}
			m.viewer.offset -= halfPage
			if m.viewer.offset < 0 {
				m.viewer.offset = 0
			}

		// Horizontal navigation
		case "left", "h":
			if m.viewer.hoffset > 0 {
				m.viewer.hoffset--
			}

		case "right", "l":
			contentWidth := m.viewer.width - 8 // Account for border and padding
			maxHOffset := m.viewer.maxWidth - contentWidth
			if maxHOffset < 0 {
				maxHOffset = 0
			}
			if m.viewer.hoffset < maxHOffset {
				m.viewer.hoffset++
			}

		// Top/bottom navigation
		case "home", "g":
			m.viewer.offset = 0

		case "end", "G":
			maxOffset := m.viewer.maxLines - m.viewer.viewport
			if maxOffset < 0 {
				maxOffset = 0
			}
			m.viewer.offset = maxOffset
		}
	}

	return m, nil
}

func (m *ViewerModel) View() string {
	var b strings.Builder

	// Header with title and scroll position
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")).  // Bright white text
		Background(lipgloss.Color("27")).  // Blue background
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("86")).
		BorderBottom(true).
		Width(m.viewer.width - 2).  // Account for border
		Padding(0, 1).              // Add horizontal padding
		Align(lipgloss.Center)

	scrollInfo := ""
	if m.viewer.maxLines > m.viewer.viewport {
		lineStart := m.viewer.offset + 1
		lineEnd := m.viewer.offset + m.viewer.viewport
		if lineEnd > m.viewer.maxLines {
			lineEnd = m.viewer.maxLines
		}
		scrollInfo = fmt.Sprintf(" | Lines %d-%d of %d", lineStart, lineEnd, m.viewer.maxLines)
	}

	// Add horizontal position if content is wider than viewport and calculate content width
	contentWidth := m.viewer.width - 8
	if m.viewer.maxWidth > contentWidth {
		hPos := m.viewer.hoffset + 1
		scrollInfo += fmt.Sprintf(" | Col %d", hPos)
	}

	// Separate title from scroll info for better formatting
	titleText := m.viewer.title
	if scrollInfo != "" {
		titleText = fmt.Sprintf("%s %s", m.viewer.title, scrollInfo)
	}
	
	// Ensure title is not empty
	if titleText == "" {
		titleText = "Content Viewer"
	}
	
	b.WriteString(headerStyle.Render(titleText))
	b.WriteString("\n\n")

	// Content area
	contentStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1).
		Width(m.viewer.width - 4)

	var contentLines []string
	end := m.viewer.offset + m.viewer.viewport
	if end > m.viewer.maxLines {
		end = m.viewer.maxLines
	}
	for i := m.viewer.offset; i < end; i++ {
		line := m.viewer.lines[i]

		// Apply horizontal scrolling
		if m.viewer.hoffset > 0 && len(line) > m.viewer.hoffset {
			line = line[m.viewer.hoffset:]
		} else if m.viewer.hoffset > 0 {
			line = ""
		}

		// Truncate line if it's too wide
		if len(line) > contentWidth {
			line = line[:contentWidth]
		}

		contentLines = append(contentLines, line)
	}

	// Pad with empty lines if needed
	for len(contentLines) < m.viewer.viewport {
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

// ShowContent displays content in a scrollable viewer and waits for user to close it
func ShowContent(title, content string) error {
	viewer := NewViewer(title, content)
	model := NewViewerModel(viewer)
	program := tea.NewProgram(model)

	_, err := program.Run()
	if err != nil {
		return err
	}

	return nil
}
