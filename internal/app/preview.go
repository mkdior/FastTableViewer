package app

import (
	"strings"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/rivo/uniseg"
)

const (
	// maxPreviewRunes is the longest value the preview box will show; longer
	// values cannot be read whole in a popup anyway.
	maxPreviewRunes = 1000
	// previewMaxWidth caps the text width of the preview box.
	previewMaxWidth = 100
)

// cellPreview is the main page (the table with its footer) plus a floating
// box that shows the full value of the selected cell whenever that cell was
// cut with an ellipsis by a column width limit.
type cellPreview struct {
	*tview.Frame
	box  *tview.TextView
	text string // full value to show; "" hides the box
	row  int    // selected row, decides whether the box goes above or below it
}

// newCellPreview wraps the main page frame.
func newCellPreview(frame *tview.Frame) *cellPreview {
	box := tview.NewTextView().SetWrap(false).SetTextColor(theme.Text)
	box.SetBackgroundColor(theme.Panel)
	box.SetBorder(true).SetBorderColor(theme.Accent).SetBorderPadding(0, 0, 1, 1)
	box.SetTitleAlign(tview.AlignLeft).SetTitleColor(theme.Accent)
	return &cellPreview{Frame: frame, box: box}
}

// show displays text in the box; an empty text hides it.
func (p *cellPreview) show(title, text string, row int) {
	p.text, p.row = text, row
	p.box.SetTitle(" " + title + " ")
}

// hide removes the box.
func (p *cellPreview) hide() { p.text = "" }

// Draw renders the page and then the box, placed in the half of the table
// that does not hold the selected row so the cursor stays visible.
func (p *cellPreview) Draw(screen tcell.Screen) {
	p.Frame.Draw(screen)
	if p.text == "" || !bufferTable.HasFocus() {
		return
	}
	tx, ty, tw, th := bufferTable.GetInnerRect()
	lines, w, h, ok := previewLayout(p.text, tw, th)
	if !ok {
		return
	}
	p.box.SetText(strings.Join(lines, "\n"))

	rowOffset, _ := bufferTable.GetOffset()
	screenRow := ty + p.row - rowOffset
	y := ty + th - h // below the cursor
	if screenRow >= ty+th/2 {
		y = ty + b.rowFreeze // above it, under the frozen header
	}
	p.box.SetRect(tx+(tw-w)/2, y, w, h)
	p.box.Draw(screen)
}

// previewLayout wraps text for a box inside a table area of availW x availH
// cells and returns the wrapped lines and the box size including its border
// and padding. ok is false when the value cannot be shown whole in half the
// area, in which case no box is drawn.
func previewLayout(text string, availW, availH int) (lines []string, width, height int, ok bool) {
	if utf8.RuneCountInString(text) > maxPreviewRunes {
		return nil, 0, 0, false
	}
	innerW := availW - 4
	if innerW > previewMaxWidth {
		innerW = previewMaxWidth
	}
	maxLines := availH/2 - 2
	if innerW < 10 || maxLines < 1 {
		return nil, 0, 0, false
	}
	lines = tview.WordWrap(text, innerW)
	if len(lines) > maxLines {
		return nil, 0, 0, false
	}
	longest := 0
	for _, line := range lines {
		if lw := uniseg.StringWidth(line); lw > longest {
			longest = lw
		}
	}
	return lines, longest + 4, len(lines) + 2, true
}

// truncatedCellText returns the full value of the cell when the column has a
// width limit that cuts this value, and false otherwise.
func truncatedCellText(row, col int) (string, bool) {
	width, limited := wrappedColumns[col]
	if !limited {
		return "", false
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if row < 0 || row >= b.rowLen || col < 0 || col >= len(b.cont[row]) {
		return "", false
	}
	text := b.cont[row][col]
	if uniseg.StringWidth(text) <= width {
		return "", false
	}
	return text, true
}

// columnTitle names a column by its header cell, or by its index without one.
func columnTitle(col int) string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.rowFreeze > 0 && b.rowLen > 0 && col < len(b.cont[0]) {
		return b.cont[0][col]
	}
	return "Column " + I2S(col)
}

// updateCellPreview shows or hides the preview for the selected cell.
func updateCellPreview(row, col int) {
	if mainView == nil {
		return
	}
	if text, ok := truncatedCellText(row, col); ok {
		mainView.show(columnTitle(col), text, row)
	} else {
		mainView.hide()
	}
}
