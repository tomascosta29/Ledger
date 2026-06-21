# ADR 0008: TUI visual chrome — sidebar, mode-aware footer, style tokens

## Status

Accepted. 2026-06-21.

## Context

The v1.1 TUI renders data rows with raw `fmt.Fprintf` and hardcoded
column padding (`internal/tui/screens/manager.go:324`,
`internal/tui/screens/stubs.go:222, 348`). Every screen is a self-
contained `View() string` that assembles its own column-aligned text
with no `lipgloss` styling, no cursor highlight, no selection marker,
no visible key hints. The only styled elements today are the statusbar
(`internal/tui/components/statusbar.go:48`) and the help overlay
(`internal/tui/components/help.go:59`); the data they frame is plain
text.

The friction cases:

1. **Ugly.** The cursor row is `"> "` (plain ASCII); the selection
   mark is `"x"`. Negative amounts are not visually distinguishable
   from positives. The active screen is identifiable only via the
   statusbar's `[ScreenName]` cell, not in the body.
2. **Unintuitive.** Keybindings for the current screen are documented
   only in the `?` overlay. The operator must press `?` to learn what
   keys are available. The footer shows a single global hint line
   (`[1-5] screen · / filter · : cmd · ? help · q quit`) that does not
   reflect screen state — when filter mode is active, the same hint
   line is shown as in normal mode.
3. **No screen map.** Five screens exist (Manager, Categorizer, Linker,
   Budget, Recipes) but nothing in the body of each screen tells the
   operator which screen they are on, and nothing in chrome shows the
   other four. Navigation by `1`–`5` is invisible.

The trade-off: introduce a `chrome` layer that owns layout (sidebar,
header, footer) and a `styles` layer that owns visual primitives
(color tokens, row styles). Each screen keeps its own data rendering
logic but uses the chrome and styles from the new packages. The
change is layout-only; no flow, no keymap, no DSL changes.

## Decision

**Add two packages: `internal/tui/styles` and `internal/tui/chrome`.**

### `internal/tui/styles`

A small token + style package. Exports:

- Color tokens: `Accent` (117 — blue), `Out` (167 — dim red, negative
  amounts), `In` (108 — dim green, positive), `Dim` (245 — unknown
  category, footer hints), `Warn` (215 — status messages), `Mute`
  (241 — subtle dividers), `Strong` (252 — header text), `Surface`
  (237 — cursor/selection fill).
- Composed styles: `HeaderText`, `HeaderRule`, `AmountOut`,
  `AmountIn`, `AmountZero`, `UnknownCategory`, `SidebarActive`,
  `SidebarInactive`, `CursorRow`, `SelectedRow`, `CursorSelectedRow`,
  `FooterMode`, `FooterKey`, `FooterKeyCount`, `SelectionGlyph`,
  `CursorGlyph`.

All styles are `lipgloss.Style` values. Screens and chrome import
them; nothing in the package is a function.

### `internal/tui/chrome`

A layout package. Exports:

- `Sidebar(entries []ScreenEntry, width, height int) string` — renders
  the 5-screen nav column. Active entry gets `►` + bold + Accent.
  Inactive entries get dim text. Returns `""` when `width <
  MinSidebarWidth`.
- `Header(title string, narrow bool, width int) string` — renders the
  header rule. In narrow mode (no sidebar), prepends the screen title
  to the rule as a breadcrumb.
- `Footer(hints FooterHints, width int) string` — renders the mode-
  aware footer. Mode label on the left (e.g., `Normal`, `Filter`,
  `Bulk: categorize`). Key hints on the right, joined by ` · `, with
  optional count badges (e.g., `C cat 3`).
- `JoinHorizontal(left, right string, width int) string` — joins a
  sidebar and the body content horizontally. Body is constrained to
  `width - sidebarWidth`. Used by `App.View()`.
- `Layout(...) string` — composes Header + JoinHorizontal(sidebar,
  body) + Footer + statusbar. The single entry point used by
  `App.View()`.
- `MinSidebarWidth = 80`.

`MinSidebarWidth = 80` is the threshold below which the sidebar is
hidden. Below 60, the footer hints truncate to fit.

### Screen interface change

`Screen.View()` becomes `View(width, height int) string`. Each screen
gets its available content area and renders into it. Screens that
don't yet respect the width/height arguments get clipped by
`lipgloss` until they are migrated; that is acceptable for the
intermediate slices.

### Mode-aware footer

Each screen's `View()` assembles a `FooterHints` struct based on its
state. The chrome package renders the hints. Examples:

| State        | Mode label        | Key hints                                            |
| ------------ | ----------------- | ---------------------------------------------------- |
| Normal       | `Normal`          | `j/k nav · / filter · x select · C cat · T tag · H hide · U undo · ? help` |
| Filter mode  | `Filter`          | `Enter apply · Esc cancel · backspace edit`          |
| Bulk: cat    | `Bulk: categorize`| `Enter apply · Esc cancel`                           |
| Bulk: tag    | `Bulk: tag`       | `Enter apply · Esc cancel`                           |
| Bulk: hide   | `Bulk: hide`      | `Enter apply · Esc cancel`                           |
| Selection    | `Normal`          | `[x] toggle · C cat 3 · T tag 3 · H hide 3 · U undo · : clear` |

The single biggest UX win in this ADR.

## Consequences

- **Layout consistency across screens.** All five screens share the
  same chrome. Operator sees the same nav column, same footer style,
  same color language regardless of which screen they're on.
- **Mode-aware hints are always visible.** No more "what does `c` do?"
  friction — the footer shows every action that can be taken from the
  current state.
- **Style is centralized.** Changing a color is a one-line edit in
  `styles`. Screens never call `lipgloss.NewStyle()` directly; they
  compose from the exported styles.
- **Screen rendering becomes width-aware.** Once all five screens
  honor `View(width, height)`, the TUI lays out correctly at any
  terminal width down to 60 cols.
- **Cost: a chrome package and a styles package.** ~300 LOC of new
  code, ~150 LOC of tests. Low risk: chrome can be added without
  changing any screen's logic, then screens adopt it incrementally
  (slice 2 onward).

## Alternatives considered

- **Top tabs instead of left sidebar.** Rejected — top tabs share the
  horizontal width with content. For a content-heavy app (transaction
  lists, budget tables), width matters more than vertical space.
- **`bubbles/table` component for the data rows.** Rejected — bubbles'
  table component constrains per-cell coloring, makes checkbox
  selection harder, and ships its own cursor styling that doesn't
  compose with the chrome. Hand-rolled `lipgloss` rows are ~150 LOC
  per screen and give full control.
- **No chrome; per-screen styling.** Rejected — leaves the "what
  color is the active screen" decision to each screen author; will
  drift.
- **Pursue option 3 (interaction redesign) instead of option 2.** Out
  of scope for this ADR. Option 3 (filter DSL rework, modal overlays,
  categorizer wizard) is a separate slice once chrome is shipped.
- **Sidebar with per-screen counters (`Manager · 47`).** Deferred to a
  follow-up. Counters require plumbing (each screen exposes a count,
  semantics differ). Mixing structural change with semantic counters
  inflates the slice.

## When this should be revisited

- If a sixth screen is added, the sidebar's fixed-width assumption
  breaks. Revisit `MinSidebarWidth` and sidebar content layout.
- If the operator wants per-screen counters, add a `Counter() string`
  method to the `Screen` interface and wire it into the sidebar.
- If mode-aware footer becomes noisy (10+ hints), introduce a
  two-line footer or a `Primary`/`Secondary` hint split.
- If the operator wants the `:` command line to be implemented (it is
  documented in the help overlay but currently a no-op), it would be
  the next interaction-redesign slice — option 3 in the original
  scope discussion.
