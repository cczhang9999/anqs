package tools

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Build returns the stable identity of one provider-side alert occurrence.
// The aggregate fingerprint is intentionally kept separate: fingerprint groups
// repeated alerts, while this key prevents the overlap window from recording
// the same provider occurrence more than once.
func EventKey(alertType, fingerprint, providerID string, providerTime *time.Time, payload map[string]any) []byte {
	identity := providerEventIdentity(alertType, providerID, providerTime, payload)
	sum := sha256.Sum256([]byte(strings.Join([]string{alertType, fingerprint, identity}, "\x00")))
	return sum[:]
}

func providerEventIdentity(alertType, providerID string, providerTime *time.Time, payload map[string]any) string {
	if alertType == "custom_incident" {
		if ids := incidentAlertIDs(payload); len(ids) > 0 {
			return "incident-alerts:" + strings.Join(ids, ",")
		}
	}
	if providerTime != nil {
		return fmt.Sprintf("provider-time:%d", providerTime.Truncate(time.Second).Unix())
	}
	return "provider-id:" + strings.TrimSpace(providerID)
}

func incidentAlertIDs(payload map[string]any) []string {
	items, ok := payload["AlertDetails"].([]any)
	if !ok {
		if typed, typedOK := payload["AlertDetails"].([]map[string]any); typedOK {
			items = make([]any, 0, len(typed))
			for _, item := range typed {
				items = append(items, item)
			}
		}
	}
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id := firstString(row, "AlertUuid", "LogUuid", "AlertUUID", "LogUUID")
		if id != "" {
			seen[id] = struct{}{}
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func firstString(row map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(fmt.Sprint(row[key])); value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}
