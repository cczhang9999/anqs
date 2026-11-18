package desensitize

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"xops-aegis-guard/config"
)

type Masker struct {
	enabled       bool
	rules         map[string]Rule
	whiteMap      map[string]bool
	textRules     []textRule
	textRuleError bool
}

type Rule struct {
	Type      string
	KeepLeft  int
	KeepRight int
	Replace   string
}

var whitelist []string = []string{
	"asset",
	"user",
	"id",
	"org_id",
	"@timestamp",
	"cmd_timestamp",
	"risk_level",
	"account",
	"log_source",
	"log_type",
	"host",
	"app",
	"ecs",
	"env",
	"name",
	"type",
	"version",
	"offset",
	"device_id",
	"inode",
	"user_id",
	"user-agent",
	"time",
	"svc_type",
	"status",
	"method",
	"login_ip",
	"domain",
	"title", "severity", "source", "producttype", "alerttype", "fingerprint",
	"provideralertid", "uniqueinfo", "alarmuniqueinfo", "instanceid", "instancename",
	"intranetip", "internetip", "privateip", "publicip", "uuid", "level",
	"namedisplay", "fieldkey", "descriptionkey", "occurrencetime", "lasttime",
	"procpath", "processpath", "workingdirectory", "cwd",
}

func NewMasker(conf *config.Desensitize) *Masker {
	m := &Masker{enabled: true, rules: make(map[string]Rule)}
	if conf == nil {
		conf = &config.Desensitize{Enabled: true}
	}
	m.enabled = conf.Enabled
	m.initTextRules(conf.TextRules)
	for _, item := range conf.Rules {
		rule := Rule{
			Type:      normalizeType(item.Type),
			KeepLeft:  item.KeepLeft,
			KeepRight: item.KeepRight,
			Replace:   item.Replace,
		}
		if rule.Replace == "" {
			rule.Replace = "*"
		}
		for _, field := range item.Fields {
			field = strings.ToLower(strings.TrimSpace(field))
			if field != "" {
				m.rules[field] = rule
			}
		}
	}
	m.whiteMap = make(map[string]bool, len(whitelist))
	for _, v := range whitelist {
		m.whiteMap[normalizeField(v)] = true
	}
	return m
}

func (m *Masker) Apply(v any) any {
	if m == nil {
		m = NewMasker(nil)
	}
	return m.maskAny("", v)
}

func (m *Masker) maskAny(key string, v any) any {
	if m.isWhitelisted(key) {
		return v
	}
	switch value := v.(type) {
	case map[string]any:
		metadataValue := false
		if m.textRuleError {
			for _, labelKey := range []string{"NameDisplay", "Name", "FieldKey"} {
				if label, ok := value[labelKey].(string); ok && label != "" {
					metadataValue = !isAlertTextField(label)
					break
				}
			}
		}
		for k, child := range value {
			if metadataValue && (strings.EqualFold(k, "Value") || strings.EqualFold(k, "ValueDisplay") || strings.EqualFold(k, "FieldValue")) {
				continue
			}
			value[k] = m.maskAny(k, child)
		}
		return value
	case []any:
		for i, child := range value {
			value[i] = m.maskAny(key, child)
		}
		return value
	case []map[string]any:
		for i, child := range value {
			value[i] = m.maskAny(key, child).(map[string]any)
		}
		return value
	case string:
		for _, textRule := range m.textRules {
			if textRule.Type == "key_value" && len(textRule.Commands) == 0 {
				for _, parameter := range textRule.Parameters {
					if value != "" && keyMatches(key, parameter) {
						return passwordMask
					}
				}
			}
		}
		rule, ok := m.rules[strings.ToLower(key)]
		if ok && m.enabled {
			return rule.apply(value)
		}
		// Provider fields can contain another JSON document as a string. Decode
		// it before scanning so escaped quotes cannot truncate a password.
		var nested any
		if (strings.HasPrefix(strings.TrimSpace(value), "{") || strings.HasPrefix(strings.TrimSpace(value), "[")) && json.Unmarshal([]byte(value), &nested) == nil {
			before, _ := json.Marshal(nested)
			cleaned := m.maskAny("", nested)
			if data, err := json.Marshal(cleaned); err == nil {
				if bytes.Equal(before, data) {
					return value
				}
				return string(data)
			}
			return passwordMask
		}
		if m.textRuleError && !isAlertTextField(key) {
			return value
		}
		return m.maskSensitiveText(value)
	default:
		return value
	}
}

// Failure fallback affects command-bearing content only; routing identifiers,
// paths, users and timestamps still have to be available for investigation.
func isAlertTextField(key string) bool {
	if key == "" {
		return true
	}
	key = normalizeField(key)
	for _, part := range []string{"cmd", "command", "desc", "message", "content", "chain", "tree", "reason", "text", "value", "命令", "描述", "内容", "进程链", "进程关系", "原因", "全文"} {
		if strings.Contains(key, part) {
			return true
		}
	}
	return false
}
func (m *Masker) isWhitelisted(key string) bool {
	if key == "" {
		return false
	}
	_, ok := m.whiteMap[normalizeField(key)]
	return ok
}
func normalizeField(value string) string {
	return strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(strings.TrimSpace(value)))
}

func (r Rule) apply(value string) string {
	switch r.Type {
	case "remove":
		return ""
	case "hash":
		sum := sha256.Sum256([]byte(value))
		return hex.EncodeToString(sum[:])
	case "fixed":
		return r.Replace
	default:
		return maskString(value, r.KeepLeft, r.KeepRight, r.Replace)
	}
}

func maskString(value string, keepLeft, keepRight int, replace string) string {
	if value == "" {
		return value
	}
	if replace == "" {
		replace = "*"
	}
	runes := []rune(value)
	if keepLeft < 0 {
		keepLeft = 0
	}
	if keepRight < 0 {
		keepRight = 0
	}
	if keepLeft+keepRight >= len(runes) {
		return strings.Repeat(replace, len(runes))
	}
	middleLen := len(runes) - keepLeft - keepRight
	return string(runes[:keepLeft]) + strings.Repeat(replace, middleLen) + string(runes[len(runes)-keepRight:])
}

func normalizeType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "fixed", "hash", "remove", "mask":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "mask"
	}
}
