// Package hints defines the FooterHints payload exchanged between
// screens (which know their own mode and current keymap) and the
// chrome layer (which renders the footer line). It lives in its own
// package so neither chrome nor screens imports the other for this
// shared data type.
package hints

// FooterHints is the mode-aware footer payload. Each screen's
// Hints(width int) method assembles this from its current state.
type FooterHints struct {
	Mode string // "Normal", "Filter", "Bulk: categorize", ...
	Keys []KeyHint
}

// KeyHint is one binding shown in the footer. Count is optional:
// when > 0, it renders as "C cat 3" with the count accented.
type KeyHint struct {
	Key   string
	Label string
	Count int
}
