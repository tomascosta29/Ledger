package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseInt64List(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []int64
		wantErr bool
	}{
		{"single id", "42", []int64{42}, false},
		{"comma-separated", "1,2,3", []int64{1, 2, 3}, false},
		{"with spaces", "1, 2, 3", []int64{1, 2, 3}, false},
		{"dedupes repeats", "1,2,1,3,2", []int64{1, 2, 3}, false},
		{"empty entry", "1,,2", nil, true},
		{"non-numeric", "1,abc,2", nil, true},
		{"zero", "0,1", nil, true},
		{"negative", "-5,1", nil, true},
		{"empty string", "", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseInt64List(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJoinIDs(t *testing.T) {
	got := joinIDs([]int64{1, 2, 3})
	if got != "1,2,3" {
		t.Fatalf("got %q, want %q", got, "1,2,3")
	}
	if got != strings.Join([]string{"1", "2", "3"}, ",") {
		t.Fatalf("round-trip via strings.Join failed: %q", got)
	}
}