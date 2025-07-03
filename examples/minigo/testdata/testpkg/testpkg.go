package testpkg

const ExportedConst = "Hello from testpkg"
const AnotherExportedConst = 12345
const nonExportedConst = "Internal constant" // 小文字で始まる非エクスポート定数

// Added for testing external struct and function
type Point struct {
	X int
	Y int
}

func NewPoint(x int, y int) *Point {
	return &Point{X: x, Y: y}
}

func NewPointValue(x int, y int) Point {
	return Point{X: x, Y: y}
}

type Figure struct {
    Name string
    P    Point  // Struct field using a type from the same package
}

func NewFigure(name string, x int, y int) Figure {
    return Figure{Name: name, P: Point{X: x, Y: y}}
}

func GetPointX(p Point) int {
	return p.X
}

func GetFigureName(f Figure) string {
    return f.Name
}

// Function returning a struct with an unexported field (for testing accessibility)
type SecretPoint struct {
    X int
    secretY int // unexported
}

func NewSecretPoint(x, y int) SecretPoint {
    return SecretPoint{X: x, secretY: y}
}
