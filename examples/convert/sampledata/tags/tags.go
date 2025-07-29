package tags

import "context"

// @derivingconvert("DstWithTags", max_errors=1)
type SrcWithTags struct {
	ID        string
	Name      string `convert:"-"`
	Age       int    `convert:"UserAge"`
	Profile   string `convert:",using=convertProfile"`
	ManagerID *int   `convert:",required"`
	TeamID    *int   `convert:",required"`
}

type DstWithTags struct {
	ID        string
	UserAge   int
	Profile   string
	ManagerID *int
	TeamID    *int
}

func convertProfile(ctx context.Context, s string) string {
	return "profile:" + s
}
