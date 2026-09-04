package app

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// setupVisualTable installs a 5x4 table (header + 4 rows) as the UI globals.
func setupVisualTable(t *testing.T) {
	t.Helper()
	data := [][]string{{"h1", "h2", "h3", "h4"}}
	for r := 1; r <= 4; r++ {
		data = append(data, []string{"a" + I2S(r), "b" + I2S(r), "c" + I2S(r), "d" + I2S(r)})
	}
	buf, err := createNewBufferWithData(data, true)
	if err != nil {
		t.Fatal(err)
	}
	buf.rowFreeze = 1
	oldB, oldTable, oldPage, oldView, oldKeys := b, bufferTable, mainPage, mainView, keys
	t.Cleanup(func() {
		b, bufferTable, mainPage, mainView, keys = oldB, oldTable, oldPage, oldView, oldKeys
		visual, pendingCount, pendingChord = visualOff, 0, nil
	})
	b = buf
	mainPage, mainView = nil, nil
	keys = defaultKeymap()
	bufferTable = tview.NewTable().SetSelectable(true, true).SetFixed(1, 1)
	drawBuffer(b, bufferTable)
	bufferTable.Select(1, 0)
	visual = visualOff
}

func press(t *testing.T, spec string) {
	t.Helper()
	for _, k := range strings.Fields(spec) {
		stroke, err := parseStroke(k)
		if err != nil {
			t.Fatal(err)
		}
		if stroke.key == tcell.KeyRune {
			handleTableKey(tcell.NewEventKey(tcell.KeyRune, stroke.ch, tcell.ModNone))
		} else {
			handleTableKey(tcell.NewEventKey(stroke.key, 0, tcell.ModNone))
		}
	}
}

func TestVisualBlockSelectionAndYank(t *testing.T) {
	setupVisualTable(t)
	ran := stubClipboard(t, map[string]bool{"xclip": true}, map[string]string{"DISPLAY": ":0"}, "linux", false)

	press(t, "j l v") // anchor at (2,1)
	press(t, "j l")   // cursor at (3,2)
	if visual != visualBlock {
		t.Fatalf("visual = %v, want block", visual)
	}
	if r1, c1, r2, c2 := visualRect(); r1 != 2 || c1 != 1 || r2 != 3 || c2 != 2 {
		t.Fatalf("rect = %d,%d..%d,%d", r1, c1, r2, c2)
	}
	if !inVisual(2, 2) || inVisual(1, 1) || inVisual(2, 3) {
		t.Error("inVisual boundaries wrong")
	}
	if got := visualStatus(); !strings.Contains(got, "2 rows x 2 columns") {
		t.Errorf("status = %q", got)
	}

	press(t, "o") // swap: cursor goes back to the anchor
	if row, col := bufferTable.GetSelection(); row != 2 || col != 1 {
		t.Errorf("after o cursor = %d,%d, want 2,1", row, col)
	}
	if r1, c1, r2, c2 := visualRect(); r1 != 2 || c1 != 1 || r2 != 3 || c2 != 2 {
		t.Errorf("swap must keep the same rectangle, got %d,%d..%d,%d", r1, c1, r2, c2)
	}

	press(t, "y")
	if visual != visualOff {
		t.Error("y must leave visual mode")
	}
	want := "b2\tc2\nb3\tc3"
	if len(*ran) != 1 || (*ran)[0] != "xclip:"+want {
		t.Errorf("yanked %v, want %q", *ran, want)
	}
}

func TestVisualRowsCountsAndCancel(t *testing.T) {
	setupVisualTable(t)
	ran := stubClipboard(t, map[string]bool{"xclip": true}, map[string]string{"DISPLAY": ":0"}, "linux", false)

	press(t, "V 2 j") // rows 1..3, all columns
	if visual != visualRows {
		t.Fatalf("visual = %v, want rows", visual)
	}
	if r1, c1, r2, c2 := visualRect(); r1 != 1 || c1 != 0 || r2 != 3 || c2 != 3 {
		t.Fatalf("rect = %d,%d..%d,%d", r1, c1, r2, c2)
	}
	press(t, "v") // switch kind keeps the anchor
	if visual != visualBlock {
		t.Errorf("v in line mode should switch to block mode, got %v", visual)
	}
	press(t, "v") // same key again leaves
	if visual != visualOff {
		t.Errorf("pressing v twice must leave visual mode, got %v", visual)
	}

	press(t, "V esc")
	if visual != visualOff {
		t.Error("Esc must cancel visual mode")
	}
	press(t, "V j t") // a non-motion command (toggle type) leaves visual mode first
	if visual != visualOff {
		t.Error("a non-motion command must leave visual mode")
	}
	if len(*ran) != 0 {
		t.Errorf("nothing should have been yanked, got %v", *ran)
	}

	press(t, "g g V j Y")
	if got := (*ran)[len(*ran)-1]; got != "xclip:a1\tb1\tc1\td1\na2\tb2\tc2\td2" {
		t.Errorf("Y in visual yanked %q", got)
	}
}

func TestSingleYanks(t *testing.T) {
	setupVisualTable(t)
	ran := stubClipboard(t, map[string]bool{"xclip": true}, map[string]string{"DISPLAY": ":0"}, "linux", false)
	press(t, "j l y")
	press(t, "Y")
	if len(*ran) != 2 || (*ran)[0] != "xclip:b2" || (*ran)[1] != "xclip:a2\tb2\tc2\td2" {
		t.Errorf("yanks = %v", *ran)
	}
}

func TestUnboundKeysNeverReachTview(t *testing.T) {
	setupVisualTable(t)
	a, _ := parseChord("a")
	keys.set(actMoveLeft, [][]keyStroke{a}) // Left is no longer bound
	keys.set(actCancel, nil)                // Esc is unbound; it used to quit via tview's done func

	bufferTable.Select(2, 2)
	for _, k := range []tcell.Key{tcell.KeyLeft, tcell.KeyEscape, tcell.KeyEnter, tcell.KeyTab, tcell.KeyF5} {
		if ev := handleTableKey(tcell.NewEventKey(k, 0, tcell.ModNone)); ev != nil {
			t.Errorf("unbound key %v must be swallowed, was passed to tview", k)
		}
	}
	if row, col := bufferTable.GetSelection(); row != 2 || col != 2 {
		t.Errorf("unbound keys must not move the cursor, now at %d,%d", row, col)
	}
	press(t, "a")
	if _, col := bufferTable.GetSelection(); col != 1 {
		t.Errorf("the remapped key must move left, col = %d", col)
	}
}

func TestPagingActions(t *testing.T) {
	setupVisualTable(t)
	bufferTable.SetRect(0, 0, 40, 3) // header plus two visible rows
	press(t, "pgdn")
	if row, _ := bufferTable.GetSelection(); row != 3 {
		t.Errorf("PgDn should move a page (2 rows) from row 1, got %d", row)
	}
	press(t, "end")
	if row, _ := bufferTable.GetSelection(); row != 4 {
		t.Errorf("End should go to the last row, got %d", row)
	}
	press(t, "home")
	if row, _ := bufferTable.GetSelection(); row != 1 {
		t.Errorf("Home should go to the first data row, got %d", row)
	}
}
