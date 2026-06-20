package main

import "fmt"

func fmtSscan(s string, ptrs ...any) (int, error) {
	return fmt.Sscan(s, ptrs...)
}