//go:build convert2_test_source

package simple

// convert:pair SrcSimple -> DstSimple
// convert:pair SrcWithAlias -> DstWithAlias
// convert:rule "time.Time" -> "string", using=timeToStringNotImplemented
// convert:rule "simple.MyTime" -> "time.Time", using=myTimeToTime
// convert:rule "string" -> "time.Time", using=stringToTimeNotImplemented
// convert:rule "simple.DstSimple", validator=validateDstSimpleNotImplemented
