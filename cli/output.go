package cli

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	c "github.com/gookit/color"

	"github.com/katbyte/go-kt/cout"
	"github.com/katbyte/nanoleaf-aurora-taproot/lib/push"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora"
)

// WouldSend is one request a dry run kept back.
type WouldSend struct {
	Controller string          `json:"controller"`
	Method     string          `json:"method"`
	Path       string          `json:"path"`
	Body       json.RawMessage `json:"body,omitempty"`
}

// wouldSend prints a request a dry run kept back, exactly as it would have
// gone, and keeps it for the --json document.
func (f *FlagData) wouldSend(controller string, r aurora.Request) {
	// one at a time: several controllers may be written to at once, and a request's lines belong together
	f.shared.mu.Lock()
	defer f.shared.mu.Unlock()
	f.shared.would = append(f.shared.would, WouldSend{Controller: controller, Method: r.Method, Path: r.Path, Body: r.Body})

	cout.Printf("<yellow>dry run:</> would send to <cyan>%s</>\n", controller)
	// the body goes out untouched: through cout a scene named <red> would be read as a colour
	w := cout.Writer()
	_, _ = fmt.Fprintf(w, "  %s %s\n", r.Method, r.Path)
	if len(r.Body) > 0 {
		_, _ = fmt.Fprintf(w, "  %s\n", r.Body)
	}
}

// Emit prints v as the command's JSON document when --json was given, and
// reports whether it did. A command builds what it found, hands it here, and
// prints it as text only when this says it has not been printed already. In a
// dry run the requests that were kept back go in the document too.
func (f *FlagData) Emit(v any) (bool, error) {
	if !f.Out.JSON {
		return false, nil
	}

	doc := v
	if f.DryRun {
		f.shared.mu.Lock()
		doc = struct {
			Result    any         `json:"result"`
			WouldSend []WouldSend `json:"wouldSend"`
		}{v, append([]WouldSend{}, f.shared.would...)}
		f.shared.mu.Unlock()
	}
	enc := json.NewEncoder(cout.Out)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)

	return true, enc.Encode(doc)
}

// The colours taproot prints in go by what a thing is, so the same kind of
// thing is the same colour wherever it turns up:
//
//	a controller, by taproot's name for it    cyan
//	a controller, by the name it gives itself bold
//	a scene                                   magenta
//	a number                                  yellow
//	detail: addresses, paths, versions        dark grey
//	it worked, it did not, take note          green, red, orange
//
// Each of these takes text as it came, from a controller or a person, and
// makes it safe to print before colouring it.

// Name is taproot's name for a controller.
func Name(s string) string { return "<lightCyan>" + Escape(s) + "</>" }

// Device is the name a controller gives itself: there for the eye to match
// against the app or the network, not the thing to read first.
func Device(s string) string { return "<gray>" + Escape(s) + "</>" }

// Addr is a controller's address, bright: it is what gets typed.
func Addr(s string) string { return "<white;op=bold>" + Escape(s) + "</>" }

// Scene is a scene's name.
func Scene(s string) string { return "<magenta>" + Escape(s) + "</>" }

// Num is a number that matters.
func Num(n int) string { return "<yellow>" + strconv.Itoa(n) + "</>" }

// Dim is detail: there to be read when wanted, not to catch the eye.
func Dim(s string) string { return "<darkGray>" + Escape(s) + "</>" }

// Note is something to take note of that is not an error.
func Note(s string) string { return "<fg=208>" + Escape(s) + "</>" }

// Swatch is a palette as a row of blocks, each in its own colour: what a
// scene looks like, in the space of a word. It is nothing at all where
// colours are not shown, since a row of blocks in one colour says nothing.
func Swatch(palette []aurora.Color) string {
	switch {
	case !c.Enable || !c.Support256Color():
		return blocks(palette, noColours)
	case c.SupportTrueColor():
		return blocks(palette, everyColour)
	default:
		return blocks(palette, colours256)
	}
}

// RGBSwatch is one colour as its hex digits with a block in that colour
// beside them where the terminal can show it: "ff8800 █".
func RGBSwatch(r, g, b uint8) string {
	hex := fmt.Sprintf("%02x%02x%02x", r, g, b)
	switch {
	case !c.Enable || !c.Support256Color():
		return hex
	case c.SupportTrueColor():
		return fmt.Sprintf("%s <fg=%d,%d,%d>█</>", hex, r, g, b)
	default:
		return fmt.Sprintf("%s <fg=%d>█</>", hex, c.RgbTo256(r, g, b))
	}
}

// colourNames are the colours that can be given by name.
var colourNames = map[string][3]uint8{
	"red": {255, 0, 0}, "orange": {255, 128, 0}, "yellow": {255, 255, 0}, "green": {0, 255, 0},
	"cyan": {0, 255, 255}, "blue": {0, 0, 255}, "purple": {128, 0, 255}, "magenta": {255, 0, 255},
	"pink": {255, 105, 180}, "white": {255, 255, 255}, "off": {0, 0, 0}, "black": {0, 0, 0},
}

// ParseColor reads a colour as six hex digits, with or without a #, or by
// one of the names in colourNames.
func ParseColor(s string) (r, g, b uint8, err error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if rgb, ok := colourNames[s]; ok {
		return rgb[0], rgb[1], rgb[2], nil
	}
	hex := strings.TrimPrefix(s, "#")
	if len(hex) == 6 {
		var rgb [3]uint8
		ok := true
		for i := range rgb {
			n, perr := strconv.ParseUint(hex[2*i:2*i+2], 16, 8)
			ok = ok && perr == nil
			rgb[i] = uint8(n)
		}
		if ok {
			return rgb[0], rgb[1], rgb[2], nil
		}
	}
	names := make([]string, 0, len(colourNames))
	for n := range colourNames {
		names = append(names, n)
	}
	slices.Sort(names)

	return 0, 0, 0, fmt.Errorf("%q is not a colour: six hex digits like ff8800, or %s", s, strings.Join(names, ", "))
}

// How many colours a terminal can show.
const (
	noColours   = iota // none, or the sixteen named ones: not enough to show a palette
	colours256         // the nearest of 256
	everyColour        // the colour itself
)

// blocks draws a palette for a terminal that can show that many colours.
func blocks(palette []aurora.Color, colours int) string {
	if colours == noColours {
		return ""
	}

	var b strings.Builder
	for _, col := range palette {
		r, g, bl := col.RGB()
		if colours == everyColour {
			fmt.Fprintf(&b, "<fg=%d,%d,%d>█</>", r, g, bl)
			continue
		}
		fmt.Fprintf(&b, "<fg=%d>█</>", c.RgbTo256(r, g, bl))
	}

	return b.String()
}

// Table prints rows in aligned columns. A cell may be coloured: a column is
// as wide as its widest cell looks, not as long as its text with the colour
// tags in it.
func Table(rows [][]string) {
	var widths []int
	for _, row := range rows {
		for i, cell := range row {
			if i == len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], width(cell))
		}
	}

	for _, row := range rows {
		var line strings.Builder
		for i, cell := range row {
			line.WriteString(cell)
			if i < len(row)-1 {
				line.WriteString(strings.Repeat(" ", widths[i]-width(cell)+2))
			}
		}
		cout.Printf("%s\n", strings.TrimRight(line.String(), " "))
	}
}

// width is how many columns a cell takes on screen.
func width(cell string) int {
	return utf8.RuneCountInString(c.ClearTag(cell))
}

// PrintReport says what a push did to a controller, a line a scene.
func PrintReport(r push.Report) {
	switch {
	case r.Backup != "" && r.DryRun:
		cout.Printf("%s: would first back it up to %s\n", Name(r.Controller), Dim(r.Backup))
	case r.Backup != "":
		cout.Printf("%s: <green>backed up</> to %s\n", Name(r.Controller), Dim(r.Backup))
	default:
		cout.Printf("%s: nothing to write\n", Name(r.Controller))
	}

	for _, res := range r.Results {
		colour, word := "green", string(res.Outcome)
		switch res.Outcome {
		case push.Unchanged:
			colour = "darkGray"
		case push.Refused, push.Failed:
			colour = "red"
		case push.Added, push.Replaced, push.Deleted, push.Renamed:
			if r.DryRun {
				word = "would be " + word
			}
		}
		line := fmt.Sprintf("  <%s>%-10s</> %s", colour, word, Scene(res.Scene))
		if res.To != "" {
			line += " → " + Scene(res.To)
		}
		if res.Reason != "" {
			line += " — " + Escape(res.Reason)
		}
		if len(res.ReadsBack) > 0 {
			line += " — " + Note("the controller changed it as it stored it:") + " it reads back with a different " + strings.Join(res.ReadsBack, ", ")
		}
		cout.Printf("%s\n", line)
	}
}

// Escape keeps text that came from a controller from being read as colour
// tags when it is printed: a scene can be named anything, <red> included.
// Only a name with both brackets in it could be read as one, and only such a
// name is changed, to brackets that look the part and mean nothing.
func Escape(s string) string {
	if !strings.Contains(s, "<") || !strings.Contains(s, ">") {
		return s
	}
	return strings.NewReplacer("<", "‹", ">", "›").Replace(s)
}
