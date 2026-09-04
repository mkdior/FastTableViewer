package app

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestPreviewLayout(t *testing.T) {
	lines, w, h, ok := previewLayout("hello world", 80, 24)
	if !ok || len(lines) != 1 || w != len("hello world")+4 || h != 3 {
		t.Fatalf("short value: lines=%v w=%d h=%d ok=%v", lines, w, h, ok)
	}

	long := strings.Repeat("word ", 40) // 200 chars
	lines, w, h, ok = previewLayout(long, 80, 24)
	if !ok || len(lines) < 2 || w > 80 || h != len(lines)+2 {
		t.Fatalf("wrapped value: lines=%d w=%d h=%d ok=%v", len(lines), w, h, ok)
	}

	if _, _, _, ok := previewLayout(strings.Repeat("x", maxPreviewRunes+1), 200, 100); ok {
		t.Error("values over maxPreviewRunes must not be previewed")
	}
	if _, _, _, ok := previewLayout(strings.Repeat("word ", 100), 40, 10); ok {
		t.Error("a value taller than half the table must not be previewed")
	}
	if _, w, _, ok := previewLayout("日本語のテキスト", 80, 24); !ok || w != 16+4 {
		t.Errorf("wide runes must be measured by display width, got w=%d ok=%v", w, ok)
	}
}

func TestTruncatedCellTextUsesDisplayWidth(t *testing.T) {
	wide := strings.Repeat("日", 30) // 30 runes, 60 cells
	buf, _ := createNewBufferWithData([][]string{{"h"}, {wide}}, true)
	oldB, oldWrapped := b, wrappedColumns
	defer func() { b, wrappedColumns = oldB, oldWrapped }()
	b = buf
	wrappedColumns = map[int]int{0: 50}
	if _, ok := truncatedCellText(1, 0); !ok {
		t.Error("a 60-cell value under a 50-cell limit is cut and must be previewed")
	}
	wrappedColumns = map[int]int{0: 60}
	if _, ok := truncatedCellText(1, 0); ok {
		t.Error("a value that fits its limit must not be previewed")
	}
}

// screenText joins the simulation screen rows into one string.
func screenText(s tcell.SimulationScreen) string {
	cells, w, h := s.GetContents()
	var sb strings.Builder
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := cells[y*w+x]
			if len(c.Runes) > 0 {
				sb.WriteString(string(c.Runes))
			}
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func TestCellPreviewShowsFullValueForTruncatedCell(t *testing.T) {
	long := "The quick brown fox jumps over the lazy dog again and again until dusk"
	data := [][]string{{"id", "text"}, {"1", long}, {"2", "short"}}
	buf, err := createNewBufferWithData(data, true)
	if err != nil {
		t.Fatal(err)
	}
	buf.rowFreeze = 1

	// Wire up the globals the UI reads, restoring them afterwards.
	oldB, oldWrapped, oldTable, oldPage, oldView := b, wrappedColumns, bufferTable, mainPage, mainView
	defer func() {
		b, wrappedColumns, bufferTable, mainPage, mainView = oldB, oldWrapped, oldTable, oldPage, oldView
	}()
	b = buf
	wrappedColumns = map[int]int{1: 20}
	setSearchResults(nil)
	searchQuery = ""

	bufferTable = tview.NewTable().SetSelectable(true, true).SetFixed(1, 1)
	drawBuffer(b, bufferTable)
	bufferTable.Focus(func(tview.Primitive) {})
	mainPage = tview.NewFrame(bufferTable)
	mainView = newCellPreview(mainPage)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(100, 24)
	mainView.SetRect(0, 0, 100, 24)

	bufferTable.Select(1, 1)
	updateCellPreview(1, 1)
	mainView.Draw(screen)
	screen.Show()
	out := screenText(screen)
	if !strings.Contains(out, "again and again until dusk") {
		t.Fatalf("full value must be visible in the preview box:\n%s", out)
	}
	if !strings.Contains(out, "...") {
		t.Errorf("the table cell itself should still be cut with an ellipsis:\n%s", out)
	}
	if !strings.Contains(out, " text ") {
		t.Errorf("the box should be titled with the column name:\n%s", out)
	}

	screen.Clear()
	bufferTable.Select(2, 1)
	updateCellPreview(2, 1)
	mainView.Draw(screen)
	screen.Show()
	if out := screenText(screen); strings.Contains(out, "until dusk") {
		t.Errorf("preview must disappear on a cell that is not cut:\n%s", out)
	}
}
