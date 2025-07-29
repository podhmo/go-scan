package external

import (
	"example.com/project/converters"
	"time"
)

// // convert:import "v" "github.com/go-playground/validator/v10"
// // convert:import "customconv" "example.com/project/converters"
//
// // convert:rule "string", validator=v.New().Var
// // convert:rule "time.Time" -> "string", using=customconv.TimeToString
//
// // @derivingconvert(Output)
type Input struct {
	ID        int
	Name      string
	CreatedAt time.Time
}

type Output struct {
	ID        int
	Name      string
	CreatedAt string
}
