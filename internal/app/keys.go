package app

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gdamore/tcell/v2"
)

// chordTimeout is how long a partial key sequence (such as the first g of gg)
// waits for its next key before being discarded.
const chordTimeout = 500 * time.Millisecond

// Pending key sequence state for multi-key bindings.
var (
	pendingChord      []keyStroke
	pendingChordSince time.Time
)

// handleTableKey is the table's input capture: it turns key events into
// actions through the active keymap. Digits form a count prefix, unbound
// printable keys are swallowed so tview's own bindings cannot bypass the
// keymap, and unbound special keys pass through to tview.
func handleTableKey(event *tcell.EventKey) *tcell.EventKey {
	// Vim-style count prefix: digits accumulate and the next action uses
	// them (5j, 3l, 12G). A leading 0 is left to the keymap (first_column).
	if event.Key() == tcell.KeyRune && pushCountDigit(event.Rune()) {
		drawFooterText(fileNameStr, statusMessage, strconv.Itoa(pendingCount)+"  |  "+cursorPosStr)
		return nil
	}

	stroke := strokeFromEvent(event)
	if len(pendingChord) > 0 && time.Since(pendingChordSince) > chordTimeout {
		pendingChord = nil
	}
	seq := append(append([]keyStroke{}, pendingChord...), stroke)
	act, prefix := keys.resolve(seq)
	if act == "" && len(pendingChord) > 0 {
		// The sequence went nowhere: drop the prefix and retry the key alone.
		seq = []keyStroke{stroke}
		act, prefix = keys.resolve(seq)
	}
	if act == "" && prefix {
		pendingChord, pendingChordSince = seq, time.Now()
		return nil
	}
	pendingChord = nil
	if act == "" {
		pendingCount = 0
		if event.Key() == tcell.KeyRune {
			return nil
		}
		return event
	}

	rawCount, count := takeCount()
	info, _ := actionByName(string(act))
	if info.motion {
		userMovedCursor = true
		if rawCount > 0 {
			// Redraw the footer after the motion so the pending count disappears
			// even when the selection-changed throttle skips this update.
			defer drawFooterText(fileNameStr, statusMessage, cursorPosStr)
		}
	}
	runAction(act, rawCount, count)
	return nil
}

// runAction performs an action. rawCount is the typed count or 0; count is
// rawCount or 1, ready to use as a repeat factor.
func runAction(act action, rawCount, count int) {
	firstRow, lastRow, numCols := firstDataRow(b), b.rowLen-1, b.colLen
	row, col := bufferTable.GetSelection()

	switch act {
	case actMoveLeft, actPrevColumn:
		bufferTable.Select(row, wrapCol(col-count, numCols))
	case actMoveRight, actNextColumn:
		bufferTable.Select(row, wrapCol(col+count, numCols))
	case actMoveDown:
		bufferTable.Select(clampInt(row+count, firstRow, lastRow), col)
	case actMoveUp:
		bufferTable.Select(clampInt(row-count, firstRow, lastRow), col)
	case actFirstRow:
		bufferTable.Select(clampInt(rawCount, firstRow, lastRow), col)
		if rawCount == 0 {
			bufferTable.ScrollToBeginning()
		}
	case actLastRow:
		if rawCount > 0 {
			bufferTable.Select(clampInt(rawCount, firstRow, lastRow), col)
			return
		}
		bufferTable.Select(lastRow, col)
		bufferTable.ScrollToEnd()
	case actFirstColumn:
		bufferTable.Select(row, 0)
	case actLastColumn:
		bufferTable.Select(row, numCols-1)
	case actHalfPageDown, actHalfPageUp:
		step := halfPageRows(bufferTable)
		if rawCount > 0 {
			step = rawCount
		}
		if act == actHalfPageUp {
			step = -step
		}
		bufferTable.Select(clampInt(row+step, firstRow, lastRow), col)
	case actSearch:
		openSearchDialog()
	case actNextMatch:
		gotoSearchResult(count)
	case actPrevMatch:
		gotoSearchResult(-count)
	case actCancel:
		clearSearch()
	case actFilter:
		openFilterDialog()
	case actRemoveFilter:
		removeCurrentFilter()
	case actSortAsc:
		sortCurrentColumn(false)
	case actSortDesc:
		sortCurrentColumn(true)
	case actToggleType:
		toggleColumnType()
	case actToggleWidth:
		toggleColumnWidth()
	case actYank:
		yankCells(row, col, row, col)
	case actYankRow:
		yankCells(row, 0, row, numCols-1)
	case actStats:
		showCurrentColumnStats()
	case actHelp:
		showHelpDialog()
	case actQuit:
		app.Stop()
	}
}

// gotoSearchResult moves delta matches forward (or back when negative),
// wrapping around the result list.
func gotoSearchResult(delta int) {
	if len(searchResults) == 0 || currentSearchIndex < 0 {
		if searchQuery != "" {
			drawFooterText(fileNameStr, "No search results. Press / to search", cursorPosStr)
		}
		return
	}
	n := len(searchResults)
	currentSearchIndex = ((currentSearchIndex+delta)%n + n) % n
	bufferTable.Select(searchResults[currentSearchIndex].Row, searchResults[currentSearchIndex].Col)
	drawBuffer(b, bufferTable) // Redraw to update highlighting
	drawFooterText(fileNameStr, fmt.Sprintf("Match %d/%d", currentSearchIndex+1, n), cursorPosStr)
}

// clearSearch drops the search highlighting.
func clearSearch() {
	if searchQuery == "" {
		return
	}
	searchQuery = ""
	setSearchResults(nil)
	currentSearchIndex = -1
	drawBuffer(b, bufferTable)
	drawFooterText(fileNameStr, "Search cleared", cursorPosStr)
}

// sortCurrentColumn sorts by the selected column using its detected type.
func sortCurrentColumn(desc bool) {
	_, column := bufferTable.GetSelection()
	drawFooterText(fileNameStr, "Sorting...", cursorPosStr)
	app.ForceDraw()
	switch b.getColType(column) {
	case colTypeFloat:
		b.sortByNum(column, desc)
	case colTypeDate:
		b.sortByDate(column, desc)
	default:
		b.sortByStr(column, desc)
	}
	drawBuffer(b, bufferTable)
	drawFooterText(fileNameStr, "All Done", cursorPosStr)
}
