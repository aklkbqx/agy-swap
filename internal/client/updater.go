package client

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"strings"
)

// NormalizedReleaseTag ensures the release tag begins with a 'v' prefix.
func NormalizedReleaseTag(tag string) string {
	tag = strings.TrimSpace(tag)
	if tag != "" && !strings.HasPrefix(tag, "v") {
		return "v" + tag
	}
	return tag
}

// ExpectedChecksum extracts the SHA-256 checksum for a binary asset name from a checksums manifest.
func ExpectedChecksum(manifest []byte, name string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(manifest)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && strings.TrimPrefix(fields[len(fields)-1], "*") == name {
			sum := strings.ToLower(fields[0])
			if len(sum) == 64 {
				if _, err := hex.DecodeString(sum); err == nil {
					return sum, nil
				}
			}
		}
	}
	return "", fmt.Errorf("checksum for %s not found in manifest", name)
}
