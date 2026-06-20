package main

import (
	"fmt"
	"strings"
)

func fmtSscan(s string, ptrs ...any) (int, error) {
	return fmt.Sscan(s, ptrs...)
}

func parseInt64List(s string) ([]int64, error) {
	parts := strings.Split(s, ",")
	out := make([]int64, 0, len(parts))
	seen := make(map[int64]struct{}, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return nil, fmt.Errorf("invalid transaction id list: %q (empty entry)", s)
		}
		var n int64
		if _, err := fmt.Sscan(p, &n); err != nil || n <= 0 {
			return nil, fmt.Errorf("invalid transaction id: %q", p)
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no transaction ids in: %q", s)
	}
	return out, nil
}

func joinIDs(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprint(id)
	}
	return strings.Join(parts, ",")
}
