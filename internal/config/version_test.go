package config

import "testing"

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"2.3.0", "2.3.0", 0},
		{"2.3.1", "2.3.0", 1},
		{"2.3.0", "2.3.1", -1},
		{"2.3.0-rc.1", "2.3.0", -1},
		{"2.3.0", "2.3.0-rc.1", 1},
		{"v2.4.0", "2.3.9", 1},
		{"3.0.0", "2.99.99", 1},
	}

	for _, c := range cases {
		got, err := CompareVersions(c.a, c.b)
		if err != nil {
			t.Fatalf("CompareVersions(%q, %q) returned error: %v", c.a, c.b, err)
		}
		if got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestDefaultPaths(t *testing.T) {
	paths, err := DefaultPaths()
	if err != nil {
		t.Fatalf("DefaultPaths failed: %v", err)
	}
	if paths.Home == "" || paths.ConfigDir == "" || paths.Accounts == "" {
		t.Fatalf("invalid paths: %#v", paths)
	}
}
