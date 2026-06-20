package screens

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
)

// Manager is the transaction list screen (filter DSL, j/k nav, ...).
// v1 ships as a stub listing the latest transactions.
type Manager struct {
	deps  Deps
	rows  []row
	cursor int
}

type row struct {
	id     int64
	date   string
	amount string
	desc   string
}

func NewManager() *Manager { return &Manager{} }

func (m *Manager) Title() string { return "Manager" }

func (m *Manager) Init(ctx context.Context, deps Deps) tea.Cmd {
	m.deps = deps
	m.reload(ctx)
	return nil
}

func (m *Manager) reload(ctx context.Context) {
	opts := overlayFindAll()
	rows, err := m.deps.OverlayRepo.FindAll(ctx, opts)
	if err != nil {
		return
	}
	m.rows = m.rows[:0]
	for _, r := range rows {
		m.rows = append(m.rows, row{
			id:     r.ID,
			date:   r.EffectiveDate,
			amount: r.Amount.DecimalString() + " " + string(r.Amount.Currency),
			desc:   r.Description,
		})
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *Manager) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if m.cursor < len(m.rows)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "g":
			m.cursor = 0
		case "G":
			m.cursor = len(m.rows) - 1
		}
	}
	return m, nil
}

func (m *Manager) View() string {
	if len(m.rows) == 0 {
		return "  (no transactions — try `ledger import` or `ledger add`)\n"
	}
	var b []byte
	b = append(b, "  ID    DATE         AMOUNT       DESCRIPTION\n"...)
	for i, r := range m.rows {
		marker := "  "
		if i == m.cursor {
			marker = "> "
		}
		b = append(b, marker...)
		b = append(b, pad(idStr(r.id), 5)...)
		b = append(b, "  "...)
		b = append(b, pad(r.date, 11)...)
		b = append(b, "  "...)
		b = append(b, pad(r.amount, 11)...)
		b = append(b, "  "...)
		b = append(b, r.desc...)
		b = append(b, '\n')
	}
	return string(b)
}

func idStr(id int64) string {
	const digits = "0123456789"
	if id == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := id < 0
	if neg {
		id = -id
	}
	for id > 0 {
		i--
		buf[i] = digits[id%10]
		id /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func pad(s string, w int) string {
	if len(s) >= w {
		return s
	}
	return s + spaces(w-len(s))
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}
