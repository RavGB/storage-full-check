package main

import "testing"

func TestParseSize(t *testing.T) {
	cases := []struct {
		input string
		want  int64
	}{
		{"0", 0},
		{"100", 100},
		{"100B", 100},
		{"100 b", 100},
		{"1KB", 1000},
		{"1kb", 1000},
		{"1KiB", 1024},
		{"1kib", 1024},
		{"100MB", 100 * 1000 * 1000},
		{"100MiB", 100 * 1024 * 1024},
		{"2GB", 2 * 1000 * 1000 * 1000},
		{"2GiB", 2 * 1024 * 1024 * 1024},
		{"1TB", 1000 * 1000 * 1000 * 1000},
		{"1TiB", 1024 * 1024 * 1024 * 1024},
		{"1.5KiB", 1536},
		{"1.5MB", 1500 * 1000},
		{"  3MB  ", 3 * 1000 * 1000},
	}
	for _, c := range cases {
		got, err := ParseSize(c.input)
		if err != nil {
			t.Errorf("ParseSize(%q): unexpected error %v", c.input, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseSize(%q) = %d, want %d", c.input, got, c.want)
		}
	}
}

func TestParseSizeErrors(t *testing.T) {
	for _, input := range []string{"", "   ", "abc", "MB", "-5MB", "10XB", "1.5", "1,000MB", "9999999999TB"} {
		if got, err := ParseSize(input); err == nil {
			t.Errorf("ParseSize(%q) = %d, want an error", input, got)
		}
	}
}
