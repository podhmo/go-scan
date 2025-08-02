package destination

type DstParent struct {
	ID    string
	Child DstChild
}

type DstChild struct {
	Value string
}
