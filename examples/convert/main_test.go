package main

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
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
	assert.Contains(t, output, "--- User Conversion Example ---")
	assert.Contains(t, output, "--- Order Conversion Example ---")
	assert.Contains(t, output, "--- User Conversion with Nil Phone and UpdatedAt ---")
	assert.Contains(t, output, `"UserID": "user-101"`)
	assert.Contains(t, output, `"FullName": "John Doe"`)
	assert.Contains(t, output, `"ID": "ORD-001"`)
	assert.Contains(t, output, `"TotalAmount": 199.99`)
	assert.Contains(t, output, `"UserID": "user-102"`)
	assert.Contains(t, output, `"FullName": "Jane Doe"`)
	assert.Contains(t, output, `"PhoneNumber": "N/A"`)
}
