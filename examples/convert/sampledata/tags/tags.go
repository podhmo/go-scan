package tags

import "context"

// @derivingconvert("DstWithTags")
type SrcWithTags struct {
	ID        string
	Name      string `convert:"-"`
	Age       int    `convert:"UserAge"`
	Profile   string `convert:",using=convertProfile"`
	ManagerID *int   `convert:",required"`
}

type DstWithTags struct {
	ID        string
	UserAge   int
	Profile   string
	ManagerID *int
}

func convertProfile(ctx context.Context, s string) string {
	return "profile:" + s
}
