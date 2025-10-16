package symgo_test

import (
	"testing"

	"github.com/podhmo/go-scan/symgo/symgotest"
)

const namedTypePointerCode = `
package main

// For Map test
type Group string
const GroupTopic Group = "topic"

type UnsubscribedGroups map[Group]bool

func (s UnsubscribedGroups) Has(group Group) bool {
	if v, ok := s[group]; ok && v {
		return true
	}
	return false
}

func NewUnsubscribedGroups(tags []string) *UnsubscribedGroups {
	result := UnsubscribedGroups{}
	for _, tag := range tags {
		result[Group(tag)] = true
	}
	return &result
}

func CheckMap() {
	unsubscribe := NewUnsubscribedGroups([]string{"topic"})
	if unsubscribe.Has(GroupTopic) {
		// method call on pointer to named map
	}
}


// For Slice test
type Tags []string

func (t Tags) Has(tag string) bool {
	for _, v := range t {
		if v == tag {
			return true
		}
	}
	return false
}

func NewTags(tags []string) *Tags {
	result := Tags(tags)
	return &result
}

func CheckSlice() {
	ts := NewTags([]string{"important", "todo"})
	if ts.Has("important") {
		// method call on pointer to named slice
	}
}

func main() {
	CheckMap()
	CheckSlice()
}
`

func TestIt_NamedTypePointer_Map(t *testing.T) {
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module example.com/m",
			"main.go": namedTypePointerCode,
		},
		EntryPoint: "example.com/m.CheckMap",
	}

	symgotest.Run(t, tc, func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("expected no error, but got: %+v", r.Error)
		}
	})
}

func TestIt_NamedTypePointer_Slice(t *testing.T) {
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module example.com/m",
			"main.go": namedTypePointerCode,
		},
		EntryPoint: "example.com/m.CheckSlice",
	}

	symgotest.Run(t, tc, func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("expected no error, but got: %+v", r.Error)
		}
	})
}
