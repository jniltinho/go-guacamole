package server

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/labstack/echo/v5"
)

// Terminal color schemes for SSH sessions: named palettes expanded to guacd's
// custom color-scheme syntax ("foreground: rgb:RR/GG/BB; background: ...; colorN: ...").
// A connection may set one (or a guacd preset / custom palette) in its
// color-scheme parameter; otherwise the user's preference applies, and
// "follow" picks the palette that matches the user's UI theme.

type termScheme struct {
	Label string     `json:"label"`
	Dark  bool       `json:"dark"`
	FG    string     `json:"foreground"`
	BG    string     `json:"background"`
	ANSI  [16]string `json:"colors"`
}

// guacd renders the custom palette.
func (s termScheme) guacd() string {
	rgb := func(h string) string {
		h = strings.TrimPrefix(h, "#")
		return fmt.Sprintf("rgb:%s/%s/%s", h[0:2], h[2:4], h[4:6])
	}
	parts := []string{"foreground: " + rgb(s.FG), "background: " + rgb(s.BG)}
	for i, c := range s.ANSI {
		parts = append(parts, fmt.Sprintf("color%d: %s", i, rgb(c)))
	}
	return strings.Join(parts, "; ")
}

const followTheme = "follow"

var terminalSchemes = map[string]termScheme{
	"gogu-dark": {"GoGuacamole Dark", true, "#d4dbe5", "#0b1120", [16]string{
		"#1c2433", "#f0716a", "#5ed39a", "#e8c26a", "#72a8f5", "#c792ea", "#5ccfe6", "#c8d1dc",
		"#4b5a70", "#ff8f87", "#7ee7b0", "#f5d88a", "#94bfff", "#dcb3f5", "#86e1f0", "#f4f7fb"}},
	"gogu-light": {"GoGuacamole Light", false, "#1b2433", "#fbfcfe", [16]string{
		"#1b2433", "#b42318", "#11722f", "#8a5a00", "#1d4ed8", "#7c3aed", "#0e7490", "#5c6a7e",
		"#3b4a60", "#d92d20", "#15803d", "#a16207", "#2563eb", "#9333ea", "#0891b2", "#94a3b8"}},
	"noc": {"NOC", true, "#c9d6e3", "#070b11", [16]string{
		"#1a222d", "#f85149", "#3fb950", "#d29922", "#58a6ff", "#bc8cff", "#39c5cf", "#b1bac4",
		"#4b5866", "#ff7b72", "#56d364", "#e3b341", "#79c0ff", "#d2a8ff", "#56d4dd", "#f0f6fc"}},
	"bionexo": {"Bionexo", true, "#e6f1f6", "#13293b", [16]string{
		"#1d3c55", "#f27a7a", "#5fd39b", "#f2c76e", "#6ea8dc", "#b79ce8", "#34cdd7", "#cfe0ea",
		"#4b6880", "#ff9b9b", "#7fe6b4", "#f8d98f", "#93c2ec", "#cfb8f2", "#6ee0e7", "#ffffff"}},
	"matrix": {"Matrix", true, "#4dff7c", "#000000", [16]string{
		"#001a08", "#3cff6a", "#00ff41", "#a8ff60", "#2bd65a", "#5cff8a", "#8dffaa", "#b6ffc8",
		"#0f7a2e", "#7dff9a", "#66ff88", "#ccff99", "#55ee80", "#99ffb3", "#b3ffcc", "#e6ffee"}},
	"high-contrast": {"High contrast", true, "#ffffff", "#000000", [16]string{
		"#000000", "#ff6e6e", "#3ff23f", "#ffff00", "#7ab8ff", "#ff7af8", "#1aebff", "#ffffff",
		"#9a9a9a", "#ff9b9b", "#8aff8a", "#ffff8a", "#a8d0ff", "#ffa8fa", "#8af4ff", "#ffffff"}},
	"solarized-dark": {"Solarized Dark", true, "#839496", "#002b36", [16]string{
		"#073642", "#dc322f", "#859900", "#b58900", "#268bd2", "#d33682", "#2aa198", "#eee8d5",
		"#002b36", "#cb4b16", "#586e75", "#657b83", "#839496", "#6c71c4", "#93a1a1", "#fdf6e3"}},
	"solarized-light": {"Solarized Light", false, "#657b83", "#fdf6e3", [16]string{
		"#073642", "#dc322f", "#859900", "#b58900", "#268bd2", "#d33682", "#2aa198", "#eee8d5",
		"#002b36", "#cb4b16", "#586e75", "#657b83", "#839496", "#6c71c4", "#93a1a1", "#fdf6e3"}},
	"dracula": {"Dracula", true, "#f8f8f2", "#282a36", [16]string{
		"#21222c", "#ff5555", "#50fa7b", "#f1fa8c", "#bd93f9", "#ff79c6", "#8be9fd", "#f8f8f2",
		"#6272a4", "#ff6e6e", "#69ff94", "#ffffa5", "#d6acff", "#ff92df", "#a4ffff", "#ffffff"}},
	"gruvbox-dark": {"Gruvbox Dark", true, "#ebdbb2", "#282828", [16]string{
		"#282828", "#cc241d", "#98971a", "#d79921", "#458588", "#b16286", "#689d6a", "#a89984",
		"#928374", "#fb4934", "#b8bb26", "#fabd2f", "#83a598", "#d3869b", "#8ec07c", "#ebdbb2"}},
	"gruvbox-light": {"Gruvbox Light", false, "#3c3836", "#fbf1c7", [16]string{
		"#fbf1c7", "#cc241d", "#98971a", "#d79921", "#458588", "#b16286", "#689d6a", "#7c6f64",
		"#928374", "#9d0006", "#79740e", "#b57614", "#076678", "#8f3f71", "#427b58", "#3c3836"}},
	"nord": {"Nord", true, "#d8dee9", "#2e3440", [16]string{
		"#3b4252", "#bf616a", "#a3be8c", "#ebcb8b", "#81a1c1", "#b48ead", "#88c0d0", "#e5e9f0",
		"#4c566a", "#bf616a", "#a3be8c", "#ebcb8b", "#81a1c1", "#b48ead", "#8fbcbb", "#eceff4"}},
	"one-dark": {"One Dark", true, "#abb2bf", "#282c34", [16]string{
		"#282c34", "#e06c75", "#98c379", "#e5c07b", "#61afef", "#c678dd", "#56b6c2", "#abb2bf",
		"#5c6370", "#e06c75", "#98c379", "#e5c07b", "#61afef", "#c678dd", "#56b6c2", "#ffffff"}},
	"monokai": {"Monokai", true, "#f8f8f2", "#272822", [16]string{
		"#272822", "#f92672", "#a6e22e", "#f4bf75", "#66d9ef", "#ae81ff", "#a1efe4", "#f8f8f2",
		"#75715e", "#f92672", "#a6e22e", "#f4bf75", "#66d9ef", "#ae81ff", "#a1efe4", "#f9f8f5"}},
	"tango-dark": {"Tango Dark", true, "#d3d7cf", "#2e3436", [16]string{
		"#2e3436", "#cc0000", "#4e9a06", "#c4a000", "#3465a4", "#75507b", "#06989a", "#d3d7cf",
		"#555753", "#ef2929", "#8ae234", "#fce94f", "#729fcf", "#ad7fa8", "#34e2e2", "#eeeeec"}},
	"tango-light": {"Tango Light", false, "#2e3436", "#eeeeec", [16]string{
		"#2e3436", "#cc0000", "#4e9a06", "#c4a000", "#3465a4", "#75507b", "#06989a", "#d3d7cf",
		"#555753", "#ef2929", "#8ae234", "#fce94f", "#729fcf", "#ad7fa8", "#34e2e2", "#eeeeec"}},
	"xterm": {"xterm", true, "#e5e5e5", "#000000", [16]string{
		"#000000", "#cd0000", "#00cd00", "#cdcd00", "#0000ee", "#cd00cd", "#00cdcd", "#e5e5e5",
		"#7f7f7f", "#ff0000", "#00ff00", "#ffff00", "#5c5cff", "#ff00ff", "#00ffff", "#ffffff"}},
	"campbell": {"Campbell (Windows Terminal)", true, "#cccccc", "#0c0c0c", [16]string{
		"#0c0c0c", "#c50f1f", "#13a10e", "#c19c00", "#0037da", "#881798", "#3a96dd", "#cccccc",
		"#767676", "#e74856", "#16c60c", "#f9f1a5", "#3b78ff", "#b4009e", "#61d6d6", "#f2f2f2"}},
	"ubuntu": {"Ubuntu", true, "#eeeeec", "#300a24", [16]string{
		"#2e3436", "#cc0000", "#4e9a06", "#c4a000", "#3465a4", "#75507b", "#06989a", "#d3d7cf",
		"#555753", "#ef2929", "#8ae234", "#fce94f", "#729fcf", "#ad7fa8", "#34e2e2", "#eeeeec"}},
	"breeze": {"Breeze (KDE)", true, "#fcfcfc", "#232627", [16]string{
		"#232627", "#ed1515", "#11d116", "#f67400", "#1d99f3", "#9b59b6", "#1abc9c", "#fcfcfc",
		"#7f8c8d", "#c0392b", "#1cdc9a", "#fdbc4b", "#3daee9", "#8e44ad", "#16a085", "#ffffff"}},
}

// themeSchemes is what "follow" picks for each UI theme.
var themeSchemes = map[string]string{
	"light": "gogu-light", "dark": "gogu-dark", "noc": "noc", "dracula": "dracula", "bionexo": "bionexo",
	"matrix": "matrix", "xterm": "xterm", "macmodern": "one-dark", "ubuntu": "ubuntu", "kde": "breeze",
	"google": "gogu-light", "contrast": "high-contrast", "win11": "campbell", "xp": "campbell",
	"win98": "xterm", "macos": "tango-light",
}

// validTerminalScheme accepts "" (follow), "follow" or a known palette name.
func validTerminalScheme(v string) bool {
	_, ok := terminalSchemes[v]
	return v == "" || v == followTheme || ok
}

// terminalColorScheme returns the guacd color-scheme for an SSH session: the
// connection's own value (named palettes expanded), else the user's preference,
// else the palette matching the user's UI theme.
func terminalColorScheme(connValue, userPref, uiTheme string) string {
	if connValue != "" {
		if s, ok := terminalSchemes[connValue]; ok {
			return s.guacd()
		}
		return connValue // guacd preset or custom palette, validated on save
	}
	name := userPref
	if name == "" || name == followTheme {
		name = themeSchemes[uiTheme]
		if name == "" {
			name = "gogu-dark"
		}
	}
	if s, ok := terminalSchemes[name]; ok {
		return s.guacd()
	}
	return terminalSchemes["gogu-dark"].guacd()
}

type schemeView struct {
	ID string `json:"id"`
	termScheme
}

// listTerminalSchemes serves the palettes for the settings and connection forms.
func (a *App) listTerminalSchemes(c *echo.Context) error {
	out := make([]schemeView, 0, len(terminalSchemes))
	for id, s := range terminalSchemes {
		out = append(out, schemeView{ID: id, termScheme: s})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return c.JSON(http.StatusOK, map[string]any{"schemes": out, "follow": themeSchemes})
}
