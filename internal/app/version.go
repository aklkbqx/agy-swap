package app

import "github.com/aklkbqx/agy-swap/v2/internal/config"

// compareVersions orders two semantic versions; see config.CompareVersions.
func compareVersions(a, b string) (int, error) { return config.CompareVersions(a, b) }
