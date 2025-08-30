package testlog

import (
	"context"
	"strconv"
	"testing"
)

// ContextKey is the type for context keys.
type ContextKey string

// ContextKeyTesting is the key for the testing object in the context.
var ContextKeyTesting = ContextKey("*testing.T")

// NewContext contextにpackするloggerの代わりに*testing.Tをwrapしたloggerの様なものを注入する。ログ出力がテスト失敗時に確認できて便利。
func NewContext(t *testing.T) context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, ContextKeyTesting, t)
	_, _ = strconv.ParseBool("foo")
	return ctx
}
