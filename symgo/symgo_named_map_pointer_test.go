package symgo_test

import (
	"testing"

	"github.com/podhmo/go-scan/symgotest"
)

func TestIt_NamedMapPointer(t *testing.T) {
	code := `
package main

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

func ExtractEmailNotificationSettingDisableTopic(tags []string) {
	unsubscribe := NewUnsubscribedGroups(tags)
	if unsubscribe.Has(GroupTopic) {
		// this line causes the "undefined method" error in symgo
	}
}

func main() {
	ExtractEmailNotificationSettingDisableTopic([]string{"topic"})
}
`
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module example.com/m",
			"main.go": code,
		},
		EntryPoint: "example.com/m.main",
		// ExpectError defaults to false. We expect this test to fail with t.Fatalf
		// because symgo will produce an error, thus proving the bug exists.
	}

	symgotest.Run(t, tc, func(t *testing.T, r *symgotest.Result) {
		// This code block will only be reached if the bug is fixed (r.Error is nil).
		// If the bug exists, symgotest.Run will call t.Fatalf and this block won't execute.
		if r.Error != nil {
			t.Fatalf("expected no error, but got: %+v", r.Error)
		}
	})
}