package app

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/gdamore/tcell/v2"
)

// maxYankBytes caps what a single yank will send to the clipboard.
const maxYankBytes = 50 << 20

// maxOSC52Bytes caps the payload sent with the OSC 52 escape; terminals and
// tmux drop very large sequences, so bigger yanks rely on a clipboard tool.
const maxOSC52Bytes = 1 << 20

// screenRef is the tcell screen the application draws on, captured so the
// clipboard code can emit OSC 52 through it.
var screenRef tcell.Screen

// Hooks for tests.
var (
	clipLookPath = exec.LookPath
	clipRun      = runClipboardTool
	clipGetenv   = os.Getenv
	clipGOOS     = runtime.GOOS
	clipIsWSL    = detectWSL
)

// clipboardCommand picks the clipboard tool for this session: clip.exe under
// WSL, pbcopy on macOS, wl-copy on Wayland, xclip or xsel on X11, and
// otherwise the first of those that is installed.
func clipboardCommand() (name string, cmdArgs []string, ok bool) {
	candidates := [][]string{
		{"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}, {"pbcopy"}, {"clip.exe"},
	}
	var preferred [][]string
	switch {
	case clipIsWSL():
		preferred = [][]string{{"clip.exe"}}
	case clipGOOS == "darwin":
		preferred = [][]string{{"pbcopy"}}
	case clipGetenv("WAYLAND_DISPLAY") != "":
		preferred = [][]string{{"wl-copy"}}
	case clipGetenv("DISPLAY") != "":
		preferred = [][]string{{"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}}
	}
	for _, c := range append(preferred, candidates...) {
		if _, err := clipLookPath(c[0]); err == nil {
			return c[0], c[1:], true
		}
	}
	return "", nil, false
}

// detectWSL reports whether the process runs under Windows Subsystem for Linux.
func detectWSL() bool {
	if clipGetenv("WSL_DISTRO_NAME") != "" || clipGetenv("WSL_INTEROP") != "" {
		return true
	}
	version, err := os.ReadFile("/proc/version")
	return err == nil && bytes.Contains(bytes.ToLower(version), []byte("microsoft"))
}

// runClipboardTool pipes text into the tool's stdin.
func runClipboardTool(name string, cmdArgs []string, text string) error {
	cmd := exec.Command(name, cmdArgs...)
	cmd.Stdin = strings.NewReader(text)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s: %s", name, msg)
	}
	return nil
}

// copyToClipboard sends text to the system clipboard through every channel
// that can reach it: the OSC 52 escape (which tmux forwards when
// set-clipboard is on) and a clipboard tool when one is installed. It returns
// a short description of the channels used, or an error when none applied.
func copyToClipboard(text string) (string, error) {
	if len(text) > maxYankBytes {
		return "", fmt.Errorf("selection is %s; the limit is %s", formatBytes(int64(len(text))), formatBytes(maxYankBytes))
	}
	var channels []string
	var problems []string

	if screenRef != nil && len(text) <= maxOSC52Bytes {
		screenRef.SetClipboard([]byte(text))
		channels = append(channels, "OSC 52")
	}
	if name, cmdArgs, ok := clipboardCommand(); ok {
		if err := clipRun(name, cmdArgs, text); err != nil {
			problems = append(problems, err.Error())
		} else {
			channels = append(channels, name)
		}
	} else if len(channels) == 0 {
		problems = append(problems, "no clipboard tool found (wl-copy, xclip, xsel, pbcopy or clip.exe)")
	}

	if len(channels) == 0 {
		return "", errors.New(strings.Join(problems, "; "))
	}
	return strings.Join(channels, " + "), nil
}

// tsv joins cells with tabs and rows with newlines; a single cell is returned as is.
func tsv(rows [][]string) string {
	if len(rows) == 1 && len(rows[0]) == 1 {
		return rows[0][0]
	}
	var sb strings.Builder
	for i, row := range rows {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(strings.Join(row, "\t"))
	}
	return sb.String()
}

// yankCells copies a rectangular block of b to the clipboard and reports the
// outcome in the footer. Rows r1..r2 and columns c1..c2 are inclusive.
func yankCells(r1, c1, r2, c2 int) {
	rows := b.cellBlock(r1, c1, r2, c2)
	text := tsv(rows)
	what := fmt.Sprintf("%d rows x %d columns", len(rows), c2-c1+1)
	if len(rows) == 1 && c1 == c2 {
		what = "1 cell"
	} else if c1 == 0 && c2 == b.colLen-1 {
		what = fmt.Sprintf("%d rows", len(rows))
	}
	channels, err := copyToClipboard(text)
	if err != nil {
		drawFooterText(fileNameStr, "Clipboard failed: "+err.Error(), cursorPosStr)
		return
	}
	drawFooterText(fileNameStr, fmt.Sprintf("Yanked %s (%s) via %s", what, formatBytes(int64(len(text))), channels), cursorPosStr)
}
