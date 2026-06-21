package chrome

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/tomascosta29/Ledger/internal/tui/components"
)

// stripANSI removes SGR escape sequences so golden tests compare
// the visible characters only. Delegates to the official ansi
// package to handle all sequence forms correctly.
func stripANSI(s string) string {
	return ansi.Strip(s)
}

func TestSidebar_RendersActiveAndInactive(t *testing.T) {
	entries := []SidebarEntry{
		{Name: "Manager", Active: true},
		{Name: "Categorizer"},
		{Name: "Linker"},
		{Name: "Budget"},
		{Name: "Recipes"},
	}
	out := Sidebar(entries, 14, 5)
	stripped := stripANSI(out)
	lines := strings.Split(stripped, "\n")
	if len(lines) != 5 {
		t.Fatalf("want 5 lines, got %d:\n%s", len(lines), stripped)
	}
	want := []string{
		"► Manager    │",
		"  Categorizer│",
		"  Linker     │",
		"  Budget     │",
		"  Recipes    │",
	}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("line %d: want %q got %q", i, w, lines[i])
		}
	}
}

func TestSidebar_TooNarrowReturnsEmpty(t *testing.T) {
	entries := []SidebarEntry{{Name: "Manager", Active: true}}
	if got := Sidebar(entries, 3, 5); got != "" {
		t.Errorf("want empty for width<4, got %q", got)
	}
}

func TestSidebar_PadsToHeight(t *testing.T) {
	entries := []SidebarEntry{
		{Name: "Manager", Active: true},
		{Name: "Other"},
	}
	out := Sidebar(entries, 14, 6)
	stripped := stripANSI(out)
	lines := strings.Split(stripped, "\n")
	if len(lines) != 6 {
		t.Fatalf("want 6 lines after padding, got %d", len(lines))
	}
	// Last line is whitespace padding of full width.
	if lines[5] != strings.Repeat(" ", 14) {
		t.Errorf("padding line: want %q got %q", strings.Repeat(" ", 14), lines[5])
	}
}

func TestHeader_WideIsJustRule(t *testing.T) {
	out := Header("Manager", false, 40)
	stripped := stripANSI(out)
	want := strings.Repeat("─", 40)
	if stripped != want {
		t.Errorf("wide header: want %q got %q", want, stripped)
	}
}

func TestHeader_NarrowIncludesBreadcrumb(t *testing.T) {
	out := Header("Manager", true, 40)
	stripped := stripANSI(out)
	if !strings.HasPrefix(stripped, " Manager ") {
		t.Errorf("narrow header should start with breadcrumb, got %q", stripped)
	}
	if !strings.HasSuffix(stripped, strings.Repeat("─", 1)) {
		t.Errorf("narrow header should end with rule, got %q", stripped)
	}
	if w := lipgloss.Width(stripped); w != 40 {
		t.Errorf("narrow header width: want 40 got %d (rendered=%q)", w, stripped)
	}
}

func TestFooter_NormalMode(t *testing.T) {
	hints := FooterHints{
		Mode: "Normal",
		Keys: []KeyHint{
			{Key: "j/k", Label: "nav"},
			{Key: "/", Label: "filter"},
			{Key: "x", Label: "select"},
		},
	}
	out := Footer(hints, 80)
	stripped := stripANSI(out)
	if !strings.Contains(stripped, "Normal") {
		t.Errorf("want mode label, got %q", stripped)
	}
	for _, want := range []string{"j/k nav", "/ filter", "x select"} {
		if !strings.Contains(stripped, want) {
			t.Errorf("missing hint %q in %q", want, stripped)
		}
	}
	if w := lipgloss.Width(stripped); w != 80 {
		t.Errorf("footer width: want 80 got %d", w)
	}
}

func TestFooter_CountBadgeAccents(t *testing.T) {
	hints := FooterHints{
		Mode: "Normal",
		Keys: []KeyHint{
			{Key: "[x]", Label: "toggle"},
			{Key: "C", Label: "cat", Count: 3},
		},
	}
	out := Footer(hints, 80)
	stripped := stripANSI(out)
	if !strings.Contains(stripped, "C cat 3") {
		t.Errorf("count badge: want %q in %q", "C cat 3", stripped)
	}
}

func TestFooter_TruncatesWhenTooNarrow(t *testing.T) {
	hints := FooterHints{
		Mode: "Normal",
		Keys: []KeyHint{
			{Key: "j/k", Label: "nav"},
			{Key: "/", Label: "filter"},
			{Key: "x", Label: "select"},
			{Key: "C", Label: "cat"},
			{Key: "T", Label: "tag"},
			{Key: "?", Label: "help"},
		},
	}
	out := Footer(hints, 40)
	if w := lipgloss.Width(out); w != 40 {
		t.Errorf("footer width: want 40 got %d (rendered=%q)", w, stripANSI(out))
	}
}

func TestFooter_EmptyKeys(t *testing.T) {
	out := Footer(FooterHints{Mode: "Normal"}, 80)
	stripped := stripANSI(out)
	if !strings.Contains(stripped, "Normal") {
		t.Errorf("want mode label, got %q", stripped)
	}
}

func TestLayout_StandardManagerActive(t *testing.T) {
	entries := []SidebarEntry{
		{Name: "Manager", Active: true},
		{Name: "Categorizer"},
		{Name: "Linker"},
		{Name: "Budget"},
		{Name: "Recipes"},
	}
	body := "Hello, world."
	status := components.Status{
		DBPath:    "/home/user/.local/share/ledger/ledger.db",
		Screen:    "Manager",
		Mode:      "NORMAL",
		StatusMsg: "showing 0",
		Width:     100,
	}
	hints := FooterHints{
		Mode: "Normal",
		Keys: []KeyHint{
			{Key: "j/k", Label: "nav"},
			{Key: "/", Label: "filter"},
			{Key: "?", Label: "help"},
		},
	}
	out := Layout(entries, "Manager", body, hints, status, 100, 24)
	stripped := stripANSI(out)
	lines := strings.Split(stripped, "\n")

	// Expect 24 lines.
	if len(lines) != 24 {
		t.Fatalf("want 24 lines, got %d:\n%s", len(lines), stripped)
	}

	// First line: header rule.
	if lines[0] != strings.Repeat("─", 100) {
		t.Errorf("header line 0:\nwant %q\ngot  %q", strings.Repeat("─", 100), lines[0])
	}

	// Sidebar lines 1..5 should each contain the screen name.
	// Sidebar portion is the first 14 visible chars (sidebarW); the
	// rest is body padding.
	const sidebarW = 14
	for i, name := range []string{"Manager", "Categorizer", "Linker", "Budget", "Recipes"} {
		line := lines[1+i]
		runes := []rune(line)
		if len(runes) < sidebarW {
			t.Fatalf("line %d too short: %q", 1+i, line)
		}
		side := string(runes[:sidebarW])
		if !strings.Contains(side, name) {
			t.Errorf("line %d sidebar missing %q: %q", 1+i, name, side)
		}
		if !strings.HasSuffix(side, "│") {
			t.Errorf("line %d sidebar missing right border: %q", 1+i, side)
		}
	}

	// Body content lives on line 1 (the first sidebar row carries
	// the body's first line). Body is "Hello, world." padded to
	// 21 lines, so only line 1 has the content.
	if !strings.Contains(lines[1], "Hello, world.") {
		t.Errorf("body line 1 missing content:\n%s", lines[1])
	}

	// Last line: statusbar with db path.
	last := lines[len(lines)-1]
	if !strings.Contains(last, "ledger.db") {
		t.Errorf("statusbar missing db path:\n%s", last)
	}
	if !strings.Contains(last, "showing 0") {
		t.Errorf("statusbar missing status msg:\n%s", last)
	}

	// Penultimate line: footer.
	penult := lines[len(lines)-2]
	if !strings.Contains(penult, "Normal") {
		t.Errorf("footer missing mode label:\n%s", penult)
	}
	if !strings.Contains(penult, "j/k nav") {
		t.Errorf("footer missing hint:\n%s", penult)
	}
}

func TestLayout_NarrowTerminalHidesSidebar(t *testing.T) {
	entries := []SidebarEntry{
		{Name: "Manager", Active: true},
		{Name: "Categorizer"},
	}
	body := "body content"
	status := components.Status{Width: 70}
	out := Layout(entries, "Manager", body, FooterHints{Mode: "Normal"}, status, 70, 24)
	stripped := stripANSI(out)
	lines := strings.Split(stripped, "\n")

	if len(lines) != 24 {
		t.Fatalf("want 24 lines, got %d", len(lines))
	}
	// No sidebar border should appear anywhere.
	for i, line := range lines {
		if strings.Contains(line, "│") {
			t.Errorf("line %d has sidebar border in narrow mode: %q", i, line)
		}
	}
	// Header should include breadcrumb.
	if !strings.Contains(lines[0], "Manager") {
		t.Errorf("narrow header missing breadcrumb: %q", lines[0])
	}
}

func TestLayout_VerySmallIsSafe(t *testing.T) {
	status := components.Status{Width: 30}
	out := Layout(nil, "M", "body", FooterHints{}, status, 30, 3)
	if out == "" {
		t.Fatal("Layout should never return empty string")
	}
}

func TestComputeSidebarWidth_IncludesLongestName(t *testing.T) {
	entries := []SidebarEntry{
		{Name: "A"},
		{Name: "Categorizer"}, // 11 chars
		{Name: "X"},
	}
	w := computeSidebarWidth(entries)
	// 11 (name) + 2 (marker) + 1 (border) = 14
	if w != 14 {
		t.Errorf("want 14, got %d", w)
	}
}
