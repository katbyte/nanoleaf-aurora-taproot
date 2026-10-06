package aurora

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// EventType is a kind of change a controller reports as it happens.
type EventType int

// The event types. Light Panels report the first three; touch is for panels
// that can feel it.
const (
	EventState   EventType = 1 // power, brightness, colour
	EventLayout  EventType = 2 // panels added, removed or turned
	EventEffects EventType = 3 // another effect started
	EventTouch   EventType = 4
)

// What Event.Attr means for an EventState.
const (
	AttrOn         = 1
	AttrBrightness = 2
	AttrHue        = 3
	AttrSaturation = 4
	AttrCCT        = 5
	AttrColorMode  = 6
)

// What Event.Attr means for an EventLayout.
const (
	AttrLayout            = 1
	AttrGlobalOrientation = 2
)

// AttrSelectedEffect is what Event.Attr is for an EventEffects, whose Value
// is the name of the effect now running.
const AttrSelectedEffect = 1

// ErrStreamClosed is returned by Events when the controller ends the stream.
var ErrStreamClosed = errors.New("the controller closed the event stream")

// Event is one change. For state, layout and effects it is an attribute and
// its new value; for touch, a gesture and the panel it was made on.
type Event struct {
	Type  EventType       `json:"-"`
	Attr  int             `json:"attr"`
	Value json.RawMessage `json:"value"`

	Gesture *int `json:"gesture"`
	PanelID *int `json:"panelId"`
}

// Events listens for changes of the given types and hands each to handle,
// until ctx ends (nil) or the stream does (ErrStreamClosed, or what broke it).
// The connection stays open for as long as that takes, whatever the client's
// time limit. Controllers need firmware 3.1.0 or later.
func (c *Client) Events(ctx context.Context, types []EventType, handle func(Event)) error {
	if c.token == "" {
		return ErrNoToken
	}
	if len(types) == 0 {
		types = []EventType{EventState, EventLayout, EventEffects}
	}
	ids := make([]string, len(types))
	for i, t := range types {
		ids[i] = strconv.Itoa(int(t))
	}

	const path = "/events"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+apiRoot+"/"+c.token+path+"?id="+strings.Join(ids, ","), http.NoBody)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, unwrapURL(err))
	}
	req.Header.Set("Accept", "text/event-stream")

	res, err := c.stream.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil // the caller stopped listening
		}
		return fmt.Errorf("GET %s on %s: %w", path, c.host, unwrapURL(err))
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return &StatusError{Method: http.MethodGet, Path: path, Status: res.StatusCode}
	}

	// an event is an "id: <type>" line and a "data: <json>" line; a layout's data runs long
	lines := bufio.NewScanner(res.Body)
	lines.Buffer(make([]byte, 0, 64<<10), maxResponseBytes)
	var current EventType
	for lines.Scan() {
		field, value, _ := strings.Cut(lines.Text(), ":")
		value = strings.TrimSpace(value)
		switch field {
		case "id":
			if n, aerr := strconv.Atoi(value); aerr == nil {
				current = EventType(n)
			}
		case "data":
			var batch struct {
				Events []Event `json:"events"`
			}
			if json.Unmarshal([]byte(value), &batch) != nil {
				continue // not something this package knows how to read: skip it rather than end the stream
			}
			for _, e := range batch.Events {
				e.Type = current
				handle(e)
			}
		}
	}
	if ctx.Err() != nil {
		return nil
	}
	if err := lines.Err(); err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}

	return ErrStreamClosed
}
