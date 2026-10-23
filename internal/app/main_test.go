package app

import (
	"os"
	"testing"
)

// TestMain runs the package with reduced motion, so a frame rendered right
// after a change shows its end state. Motion tests turn motion on for their
// own state.
func TestMain(m *testing.M) {
	if err := os.Setenv("AGY_SWAP_REDUCED_MOTION", "1"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
