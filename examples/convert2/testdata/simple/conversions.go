//go:build convert2_test_source

package simple

import (
	"fmt"
	"time"
)

// convert:pair SrcSimple -> DstSimple
// convert:pair SrcWithAlias -> DstWithAlias
// convert:pair InnerSrc -> InnerDst
// convert:pair OuterSrc -> OuterDst
// convert:pair InnerSrcDiff -> InnerDstDiff
// convert:pair OuterSrcDiff -> OuterDstDiff
// convert:pair SrcUnderlying -> DstUnderlying
// convert:rule "time.Time" -> "string", using=timeToStringNotImplemented
// convert:rule "simple.MyTime" -> "time.Time", using=myTimeToTime // Placeholder, myTimeToTime needs definition
// convert:rule "string" -> "time.Time", using=stringToTimeNotImplemented
// convert:rule "simple.DstSimple", validator=validateDstSimpleNotImplemented // Placeholder

// IntToStr is a dummy function for testing 'using' tag.
// It expects an errorCollector type to be available, which will be in the generated code.
// The actual errorCollector will be in the generated file (e.g., simple_gen.go).
func IntToStr(ec *errorCollector, val int) string {
	// In a real scenario, ec might be used.
	// For this dummy, we just convert int to string.
	// Using a simple conversion for now.
	// To make this file compilable standalone for dev, ec type might need to be defined here too.
	// Let's assume it's defined or we use a placeholder.
	var tempEc interface{} = ec // To avoid "unused" if not used below
	_ = tempEc

	// A simple conversion, e.g., just stringifying.
	// In a real case, you might use strconv.Itoa or similar.
	// For testing, we need a way to check it was called.
	return fmt.Sprintf("converted_%d", val) // Using fmt to avoid strconv import for this dummy
}

// myTimeToTime is a placeholder for a custom conversion function.
func myTimeToTime(ec *errorCollector, mt MyTime) time.Time {
	return time.Time(mt)
}

// timeToStringNotImplemented placeholder
func timeToStringNotImplemented(ec *errorCollector, t time.Time) string {
	return "timeToStringNotImplemented_called"
}

// stringToTimeNotImplemented placeholder
func stringToTimeNotImplemented(ec *errorCollector, s string) time.Time {
	return time.Time{} // return zero time
}

// validateDstSimpleNotImplemented placeholder
func validateDstSimpleNotImplemented(ec *errorCollector, d DstSimple) {
	// do nothing
}
