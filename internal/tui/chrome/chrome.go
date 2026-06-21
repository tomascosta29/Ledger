// Package chrome owns the visual layout that wraps every screen:
// the sidebar, header, mode-aware footer, and statusbar. Screens
// render their own data; chrome wraps that data with the frame the
// operator sees at every width and every mode.
//
// The exported entry point is Layout. App.View() composes the screen
// body's string and hands it to Layout with the current sidebar
// entries, footer hints, and status payload.
package chrome

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/tomascosta29/Ledger/internal/tui/components"
	"github.com/tomascosta29/Ledger/internal/tui/hints"
	"github.com/tomascosta29/Ledger/internal/tui/styles"
)

// Glyphs — Unicode characters that make up the visual chrome.
// Defined in styles package; aliases here for back-compat.
const (
	CursorGlyph       = styles.CursorGlyph
	ActiveScreenGlyph = styles.ActiveScreenGlyph
	SelectionGlyph    = styles.SelectionGlyph
	InactiveSelGlyph  = styles.InactiveSelGlyph
	RuleChar          = styles.RuleChar
	BulletChar        = styles.BulletChar
)

// MinSidebarWidth is the terminal width below which the sidebar is
// hidden and the screen title is folded into the header as a
// breadcrumb. Below this width the content area cannot afford the
// 14 cols the sidebar consumes.
const MinSidebarWidth = 80

// SidebarEntry is one row in the navigation column. Add fields here
// (e.g., Counter, Badge) when the per-screen counts feature is
// reintroduced — see ADR 0008 "When this should be revisited".
type SidebarEntry struct {
	Name   string
	Active bool
}

// FooterHints is the mode-aware footer payload. Each screen's
// View() assembles this from its current state.
//
// This type used to live in chrome; it moved to internal/tui/hints
// so screens can return it without importing chrome.
type FooterHints = hints.FooterHints

// KeyHint is one binding shown in the footer. See hints.KeyHint.
type KeyHint = hints.KeyHint

// Layout renders the full chrome: header, sidebar (if width permits),
// body, footer, statusbar. Height must be at least 4 for meaningful
// output; below that the body is returned unchanged.
func Layout(
	entries []SidebarEntry,
	currentTitle string,
	body string,
	hints FooterHints,
	status components.Status,
	width, height int,
) string {
	if height < 4 || width < 10 {
		return body
	}
	contentH := height - 3 // header + footer + statusbar

	var sidebar string
	bodyW := width
	if width >= MinSidebarWidth {
		sideW := computeSidebarWidth(entries)
		sidebar = Sidebar(entries, sideW, contentH)
		bodyW = width - sideW
	}

	body = padToHeight(body, contentH)
	body = lipgloss.NewStyle().Width(bodyW).Render(body)
	combined := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, body)

	header := Header(currentTitle, sidebar == "", width)
	footer := Footer(hints, width)
	sbar := components.StatusBar(status)

	return lipgloss.JoinVertical(lipgloss.Left, header, combined, footer, sbar)
}

func padToHeight(s string, height int) string {
	lines := strings.Split(s, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

// Sidebar renders the navigation column. Each line is exactly
// `width` cells wide; the last cell is a vertical rule character.
// `width` should be >= max(name) + 3 (marker + border); computeSidebarWidth
// returns a safe value.
func Sidebar(entries []SidebarEntry, width, height int) string {
	if width < 4 || height < 1 {
		return ""
	}
	contentW := width - 1 // reserve last col for the right border
	markerW := 2
	nameW := contentW - markerW
	if nameW < 1 {
		nameW = 1
	}

	var lines []string
	for _, e := range entries {
		marker := "  "
		if e.Active {
			marker = ActiveScreenGlyph + " "
		}
		// Truncate name to nameW so a future longer screen name
		// does not break the right border alignment.
		name := e.Name
		if len(name) > nameW {
			name = name[:nameW]
		}
		padded := strings.Repeat(" ", nameW-len(name))

		var styled string
		if e.Active {
			styled = marker + styles.SidebarActive.Render(name) + padded
		} else {
			styled = marker + styles.SidebarInactive.Render(name) + padded
		}
		styled = lipgloss.NewStyle().Width(contentW).Render(styled)
		lines = append(lines, styled+styles.SidebarBorder.Render("│"))
	}
	// Pad to the requested height with empty sidebar rows.
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

// SidebarWidth returns the column width the sidebar will occupy
// when rendered with the given entries. Useful for callers that
// need to compute the content area width before calling Layout.
func SidebarWidth(entries []SidebarEntry) int {
	return computeSidebarWidth(entries)
}

func computeSidebarWidth(entries []SidebarEntry) int {
	maxName := 0
	for _, e := range entries {
		if len(e.Name) > maxName {
			maxName = len(e.Name)
		}
	}
	// marker (2) + name + right border (1)
	w := maxName + 3
	if w < 12 {
		w = 12
	}
	return w
}

// Header renders the top rule. When the sidebar is hidden (narrow
// mode), the current screen title is prepended as a breadcrumb so
// the operator still knows where they are.
func Header(title string, narrow bool, width int) string {
	if width < 3 {
		return ""
	}
	rule := styles.HeaderRule.Render(strings.Repeat(RuleChar, width))
	if !narrow {
		return rule
	}
	styled := styles.HeaderText.Render(title)
	restW := width - lipgloss.Width(styled) - 2
	if restW < 1 {
		restW = 1
	}
	return " " + styled + " " + styles.HeaderRule.Render(strings.Repeat(RuleChar, restW))
}

// Footer renders the mode label on the left and key hints on the
// right, separated by whitespace. Single-line. Truncates by
// dropping trailing key hints if the line would overflow width.
func Footer(hints FooterHints, width int) string {
	if width < 3 {
		return ""
	}
	mode := styles.FooterMode.Render(hints.Mode)
	left := " " + mode

	rightParts := make([]string, 0, len(hints.Keys))
	for _, k := range hints.Keys {
		var label string
		if k.Count > 0 {
			count := styles.FooterKeyCount.Render(fmt.Sprintf("%d", k.Count))
			label = fmt.Sprintf("%s %s %s",
				styles.FooterKey.Render(k.Key),
				styles.FooterKey.Render(k.Label),
				count,
			)
		} else {
			label = fmt.Sprintf("%s %s",
				styles.FooterKey.Render(k.Key),
				styles.FooterKey.Render(k.Label),
			)
		}
		rightParts = append(rightParts, label)
	}
	right := " " + strings.Join(rightParts, " "+BulletChar+" ") + " "

	// Drop trailing key hints if the line would overflow width.
	for len(rightParts) > 0 {
		gap := width - lipgloss.Width(left) - lipgloss.Width(right)
		if gap >= 1 {
			break
		}
		rightParts = rightParts[:len(rightParts)-1]
		right = " " + strings.Join(rightParts, " "+BulletChar+" ") + " "
	}

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}
