package main

import "testing"

func TestSetupFreezeMode(t *testing.T) {
	tests := []struct {
		header             int
		wantRows, wantCols int
	}{
		{-1, 0, 0},
		{0, 1, 1},
		{1, 1, 0},
		{2, 0, 1},
	}
	defer args.setDefault()
	for _, tt := range tests {
		args.Header = tt.header
		b := createNewBuffer()
		setupFreezeMode(b)
		if b.rowFreeze != tt.wantRows || b.colFreeze != tt.wantCols {
			t.Errorf("freeze mode %d: rowFreeze=%d colFreeze=%d, want %d/%d", tt.header, b.rowFreeze, b.colFreeze, tt.wantRows, tt.wantCols)
		}
	}
}
