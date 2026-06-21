package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tomascosta29/Ledger/internal/tui/chrome"
	"github.com/tomascosta29/Ledger/internal/tui/components"
	"github.com/tomascosta29/Ledger/internal/tui/screens"
)

// Mode labels the current keybinding mode of the App.
type Mode int

const (
	ModeNormal Mode = iota
	ModeCommand
	ModeHelp
)

// App is the root Bubble Tea model. It owns the current screen,
// the status bar, the help overlay, and a small message line.
type App struct {
	deps   screens.Deps
	width  int
	height int

	current    screens.Screen
	screenList []screens.Screen

	mode      Mode
	statusMsg string

	help components.Help
}

// NewApp wires the root model and registers the five screens.
func NewApp(ctx context.Context, deps screens.Deps) *App {
	list := []screens.Screen{
		screens.NewManager(),
		screens.NewCategorizer(),
		screens.NewLinker(),
		screens.NewBudget(),
		screens.NewRecipes(),
	}
	for _, s := range list {
		_ = s.Init(ctx, deps)
	}
	return &App{
		deps:       deps,
		current:    list[0],
		screenList: list,
		help:       components.NewHelp(),
	}
}

func (a *App) Init() tea.Cmd { return nil }

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = m.Width
		a.height = m.Height
		return a, nil
	case tea.KeyMsg:
		return a.handleKey(m)
	}
	// Forward to the current screen for non-key messages.
	next, cmd := a.current.Update(msg)
	if next != nil {
		a.current = next
	}
	return a, cmd
}

func (a *App) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if a.mode == ModeHelp {
		// Any key dismisses the help overlay.
		a.mode = ModeNormal
		return a, nil
	}

	if a.mode == ModeCommand {
		// Reserved for the : command line (v2). For now, Esc returns to Normal.
		if key == "esc" {
			a.mode = ModeNormal
			a.statusMsg = ""
		}
		return a, nil
	}

	// Global keys (Normal mode).
	switch key {
	case "ctrl+c", "q":
		return a, tea.Quit
	case "?":
		a.mode = ModeHelp
		return a, nil
	case "1", "2", "3", "4", "5":
		idx := int(key[0] - '1')
		if idx >= 0 && idx < len(a.screenList) {
			a.current = a.screenList[idx]
			a.statusMsg = ""
		}
		return a, nil
	}

	// Delegate to the current screen.
	next, cmd := a.current.Update(msg)
	if next != nil {
		a.current = next
	}
	return a, cmd
}

func (a *App) View() string {
	if a.mode == ModeHelp {
		return a.help.View(a.width, a.height)
	}

	// Build sidebar entries from the registered screen list.
	entries := make([]chrome.SidebarEntry, len(a.screenList))
	currentIdx := 0
	for i, s := range a.screenList {
		entries[i] = chrome.SidebarEntry{Name: s.Title(), Active: s == a.current}
		if s == a.current {
			currentIdx = i
		}
	}
	_ = currentIdx

	// Compute content area dimensions: header + footer + statusbar
	// reserve 3 rows; sidebar (if visible) reserves its own width.
	contentH := a.height - 3
	if contentH < 1 {
		contentH = 1
	}
	contentW := a.width
	if a.width >= chrome.MinSidebarWidth {
		contentW = a.width - chrome.SidebarWidth(entries)
	}
	if contentW < 1 {
		contentW = 1
	}

	body := a.current.View(contentW, contentH)
	hints := a.current.Hints(a.width)
	status := components.Status{
		DBPath:    a.deps.DBPath,
		Screen:    a.current.Title(),
		Mode:      modeLabel(a.mode),
		StatusMsg: a.statusMsg,
		Width:     a.width,
	}

	return chrome.Layout(entries, a.current.Title(), body, hints, status, a.width, a.height)
}

func modeLabel(m Mode) string {
	switch m {
	case ModeNormal:
		return "NORMAL"
	case ModeCommand:
		return "COMMAND"
	case ModeHelp:
		return "HELP"
	}
	return fmt.Sprintf("M%d", int(m))
}
