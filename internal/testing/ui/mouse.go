package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// A test builds every mouse message through these helpers, for the reason
// keys.go gives for keys. The cell is a terminal cell, (0, 0) the first.

// Click is a press of button on the cell.
func Click(x, y int, button tea.MouseButton) tea.MouseMsg {
	return tea.MouseClickMsg{X: x, Y: y, Button: button}
}

// Release is the release of the left button on the cell.
func Release(x, y int) tea.MouseMsg {
	return tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft}
}

// Move is the pointer moving onto the cell with no button held.
func Move(x, y int) tea.MouseMsg {
	return tea.MouseMotionMsg{X: x, Y: y, Button: tea.MouseNone}
}

// Drag is the pointer moving onto the cell with the left button held.
func Drag(x, y int) tea.MouseMsg {
	return tea.MouseMotionMsg{X: x, Y: y, Button: tea.MouseLeft}
}

// Wheel is a notch of the wheel over the cell; button is tea.MouseWheelUp or
// tea.MouseWheelDown.
func Wheel(x, y int, button tea.MouseButton) tea.MouseMsg {
	return tea.MouseWheelMsg{X: x, Y: y, Button: button}
}

// Offset returns msg moved dx cells right and dy cells down.
func Offset(msg tea.MouseMsg, dx, dy int) tea.MouseMsg {
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		msg.X, msg.Y = msg.X+dx, msg.Y+dy
		return msg
	case tea.MouseReleaseMsg:
		msg.X, msg.Y = msg.X+dx, msg.Y+dy
		return msg
	case tea.MouseMotionMsg:
		msg.X, msg.Y = msg.X+dx, msg.Y+dy
		return msg
	case tea.MouseWheelMsg:
		msg.X, msg.Y = msg.X+dx, msg.Y+dy
		return msg
	}
	panic(fmt.Sprintf("testing/ui.Offset: unknown mouse message %T", msg))
}
