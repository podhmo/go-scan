package simple

// @deriving:unmarshall
type Shape interface {
	isShape()
}

// @deriving:unmarshall
type Circle struct {
	Radius int
}

func (Circle) isShape() {}

// @deriving:unmarshall
type Rectangle struct {
	Width  int
	Height int
}

func (Rectangle) isShape() {}

// @deriving:unmarshall
// Other is a struct that is not part of the oneOf
type Other struct {
	Name string
}

// @deriving:unmarshall
type Container struct {
	Content      Shape  `json:"content"` // oneOf
	Type         string `json:"type"`    // discriminator
	OtherContent Other  `json:"other_content"`
}
