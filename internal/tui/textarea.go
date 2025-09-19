// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package tui

import (
	"errors"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

// TextArea represents a multiline text input prompt using bubbles textarea
type TextArea struct {
	message      string
	defaultValue string
	textarea     textarea.Model
	focused      bool
	error        string
	cancelled    bool
}

// NewTextArea creates a new textarea prompt
func NewTextArea(message, defaultValue string) *TextArea {
	ta := textarea.New()
	ta.Placeholder = "Enter your text here... (ESC to cancel, Ctrl+D to submit)"
	ta.SetWidth(80)
	ta.SetHeight(8)
	ta.Focus()
	ta.SetValue(defaultValue)

	// Custom key bindings - disable the default submit on enter
	ta.KeyMap.InsertNewline.SetEnabled(true)

	return &TextArea{
		message:      message,
		defaultValue: defaultValue,
		textarea:     ta,
		focused:      true,
	}
}

func (t *TextArea) Message() string      { return t.message }
func (t *TextArea) Default() interface{} { return t.defaultValue }
func (t *TextArea) SetError(err string)  { t.error = err }
func (t *TextArea) SetFocused(focused bool) {
	t.focused = focused
	if focused {
		t.textarea.Focus()
	} else {
		t.textarea.Blur()
	}
}

// Value returns the current value
func (t *TextArea) Value() interface{} {
	if t.cancelled {
		return nil
	}
	return strings.TrimSpace(t.textarea.Value())
}

// IsCancelled returns true if the user pressed ESC
func (t *TextArea) IsCancelled() bool {
	return t.cancelled
}

func (t *TextArea) Update(msg tea.Msg) (Prompt, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			// User wants to cancel
			t.cancelled = true
			return t, nil
		case "ctrl+d":
			// User wants to submit (alternative to enter since enter adds newlines)
			return t, nil
		}
	}

	var cmd tea.Cmd
	t.textarea, cmd = t.textarea.Update(msg)
	return t, cmd
}

func (t *TextArea) Render() string {
	var b strings.Builder

	// Question message
	style := blurredStyle
	if t.focused {
		style = focusedStyle
	}
	b.WriteString(style.Render(t.message))
	b.WriteString("\n")

	// Instructions
	if t.focused {
		b.WriteString(helpStyle.Render("  Use Ctrl+D to submit, ESC to cancel"))
		b.WriteString("\n")
	}

	// TextArea
	b.WriteString(t.textarea.View())

	// Error message
	if t.error != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render("✗ " + t.error))
	}

	return b.String()
}

// textAreaModel is a standalone model for textarea input that doesn't interfere with questionnaire workflow
type textAreaModel struct {
	textarea  textarea.Model
	message   string
	submitted bool
	cancelled bool
	err       string
}

func newTextAreaModel(message string) textAreaModel {
	ta := textarea.New()
	ta.Placeholder = "Enter your text here... (ESC to cancel, Ctrl+D to submit)"
	ta.SetWidth(80)
	ta.SetHeight(8)
	ta.Focus()

	return textAreaModel{
		textarea: ta,
		message:  message,
	}
}

func (m textAreaModel) Init() tea.Cmd {
	return textarea.Blink
}

func (m textAreaModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.cancelled = true
			return m, tea.Quit
		case "ctrl+d":
			m.submitted = true
			return m, tea.Quit
		case "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

func (m textAreaModel) View() string {
	var b strings.Builder

	// Question message
	b.WriteString(focusedStyle.Render(m.message))
	b.WriteString("\n")

	// Instructions
	b.WriteString(helpStyle.Render("  Use Ctrl+D to submit, ESC to cancel"))
	b.WriteString("\n\n")

	// TextArea
	b.WriteString(m.textarea.View())

	// Error message
	if m.err != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render("✗ " + m.err))
	}

	return b.String()
}

// AskTextArea runs a standalone textarea dialog that doesn't interfere with questionnaire workflow
func AskTextArea(message string) (string, error) {
	model := newTextAreaModel(message)
	program := tea.NewProgram(model)

	finalModel, err := program.Run()
	if err != nil {
		return "", err
	}

	result := finalModel.(textAreaModel)
	if result.cancelled {
		return "", ErrCancelled
	}

	if result.submitted {
		return strings.TrimSpace(result.textarea.Value()), nil
	}

	return "", ErrCancelled
}

// ErrCancelled is returned when user cancels the textarea dialog
var ErrCancelled = errors.New("cancelled by user")
