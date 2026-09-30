package minigo2_test

import (
	"context"
	"strings"
	"testing"
)

// TestConversions covers `T(x)` conversion calls: the special string <->
// []byte/[]rune forms, identical-underlying slice/map/chan/ptr/struct
// conversions through named types and aliases, and []T under generics.
// Illegal forms (mismatched element/key/field types) must trap like real Go.
func TestConversions(t *testing.T) {
	e := newEngine(t)

	t.Run("ok", func(t *testing.T) {
		cases := []struct {
			fn   string
			want any
		}{
			// string <-> []byte / []rune
			{"ByteFromString", int64(304)},
			{"RuneFromString", int64(733)},
			{"StringFromBytes", "yo!"},
			{"StringFromRunes", "abz"},
			{"ByteFromByteSlice", int64(124)},
			{"ByteSliceFromUint8", int64(213)},
			{"Uint8SliceFromByte", int64(213)},

			// named slice types and aliases
			{"NamedByteCast", int64(305)},
			{"NamedByteCastBack", "hi"},
			{"NamedByteNil", int64(1)},
			{"ChainByteCast", int64(304)},
			{"AliasByteCast", int64(304)},
			{"NamedSliceCast", int64(21)},
			{"NamedSliceOfNamed", int64(7)},
			{"NamedToNamedSlice", int64(15)},
			{"NamedElemCast", int64(304)},
			{"NamedElemCastBack", "hi"},

			// generics: []T with the bound element type
			{"GenericCast", int64(9)},
			{"GenericSliceCast", int64(42)},
			{"GenericNamedSlice", int64(6)},
			{"GenericNamedSliceString", int64(21)},
			{"GenericTByteCast", int64(304)},
			{"GenericWrapElem", int64(41)},

			// maps / chans / pointers / structs with identical underlying
			{"NamedMapCast", int64(5)},
			{"NamedChanCast", int64(7)},
			{"NamedPtrCast", int64(9)},
			{"StructCast", int64(8)},

			// nil and misc
			{"NilToSliceOK", int64(1)},
			{"SlicePtrCast", int64(1)},
			{"ByteOfStringIdx", int64(-23)},
		}
		for _, c := range cases {
			got := run(t, e, "./testdata/conversions", c.fn)
			if got != c.want {
				t.Errorf("%s: got %v (%T), want %v (%T)", c.fn, got, got, c.want, c.want)
			}
		}
	})

	t.Run("trap", func(t *testing.T) {
		// every one of these is also rejected by the Go compiler
		cases := []struct {
			fn  string
			msg string
		}{
			{"ByteFromInts", "cannot convert []int to []byte"},
			{"NamedSliceRoundTrip", "cannot convert []int to Ints"},
			{"SliceCastToInts", "cannot convert Ints to []int"},
			{"GenericTByteCastBad", "cannot convert string to []T"},
			{"NamedMapCastBad", "cannot convert M1 to MInt"},
			{"StructCastBad", "cannot convert Sq3 to Sq2"},
			{"SliceToStringBad", "cannot convert []int to string"},
			{"SliceToSliceBad", "cannot convert []rune to []byte"},
			{"StringToSliceBad", "cannot convert string to []int"},
			{"PtrToSliceBad", "cannot convert *byte to []byte"},
			{"NilToSliceBad", "cannot convert []rune to []byte"},
		}
		for _, c := range cases {
			_, err := e.Run(context.Background(), "./testdata/conversions", c.fn)
			if err == nil || !strings.Contains(err.Error(), c.msg) {
				t.Errorf("%s: expected %q trap, got %v", c.fn, c.msg, err)
			}
		}
	})
}
