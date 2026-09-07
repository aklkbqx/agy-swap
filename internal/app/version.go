package app

import (
	"errors"
	"regexp"
	"strings"
)

var semverPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)

func compareVersions(a, b string) (int, error) {
	parse := func(value string) ([]string, error) {
		match := semverPattern.FindStringSubmatch(strings.TrimPrefix(strings.TrimSpace(value), "v"))
		if match == nil {
			return nil, errors.New("invalid semantic version")
		}
		for _, part := range strings.Split(match[4], ".") {
			if numericIdentifier(part) && len(part) > 1 && part[0] == '0' {
				return nil, errors.New("invalid numeric prerelease identifier")
			}
		}
		return match, nil
	}
	av, err := parse(a)
	if err != nil {
		return 0, err
	}
	bv, err := parse(b)
	if err != nil {
		return 0, err
	}
	number := func(a, b string) int {
		if len(a) < len(b) {
			return -1
		}
		if len(a) > len(b) {
			return 1
		}
		return strings.Compare(a, b)
	}
	for i := 1; i <= 3; i++ {
		if c := number(av[i], bv[i]); c != 0 {
			return c, nil
		}
	}
	if av[4] == bv[4] {
		return 0, nil
	}
	if av[4] == "" {
		return 1, nil
	}
	if bv[4] == "" {
		return -1, nil
	}
	ap, bp := strings.Split(av[4], "."), strings.Split(bv[4], ".")
	for i := 0; i < len(ap) && i < len(bp); i++ {
		an, bn := numericIdentifier(ap[i]), numericIdentifier(bp[i])
		c := 0
		if an && bn {
			c = number(ap[i], bp[i])
		} else if an {
			c = -1
		} else if bn {
			c = 1
		} else {
			c = strings.Compare(ap[i], bp[i])
		}
		if c != 0 {
			return c, nil
		}
	}
	if len(ap) < len(bp) {
		return -1, nil
	}
	return 1, nil
}

func numericIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
