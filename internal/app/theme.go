package app

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// The palette mirrors the "Subcore" colours of the maintainer's tmux status
// bar, given as xterm-256 indices. tcell.PaletteColor emits the same indices
// tmux does, so a terminal palette override applies to both alike.
var (
	colBg     = tcell.PaletteColor(233) // background (tmux status-bg)
	colText   = tcell.PaletteColor(252) // default text (tmux status-fg)
	colDim    = tcell.PaletteColor(239) // secondary text, inactive windows
	colPanel  = tcell.PaletteColor(237) // raised surfaces: header, fields, mode-style
	colStripe = tcell.PaletteColor(235) // alternate rows in the statistics table
	colBorder = tcell.PaletteColor(238) // separators (tmux pane-border)
	colAccent = tcell.PaletteColor(101) // olive: active pane, current window, keys
	colAlert  = tcell.PaletteColor(131) // muted red: filters and attention
)

// Hex twins of the palette for tview's inline colour tags in the help text,
// which accept names and #rrggbb but not palette indices.
const (
	hexText   = "#d0d0d0" // colour252
	hexDim    = "#4e4e4e" // colour239
	hexAccent = "#87875f" // colour101
)

// selectedStyle is the cursor cell, styled like tmux's status-left segment.
var selectedStyle = tcell.Style{}.Background(colAccent).Foreground(colBg).Bold(true)

// highlightStyle marks secondary highlights such as other search matches,
// styled like tmux's mode-style (copy-mode selection).
var highlightStyle = tcell.Style{}.Background(colPanel).Foreground(colAccent)

// styleForm applies the palette to a dialog form, its checkboxes and buttons.
// Call it after all items and buttons have been added.
func styleForm(form *tview.Form) {
	form.SetBorder(true)
	form.SetBorderColor(colAccent)
	form.SetBackgroundColor(colBg)
	form.SetLabelColor(colText)
	form.SetFieldBackgroundColor(colPanel)
	form.SetFieldTextColor(colText)
	form.SetButtonBackgroundColor(colPanel)
	form.SetButtonTextColor(colText)
	for i := 0; i < form.GetButtonCount(); i++ {
		form.GetButton(i).SetActivatedStyle(selectedStyle)
	}
	for i := 0; i < form.GetFormItemCount(); i++ {
		if cb, ok := form.GetFormItem(i).(*tview.Checkbox); ok {
			cb.SetLabelColor(colText).SetFieldBackgroundColor(colPanel).SetFieldTextColor(colAccent)
		}
	}
}
