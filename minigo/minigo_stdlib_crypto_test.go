package minigo_test

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan/minigo"
	"github.com/podhmo/go-scan/minigo/object"
	"github.com/podhmo/go-scan/minigo/stdlib/crypto/md5"
	"github.com/podhmo/go-scan/minigo/stdlib/encoding/hex"
)

func TestStdlib_Crypto_SliceGoArray(t *testing.T) {
	input := `
package main
import "crypto/md5"
import "encoding/hex"

func main() {
	data := "hello"
	sum := md5.Sum([]byte(data))
	sliced := sum[0:6]
	return hex.EncodeToString(sliced)
}
`
	want := "5d41402abc4b" // hex of the first 6 bytes of md5("hello")

	var stdout bytes.Buffer
	i, err := minigo.NewInterpreter(minigo.WithStdout(&stdout))
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %+v", err)
	}

	md5.Install(i)
	hex.Install(i)

	resultObject, err := i.EvalString(input)
	if err != nil {
		t.Fatalf("EvalString() failed unexpectedly: %+v", err)
	}

	if resultObject == nil {
		t.Fatal("eval result is nil, expected a return value")
	}

	// EvalString returns the unwrapped return value from the script.
	stringValue, ok := resultObject.(*object.String)
	if !ok {
		t.Fatalf("expected return value to be a String, but got %T", resultObject)
	}

	if diff := cmp.Diff(want, stringValue.Value); diff != "" {
		t.Errorf("mismatched string result (-want +got):\n%s", diff)
	}
}
