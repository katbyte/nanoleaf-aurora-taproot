package aurora

import "context"

// Layout is where each panel sits, as the controller worked it out from how
// the panels are joined.
type Layout struct {
	// NumPanels leaves out the controller on Light Panels, which is not lit.
	NumPanels int `json:"numPanels"`
	// SideLength is the length of a panel's side in the units X and Y are in.
	// The documentation says firmware from 5.0.0 reports 0 here and expects it
	// to be taken from each panel's shape (ShapeType.SideLength); an NL22 on
	// 5.2.1 still reports 150.
	SideLength float64 `json:"sideLength"`
	Panels     []Panel `json:"positionData"`
}

// Panel is one panel's place in the layout: the centre of the panel, and how
// far it is turned, anticlockwise in degrees. Y grows upwards.
type Panel struct {
	ID        int       `json:"panelId"`
	X         float64   `json:"x"`
	Y         float64   `json:"y"`
	O         float64   `json:"o"`
	ShapeType ShapeType `json:"shapeType"`
}

// ShapeType says what a panel is.
type ShapeType int

// The shapes the documentation lists. Light Panels are all ShapeTriangle,
// with the Rhythm module as ShapeRhythm where a controller lists it.
const (
	ShapeTriangle              ShapeType = 0
	ShapeRhythm                ShapeType = 1
	ShapeSquare                ShapeType = 2
	ShapeControlSquareMaster   ShapeType = 3
	ShapeControlSquarePassive  ShapeType = 4
	ShapeHexagon               ShapeType = 7
	ShapeShapesTriangle        ShapeType = 8
	ShapeShapesMiniTriangle    ShapeType = 9
	ShapeShapesController      ShapeType = 12
	ShapeElementsHexagon       ShapeType = 14
	ShapeElementsHexagonCorner ShapeType = 15
	ShapeLinesConnector        ShapeType = 16
	ShapeLightLines            ShapeType = 17
	ShapeLightLinesSingleZone  ShapeType = 18
	ShapeControllerCap         ShapeType = 19
	ShapePowerConnector        ShapeType = 20
	ShapeLightstrip4D          ShapeType = 29
	ShapeSkylightPanel         ShapeType = 30
	ShapeSkylightController    ShapeType = 31
	ShapeSkylightPassive       ShapeType = 32
)

// shapes is the documentation's table of names and side lengths.
var shapes = map[ShapeType]struct {
	name string
	side float64
}{
	ShapeTriangle:              {"triangle", 150},
	ShapeRhythm:                {"rhythm", 0},
	ShapeSquare:                {"square", 100},
	ShapeControlSquareMaster:   {"control square", 100},
	ShapeControlSquarePassive:  {"control square (passive)", 100},
	ShapeHexagon:               {"hexagon", 67},
	ShapeShapesTriangle:        {"triangle (shapes)", 134},
	ShapeShapesMiniTriangle:    {"mini triangle (shapes)", 67},
	ShapeShapesController:      {"shapes controller", 0},
	ShapeElementsHexagon:       {"elements hexagon", 134},
	ShapeElementsHexagonCorner: {"elements hexagon corner", 33.5},
	ShapeLinesConnector:        {"lines connector", 11},
	ShapeLightLines:            {"light lines", 154},
	ShapeLightLinesSingleZone:  {"light lines (single zone)", 77},
	ShapeControllerCap:         {"controller cap", 11},
	ShapePowerConnector:        {"power connector", 11},
	ShapeLightstrip4D:          {"4D lightstrip", 50},
	ShapeSkylightPanel:         {"skylight panel", 180},
	ShapeSkylightController:    {"skylight controller", 180},
	ShapeSkylightPassive:       {"skylight controller (passive)", 180},
}

// String is the shape's name, or "unknown" for one the documentation does not
// list.
func (s ShapeType) String() string {
	if shape, ok := shapes[s]; ok {
		return shape.name
	}
	return "unknown"
}

// SideLength is the length of the shape's side, which firmware from 5.0.0 no
// longer reports for the layout as a whole. Zero for a shape with no sides to
// draw, and for one the documentation does not list.
func (s ShapeType) SideLength() float64 {
	return shapes[s].side
}

// Layout reads where each panel sits.
func (c *Client) Layout(ctx context.Context) (Layout, error) {
	return read[Layout](ctx, c, "/panelLayout/layout")
}

// GlobalOrientation reads how far the person who set the panels up turned the
// whole layout to match their wall, 0 to 360 degrees. The layout itself is
// reported unturned.
func (c *Client) GlobalOrientation(ctx context.Context) (Range, error) {
	return read[Range](ctx, c, "/panelLayout/globalOrientation")
}

// SetGlobalOrientation sets how far the whole layout is turned, 0 to 360
// degrees. Effects with a direction follow it.
func (c *Client) SetGlobalOrientation(ctx context.Context, degrees int) error {
	return c.put(ctx, "/panelLayout", set("globalOrientation", degrees))
}
