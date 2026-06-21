package screens

import (
	"testing"
)

// TestManagerJumpToNextUnknown covers the `n` key handler. It does
// not exercise the persistence layer — only the cursor movement
// across a synthetic rows slice.
func TestManagerJumpToNextUnknown(t *testing.T) {
	cases := []struct {
		name      string
		rows      []managerRow
		startAt   int
		wantIdx   int
		wantMsgOK bool // empty statusMsg means "no Unknown rows" message
	}{
		{
			name: "first Unknown after cursor",
			rows: []managerRow{
				{id: 1, cat: ""},
				{id: 2, cat: "food"},
				{id: 3, cat: ""},
				{id: 4, cat: ""},
			},
			startAt: 0,
			wantIdx: 2,
		},
		{
			name: "wraps around to the first Unknown",
			rows: []managerRow{
				{id: 1, cat: ""},
				{id: 2, cat: "food"},
				{id: 3, cat: "transit"},
				{id: 4, cat: "food"},
			},
			startAt: 1,
			wantIdx: 0,
		},
		{
			name: "no Unknown rows anywhere",
			rows: []managerRow{
				{id: 1, cat: "food"},
				{id: 2, cat: "rent"},
			},
			startAt: 0,
			wantIdx: 0, // unchanged
		},
		{
			name: "Unknown on current row, jumps to next",
			rows: []managerRow{
				{id: 1, cat: ""},
				{id: 2, cat: "food"},
				{id: 3, cat: ""},
			},
			startAt: 0,
			wantIdx: 2,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := &Manager{rows: c.rows, cursor: c.startAt}
			m.jumpToNextUnknown()
			if m.cursor != c.wantIdx {
				t.Errorf("cursor: want %d got %d", c.wantIdx, m.cursor)
			}
		})
	}
}

func TestInputKindLabel(t *testing.T) {
	cases := []struct {
		k    inputKind
		want string
	}{
		{inputNone, ""},
		{inputCategory, "category"},
		{inputBucket, "bucket"},
		{inputTag, "tag"},
	}
	for _, c := range cases {
		if got := c.k.label(); got != c.want {
			t.Errorf("%d.label() = %q, want %q", int(c.k), got, c.want)
		}
	}
}
