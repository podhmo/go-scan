package complex

// @derivingconvert("Dst")
type Src struct {
	Items   []*SrcItem
	ItemMap map[string]*SrcItem
}

type Dst struct {
	Items   []*DstItem
	ItemMap map[string]*DstItem
}

// @derivingconvert("DstItem")
type SrcItem struct {
	Value string
}

type DstItem struct {
	Value string
}
