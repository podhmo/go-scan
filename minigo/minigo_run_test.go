package minigo_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/podhmo/go-scan/minigo"
)

func TestRun(t *testing.T) {
	type Config struct {
		Name    string
		Version int
		Active  bool
	}

	script := `
package main

func GetConfig() {
	return struct{
		Name    string
		Version int
		Active  bool
	}{
		Name: "MyApp",
		Version: getVersion(),
		Active: true,
	}
}
`
	getVersion := func() int {
		return 101
	}

	ctx := context.Background()
	opts := minigo.Options{
		Source:     []byte(script),
		EntryPoint: "GetConfig",
		Globals: map[string]any{
			"getVersion": getVersion,
		},
	}

	result, err := minigo.Run(ctx, opts)
	if err != nil {
		t.Fatalf("minigo.Run() failed: %v", err)
	}

	var got Config
	if err := result.As(&got); err != nil {
		t.Fatalf("result.As() failed: %v", err)
	}

	want := Config{
		Name:    "MyApp",
		Version: 101,
		Active:  true,
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("unmarshaled config mismatch:\n- want: %+v\n- got:  %+v", want, got)
	}
}
