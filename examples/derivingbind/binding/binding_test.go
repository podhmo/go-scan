package binding_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/podhmo/go-scan/examples/derivingbind/binding"
	"github.com/podhmo/go-scan/examples/derivingbind/parser"
)

func newBindingForTest(req *http.Request, pathParams map[string]string) *binding.Binding {
	var pathValue func(string) string
	if pathParams != nil {
		pathValue = func(key string) string {
			return pathParams[key]
		}
	}
	return binding.New(req, pathValue)
}

func TestOne(t *testing.T) {
	t.Run("success from query", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/?id=123", nil)
		b := newBindingForTest(req, nil)

		var id int
		err := binding.One(b, &id, binding.Query, "id", parser.Int, binding.Required)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != 123 {
			t.Errorf("expected id to be 123, got %d", id)
		}
	})

	t.Run("success from header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-API-Key", "secret-key")
		b := newBindingForTest(req, nil)

		var key string
		err := binding.One(b, &key, binding.Header, "x-api-key", parser.String, binding.Required)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if key != "secret-key" {
			t.Errorf("expected key to be 'secret-key', got %q", key)
		}
	})

	t.Run("success from cookie", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.AddCookie(&http.Cookie{Name: "session_id", Value: "session-123"})
		b := newBindingForTest(req, nil)

		var sid string
		err := binding.One(b, &sid, binding.Cookie, "session_id", parser.String, binding.Required)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sid != "session-123" {
			t.Errorf("expected sid to be 'session-123', got %q", sid)
		}
	})

	t.Run("success from path", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/users/456", nil)
		params := map[string]string{"userId": "456"}
		b := newBindingForTest(req, params)

		var uid int
		err := binding.One(b, &uid, binding.Path, "userId", parser.Int, binding.Required)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if uid != 456 {
			t.Errorf("expected uid to be 456, got %d", uid)
		}
	})

	t.Run("error when required key is missing", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		b := newBindingForTest(req, nil)

		var val string
		err := binding.One(b, &val, binding.Query, "missing_key", parser.String, binding.Required)
		if err == nil {
			t.Fatal("expected an error, but got nil")
		}
		expectedMsg := "binding: query key 'missing_key' is required"
		if err.Error() != expectedMsg {
			t.Errorf("expected error message %q, got %q", expectedMsg, err.Error())
		}
	})

	t.Run("no error when optional key is missing", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		b := newBindingForTest(req, nil)

		var val string = "default"
		err := binding.One(b, &val, binding.Query, "missing_key", parser.String, binding.Optional)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if val != "default" {
			t.Errorf("expected val to remain 'default', got %q", val)
		}
	})

	t.Run("error on parse failure", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/?id=abc", nil)
		b := newBindingForTest(req, nil)

		var id int
		err := binding.One(b, &id, binding.Query, "id", parser.Int, binding.Required)
		if err == nil {
			t.Fatal("expected an error, but got nil")
		}
		if !errors.Is(err, strconv.ErrSyntax) {
			t.Errorf("expected error to wrap strconv.ErrSyntax, but it did not")
		}
		expectedMsg := `binding: failed to parse query key 'id' with value "abc": strconv.Atoi: parsing "abc": invalid syntax`
		if err.Error() != expectedMsg {
			t.Errorf("expected error message %q, got %q", expectedMsg, err.Error())
		}
	})
}

func TestOnePtr(t *testing.T) {
	t.Run("success binding to pointer", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/?name=gopher", nil)
		b := newBindingForTest(req, nil)

		var name *string
		err := binding.OnePtr(b, &name, binding.Query, "name", parser.String, binding.Required)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if name == nil {
			t.Fatal("expected name to be non-nil")
		}
		if *name != "gopher" {
			t.Errorf("expected name to be 'gopher', got %q", *name)
		}
	})

	t.Run("optional key missing results in nil pointer", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		b := newBindingForTest(req, nil)

		// Initialize with a non-nil pointer to check if it's set to nil
		initialVal := "not-nil"
		var name *string = &initialVal

		err := binding.OnePtr(b, &name, binding.Query, "name", parser.String, binding.Optional)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if name != nil {
			t.Errorf("expected name to be nil, got %q", *name)
		}
	})
}

func TestSlice(t *testing.T) {
	t.Run("success binding slice", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/?ids=1,2,3", nil)
		b := newBindingForTest(req, nil)

		var ids []int
		err := binding.Slice(b, &ids, binding.Query, "ids", parser.Int, binding.Required)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ids) != 3 {
			t.Fatalf("expected slice length 3, got %d", len(ids))
		}
		if ids[0] != 1 || ids[1] != 2 || ids[2] != 3 {
			t.Errorf("expected ids [1 2 3], got %v", ids)
		}
	})

	t.Run("success with spaces and empty items", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/?tags=go,%20generics,,%20fun%20", nil)
		b := newBindingForTest(req, nil)

		var tags []string
		err := binding.Slice(b, &tags, binding.Query, "tags", parser.String, binding.Required)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := []string{"go", "generics", "fun"}
		if len(tags) != len(expected) {
			t.Fatalf("expected slice length %d, got %d", len(expected), len(tags))
		}
		for i := range tags {
			if tags[i] != expected[i] {
				t.Errorf("expected tags %v, got %v", expected, tags)
				break
			}
		}
	})

	t.Run("optional and missing results in nil slice", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		b := newBindingForTest(req, nil)

		ids := []int{99} // pre-fill to check if it's set to nil
		err := binding.Slice(b, &ids, binding.Query, "ids", parser.Int, binding.Optional)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ids != nil {
			t.Errorf("expected slice to be nil, got %v", ids)
		}
	})

	t.Run("error on partial parse failure", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/?ids=1,two,3", nil)
		b := newBindingForTest(req, nil)

		var ids []int
		err := binding.Slice(b, &ids, binding.Query, "ids", parser.Int, binding.Required)
		if err == nil {
			t.Fatal("expected an error, but got nil")
		}
		if !strings.Contains(err.Error(), "failed to parse item #1") {
			t.Errorf("error message should contain 'failed to parse item #1', got %q", err.Error())
		}
		// The slice should contain the successfully parsed items
		if len(ids) != 2 || ids[0] != 1 || ids[1] != 3 {
			t.Errorf("expected successfully parsed items [1 3], got %v", ids)
		}
	})
}

func TestSlicePtr(t *testing.T) {
	t.Run("success binding pointer slice", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/?ids=10,20", nil)
		b := newBindingForTest(req, nil)

		var ids []*int
		err := binding.SlicePtr(b, &ids, binding.Query, "ids", parser.Int, binding.Required)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ids) != 2 {
			t.Fatalf("expected slice length 2, got %d", len(ids))
		}
		if *ids[0] != 10 || *ids[1] != 20 {
			t.Errorf("expected ids [10 20], got [%d %d]", *ids[0], *ids[1])
		}
	})
}
