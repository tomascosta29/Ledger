package chrome

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/tomascosta29/Ledger/internal/tui/components"
)

// TestPreviewLayout writes a stripped, visible-only rendering of the
// chrome at three terminal widths to /tmp/chrome_preview.txt. Run with:
//
//	go test -run TestPreviewLayout ./internal/tui/chrome/...
//
// to refresh the file. Useful for manual eyeball before wiring chrome
// into App.View().
func TestPreviewLayout(t *testing.T) {
	if os.Getenv("WRITE_PREVIEW") == "" {
		t.Skip("set WRITE_PREVIEW=1 to write /tmp/chrome_preview.txt")
	}
	entries := []SidebarEntry{
		{Name: "Manager", Active: true},
		{Name: "Categorizer"},
		{Name: "Linker"},
		{Name: "Budget"},
		{Name: "Recipes"},
	}
	status := components.Status{
		DBPath:    "/home/fcosta/.local/share/ledger/ledger.db",
		Screen:    "Manager",
		Mode:      "NORMAL",
		StatusMsg: "showing 47 txns",
	}

	var b strings.Builder

	b.WriteString("===== width=100, mode=Normal =====\n")
	body := "  ID    DATE         AMOUNT       CAT        DESCRIPTION\n  >  47  2026-06-20   -42.00 EUR   food       rewe\n     46  2026-06-20   -12.50 EUR   transit    s-bahn\n     45  2026-06-19   -88.00 EUR   food       lidl\n     44  2026-06-19  -200.00 EUR   rent       miete\n"
	hints := FooterHints{
		Mode: "Normal",
		Keys: []KeyHint{
			{Key: "j/k", Label: "nav"},
			{Key: "/", Label: "filter"},
			{Key: "x", Label: "select"},
			{Key: "C", Label: "cat"},
			{Key: "T", Label: "tag"},
			{Key: "H", Label: "hide"},
			{Key: "U", Label: "undo"},
			{Key: "?", Label: "help"},
		},
	}
	b.WriteString(ansi.Strip(Layout(entries, "Manager", body, hints, status, 100, 24)))
	b.WriteString("\n\n===== width=100, mode=Bulk: categorize =====\n")
	hintsBulk := FooterHints{
		Mode: "Bulk: categorize",
		Keys: []KeyHint{
			{Key: "Enter", Label: "apply"},
			{Key: "Esc", Label: "cancel"},
		},
	}
	b.WriteString(ansi.Strip(Layout(entries, "Manager", body, hintsBulk, status, 100, 24)))
	b.WriteString("\n\n===== width=100, mode=Filter =====\n")
	hintsFilter := FooterHints{
		Mode: "Filter",
		Keys: []KeyHint{
			{Key: "Enter", Label: "apply"},
			{Key: "Esc", Label: "cancel"},
			{Key: "Bksp", Label: "edit"},
		},
	}
	b.WriteString(ansi.Strip(Layout(entries, "Manager", "  filter: cat:foo_\n"+body, hintsFilter, status, 100, 24)))
	b.WriteString("\n\n===== width=80 =====\n")
	b.WriteString(ansi.Strip(Layout(entries, "Manager", body, hints, status, 80, 24)))
	b.WriteString("\n\n===== width=60 (narrow, no sidebar) =====\n")
	b.WriteString(ansi.Strip(Layout(entries, "Manager", body, hints, status, 60, 24)))

	// Width report
	for _, w := range []int{100, 80, 60} {
		_ = w
	}

	_ = lipgloss.Width

	if err := os.WriteFile("/tmp/chrome_preview.txt", []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write preview: %v", err)
	}
	t.Logf("wrote /tmp/chrome_preview.txt (%d bytes)", b.Len())
}
