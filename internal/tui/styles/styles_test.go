package styles

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestStylesRender(t *testing.T) {
	cases := []struct {
		name  string
		style lipgloss.Style
	}{
		{"HeaderText", HeaderText},
		{"HeaderRule", HeaderRule},
		{"AmountOut", AmountOut},
		{"AmountIn", AmountIn},
		{"AmountZero", AmountZero},
		{"UnknownCategory", UnknownCategory},
		{"SidebarActive", SidebarActive},
		{"SidebarInactive", SidebarInactive},
		{"CursorRow", CursorRow},
		{"SelectedRow", SelectedRow},
		{"CursorSelectedRow", CursorSelectedRow},
		{"FooterMode", FooterMode},
		{"FooterKey", FooterKey},
		{"FooterKeyCount", FooterKeyCount},
		{"SidebarBorder", SidebarBorder},
		{"StatusBarBg", StatusBarBg},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.style.Render("x")
			if got == "" {
				t.Errorf("%s.Render(\"x\") returned empty string", c.name)
			}
			if lipgloss.Width(got) != 1 {
				t.Errorf("%s: want width 1, got %d (rendered=%q)", c.name, lipgloss.Width(got), got)
			}
		})
	}
}

func TestColorTokens(t *testing.T) {
	// Sanity: tokens are non-empty strings.
	tokens := map[string]lipgloss.Color{
		"Accent":  Accent,
		"Out":     Out,
		"In":      In,
		"Dim":     Dim,
		"Warn":    Warn,
		"Mute":    Mute,
		"Strong":  Strong,
		"Surface": Surface,
	}
	for name, c := range tokens {
		if string(c) == "" {
			t.Errorf("%s color token is empty", name)
		}
	}
}
