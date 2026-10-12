package main

import (
	"fmt"
	"os"
	"strings"
)

// releaseNotes returns the body of CHANGELOG's "## VERSION" section, from the
// line after its heading up to the next level-two heading.
func releaseNotes(releaseVersion, changelogPath string) (string, error) {
	data, err := os.ReadFile(changelogPath)
	if err != nil {
		return "", err
	}
	heading := "## " + strings.TrimPrefix(strings.TrimSpace(releaseVersion), "v")
	var body []string
	found := false
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "## ") {
			if found {
				break
			}
			found = strings.TrimSpace(line) == heading
			continue
		}
		if found {
			body = append(body, line)
		}
	}
	if !found {
		return "", fmt.Errorf("%s has no %q section", changelogPath, heading)
	}
	notes := strings.TrimSpace(strings.Join(body, "\n"))
	if notes == "" {
		return "", fmt.Errorf("%s section %q is empty", changelogPath, heading)
	}
	return notes + "\n", nil
}
