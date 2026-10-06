package cli

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	c "github.com/gookit/color"

	"github.com/katbyte/go-kt/cout"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora"
)

var ansiCodes = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// printed runs something that prints and returns what it printed, without
// its colours.
func printed(t *testing.T, write func()) string {
	t.Helper()

	var out bytes.Buffer
	old := cout.Out
	cout.Out = &out
	defer func() { cout.Out = old }()
	write()

	return ansiCodes.ReplaceAllString(out.String(), "")
}

// A coloured cell is as wide as it looks: the tags that colour it take no
// room on screen and must take none in the sums.
func TestTableLinesUpWhateverTheColours(t *testing.T) { //nolint:paralleltest // prints through the process's console
	got := printed(t, func() {
		Table([][]string{
			{Dim("NAME"), Dim("RUNNING"), Dim("PALETTE")},
			{Name("office"), Scene("Northern Lights"), blocks([]aurora.Color{{Hue: 0, Saturation: 100, Brightness: 100}, {Hue: 120, Saturation: 100, Brightness: 100}}, everyColour)},
			{Name("a"), "<magenta;op=bold>Été ▶</>", "plain"},
			{Name("short row")},
		})
	})
	want := strings.Join([]string{
		"NAME       RUNNING          PALETTE",
		"office     Northern Lights  ██",
		"a          Été ▶            plain",
		"short row",
		"",
	}, "\n")
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}

	if got := printed(t, func() { Table(nil) }); got != "" {
		t.Errorf("no rows printed %q", got)
	}
}

func TestWidth(t *testing.T) {
	t.Parallel()

	for cell, want := range map[string]int{
		"":                               0,
		"plain":                          5,
		"<cyan>office</>":                6,
		"<green>on</> <yellow>33%</>":    6,
		"<fg=255,0,0>█</><fg=208>█</>":   2,
		"<white;op=bold>Light Panels</>": 12,
		"‹red›":                          5, // a name that looked like a tag, defused: it is text, and takes room
		"Été ▶":                          5, // letters and marks beyond ASCII are one column each
	} {
		if got := width(cell); got != want {
			t.Errorf("width(%q) = %d, want %d", cell, got, want)
		}
	}
}

func TestColoursByWhatAThingIs(t *testing.T) {
	t.Parallel()

	for got, want := range map[string]string{
		Name("office"):         "<cyan>office</>",
		Device("Light Panels"): "<white;op=bold>Light Panels</>",
		Scene("Flames"):        "<magenta>Flames</>",
		Num(17):                "<yellow>17</>",
		Dim("10.0.5.183"):      "<darkGray>10.0.5.183</>",
		Note("take note"):      "<fg=208>take note</>",
		// whatever a controller or a person called something, it is printed as text, never read as a colour
		Scene("<red>alert</>"): "<magenta>‹red›alert‹/›</>",
		Name("<b>"):            "<cyan>‹b›</>",
	} {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}

	for in, want := range map[string]string{
		"Northern Lights": "Northern Lights",
		"a < b":           "a < b", // one bracket cannot be a tag
		"b > a":           "b > a",
		"100%":            "100%",
		"<red>x</>":       "‹red›x‹/›",
	} {
		if got := Escape(in); got != want {
			t.Errorf("Escape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSwatch(t *testing.T) {
	t.Parallel()

	palette := []aurora.Color{
		{Hue: 0, Saturation: 100, Brightness: 100},
		{Hue: 120, Saturation: 100, Brightness: 100},
		{Hue: 240, Saturation: 100, Brightness: 50},
	}
	if got := blocks(palette, everyColour); got != "<fg=255,0,0>█</><fg=0,255,0>█</><fg=0,0,128>█</>" {
		t.Errorf("in every colour: %q", got)
	}
	// a terminal with 256 colours gets the nearest of them
	got := blocks(palette, colours256)
	if strings.Count(got, "█") != 3 || !regexp.MustCompile(`^(<fg=\d{1,3}>█</>){3}$`).MatchString(got) {
		t.Errorf("in 256 colours: %q", got)
	}
	if width(got) != 3 || c.ClearTag(got) != "███" {
		t.Errorf("a swatch is a block a colour: %q", c.ClearTag(got))
	}
	// with no colours to show them in, a row of blocks says nothing: there is none
	if got := blocks(palette, noColours); got != "" {
		t.Errorf("with no colours: %q", got)
	}
	if got := blocks(nil, everyColour); got != "" {
		t.Errorf("no palette: %q", got)
	}
	// whatever this terminal can do, the answer is one of the three
	if got := Swatch(palette); got != "" && strings.Count(got, "█") != 3 {
		t.Errorf("on this terminal: %q", got)
	}
}
