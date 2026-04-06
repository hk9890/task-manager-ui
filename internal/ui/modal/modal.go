// Package modal provides a reusable confirmation and input modal.
package modal

import (
	"strings"

	"github.com/hk9890/beads-workbench/internal/ui/overlay"
	"github.com/hk9890/beads-workbench/internal/ui/styles"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ButtonVariant controls confirm button styling.
type ButtonVariant int

const (
	ButtonPrimary ButtonVariant = iota
	ButtonDanger
)

// InputConfig defines one modal input field.
type InputConfig struct {
	Key         string
	Label       string
	Placeholder string
	Value       string
	MaxLength   int
}

// Config controls modal rendering and behavior.
type Config struct {
	Title          string
	Message        string
	Inputs         []InputConfig
	ConfirmVariant ButtonVariant
	ConfirmText    string
	CancelText     string
	MinWidth       int
	HideButtons    bool
	Required       bool
}

// SubmitMsg is emitted when confirm is triggered.
type SubmitMsg struct {
	Values map[string]string
}

// CancelMsg is emitted when cancel is triggered.
type CancelMsg struct{}

// Field identifies focused button field.
type Field int

const (
	FieldSave Field = iota
	FieldCancel
)

// KeyMap controls modal keyboard navigation.
type KeyMap struct {
	Next   []key.Binding
	Prev   []key.Binding
	Left   []key.Binding
	Right  []key.Binding
	Enter  []key.Binding
	Escape []key.Binding
}

// DefaultKeyMap provides sensible default bindings.
var DefaultKeyMap = KeyMap{
	Next: []key.Binding{
		key.NewBinding(key.WithKeys("tab")),
		key.NewBinding(key.WithKeys("down")),
	},
	Prev: []key.Binding{
		key.NewBinding(key.WithKeys("shift+tab")),
		key.NewBinding(key.WithKeys("up")),
	},
	Left:   []key.Binding{key.NewBinding(key.WithKeys("left"))},
	Right:  []key.Binding{key.NewBinding(key.WithKeys("right"))},
	Enter:  []key.Binding{key.NewBinding(key.WithKeys("enter"))},
	Escape: []key.Binding{key.NewBinding(key.WithKeys("esc"))},
}

// Model stores modal state.
type Model struct {
	config       Config
	keys         KeyMap
	inputs       []textinput.Model
	inputKeys    []string
	hasInputs    bool
	focusedInput int
	focusedField Field
	width        int
	height       int
}

// New creates a modal with default key bindings.
func New(cfg Config) Model {
	return NewWithKeys(cfg, DefaultKeyMap)
}

// NewWithKeys creates a modal with custom key bindings.
func NewWithKeys(cfg Config, km KeyMap) Model {
	m := Model{
		config:       cfg,
		keys:         km,
		hasInputs:    len(cfg.Inputs) > 0,
		focusedInput: 0,
		focusedField: FieldSave,
	}

	if m.hasInputs {
		m.inputs = make([]textinput.Model, len(cfg.Inputs))
		m.inputKeys = make([]string, len(cfg.Inputs))
		for i, inputCfg := range cfg.Inputs {
			ti := textinput.New()
			ti.Placeholder = inputCfg.Placeholder
			ti.Width = 36
			ti.Prompt = ""
			if inputCfg.MaxLength > 0 {
				ti.CharLimit = inputCfg.MaxLength
			}
			if inputCfg.Value != "" {
				ti.SetValue(inputCfg.Value)
			}
			if i == 0 {
				ti.Focus()
			}
			m.inputs[i] = ti
			m.inputKeys[i] = inputCfg.Key
		}
	} else {
		m.focusedInput = -1
	}

	return m
}

// Init returns initial cursor blink command for input mode.
func (m Model) Init() tea.Cmd {
	if m.hasInputs {
		return textinput.Blink
	}
	return nil
}

// Update handles keyboard and size messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Next...):
			m = m.nextField()
			return m, nil
		case key.Matches(msg, m.keys.Prev...):
			m = m.prevField()
			return m, nil
		case key.Matches(msg, m.keys.Left...):
			if m.focusedInput == -1 && m.focusedField == FieldCancel {
				m.focusedField = FieldSave
				return m, nil
			}
		case key.Matches(msg, m.keys.Right...):
			if m.focusedInput == -1 && m.focusedField == FieldSave {
				m.focusedField = FieldCancel
				return m, nil
			}
		case key.Matches(msg, m.keys.Enter...):
			if m.focusedInput >= 0 {
				m = m.nextField()
				return m, nil
			}
			if m.focusedField == FieldCancel {
				return m, func() tea.Msg { return CancelMsg{} }
			}
			if m.hasInputs && m.config.Required {
				for _, input := range m.inputs {
					if input.Value() == "" {
						return m, nil
					}
				}
			}
			return m, func() tea.Msg { return SubmitMsg{Values: m.values()} }
		case key.Matches(msg, m.keys.Escape...):
			if !m.config.Required {
				return m, func() tea.Msg { return CancelMsg{} }
			}
		case msg.String() == "y":
			if m.focusedInput == -1 || !m.hasInputs {
				if m.hasInputs && m.config.Required {
					for _, input := range m.inputs {
						if input.Value() == "" {
							return m, nil
						}
					}
				}
				return m, func() tea.Msg { return SubmitMsg{Values: m.values()} }
			}
		case msg.String() == "n":
			if m.focusedInput == -1 || !m.hasInputs {
				return m, func() tea.Msg { return CancelMsg{} }
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	if m.hasInputs && m.focusedInput >= 0 && m.focusedInput < len(m.inputs) {
		var cmd tea.Cmd
		m.inputs[m.focusedInput], cmd = m.inputs[m.focusedInput].Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) values() map[string]string {
	values := make(map[string]string, len(m.inputs))
	for i, input := range m.inputs {
		values[m.inputKeys[i]] = input.Value()
	}
	return values
}

func (m Model) nextField() Model {
	if m.focusedInput >= 0 {
		m.inputs[m.focusedInput].Blur()
		if m.focusedInput < len(m.inputs)-1 {
			m.focusedInput++
			m.inputs[m.focusedInput].Focus()
		} else {
			m.focusedInput = -1
			m.focusedField = FieldSave
		}
		return m
	}

	if m.focusedField == FieldSave {
		m.focusedField = FieldCancel
		return m
	}

	if m.hasInputs {
		m.focusedInput = 0
		m.inputs[0].Focus()
	} else {
		m.focusedField = FieldSave
	}

	return m
}

func (m Model) prevField() Model {
	if m.focusedInput >= 0 {
		m.inputs[m.focusedInput].Blur()
		if m.focusedInput > 0 {
			m.focusedInput--
			m.inputs[m.focusedInput].Focus()
		} else {
			m.focusedInput = -1
			m.focusedField = FieldCancel
		}
		return m
	}

	if m.focusedField == FieldCancel {
		m.focusedField = FieldSave
		return m
	}

	if m.hasInputs {
		m.focusedInput = len(m.inputs) - 1
		m.inputs[m.focusedInput].Focus()
	} else {
		m.focusedField = FieldCancel
	}

	return m
}

// View renders the modal content without a background overlay.
func (m Model) View() string {
	minWidth := maxInt(40, m.config.MinWidth)
	contentWidth := maxInt(minWidth, lipgloss.Width(m.config.Title))
	boxWidth := contentWidth + 2

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.OverlayTitleColor).
		PaddingLeft(1)

	divider := lipgloss.NewStyle().
		Foreground(styles.OverlayBorderColor).
		Render(strings.Repeat("─", boxWidth))

	var content strings.Builder
	if m.config.Message != "" {
		msg := lipgloss.NewStyle().
			Foreground(styles.TextPrimaryColor).
			Width(contentWidth).
			Render(m.config.Message)
		content.WriteString(msg)
		content.WriteString("\n\n")
	}

	for i, inputCfg := range m.config.Inputs {
		content.WriteString(m.renderInputSection(i, inputCfg.Label, contentWidth))
		content.WriteString("\n\n")
	}

	if !m.config.HideButtons {
		content.WriteString(m.renderButtons())
	}

	body := titleStyle.Render(m.config.Title) + "\n" + divider + "\n" + lipgloss.NewStyle().Padding(1, 1).Render(content.String())
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.OverlayBorderColor).
		Width(boxWidth).
		Render(body)
}

func (m Model) renderInputSection(index int, label string, width int) string {
	if label == "" {
		label = "Input"
	}
	return styles.FormSection(styles.FormSectionConfig{
		Content:            []string{m.inputs[index].View()},
		Width:              width,
		TopLeft:            label,
		Focused:            m.focusedInput == index,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})
}

func (m Model) renderButtons() string {
	onButtons := m.focusedInput == -1

	saveStyle := styles.PrimaryButtonStyle
	if m.config.ConfirmVariant == ButtonDanger {
		saveStyle = styles.DangerButtonStyle
		if onButtons && m.focusedField == FieldSave {
			saveStyle = styles.DangerButtonFocusedStyle
		}
	} else if onButtons && m.focusedField == FieldSave {
		saveStyle = styles.PrimaryButtonFocusedStyle
	}

	saveLabel := m.config.ConfirmText
	if saveLabel == "" {
		if m.hasInputs {
			saveLabel = "Save"
		} else {
			saveLabel = "Confirm"
		}
	}

	cancelStyle := styles.SecondaryButtonStyle
	if onButtons && m.focusedField == FieldCancel {
		cancelStyle = styles.SecondaryButtonFocusedStyle
	}
	cancelLabel := m.config.CancelText
	if cancelLabel == "" {
		cancelLabel = "Cancel"
	}

	return saveStyle.Render(saveLabel) + "  " + cancelStyle.Render(cancelLabel)
}

// Overlay centers the modal over a background.
func (m Model) Overlay(bg string) string {
	return overlay.Place(overlay.Config{
		Width:    m.width,
		Height:   m.height,
		Position: overlay.Center,
	}, m.View(), bg)
}

// SetSize updates viewport dimensions used for Overlay.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// FocusedInput returns the focused input index; -1 means buttons.
func (m Model) FocusedInput() int { return m.focusedInput }

// FocusedField returns the focused button field.
func (m Model) FocusedField() Field { return m.focusedField }

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
