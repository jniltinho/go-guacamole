package server

import (
	"strings"
	"testing"
)

func TestTerminalColorScheme(t *testing.T) {
	for id, s := range terminalSchemes {
		g := s.guacd()
		if !strings.HasPrefix(g, "foreground: rgb:") || !strings.Contains(g, "color15: rgb:") || strings.Count(g, ";") != 17 {
			t.Fatalf("%s: bad palette %q", id, g)
		}
	}
	for theme, id := range themeSchemes {
		if _, ok := terminalSchemes[id]; !ok {
			t.Fatalf("theme %s follows unknown scheme %s", theme, id)
		}
	}
	if got := terminalColorScheme("green-black", "dracula", "light"); got != "green-black" {
		t.Fatalf("connection preset must win: %q", got)
	}
	if got := terminalColorScheme("nord", "dracula", "light"); got != terminalSchemes["nord"].guacd() {
		t.Fatal("named connection scheme not expanded")
	}
	if got := terminalColorScheme("", "dracula", "light"); got != terminalSchemes["dracula"].guacd() {
		t.Fatal("user preference ignored")
	}
	if got := terminalColorScheme("", "follow", "noc"); got != terminalSchemes["noc"].guacd() {
		t.Fatal("follow must use the UI theme palette")
	}
	if got := terminalColorScheme("", "", ""); got != terminalSchemes["gogu-dark"].guacd() {
		t.Fatal("default palette")
	}
	if !validTerminalScheme("") || !validTerminalScheme("follow") || !validTerminalScheme("monokai") || validTerminalScheme("pink") {
		t.Fatal("validation")
	}
}
