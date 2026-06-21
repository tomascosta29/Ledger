package screens

import (
	"io"
	"os"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/tui/chrome"
	"github.com/tomascosta29/Ledger/internal/tui/components"
)

// TestPreviewManager writes a stripped, visible-only rendering of the
// Manager screen wrapped in chrome at several widths and states to
// /tmp/manager_preview.txt. Run with:
//
//	WRITE_PREVIEW=1 go test -run TestPreviewManager ./internal/tui/screens/...
//
// to refresh. Useful for manual eyeball during slice 2.
func TestPreviewManager(t *testing.T) {
	if os.Getenv("WRITE_PREVIEW") == "" {
		t.Skip("set WRITE_PREVIEW=1 to write /tmp/manager_preview.txt")
	}

	m := NewManager()
	m.rows = []managerRow{
		{id: 47, date: "2026-06-20", amount: "-42.00 EUR", cat: "food", desc: "rewe"},
		{id: 46, date: "2026-06-20", amount: "-12.50 EUR", cat: "transit", desc: "s-bahn"},
		{id: 45, date: "2026-06-19", amount: "-88.00 EUR", cat: "food", desc: "lidl"},
		{id: 44, date: "2026-06-19", amount: "-200.00 EUR", cat: "rent", desc: "miete juni"},
		{id: 43, date: "2026-06-18", amount: "2500.00 EUR", cat: "income", desc: "gehalt"},
		{id: 42, date: "2026-06-18", amount: "0.00 EUR", cat: "", desc: "null tx"},
	}
	// Pretend rows 47 and 46 are already linked (a transfer pair)
	// so the preview shows the ⇄ glyph in action.
	groupID := int64(99)
	m.rows[0].linked = true
	m.rows[0].groupID = &groupID
	m.rows[1].linked = true
	m.rows[1].groupID = &groupID
	m.cursor = 1
	m.selected = map[int64]bool{45: true, 44: true}

	entries := []chrome.SidebarEntry{
		{Name: "Manager", Active: true},
		{Name: "Linker"},
		{Name: "Budget"},
		{Name: "Recipes"},
	}
	status := components.Status{
		DBPath:    "/home/fcosta/.local/share/ledger/ledger.db",
		Screen:    "Manager",
		Mode:      "NORMAL",
		StatusMsg: "showing 6",
	}

	renderTo := func(w io.Writer, label string, width, height int, mode mgrMode) {
		contentW := width
		if width >= chrome.MinSidebarWidth {
			contentW = width - chrome.SidebarWidth(entries)
		}
		contentH := height - 3
		if contentH < 1 {
			contentH = 1
		}
		var body string
		var hints chrome.FooterHints
		switch mode {
		case modeNormal:
			body = m.View(contentW, contentH)
			hints = m.Hints(width)
		case modeFilter:
			m.filterMode = true
			m.filterInput = "cat:foo"
			body = m.View(contentW, contentH)
			hints = m.Hints(width)
			m.filterMode = false
		case modeInputCat:
			m.inputMode = inputCategory
			m.inputScope = scopeCursor
			m.input = "food"
			body = m.View(contentW, contentH)
			hints = m.Hints(width)
			m.inputMode = inputNone
			m.input = ""
		case modeInputBucket:
			m.inputMode = inputBucket
			m.inputScope = scopeCursor
			m.input = "rent"
			body = m.View(contentW, contentH)
			hints = m.Hints(width)
			m.inputMode = inputNone
			m.input = ""
		case modeInputTagSel:
			m.inputMode = inputTag
			m.inputScope = scopeSelection
			m.input = "recurring"
			body = m.View(contentW, contentH)
			hints = m.Hints(width)
			m.inputMode = inputNone
			m.input = ""
		}
		out := chrome.Layout(entries, "Manager", body, hints, status, width, height)
		_, _ = io.WriteString(w, "===== "+label+" =====\n")
		_, _ = io.WriteString(w, ansi.Strip(out))
		_, _ = io.WriteString(w, "\n\n")
	}

	f, err := os.Create("/tmp/manager_preview.txt")
	if err != nil {
		t.Fatalf("create preview: %v", err)
	}
	defer f.Close()

	renderTo(f, "width=100, mode=Normal, 2 selected", 100, 24, modeNormal)
	renderTo(f, "width=100, mode=Filter", 100, 24, modeFilter)
	renderTo(f, "width=100, mode=Input: category", 100, 24, modeInputCat)
	renderTo(f, "width=100, mode=Input: bucket", 100, 24, modeInputBucket)
	renderTo(f, "width=100, mode=Input: tag (on 2 selected)", 100, 24, modeInputTagSel)
	renderTo(f, "width=60, mode=Normal (no sidebar)", 60, 24, modeNormal)

	// ---- Linker ----
	lnk := NewLinker()
	lnk.cands = []linkerCand{
		{score: 100, outID: 1, inID: 2, outTxt: "1  2026-06-15  PARTNER  -50.00 EUR", inTxt: "2  2026-06-15  PARTNER  50.00 EUR"},
		{score: 95, outID: 3, inID: 4, outTxt: "3  2026-06-14  ACME  -120.00 EUR", inTxt: "4  2026-06-14  ACME  120.00 EUR"},
		{score: 80, outID: 5, inID: 6, outTxt: "5  2026-06-13  SHOP  -10.00 EUR", inTxt: "6  2026-06-13  SHOP  10.00 EUR"},
	}
	lnk.groups = []linkerGroup{
		{id: 10, note: "rent-payment"},
		{id: 11, note: "groceries-rewe"},
	}
	lnk.cursor = 1
	lnk.focus = 0
	renderScreenTo(f, "===== Linker, width=100, 3 candidates 2 groups =====", 100, 24, lnk, true)
	lnk.cursor = 3 // on first group
	lnk.focus = 1
	renderScreenTo(f, "===== Linker, cursor on first group =====", 100, 24, lnk, true)

	// ---- Budget ----
	bdg := NewBudget()
	bdg.month = "2026-06"
	bdg.spends = []ports.BucketSpend{
		{BucketName: "rent", Currency: "EUR", AllocatedMinor: 80000, SpentMinor: 80000, Count: 1},
		{BucketName: "groceries", Currency: "EUR", AllocatedMinor: 30000, SpentMinor: 21000, Count: 14},
		{BucketName: "transit", Currency: "EUR", AllocatedMinor: 5000, SpentMinor: 6200, Count: 22},
		{BucketName: "fun", Currency: "EUR", AllocatedMinor: 10000, SpentMinor: 4500, Count: 3},
	}
	bdg.unassigned = []ports.BucketSpend{
		{Currency: "EUR", SpentMinor: 1850, Count: 4},
	}
	renderScreenTo(f, "===== Budget, width=100, 4 buckets + unassigned =====", 100, 24, bdg, true)
	renderScreenTo(f, "===== Budget, width=60 (no sidebar) =====", 60, 24, bdg, true)

	// ---- Recipes ----
	rcp := NewRecipes()
	rcp.rows = []recipeRow{
		{name: "monthly-snapshot", net: true, description: "all categories, last 30d"},
		{name: "rent-trend", net: false, description: "rent by month, last 12mo"},
		{name: "uncategorized-watch", net: false, description: "transactions without category"},
	}
	rcp.active = "monthly-snapshot"
	rcp.cursor = 1
	renderScreenTo(f, "===== Recipes, width=100, 3 recipes, 1 active =====", 100, 24, rcp, true)
	renderScreenTo(f, "===== Recipes, width=60 (no sidebar) =====", 60, 24, rcp, true)

	t.Logf("wrote /tmp/manager_preview.txt")
}

// renderScreenTo renders a Screen through chrome.Layout. used for
// Linker, Budget, Recipes in the preview.
func renderScreenTo(w io.Writer, label string, width, height int, s Screen, active bool) {
	entries := []chrome.SidebarEntry{
		{Name: "Manager"},
		{Name: "Linker"},
		{Name: "Budget"},
		{Name: "Recipes"},
	}
	for i := range entries {
		entries[i].Active = false
	}
	for i := range entries {
		if entries[i].Name == s.Title() && active {
			entries[i].Active = true
		}
	}
	contentW := width
	if width >= chrome.MinSidebarWidth {
		contentW = width - chrome.SidebarWidth(entries)
	}
	contentH := height - 3
	if contentH < 1 {
		contentH = 1
	}
	body := s.View(contentW, contentH)
	hints := s.Hints(width)
	status := components.Status{
		DBPath:    "/home/fcosta/.local/share/ledger/ledger.db",
		Screen:    s.Title(),
		Mode:      "NORMAL",
		StatusMsg: "",
		Width:     width,
	}
	out := chrome.Layout(entries, s.Title(), body, hints, status, width, height)
	_, _ = io.WriteString(w, label+"\n")
	_, _ = io.WriteString(w, ansi.Strip(out))
	_, _ = io.WriteString(w, "\n\n")
}

type mgrMode int

const (
	modeNormal mgrMode = iota
	modeFilter
	modeInputCat
	modeInputBucket
	modeInputTagSel
)
