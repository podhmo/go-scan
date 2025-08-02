package simple

// Shape is an interface for geometric shapes.
// @deriving:unmarshal
type Shape interface {
	isShape()
	GetType() string // Added: Method to get discriminator value
}

// Circle represents a circle.
// @deriving:unmarshal
type Circle struct {
	Type   string `json:"type"` // Added: Discriminator field
	Radius int    `json:"radius"`
}

// isShape marks Circle as implementing Shape.
func (Circle) isShape() {}

// GetType returns the type of the shape.
func (c Circle) GetType() string {
	if c.Type == "" {
		return "circle" // Default or derived if not set
	}
	return c.Type
}

// Rectangle represents a rectangle.
// @deriving:unmarshal
type Rectangle struct {
	Type   string `json:"type"` // Added: Discriminator field
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// isShape marks Rectangle as implementing Shape.
func (Rectangle) isShape() {}

// GetType returns the type of the shape.
func (r Rectangle) GetType() string {
	if r.Type == "" {
		return "rectangle" // Default or derived if not set
	}
	return r.Type
}

// @deriving:unmarshal
// Other is a struct that is not part of the oneOf
type Other struct {
	Name string `json:"name"`
}

// @deriving:unmarshal
type Container struct {
	Content Shape `json:"content"` // oneOf
	// Type field removed from Container
	OtherContent Other `json:"other_content"`
}
