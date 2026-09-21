//go:build !darwin || !cgo

package app

import "context"

// Darwin release builds enable cgo. Builds without Security.framework report
// vault unavailability instead of putting encoded secrets into process args.
func keychainSet(context.Context, string, string, string) bool { return false }
func keychainGet(context.Context, string, string) string       { return "" }
func keychainDelete(context.Context, string, string) bool      { return false }
