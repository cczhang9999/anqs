package tools

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

var firstTimeKeys = map[string][]string{
	"suspicious_event": {"OccurrenceTimeStamp", "OccurrenceTime"},
	"honeypot":         {"FirstTime", "OccurrenceTimeStamp", "OccurrenceTime"},
	"core_file":        {"FirstTime", "OccurrenceTimeStamp", "OccurrenceTime"},
	"custom_incident":  {"CreateTime", "FirstOccurrenceTime"},
}

// FirstProviderTime extracts the provider's first occurrence time from a source
// payload. The fallback keeps records usable when an older API omits it.
func FirstProviderTime(alertType string, payload map[string]any, fallback *time.Time) *time.Time {
	for _, key := range firstTimeKeys[alertType] {
		if value, ok := valueFold(payload, key); ok {
			if parsed := parseTime(value); parsed != nil {
				return parsed
			}
		}
	}
	if fallback == nil {
		return nil
	}
	value := fallback.Truncate(time.Second)
	return &value
}

func valueFold(payload map[string]any, target string) (any, bool) {
	for key, value := range payload {
		if strings.EqualFold(strings.TrimSpace(key), target) {
			return value, true
		}
	}
	return nil, false
}

func parseTime(value any) *time.Time {
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "<nil>" {
		return nil
	}
	if number, err := strconv.ParseInt(strings.TrimSuffix(text, ".0"), 10, 64); err == nil && number > 0 {
		var parsed time.Time
		if number > 1_000_000_000_000 {
			parsed = time.UnixMilli(number)
		} else {
			parsed = time.Unix(number, 0)
		}
		parsed = parsed.Truncate(time.Second)
		return &parsed
	}
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339, "2006-01-02T15:04:05.000Z"} {
		if parsed, err := time.ParseInLocation(layout, text, time.Local); err == nil {
			parsed = parsed.Truncate(time.Second)
			return &parsed
		}
	}
	return nil
}
