package object

// Validation limit constants
const (
	MaxStringLength  = 1000
	MaxURLLength     = 2048
	MaxPointsInPath  = 10000
	MaxCoordinate    = 1000000
	MinCoordinate    = -1000000
	MaxStrokeWidth   = 1000
	MaxFontSize      = 500
	MaxColorLength   = 50
)

var AllowedObjectTypes = map[string]bool{
	"rect":    true,
	"circle":  true,
	"line":    true,
	"path":    true,
	"text":    true,
}

func GetSchemaForType(objType string) interface{} {
	switch objType {
	case "rect":
		return &RectangleData{}
	case "circle":
		return &CircleData{}
	case "line":
		return &LineData{}
	case "brush":
		return &BrushData{}
	case "text":
		return &TextData{}
	default:
		return nil
	}
}

// =============================================================================
// Common Embedded Structs
// =============================================================================

//  x,y coordinates for positioning shapes on the canvas
type Position struct {
	X float64 `json:"x" validate:"required,min=-1000000,max=1000000"`
	Y float64 `json:"y" validate:"required,min=-1000000,max=1000000"`
}

//  center x,y coordinates (cx, cy) for circular shapes
type CenterPosition struct {
	CX float64 `json:"cx" validate:"required,min=-1000000,max=1000000"`
	CY float64 `json:"cy" validate:"required,min=-1000000,max=1000000"`
}

//  width and height dimensions
type Size struct {
	Width  float64 `json:"width" validate:"required,min=0,max=1000000"`
	Height float64 `json:"height" validate:"required,min=0,max=1000000"`
}

//  start and end points for line-based shapes
type LineCoordinates struct {
	X1 float64 `json:"x1" validate:"required,min=-1000000,max=1000000"`
	Y1 float64 `json:"y1" validate:"required,min=-1000000,max=1000000"`
	X2 float64 `json:"x2" validate:"required,min=-1000000,max=1000000"`
	Y2 float64 `json:"y2" validate:"required,min=-1000000,max=1000000"`
}

//  common styling properties for shapes
type StyleProps struct {
	Fill        string  `json:"fill,omitempty" validate:"omitempty,max=50"`
	Stroke      string  `json:"stroke,omitempty" validate:"omitempty,max=50"`
	StrokeWidth float64 `json:"strokeWidth,omitempty" validate:"omitempty,min=0,max=1000"`
	Opacity     float64 `json:"opacity,omitempty" validate:"omitempty,min=0,max=1"`
}

//  transformation properties for shapes
type Transform struct {
	Rotation float64 `json:"rotation,omitempty" validate:"omitempty,min=-360,max=360"`
}

//  single point in a path or polygon
type Point struct {
	X float64 `json:"x" validate:"required,min=-1000000,max=1000000"`
	Y float64 `json:"y" validate:"required,min=-1000000,max=1000000"`
}

// =============================================================================
// Simple Shape Types
// =============================================================================

type RectangleData struct {
	Position
	Size
	StyleProps
	Transform
}

type CircleData struct {
	CenterPosition
	Radius float64 `json:"radius" validate:"required,min=0,max=1000000"`
	StyleProps
}

// =============================================================================
// Line-Based Shape Types
// =============================================================================

type LineData struct {
	LineCoordinates
	Stroke      string  `json:"stroke,omitempty" validate:"omitempty,max=50"`
	StrokeWidth float64 `json:"strokeWidth,omitempty" validate:"omitempty,min=0,max=1000"`
	Opacity     float64 `json:"opacity,omitempty" validate:"omitempty,min=0,max=1"`
}

// =============================================================================
// Complex Shape Types
// =============================================================================

type BrushData struct {
	Points      []Point `json:"points" validate:"required,min=2,max=10000,dive"`
	Stroke      string  `json:"stroke,omitempty" validate:"omitempty,max=50"`
	StrokeWidth float64 `json:"strokeWidth,omitempty" validate:"omitempty,min=0,max=1000"`
	Fill        string  `json:"fill,omitempty" validate:"omitempty,max=50"`
	Opacity     float64 `json:"opacity,omitempty" validate:"omitempty,min=0,max=1"`
	Smooth      bool    `json:"smooth,omitempty"`
}

// =============================================================================
// Content Shape Types
// =============================================================================

type TextData struct {
	Position
	Text       string  `json:"text" validate:"required,max=1000"`
	FontSize   float64 `json:"fontSize,omitempty" validate:"omitempty,min=1,max=500"`
	FontFamily string  `json:"fontFamily,omitempty" validate:"omitempty,max=100"`
	Fill       string  `json:"fill,omitempty" validate:"omitempty,max=50"`
	Bold       bool    `json:"bold,omitempty"`
	Italic     bool    `json:"italic,omitempty"`
	Underline  bool    `json:"underline,omitempty"`
	Transform
}


