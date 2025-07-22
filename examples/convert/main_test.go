package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestRunConversionExamples(t *testing.T) {
	// Keep old stdout
	old := os.Stdout
	// Create a new pipe
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Run the function
	runConversionExamples()

	// Close the writer
	w.Close()
	// Restore old stdout
	os.Stdout = old

	// Read the output
	var buf bytes.Buffer
	io.Copy(&buf, r)

	// Assert the output
	output := buf.String()
	if !strings.Contains(output, "--- User Conversion Example ---") {
		t.Errorf("expected to contain user conversion example")
	}
	if !strings.Contains(output, "--- Order Conversion Example ---") {
		t.Errorf("expected to contain order conversion example")
	}
	if !strings.Contains(output, "--- User Conversion with Nil Phone and UpdatedAt ---") {
		t.Errorf("expected to contain user conversion with nil example")
	}
	if !strings.Contains(output, `"UserID": "user-101"`) {
		t.Errorf("expected to contain user-101")
	}
	if !strings.Contains(output, `"FullName": "John Doe"`) {
		t.Errorf("expected to contain John Doe")
	}
	if !strings.Contains(output, `"ID": "ORD-001"`) {
		t.Errorf("expected to contain ORD-001")
	}
	if !strings.Contains(output, `"TotalAmount": 199.99`) {
		t.Errorf("expected to contain 199.99")
	}
	if !strings.Contains(output, `"UserID": "user-102"`) {
		t.Errorf("expected to contain user-102")
	}
	if !strings.Contains(output, `"FullName": "Jane Doe"`) {
		t.Errorf("expected to contain Jane Doe")
	}
	if !strings.Contains(output, `"PhoneNumber": "N/A"`) {
		t.Errorf("expected to contain N/A phone number")
	}
}
