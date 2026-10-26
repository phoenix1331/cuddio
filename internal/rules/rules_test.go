package rules

import (
	"testing"

	"github.com/phoenix1331/cuddio/internal/detect"
)

func TestBuiltinRulesFormAValidRegistry(t *testing.T) {
	t.Parallel()

	if _, err := detect.NewRegistry(Builtin()); err != nil {
		t.Fatalf("NewRegistry(Builtin()) error = %v", err)
	}
}
