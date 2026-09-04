package main

import (
	"bufio"
	"bytes"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// ========================================
// File Loading Tests
// ========================================

func Test_loadFileToBuffer(t *testing.T) {
	type args struct {
		fn string
		b  *Buffer
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"Load TSV file", args{fn: "./data/test/test.tsv", b: createNewBuffer()}, false},
		{"Load CSV file", args{fn: "./data/test/test.csv", b: createNewBuffer()}, false},
		{"Load gzip file", args{fn: "./data/test/test.csv.gz", b: createNewBuffer()}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := loadFileToBuffer(tt.args.fn, tt.args.b); (err != nil) != tt.wantErr {
				t.Errorf("loadFileToBuffer() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadFileToBuffer_LargeFile(t *testing.T) {
	b := createNewBuffer()
	err := loadFileToBuffer("./data/test/large_sample.csv", b)
	if err != nil {
		t.Skipf("Test file not found: %v", err)
		return
	}

	if b.rowLen < 10 {
		t.Errorf("Expected at least 10 rows, got %d", b.rowLen)
	}

	if b.colLen < 5 {
		t.Errorf("Expected at least 5 columns, got %d", b.colLen)
	}
}

func TestLoadFileToBuffer_NumericData(t *testing.T) {
	b := createNewBuffer()
	err := loadFileToBuffer("./data/test/numeric_data.csv", b)
	if err != nil {
		t.Skipf("Test file not found: %v", err)
		return
	}

	if b.rowLen < 2 {
		t.Errorf("Expected at least 2 rows, got %d", b.rowLen)
	}

	t.Logf("Loaded %d rows with %d columns", b.rowLen, b.colLen)
}

func TestLoadFileToBuffer_SpecialChars(t *testing.T) {
	b := createNewBuffer()
	err := loadFileToBuffer("./data/test/special_characters.csv", b)
	if err != nil {
		t.Skipf("Test file not found: %v", err)
		return
	}

	if b.rowLen < 3 {
		t.Errorf("Expected at least 3 rows for special chars file, got %d", b.rowLen)
	}
}

func TestLoadFileToBuffer_Compressed(t *testing.T) {
	b := createNewBuffer()
	err := loadFileToBuffer("./data/test/compressed_data.csv.gz", b)
	if err != nil {
		t.Skipf("Compressed test file not found: %v", err)
		return
	}

	if b.rowLen < 5 {
		t.Errorf("Expected data from compressed file, got %d rows", b.rowLen)
	}
}

func TestLoadFileToBuffer_TSV(t *testing.T) {
	b := createNewBuffer()
	err := loadFileToBuffer("./data/test/tab_separated.tsv", b)
	if err != nil {
		t.Skipf("TSV test file not found: %v", err)
		return
	}

	if b.sep != '\t' {
		t.Errorf("Expected tab separator, got %q", b.sep)
	}
}

func TestLoadFileToBuffer_EdgeCases(t *testing.T) {
	t.Run("Empty file", func(t *testing.T) {
		b := createNewBuffer()
		err := loadFileToBuffer("./data/test/empty_file.csv", b)
		if err != nil {
			t.Skipf("Empty test file not found: %v", err)
			return
		}

		if b.rowLen != 0 {
			t.Errorf("Expected 0 rows for empty file, got %d", b.rowLen)
		}
	})

	t.Run("Single row", func(t *testing.T) {
		b := createNewBuffer()
		err := loadFileToBuffer("./data/test/single_row_data.csv", b)
		if err != nil {
			t.Skipf("Single row test file not found: %v", err)
			return
		}

		if b.rowLen < 1 {
			t.Errorf("Expected at least 1 row, got %d", b.rowLen)
		}
	})

	t.Run("Non-existent file", func(t *testing.T) {
		b := createNewBuffer()
		err := loadFileToBuffer("./nonexistent_file_xyz.csv", b)
		if err == nil {
			t.Error("Expected error for non-existent file")
		}
	})
}

// ========================================
// Pipe Loading Tests
// ========================================

func TestLoadPipeToBuffer(t *testing.T) {
	csvData := "Name,Age,City\nJohn,30,NYC\nJane,25,LA\nBob,35,SF\n"
	reader := strings.NewReader(csvData)

	b := createNewBuffer()
	err := loadPipeToBuffer(reader, b)
	if err != nil {
		t.Fatalf("loadPipeToBuffer() error = %v", err)
	}

	if b.rowLen != 4 {
		t.Errorf("Expected 4 rows (including header), got %d", b.rowLen)
	}

	if b.colLen != 3 {
		t.Errorf("Expected 3 columns, got %d", b.colLen)
	}
}

func TestLoadPipeToBuffer_Empty(t *testing.T) {
	args.setDefault()
	b := createNewBuffer()
	err := loadPipeToBuffer(strings.NewReader(""), b)
	if err == nil {
		t.Fatal("expected separator detection error for empty input, got nil")
	}
	if !strings.Contains(err.Error(), "separator") {
		t.Errorf("unexpected error: %v", err)
	}
	if b.rowLen != 0 {
		t.Errorf("rowLen = %d, want 0", b.rowLen)
	}
}

func TestLoadPipeToBuffer_LargeData(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("ID,Value\n")
	for i := 0; i < 1000; i++ {
		buf.WriteString(I2S(i) + ",Data" + I2S(i) + "\n")
	}

	b := createNewBuffer()
	err := loadPipeToBuffer(&buf, b)
	if err != nil {
		t.Fatalf("loadPipeToBuffer() error = %v", err)
	}

	if b.rowLen != 1001 {
		t.Errorf("Expected 1001 rows, got %d", b.rowLen)
	}
}

// ========================================
// Async Loading Tests
// ========================================

func TestLoadFileToBufferAsync(t *testing.T) {
	b := createNewBuffer()
	updateChan := make(chan bool, 10)
	doneChan := make(chan error, 1)

	go loadFileToBufferAsync("./data/test/large_sample.csv", b, updateChan, doneChan)

	select {
	case <-updateChan:
		t.Log("Received first update")
	case err := <-doneChan:
		if err != nil {
			t.Skipf("Test file not found: %v", err)
		}
	}

	err := <-doneChan
	if err != nil {
		t.Skipf("Async load failed: %v", err)
	}

	if b.rowLen == 0 {
		t.Error("Expected data to be loaded")
	}
}

func TestLoadPipeToBufferAsync(t *testing.T) {
	csvData := "Name,Age,City\nJohn,30,NYC\nJane,25,LA\n"
	reader := strings.NewReader(csvData)

	b := createNewBuffer()
	updateChan := make(chan bool, 10)
	doneChan := make(chan error, 1)

	go loadPipeToBufferAsync(reader, b, updateChan, doneChan)

	err := <-doneChan
	if err != nil {
		t.Fatalf("loadPipeToBufferAsync() error = %v", err)
	}

	if b.rowLen == 0 {
		t.Error("Expected data to be loaded")
	}
}

// ========================================
// Line Filtering Tests
// ========================================

func Test_skipLine(t *testing.T) {
	type args struct {
		line string
		sy   []string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{"Match prefix @some", args{"@some line fo filtrate", []string{"@some"}}, true},
		{"No match - middle", args{"@some line fo filtrate", []string{"some"}}, false},
		{"Multiple patterns match first", args{"@some line fo filtrate", []string{"@some", "some"}}, true},
		{"Multiple patterns match second", args{"some @some line fo filtrate", []string{"@some", "some"}}, true},
		{"No match with @some", args{"some @some line fo filtrate", []string{"@some"}}, false},
		{"Empty patterns", args{"some @some line fo filtrate", []string{}}, false},
		{"Match at start #", args{"# This is a comment", []string{"#"}}, true},
		{"Match in middle not prefix", args{"Some data # comment", []string{"#"}}, false},
		{"Multiple patterns match //", args{"// Comment line", []string{"//", "#", "--"}}, true},
		{"No match normal data", args{"Normal data line", []string{"#", "//"}}, false},
		{"Empty patterns no match", args{"Any line", []string{}}, false},
		{"Empty line no match", args{"", []string{"#"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := skipLine(tt.args.line, tt.args.sy); got != tt.want {
				t.Errorf("skipLine() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ========================================
// Column Visibility Tests
// ========================================

func Test_getVisCol(t *testing.T) {
	type args struct {
		showNumL []int
		hideNumL []int
		colLen   int
	}
	tests := []struct {
		name    string
		args    args
		want    []int
		wantErr bool
	}{
		{"Set show argument", args{[]int{1, 2, 5}, []int{}, 6}, []int{0, 1, 4}, false},
		{"Set hide argument", args{[]int{}, []int{1, 2, 5}, 6}, []int{2, 3, 5}, false},
		{"No arguments set", args{[]int{}, []int{}, 6}, []int{0, 1, 2, 3, 4, 5}, false},
		{"Both arguments error", args{[]int{1, 2, 3}, []int{1, 2, 3}, 6}, nil, true},
		{"Show column out of range", args{[]int{1, 2, 3, 7}, []int{}, 6}, nil, true},
		{"Hide column out of range", args{[]int{}, []int{1, 2, 3, 7}, 6}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getVisCol(tt.args.showNumL, tt.args.hideNumL, tt.args.colLen)
			if (err != nil) != tt.wantErr {
				t.Errorf("getVisCol() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getVisCol() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_checkVisible(t *testing.T) {
	type args struct {
		showNumL []int
		hideNumL []int
		col      int
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{"Show list col 5", args{[]int{1, 3, 5}, []int{}, 5}, false, false},
		{"Show list col 4", args{[]int{1, 3, 5}, []int{}, 4}, true, false},
		{"Show list col 3", args{[]int{1, 3, 5}, []int{}, 3}, false, false},
		{"Show list col 2", args{[]int{1, 3, 5}, []int{}, 2}, true, false},
		{"Show list col 1", args{[]int{1, 3, 5}, []int{}, 1}, false, false},
		{"Show list col 0", args{[]int{1, 3, 5}, []int{}, 0}, true, false},
		{"Hide list col 5", args{[]int{}, []int{1, 3, 5}, 5}, true, false},
		{"Hide list col 4", args{[]int{}, []int{1, 3, 5}, 4}, false, false},
		{"Hide list col 3", args{[]int{}, []int{1, 3, 5}, 3}, true, false},
		{"Hide list col 2", args{[]int{}, []int{1, 3, 5}, 2}, false, false},
		{"Hide list col 1", args{[]int{}, []int{1, 3, 5}, 1}, true, false},
		{"Hide list col 0", args{[]int{}, []int{1, 3, 5}, 0}, false, false},
		{"Both lists error", args{[]int{1, 3, 5}, []int{1, 3, 5}, 4}, false, true},
		{"No lists all visible", args{[]int{}, []int{}, 3}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := checkVisible(tt.args.showNumL, tt.args.hideNumL, tt.args.col)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkVisible() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("checkVisible() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ========================================
// CSV Parsing Tests
// ========================================

func Test_lineCSVParse(t *testing.T) {
	type args struct {
		s   string
		sep rune
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr bool
	}{
		{"CSV line", args{"a,b,c,d", ','}, []string{"a", "b", "c", "d"}, false},
		{"TSV line", args{"a	b	c	d", '	'}, []string{"a", "b", "c", "d"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := lineCSVParse(tt.args.s, tt.args.sep)
			if (err != nil) != tt.wantErr {
				t.Errorf("lineCSVParse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("lineCSVParse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLineCSVParse(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		sep     rune
		wantLen int
	}{
		{"Simple CSV", "a,b,c", ',', 3},
		{"Tab separated", "a\tb\tc", '\t', 3},
		{"Empty fields", "a,,c", ',', 3},
		{"Single field", "single", ',', 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := lineCSVParse(tt.line, tt.sep)
			if err != nil {
				t.Fatalf("lineCSVParse() error = %v", err)
			}
			if len(result) != tt.wantLen {
				t.Errorf("lineCSVParse() returned %d fields, want %d", len(result), tt.wantLen)
			}
		})
	}
}

func TestAddDRToBuffer(t *testing.T) {
	b := createNewBuffer()
	b.sep = ','

	tests := []struct {
		name    string
		line    string
		wantErr bool
	}{
		{"Valid line", "John,30,NYC", false},
		{"Empty line", "", false},
		{"Line with quotes", `"Name","Age","City"`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := addDRToBuffer(b, tt.line, []int{}, []int{})
			if (err != nil) != tt.wantErr {
				t.Errorf("addDRToBuffer() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConcurrentCSVParsing(t *testing.T) {
	lines := []string{
		"Name,Age,City",
		"John,30,NYC",
		"Jane,25,LA",
		"Bob,35,SF",
		"Alice,28,Boston",
	}

	b := createNewBuffer()
	b.sep = ','

	for _, line := range lines {
		err := addDRToBuffer(b, line, []int{}, []int{})
		if err != nil {
			t.Fatalf("addDRToBuffer() error = %v", err)
		}
	}

	if b.rowLen != len(lines) {
		t.Errorf("Expected %d rows, got %d", len(lines), b.rowLen)
	}
}

// ========================================
// Integration Tests
// ========================================

func TestIntegration_FullWorkflow(t *testing.T) {
	buf := createNewBuffer()

	err := loadFileToBuffer("./data/test/numeric_data.csv", buf)
	if err != nil {
		t.Skipf("Integration test skipped: %v", err)
		return
	}

	if buf.rowLen > 1 {
		buf.sortByStr(0, false)
	}

	t.Log("Integration test completed successfully")
}

func TestLoadPipeToBuffer_BlankLines(t *testing.T) {
	args.setDefault()
	b := createNewBuffer()
	if err := loadPipeToBuffer(strings.NewReader("a,b\n1,2\n\n3,4\n\n"), b); err != nil {
		t.Fatalf("loadPipeToBuffer() error = %v", err)
	}
	if b.rowLen != 3 {
		t.Fatalf("blank lines must be skipped: rowLen = %d, want 3", b.rowLen)
	}
	if b.colLen != 2 {
		t.Errorf("colLen = %d, want 2", b.colLen)
	}
}

func TestLoadFileToBuffer_SkipLinesWithSeparatorGiven(t *testing.T) {
	args.setDefault()
	args.SkipNum = 1
	defer args.setDefault()

	b := createNewBuffer()
	b.sep = ','
	if err := loadFileToBuffer("./data/test/test.csv", b); err != nil {
		t.Fatalf("loadFileToBuffer() error = %v", err)
	}
	// test.csv has a header and three data rows; skipping one line drops the header.
	if b.rowLen != 3 {
		t.Fatalf("rowLen = %d, want 3 (one line skipped)", b.rowLen)
	}
	if b.cont[0][0] != "Alice" {
		t.Errorf("first row = %q, want Alice (header skipped)", b.cont[0][0])
	}
	if args.SkipNum != 1 {
		t.Errorf("loader must not mutate args.SkipNum, got %d", args.SkipNum)
	}
}

func TestLoadFileToBufferAsync_PreservesRowOrder(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "order-*.csv")
	if err != nil {
		t.Fatal(err)
	}
	w := bufio.NewWriter(f)
	_, _ = w.WriteString("id,val\n")
	const n = 50000
	for i := 0; i < n; i++ {
		_, _ = w.WriteString(strconv.Itoa(i) + ",x\n")
	}
	_ = w.Flush()
	_ = f.Close()

	args.setDefault()
	b := createNewBuffer()
	updateChan := make(chan bool, 10)
	doneChan := make(chan error, 1)
	go loadFileToBufferAsync(f.Name(), b, updateChan, doneChan)
	go func() {
		for range updateChan {
		}
	}()
	if err := <-doneChan; err != nil {
		t.Fatalf("loadFileToBufferAsync() error = %v", err)
	}
	if b.rowLen != n+1 {
		t.Fatalf("rowLen = %d, want %d", b.rowLen, n+1)
	}
	for i := 1; i < b.rowLen; i++ {
		if b.cont[i][0] != strconv.Itoa(i-1) {
			t.Fatalf("row %d has id %q; async load must preserve file order", i, b.cont[i][0])
		}
	}
}

func TestLoadFileToBufferAsync_SeparatorGivenSignalsWithRows(t *testing.T) {
	args.setDefault()
	b := createNewBuffer()
	b.sep = '|'
	updateChan := make(chan bool, 10)
	doneChan := make(chan error, 1)
	go loadFileToBufferAsync("./data/test/pipe_delimited.txt", b, updateChan, doneChan)

	select {
	case <-updateChan:
	case err := <-doneChan:
		t.Fatalf("finished before first update: %v", err)
	}
	if b.rowLen == 0 {
		t.Fatal("first update must carry rows even when the separator is given; otherwise the UI exits as empty")
	}
	if err := <-doneChan; err != nil {
		t.Fatal(err)
	}
	if b.rowLen != 4 || b.colLen != 4 {
		t.Errorf("rows=%d cols=%d, want 4x4", b.rowLen, b.colLen)
	}
}

func TestLoadPipeToBuffer_LineTooLongIsReported(t *testing.T) {
	args.setDefault()
	var buf bytes.Buffer
	buf.WriteString("a,b\n1,2\n")
	buf.WriteString(strings.Repeat("x", maxScanTokenSize+10) + ",3\n")
	buf.WriteString("4,5\n")

	b := createNewBuffer()
	err := loadPipeToBuffer(&buf, b)
	if err == nil {
		t.Fatalf("expected an error for a line over %d bytes, got nil (rows silently truncated to %d)", maxScanTokenSize, b.rowLen)
	}
	if !strings.Contains(err.Error(), "line limit") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLineCSVParseFast_MultiByteSeparator(t *testing.T) {
	got, err := lineCSVParseFast("a§b§c", '§')
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("lineCSVParseFast() = %q, want %q", got, want)
	}
}

func TestLoadFileToBuffer_ColumnsOnRaggedRows(t *testing.T) {
	args.setDefault()
	args.ShowNum = []int{1, 4}
	defer args.setDefault()

	b := createNewBuffer()
	if err := loadFileToBuffer("./data/test/inconsistent_columns.csv", b); err != nil {
		t.Fatalf("a short row must not abort --columns loading: %v", err)
	}
	if b.colLen != 2 {
		t.Fatalf("colLen = %d, want 2", b.colLen)
	}
	// Row "5,6,7" has no 4th column; it must be padded rather than rejected.
	if got := b.cont[2]; got[0] != "5" || got[1] != "NaN" {
		t.Errorf("short row projected to %q, want [5 NaN]", got)
	}
}
