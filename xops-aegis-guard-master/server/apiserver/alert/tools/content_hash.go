package tools

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode"
)

// Build hashes the meaningful text shown to users. Time fields are omitted so
// a provider-side LastTime refresh can be handled separately from a real
// command/IP/account/content change.
func ContentHash(message string) string {
	lines := make([]string, 0)
	for _, raw := range strings.Split(strings.ReplaceAll(message, "\r\n", "\n"), "\n") {
		line := normalizeLine(raw)
		if line == "" || isTimeLine(line) {
			continue
		}
		lines = append(lines, line)
	}
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}

func normalizeLine(value string) string {
	value = strings.ReplaceAll(value, "**", "")
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func isTimeLine(line string) bool {
	separator := strings.IndexAny(line, "：:")
	if separator < 0 {
		return false
	}
	label := strings.TrimSpace(line[:separator])
	label = strings.TrimLeftFunc(label, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
	})
	return strings.Contains(label, "时间")
}
