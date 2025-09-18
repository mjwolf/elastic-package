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
	title       string
	content     string
	lines       []string
	viewport    int
	offset      int
	width       int
	height      int
	maxLines    int
	finished    bool
}

// NewViewer creates a new scrollable text viewer
func NewViewer(title, content string) *Viewer {
	lines := strings.Split(content, "\n")
	return &Viewer{
		title:    title,
		content:  content,
		lines:    lines,
		maxLines: len(lines),
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

		case "pgup":
			m.viewer.offset -= m.viewer.viewport
			if m.viewer.offset < 0 {
				m.viewer.offset = 0
			}

		case "pgdown":
			maxOffset := m.viewer.maxLines - m.viewer.viewport
			if maxOffset < 0 {
				maxOffset = 0
			}
			m.viewer.offset += m.viewer.viewport
			if m.viewer.offset > maxOffset {
				m.viewer.offset = maxOffset
			}

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
		Foreground(lipgloss.Color("86")).
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		Width(m.viewer.width).
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

	title := m.viewer.title + scrollInfo
	b.WriteString(headerStyle.Render(title))
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
		contentLines = append(contentLines, m.viewer.lines[i])
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

	instructions := "↑↓/jk: scroll | PgUp/PgDn: page | Home/End: top/bottom | Enter/q/Esc: close"
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
