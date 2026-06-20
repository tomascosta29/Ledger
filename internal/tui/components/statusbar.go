package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type Status struct {
	DBPath    string
	Screen    string
	Mode      string
	StatusMsg string
	Width     int
}

func StatusBar(s Status) string {
	db := truncate(s.DBPath, 40)
	left := fmt.Sprintf("  %s  %s  %s",
		screenStyle.Render(s.Screen),
		modeStyle.Render("["+s.Mode+"]"),
		statusDim.Render(db),
	)
	right := statusAccent.Render(s.StatusMsg)
	gap := s.Width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}
	return statusBarStyle.Width(s.Width).Render(left + strings.Repeat(" ", gap) + right)
}

func HintLine(width int) string {
	hints := "[1-5] screen  / filter  : cmd  ? help  q quit"
	return hintStyle.Width(width).Align(lipgloss.Right).Render(hints)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return "…" + s[len(s)-(n-1):]
}

var (
	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Background(lipgloss.Color("236"))
	screenStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("117")).
			Bold(true)
	modeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("223")).
			Bold(true)
	statusDim = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))
	statusAccent = lipgloss.NewStyle().
			Foreground(lipgloss.Color("215")).
			Bold(true)
	hintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
)
