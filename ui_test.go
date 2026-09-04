package main

import "testing"

func TestCountPrefix(t *testing.T) {
	pendingCount = 0
	defer func() { pendingCount = 0 }()

	if pushCountDigit('0') {
		t.Fatal("a leading 0 must not start a count; it is the first-column binding")
	}
	for _, r := range "120" {
		if !pushCountDigit(r) {
			t.Fatalf("digit %q was not consumed", r)
		}
	}
	if pushCountDigit('j') {
		t.Fatal("non-digits must not be consumed as count digits")
	}
	if raw, count := takeCount(); raw != 120 || count != 120 {
		t.Fatalf("takeCount() = (%d, %d), want (120, 120)", raw, count)
	}
	if raw, count := takeCount(); raw != 0 || count != 1 {
		t.Fatalf("takeCount() after consumption = (%d, %d), want (0, 1)", raw, count)
	}
}

func TestCountPrefixIsCapped(t *testing.T) {
	pendingCount = 0
	defer func() { pendingCount = 0 }()
	for i := 0; i < 30; i++ {
		pushCountDigit('9')
	}
	raw, _ := takeCount()
	if raw <= 0 || raw > maxCountPrefix*10 {
		t.Fatalf("count overflowed or went negative: %d", raw)
	}
}

func TestClampInt(t *testing.T) {
	tests := []struct{ v, lo, hi, want int }{
		{5, 0, 10, 5}, {-3, 0, 10, 0}, {42, 0, 10, 10}, {3, 0, -1, 0},
	}
	for _, tt := range tests {
		if got := clampInt(tt.v, tt.lo, tt.hi); got != tt.want {
			t.Errorf("clampInt(%d, %d, %d) = %d, want %d", tt.v, tt.lo, tt.hi, got, tt.want)
		}
	}
}
