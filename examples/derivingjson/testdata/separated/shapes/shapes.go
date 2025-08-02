package shapes

// Shape is an interface for geometric shapes.
type Shape interface {
	isShape()
	GetType() string
}

// Circle represents a circle.
type Circle struct {
	Type   string `json:"type"` // Discriminator field
	Radius int    `json:"radius"`
}

// isShape marks Circle as implementing Shape.
func (Circle) isShape() {}

// GetType returns the type of the shape.
func (c Circle) GetType() string {
	return "circle"
}

// Rectangle represents a rectangle.
type Rectangle struct {
	Type   string `json:"type"` // Discriminator field
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// isShape marks Rectangle as implementing Shape.
func (Rectangle) isShape() {}

// GetType returns the type of the shape.
func (r Rectangle) GetType() string {
	return "rectangle"
}
