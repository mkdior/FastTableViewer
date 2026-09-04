package app

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

func TestClampRow_NeverSelectsHeader(t *testing.T) {
	// 150 data rows under a frozen header, as in the reported bug.
	rows := [][]string{{"id", "v"}}
	for i := 1; i <= 150; i++ {
		rows = append(rows, []string{I2S(i), "x"})
	}
	buf, err := createNewBufferWithData(rows, true)
	if err != nil {
		t.Fatal(err)
	}
	buf.rowFreeze = 1

	last := buf.rowLen - 1
	if got := firstDataRow(buf); got != 1 {
		t.Fatalf("firstDataRow = %d, want 1", got)
	}
	tests := []struct {
		name              string
		from, delta, want int
	}{
		{"200k from the bottom lands on the first data row", last, -200, 1},
		{"200000k from the bottom lands on the first data row", last, -200000, 1},
		{"1k from the first data row stays there", 1, -1, 1},
		{"200j from the bottom stays on the last row", last, 200, last},
		{"5j from the top", 1, 5, 6},
	}
	for _, tt := range tests {
		if got := clampRow(tt.from+tt.delta, buf); got != tt.want {
			t.Errorf("%s: clampRow(%d) = %d, want %d", tt.name, tt.from+tt.delta, got, tt.want)
		}
	}

	noHeader := createNewBuffer()
	_ = noHeader.contAppendSli([]string{"a"}, false)
	_ = noHeader.contAppendSli([]string{"b"}, false)
	noHeader.rowFreeze = 0
	if got := clampRow(-5, noHeader); got != 0 {
		t.Errorf("without a header row 0 is selectable, got %d", got)
	}
}
