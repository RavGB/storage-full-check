package main

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// sizeSuffixes maps an upper-case suffix to its multiplier in bytes.
// KB-style suffixes are decimal (powers of 1000) and KiB-style
// suffixes are binary (powers of 1024), matching the way disk vendors
// and operating systems usually label sizes.
var sizeSuffixes = map[string]int64{
	"":    1,
	"B":   1,
	"K":   1024,
	"KB":  1000,
	"KIB": 1024,
	"M":   1024 * 1024,
	"MB":  1000 * 1000,
	"MIB": 1024 * 1024,
	"G":   1024 * 1024 * 1024,
	"GB":  1000 * 1000 * 1000,
	"GIB": 1024 * 1024 * 1024,
	"T":   1024 * 1024 * 1024 * 1024,
	"TB":  1000 * 1000 * 1000 * 1000,
	"TIB": 1024 * 1024 * 1024 * 1024,
}

// ParseSize parses a human size such as "100", "100B", "1KB", "100MB",
// "1.5GiB" or "2tb" into a byte count.
//
// A plain number with no suffix is a byte count. A fractional value is
// allowed only together with a suffix, and the result is rounded down
// to a whole byte. Empty, negative, unknown-suffix and overflowing
// values are errors.
func ParseSize(text string) (int64, error) {
	s := strings.TrimSpace(text)
	if s == "" {
		return 0, errors.New("size is empty")
	}

	// Split into the leading numeric part and the suffix that follows.
	i := 0
	for i < len(s) && (isDigit(s[i]) || s[i] == '.') {
		i++
	}
	if i == 0 {
		return 0, fmt.Errorf("size %q does not start with a number", text)
	}
	numberPart := s[:i]
	suffixPart := strings.ToUpper(strings.TrimSpace(s[i:]))

	multiplier, ok := sizeSuffixes[suffixPart]
	if !ok {
		return 0, fmt.Errorf("size %q has an unknown suffix %q", text, suffixPart)
	}

	value, err := strconv.ParseFloat(numberPart, 64)
	if err != nil {
		return 0, fmt.Errorf("size %q has an invalid number: %w", text, err)
	}
	if value < 0 {
		return 0, fmt.Errorf("size %q is negative", text)
	}
	if suffixPart == "" && strings.Contains(numberPart, ".") {
		return 0, fmt.Errorf("size %q: a fractional value needs a suffix", text)
	}

	bytes := value * float64(multiplier)
	if bytes > math.MaxInt64 {
		return 0, fmt.Errorf("size %q is too large", text)
	}
	return int64(bytes), nil
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

// formatBytes renders a byte count with a binary unit for people and
// the exact byte count in brackets, so a report is both quick to read
// and exact.
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	value := float64(bytes) / float64(div)
	suffix := "KMGTPE"[exp]
	return fmt.Sprintf("%.2f %ciB (%d bytes)", value, suffix, bytes)
}

// formatPercent renders a part/whole share, guarding the empty case.
func sharePercent(part, whole int64) float64 {
	if whole <= 0 {
		return 0
	}
	return float64(part) / float64(whole) * 100
}
