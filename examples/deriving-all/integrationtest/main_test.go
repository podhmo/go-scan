//go:build e2e

package integrationtest

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan/examples/deriving-all/testdata/e2e/models"
)

func TestUnmarshalUser(t *testing.T) {
	t.Run("valid json", func(t *testing.T) {
		jsonData := `{"id": "user-001", "name": "John Doe", "birthDate": "1990-01-15T00:00:00Z"}`
		expectedBirthDate, _ := time.Parse(time.RFC3339, "1990-01-15T00:00:00Z")
		expected := models.User{
			ID:        "user-001",
			Name:      "John Doe",
			BirthDate: expectedBirthDate,
		}

		var got models.User
		if err := json.Unmarshal([]byte(jsonData), &got); err != nil {
			t.Fatalf("Unmarshal() failed: %v", err)
		}

		if diff := cmp.Diff(expected, got); diff != "" {
			t.Errorf("Unmarshal() mismatch (-want +got):\n%s", diff)
		}
	})

}

func TestBindUser(t *testing.T) {
	t.Run("valid request body", func(t *testing.T) {
		jsonData := `{"id": "user-002", "name": "Jane Smith", "birthDate": "1988-05-20T10:00:00Z"}`
		req := httptest.NewRequest("POST", "/users", bytes.NewBufferString(jsonData))
		req.Header.Set("Content-Type", "application/json")

		var got models.User
		if err := got.Bind(req, nil); err != nil {
			t.Fatalf("Bind() failed: %v", err)
		}

		expectedBirthDate, _ := time.Parse(time.RFC3339, "1988-05-20T10:00:00Z")
		expected := models.User{
			ID:        "user-002",
			Name:      "Jane Smith",
			BirthDate: expectedBirthDate,
		}

		if diff := cmp.Diff(expected, got); diff != "" {
			t.Errorf("Bind() mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestMarshalEvent(t *testing.T) {
	t.Run("user created event", func(t *testing.T) {
		eventTime, _ := time.Parse(time.RFC3339, "2023-01-01T12:00:00Z")
		event := models.Event{
			ID:        "evt-001",
			CreatedAt: eventTime,
			Data:      &models.UserCreated{UserID: "user-123", Username: "tester"},
		}

		// Standard library's json.Marshal escapes HTML, so we use an encoder
		// to get the raw JSON string without escaping.
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(event); err != nil {
			t.Fatalf("Marshal() failed: %v", err)
		}
		got := strings.TrimSpace(buf.String())

		// Note: The order of fields in the output JSON is not guaranteed.
		// A simple string comparison is brittle. A better approach would be to
		// unmarshal both `got` and `expected` into a generic map[string]any
		// and then compare them. For this test, we'll stick to string
		// comparison but be mindful of its limitations.

		expected := `{"id":"evt-001","createdAt":"2023-01-01T12:00:00Z","data":{"type":"usercreated","userId":"user-123","username":"tester"}}`

		// To make the test robust against JSON key order variations,
		// we unmarshal both the generated and expected JSON into maps
		// and compare the maps.
		var gotMap, expectedMap map[string]any
		if err := json.Unmarshal([]byte(got), &gotMap); err != nil {
			t.Fatalf("failed to unmarshal actual JSON: %v", err)
		}
		if err := json.Unmarshal([]byte(expected), &expectedMap); err != nil {
			t.Fatalf("failed to unmarshal expected JSON: %v", err)
		}
		if diff := cmp.Diff(expectedMap, gotMap); diff != "" {
			t.Errorf("Marshal() structural mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("message posted event", func(t *testing.T) {
		eventTime, _ := time.Parse(time.RFC3339, "2023-01-02T15:30:00Z")
		event := models.Event{
			ID:        "evt-002",
			CreatedAt: eventTime,
			Data:      &models.MessagePosted{MessageID: "msg-456", Content: "Hello world"},
		}

		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(event); err != nil {
			t.Fatalf("Marshal() failed: %v", err)
		}
		got := strings.TrimSpace(buf.String())
		expected := `{"id":"evt-002","createdAt":"2023-01-02T15:30:00Z","data":{"type":"messageposted","messageId":"msg-456","content":"Hello world"}}`

		var gotMap, expectedMap map[string]any
		if err := json.Unmarshal([]byte(got), &gotMap); err != nil {
			t.Fatalf("failed to unmarshal actual JSON: %v", err)
		}
		if err := json.Unmarshal([]byte(expected), &expectedMap); err != nil {
			t.Fatalf("failed to unmarshal expected JSON: %v", err)
		}
		if diff := cmp.Diff(expectedMap, gotMap); diff != "" {
			t.Errorf("Marshal() structural mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestUnmarshalEvent(t *testing.T) {
	t.Run("user created event", func(t *testing.T) {
		jsonData := `{
			"id": "evt-001",
			"createdAt": "2023-01-01T12:00:00Z",
			"data": {
				"type": "usercreated",
				"userId": "user-123",
				"username": "tester"
			}
		}`
		expectedTime, _ := time.Parse(time.RFC3339, "2023-01-01T12:00:00Z")
		expected := models.Event{
			ID:        "evt-001",
			CreatedAt: expectedTime,
			Data:      &models.UserCreated{UserID: "user-123", Username: "tester"},
		}

		var got models.Event
		if err := json.Unmarshal([]byte(jsonData), &got); err != nil {
			t.Fatalf("Unmarshal() failed: %v", err)
		}

		if diff := cmp.Diff(expected, got); diff != "" {
			t.Errorf("Unmarshal() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("message posted event", func(t *testing.T) {
		jsonData := `{
			"id": "evt-002",
			"createdAt": "2023-01-02T15:30:00Z",
			"data": {
				"type": "messageposted",
				"messageId": "msg-456",
				"content": "Hello world"
			}
		}`
		expectedTime, _ := time.Parse(time.RFC3339, "2023-01-02T15:30:00Z")
		expected := models.Event{
			ID:        "evt-002",
			CreatedAt: expectedTime,
			Data:      &models.MessagePosted{MessageID: "msg-456", Content: "Hello world"},
		}

		var got models.Event
		if err := json.Unmarshal([]byte(jsonData), &got); err != nil {
			t.Fatalf("Unmarshal() failed: %v", err)
		}

		if diff := cmp.Diff(expected, got); diff != "" {
			t.Errorf("Unmarshal() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("unknown event type", func(t *testing.T) {
		jsonData := `{
			"id": "evt-003",
			"createdAt": "2023-01-03T10:00:00Z",
			"data": {
				"type": "unknown_event"
			}
		}`
		var got models.Event
		err := json.Unmarshal([]byte(jsonData), &got)
		if err == nil {
			t.Fatal("expected an error for unknown event type, but got nil")
		}
		if !strings.Contains(err.Error(), "unknown data type 'unknown_event'") {
			t.Errorf("expected error about unknown event type, but got: %v", err)
		}
	})
}
