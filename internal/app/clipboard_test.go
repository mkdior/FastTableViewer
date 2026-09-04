package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func stubClipboard(t *testing.T, tools map[string]bool, env map[string]string, goos string, wsl bool) *[]string {
	t.Helper()
	oldLook, oldRun, oldEnv, oldGOOS, oldWSL, oldScreen := clipLookPath, clipRun, clipGetenv, clipGOOS, clipIsWSL, screenRef
	t.Cleanup(func() {
		clipLookPath, clipRun, clipGetenv, clipGOOS, clipIsWSL, screenRef = oldLook, oldRun, oldEnv, oldGOOS, oldWSL, oldScreen
	})
	var ran []string
	clipLookPath = func(name string) (string, error) {
		if tools[name] {
			return "/usr/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
	clipRun = func(tool clipTool, text string) error {
		ran = append(ran, tool.label+":"+text)
		if tool.name == "broken" {
			return errors.New("broken: boom")
		}
		return nil
	}
	oldOverride, oldOSC := clipboardOverride, clipboardOSC52
	t.Cleanup(func() { clipboardOverride, clipboardOSC52 = oldOverride, oldOSC })
	clipboardOverride, clipboardOSC52 = "", true
	clipGetenv = func(k string) string { return env[k] }
	clipGOOS = goos
	clipIsWSL = func() bool { return wsl }
	screenRef = nil
	return &ran
}

func TestClipboardCommandPreference(t *testing.T) {
	all := map[string]bool{"wl-copy": true, "xclip": true, "xsel": true, "pbcopy": true, "cmd.exe": true, "clip.exe": true, "termux-clipboard-set": true}
	cases := []struct {
		name  string
		tools map[string]bool
		env   map[string]string
		goos  string
		wsl   bool
		want  string // executable name
	}{
		{"windows copies through cmd.exe with a UTF-8 code page", all, nil, "windows", false, "cmd.exe"},
		{"wsl copies through cmd.exe too", all, nil, "linux", true, "cmd.exe"},
		{"wsl without cmd.exe falls back to clip.exe", map[string]bool{"clip.exe": true, "xclip": true}, nil, "linux", true, "clip.exe"},
		{"macOS prefers pbcopy", all, nil, "darwin", false, "pbcopy"},
		{"termux prefers its helper", all, map[string]string{"TERMUX_VERSION": "0.118"}, "android", false, "termux-clipboard-set"},
		{"wayland prefers wl-copy", all, map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, "linux", false, "wl-copy"},
		{"x11 prefers xclip", all, map[string]string{"DISPLAY": ":0"}, "linux", false, "xclip"},
		{"x11 without xclip uses xsel", map[string]bool{"xsel": true}, map[string]string{"DISPLAY": ":0"}, "linux", false, "xsel"},
		{"no hints take the first installed", all, nil, "linux", false, "wl-copy"},
	}
	for _, tc := range cases {
		stubClipboard(t, tc.tools, tc.env, tc.goos, tc.wsl)
		tool, ok := clipboardCommand()
		if !ok || tool.name != tc.want {
			t.Errorf("%s: got %q ok=%v, want %q", tc.name, tool.name, ok, tc.want)
		}
	}
	stubClipboard(t, map[string]bool{}, nil, "linux", false)
	if _, ok := clipboardCommand(); ok {
		t.Error("no tools installed must report ok=false")
	}

	stubClipboard(t, map[string]bool{}, nil, "linux", false)
	clipboardOverride = "my-copier --to clipboard"
	tool, ok := clipboardCommand()
	if !ok || tool.name != "my-copier" || len(tool.args) != 2 || tool.args[1] != "clipboard" {
		t.Errorf("config command must replace detection, got %+v ok=%v", tool, ok)
	}
}

func TestCopyToClipboard(t *testing.T) {
	ran := stubClipboard(t, map[string]bool{"xclip": true}, map[string]string{"DISPLAY": ":0"}, "linux", false)
	channels, err := copyToClipboard("hello")
	if err != nil || channels != "xclip" || len(*ran) != 1 || (*ran)[0] != "xclip:hello" {
		t.Errorf("tool path: channels=%q err=%v ran=%v", channels, err, *ran)
	}

	stubClipboard(t, map[string]bool{}, nil, "linux", false)
	if _, err := copyToClipboard("hello"); err == nil || !strings.Contains(err.Error(), "no clipboard tool") {
		t.Errorf("without a screen or tool the failure must be explained, got %v", err)
	}

	stubClipboard(t, map[string]bool{}, nil, "linux", false)
	screenRef = tcell.NewSimulationScreen("UTF-8")
	channels, err = copyToClipboard("hello")
	if err != nil || !strings.HasPrefix(channels, "OSC 52 only") {
		t.Errorf("OSC 52 alone must succeed but say it is unverified, got %q %v", channels, err)
	}
	clipboardOSC52 = false
	if _, err := copyToClipboard("hello"); err == nil {
		t.Error("with OSC 52 disabled and no tool, copying must fail")
	}

	stubClipboard(t, map[string]bool{}, nil, "linux", false)
	if _, err := copyToClipboard(strings.Repeat("x", maxYankBytes+1)); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Errorf("oversized yank must be refused, got %v", err)
	}
}

func TestTSV(t *testing.T) {
	if got := tsv([][]string{{"only"}}); got != "only" {
		t.Errorf("single cell = %q", got)
	}
	if got := tsv([][]string{{"a", "b"}, {"c", "d"}}); got != "a\tb\nc\td" {
		t.Errorf("block = %q", got)
	}
}

func TestCellBlock(t *testing.T) {
	buf, _ := createNewBufferWithData([][]string{{"h1", "h2", "h3"}, {"a", "b", "c"}, {"d", "e", "f"}}, true)
	got := buf.cellBlock(2, 2, 1, 0) // reversed corners are normalised
	want := [][]string{{"a", "b", "c"}, {"d", "e", "f"}}
	if len(got) != 2 || strings.Join(got[0], ",") != strings.Join(want[0], ",") || strings.Join(got[1], ",") != strings.Join(want[1], ",") {
		t.Errorf("cellBlock = %v, want %v", got, want)
	}
	if got := buf.cellBlock(1, 1, 1, 1); len(got) != 1 || got[0][0] != "b" {
		t.Errorf("single cell block = %v", got)
	}
	if got := buf.cellBlock(0, 5, 0, 9); len(got) != 1 || len(got[0]) != 1 || got[0][0] != "h3" {
		t.Errorf("out-of-range columns must clamp, got %v", got)
	}
}
