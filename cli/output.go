package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

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

// Table prints rows in aligned columns. Colour tags are not rendered inside
// it: a tag would count towards a column's width and push the rest out.
func Table(rows [][]string) {
	w := tabwriter.NewWriter(cout.Writer(), 0, 4, 2, ' ', 0)
	for _, r := range rows {
		_, _ = fmt.Fprintln(w, strings.Join(r, "\t"))
	}
	_ = w.Flush() // a console that cannot be written to has nowhere to say so
}

// PrintReport says what a push did to a controller, a line a scene.
func PrintReport(r push.Report) {
	switch {
	case r.Backup != "" && r.DryRun:
		cout.Printf("<cyan>%s</>: would first back it up to %s\n", r.Controller, r.Backup)
	case r.Backup != "":
		cout.Printf("<cyan>%s</>: backed up to %s\n", r.Controller, r.Backup)
	default:
		cout.Printf("<cyan>%s</>: nothing to write\n", r.Controller)
	}

	for _, res := range r.Results {
		colour, word := "green", string(res.Outcome)
		switch res.Outcome {
		case push.Unchanged:
			colour = "gray"
		case push.Refused, push.Failed:
			colour = "red"
		case push.Added, push.Replaced:
			if r.DryRun {
				word = "would be " + word
			}
		}
		line := fmt.Sprintf("  <%s>%-10s</> %s", colour, word, Escape(res.Scene))
		if res.Reason != "" {
			line += " — " + Escape(res.Reason)
		}
		if len(res.ReadsBack) > 0 {
			line += " — <yellow>the controller stored it its own way:</> reads back differently in " + strings.Join(res.ReadsBack, ", ")
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
