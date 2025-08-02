package anotherpkg

// @deriving:binding
type AnotherModel struct {
	ItemName  string `in:"query" query:"item_name" required:"true"`
	Quantity  *int   `in:"query" query:"quantity"`
	IsSpecial bool   `in:"header" header:"X-Special"`
}
