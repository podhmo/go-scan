package integrationtest

// --- Interfaces for multi oneOf test ---

// Animal interface (for first oneOf field)
type Animal interface {
	Speak() string
	AnimalKind() string // Discriminator value provider
}

// Vehicle interface (for second oneOf field)
type Vehicle interface {
	Move() string
	VehicleType() string // Discriminator value provider
}

// --- Implementers for Animal ---

// Dog implements Animal
// @derivingmarshal
type Dog struct {
	Breed string `json:"breed"`
	Noise string `json:"noise"`
}

func (d *Dog) Speak() string      { return d.Noise }
func (d *Dog) AnimalKind() string { return "dog" }

// Cat implements Animal
// @derivingmarshal
type Cat struct {
	Color string `json:"color"`
	Purr  bool   `json:"purr"`
}

func (c *Cat) Speak() string      { return "meow" }
func (c *Cat) AnimalKind() string { return "cat" }

// --- Implementers for Vehicle ---

// Car implements Vehicle
// @derivingmarshal
type Car struct {
	Make   string `json:"make"`
	Wheels int    `json:"wheels"`
}

func (c *Car) Move() string        { return "vroom" }
func (c *Car) VehicleType() string { return "car" }

// Bicycle implements Vehicle
// @derivingmarshal
type Bicycle struct {
	Gears   int  `json:"gears"`
	HasBell bool `json:"has_bell"`
}

func (b *Bicycle) Move() string        { return "ding ding" }
func (b *Bicycle) VehicleType() string { return "bicycle" }

// --- Structs with multiple oneOf fields ---

// Scene contains multiple different oneOf fields
// @deriving:unmarshal
type Scene struct {
	Name        string  `json:"name"`
	MainAnimal  Animal  `json:"main_animal,omitempty"`  // oneOf
	MainVehicle Vehicle `json:"main_vehicle,omitempty"` // oneOf
	Description string  `json:"description,omitempty"`
}

// Parade contains multiple oneOf fields of the same interface type
// @deriving:unmarshal
type Parade struct {
	EventName      string `json:"event_name"`
	LeadAnimal     Animal `json:"lead_animal,omitempty"`     // oneOf (Animal)
	TrailingAnimal Animal `json:"trailing_animal,omitempty"` // oneOf (Animal)
	Floats         int    `json:"floats,omitempty"`
}

// For these to work with the current generator, the generator needs to be updated
// to extract the discriminator value (e.g., "dog", "cat") from a method like AnimalKind() or VehicleType()
// or from a field within the JSON of the implementer (e.g. a "type" field).
// The current generator hardcodes "circle" and "rectangle" or uses ToLower(TypeName).
// I will adjust the generator's discriminator logic slightly to look for a "type" field
// in the JSON if the hardcoded values don't match, and then fall back to ToLower(TypeName).
// For test data, I will ensure the JSON payloads include a "type" field.

// --- Another interface and implementer for testing same package resolution ---
type Pet interface {
	IsFriendly() bool
	PetKind() string
}

// @derivingmarshal
type Goldfish struct {
	Name      string `json:"name"`
	BowlShape string `json:"bowl_shape"`
}

func (g *Goldfish) IsFriendly() bool { return true }
func (g *Goldfish) PetKind() string  { return "goldfish" }

// @deriving:unmarshal
type PetOwner struct {
	OwnerName string  `json:"owner_name"`
	Pet       Pet     `json:"pet_data,omitempty"`  // oneOf
	Accessory Vehicle `json:"accessory,omitempty"` // another oneOf, different type
}
