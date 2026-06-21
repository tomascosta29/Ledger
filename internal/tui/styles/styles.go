// Package styles defines the visual language for the LedgerPro TUI:
// color tokens and composed lipgloss.Style values. Screens and chrome
// import from this package; nothing else in the codebase should call
// lipgloss.NewStyle() directly. Changing a color or weight is a one-
// line edit here.
package styles

import "github.com/charmbracelet/lipgloss"

// Glyphs — Unicode characters that make up the visual language.
// Kept here so screens and chrome agree on the markers without one
// importing the other.
const (
	// CursorGlyph marks the row the operator is on.
	CursorGlyph = "▶"
	// ActiveScreenGlyph marks the active entry in the sidebar.
	ActiveScreenGlyph = "►"
	// SelectionGlyph marks a row toggled with `x`.
	SelectionGlyph = "[x]"
	// InactiveSelGlyph is the empty slot in the selection column.
	InactiveSelGlyph = "   "
	// LinkedGlyph marks a row that is part of a transfer/reimbursement
	// group (the result of `l` link or the Linker screen confirming a
	// candidate).
	LinkedGlyph = "⇄"
	// InactiveLinkedGlyph is the empty slot in the link column.
	InactiveLinkedGlyph = " "
	// RuleChar is the horizontal divider character.
	RuleChar = "─"
	// BulletChar separates key hints in the footer.
	BulletChar = "·"
	// EllipsisChar is the truncation suffix.
	EllipsisChar = "…"
)

// Color tokens. ANSI 256 codes that read well on both dark and light
// terminal backgrounds; chosen to match the existing statusbar palette
// (internal/tui/components/statusbar.go) so the new chrome blends with
// what is already shipping.
const (
	// Accent is the active-screen / focus color (cool blue).
	Accent lipgloss.Color = "117"
	// Out is for negative amounts (muted red; never screams).
	Out lipgloss.Color = "167"
	// In is for positive amounts (muted green).
	In lipgloss.Color = "108"
	// Dim is for unknown category, footer hints, anything secondary.
	Dim lipgloss.Color = "245"
	// Warn is for status messages, count badges (warm orange).
	Warn lipgloss.Color = "215"
	// Mute is for subtle dividers and inactive sidebar entries.
	Mute lipgloss.Color = "241"
	// Strong is for header text and mode labels (near-white).
	Strong lipgloss.Color = "252"
	// Surface is the cursor / selection fill (dark gray bg).
	Surface lipgloss.Color = "237"
)

// Composed styles. Names describe intent, not appearance, so a
// future color change does not require renaming.
var (
	HeaderText = lipgloss.NewStyle().
			Foreground(Strong).
			Bold(true)

	HeaderRule = lipgloss.NewStyle().
			Foreground(Mute)

	AmountOut = lipgloss.NewStyle().
			Foreground(Out)

	AmountIn = lipgloss.NewStyle().
			Foreground(In)

	AmountZero = lipgloss.NewStyle().
			Foreground(Dim)

	UnknownCategory = lipgloss.NewStyle().
			Foreground(Dim)

	SidebarActive = lipgloss.NewStyle().
			Foreground(Accent).
			Bold(true)

	SidebarInactive = lipgloss.NewStyle().
			Foreground(Mute)

	// CursorRow is the highlighted row the operator is on. Reverse
	// video works in every terminal; the surface background adds
	// contrast where reverse-video is overridden by the theme.
	CursorRow = lipgloss.NewStyle().
			Reverse(true).
			Background(Surface)

	// SelectedRow marks rows the operator has toggled with `x`
	// without moving the cursor onto them. Lighter than CursorRow
	// so the cursor remains the primary visual focus.
	SelectedRow = lipgloss.NewStyle().
			Background(Surface)

	// CursorSelectedRow is the row that is both the cursor and
	// selected: full cursor emphasis, with a Warn-colored selection
	// glyph so the dual state reads at a glance.
	CursorSelectedRow = lipgloss.NewStyle().
				Reverse(true).
				Background(Surface)

	// LinkedGlyphStyle is the color of the ⇄ marker on rows that
	// are part of a transfer/reimbursement group. Accent so it
	// reads as a positive state (you successfully linked this).
	LinkedGlyphStyle = lipgloss.NewStyle().
				Foreground(Accent)

	// LinkedRow is a subtle accent-tinted background for linked
	// rows that aren't otherwise highlighted (no cursor, no
	// selection). The tint makes "this is grouped" visible from
	// across the screen even when the operator isn't focused on
	// the row.
	LinkedRow = lipgloss.NewStyle().
			Foreground(Strong)

	FooterMode = lipgloss.NewStyle().
			Foreground(Strong).
			Bold(true)

	FooterKey = lipgloss.NewStyle().
			Foreground(Dim)

	// FooterKeyCount accents the optional count badge on a key
	// hint (e.g., "C cat 3" — the "3" uses this style).
	FooterKeyCount = lipgloss.NewStyle().
			Foreground(Warn).
			Bold(true)

	// FooterMode is shown on the left; FooterKey on the right.
	// When the footer is in filter or bulk mode, the mode label
	// changes and the keys shrink.

	SidebarBorder = lipgloss.NewStyle().
			Foreground(Mute)

	StatusBarBg = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(Strong)
)
