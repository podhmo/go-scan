package testdata

type Person struct {
	Name    string   `json:"name"`
	Age     int      `json:"age,omitempty"`
	Email   *string  `json:"email"`
	Hobbies []string `json:"hobbies"`
	Profile Profile  `json:"profile"`
}

type Profile struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Team        *Team  `json:"team"`
}

type Team struct {
	Name string `json:"name"`
	// Circular reference
	Members []*Person `json:"members"`
}

type Product struct {
	ID          string            `json:"id" required:"true"`
	Price       float64           `json:"price" jsonschema-override:"{\"minimum\": 0}"`
	Attributes  map[string]string `json:"attributes"`
	Tags        []string          `json:"-"` // This field should be ignored
	IsAvailable bool              `json:"is_available" required:"false"`
}

// OrderStatus represents the status of an order.
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusShipped   OrderStatus = "shipped"
	OrderStatusDelivered OrderStatus = "delivered"
	OrderStatusCancelled OrderStatus = "cancelled"
)

// MyOrderStatus is an alias for OrderStatus.
type MyOrderStatus OrderStatus
