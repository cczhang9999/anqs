package desensitize

import (
	"log"
	"path"
	"regexp"
	"sort"
	"strings"

	"xops-aegis-guard/config"
	"xops-aegis-guard/logger"
)

const passwordMask = "******"

type textRule struct {
	config.TextRule
	re *regexp.Regexp
}

type textSpan struct{ start, end int }
type shellWord struct {
	textSpan
	separator bool
}

func defaultTextRules() []config.TextRule {
	return []config.TextRule{
		{Type: "key_value", Parameters: []string{"password", "passwd", "pwd", "secret", "secret_key", "token", "access_key", "*.password", "*_password", "*_secret", "*_token"}},
		{Type: "credential", Commands: []string{"ncat"}, Parameters: []string{"--proxy-auth"}},
		{Type: "credential", Commands: []string{"curl"}, Parameters: []string{"-u", "--user", "-U", "--proxy-user"}},
		{Type: "argument", Parameters: []string{"--password", "--passwd"}},
		{Type: "argument", Commands: []string{"sshpass"}, Parameters: []string{"-p"}},
		{Type: "argument", Commands: []string{"redis-cli"}, Parameters: []string{"-a"}},
		{Type: "attached", Commands: []string{"mysql", "mysqldump", "mysqladmin"}, Parameters: []string{"-p"}},
		{Type: "url"},
	}
}

func (m *Masker) initTextRules(rules []config.TextRule) {
	if rules == nil {
		rules = defaultTextRules()
	}
	// An empty or invalid list must never silently disable password masking.
	if len(rules) == 0 {
		m.textRuleError = true
	}
	for _, rule := range rules {
		compiled := textRule{TextRule: rule}
		valid := true
		switch rule.Type {
		case "key_value", "credential", "argument", "attached":
			valid = len(rule.Parameters) > 0
			for _, parameter := range rule.Parameters {
				if parameter == "" {
					valid = false
				}
			}
		case "url":
		case "regex":
			var err error
			compiled.re, err = regexp.Compile(rule.Pattern)
			valid = err == nil && compiled.re.SubexpIndex("password") > 0 && len(rule.Commands) == 0
		default:
			valid = false
		}
		if !valid {
			m.textRuleError = true
			continue
		}
		m.textRules = append(m.textRules, compiled)
	}
	if m.textRuleError {
		logMaskFailure()
	}
}

// Deliberately omit both input and error values: a regexp or panic value may
// itself contain credentials. Failures must not create a second leak in logs.
func logMaskFailure() {
	if logger.GetLogger() != nil {
		logger.Errorf("password masking failed; alert text replaced with fixed mask")
	} else {
		log.Print("password masking failed; alert text replaced with fixed mask")
	}
}

func (m *Masker) maskSensitiveText(value string) (result string) {
	if value == "" {
		return value
	}
	defer func() {
		if recover() != nil {
			result = passwordMask
			logMaskFailure()
		}
	}()
	if m.textRuleError {
		return passwordMask
	}
	spans := m.passwordSpans(value, 0)
	if len(spans) == 0 {
		return value
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	// Rules may overlap (e.g. password=mysql://user:secret@host). Redact their
	// union once, without changing offsets or exposing fragments of a value.
	merged := make([]textSpan, 0, len(spans))
	for _, span := range spans {
		if span.start >= span.end {
			continue
		}
		if len(merged) > 0 && span.start <= merged[len(merged)-1].end {
			if span.end > merged[len(merged)-1].end {
				merged[len(merged)-1].end = span.end
			}
		} else {
			merged = append(merged, span)
		}
	}
	var out strings.Builder
	at := 0
	for _, span := range merged {
		out.WriteString(value[at:span.start])
		out.WriteString(passwordMask)
		at = span.end
	}
	out.WriteString(value[at:])
	return out.String()
}

func (r textRule) forCommand(command string) bool {
	if len(r.Commands) == 0 {
		return true
	}
	for _, candidate := range r.Commands {
		if command == candidate {
			return true
		}
	}
	return false
}

func parameterEqual(a, b string) bool {
	if strings.HasPrefix(a, "--") && strings.HasPrefix(b, "--") {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func keyMatches(key, pattern string) bool {
	key = strings.TrimPrefix(strings.TrimPrefix(key, "--"), "-")
	// Java properties use -D; only remove D when it was a flag prefix.
	key = strings.ToLower(key)
	pattern = strings.ToLower(pattern)
	if key == pattern {
		return true
	}
	if strings.HasPrefix(pattern, "*") {
		suffix := pattern[1:]
		return len(key) > len(suffix) && strings.HasSuffix(key, suffix)
	}
	return false
}

func (m *Masker) passwordSpans(value string, depth int) []textSpan {
	if depth > 32 {
		// Deeply nested shell wrappers are malformed/unreasonable input. The
		// caller recovers and masks this complete text instead of forwarding it.
		panic("command nesting limit")
	}
	words := shellWords(value)
	spans := make([]textSpan, 0)
	command := ""
	wrappedCommandAt := -1
	wrapperOptionValue := false
	for i, word := range words {
		if i == wrappedCommandAt {
			command = ""
		}
		if word.separator {
			command = ""
			wrapperOptionValue = false
			continue
		}
		body := unquote(value, word.textSpan)
		raw := value[body.start:body.end]
		// Provider command lines may flatten argv without quoting the shell's
		// command string (e.g. bash -c exec ncat ...). Scan quoted strings
		// recursively, and resume executable detection for flattened commands.
		if (command == "sh" || command == "bash" || command == "dash" || command == "zsh") && i > 0 {
			previous := value[words[i-1].start:words[i-1].end]
			if strings.HasPrefix(previous, "-") && strings.Contains(previous, "c") {
				for _, span := range m.passwordSpans(raw, depth+1) {
					spans = append(spans, textSpan{body.start + span.start, body.start + span.end})
				}
				if body == word.textSpan {
					command = ""
					wrapperOptionValue = false
				}
			}
		}
		if command == "" {
			if wrapperOptionValue {
				wrapperOptionValue = false
			} else if path.Base(raw) == "sudo" || path.Base(raw) == "env" || raw == "command" || raw == "exec" || path.Base(raw) == "nohup" {
				// Wrappers can be stacked; executable paths are recognized below.
			} else if raw == "-u" || raw == "-g" || raw == "--user" || raw == "--group" {
				wrapperOptionValue = true
			} else if !strings.HasPrefix(raw, "-") && !strings.Contains(raw, "=") {
				command = path.Base(raw)
			}
		}
		for _, rule := range m.textRules {
			if !rule.forCommand(command) {
				continue
			}
			switch rule.Type {
			case "key_value":
				if equals := strings.IndexByte(raw, '='); equals >= 0 {
					key := raw[:equals]
					if strings.HasPrefix(key, "-D") || strings.HasPrefix(key, "-d") {
						key = key[2:]
					}
					for _, parameter := range rule.Parameters {
						if keyMatches(key, parameter) {
							spans = append(spans, unquote(value, textSpan{body.start + equals + 1, body.end}))
							break
						}
					}
				}
			case "argument", "credential", "attached":
				for _, parameter := range rule.Parameters {
					secret := textSpan{}
					if rule.Type == "attached" {
						if len(raw) > len(parameter) && parameterEqual(raw[:len(parameter)], parameter) {
							secret = unquote(value, textSpan{body.start + len(parameter), body.end})
						}
					} else if parameterEqual(raw, parameter) && i+1 < len(words) && !words[i+1].separator {
						secret = unquote(value, words[i+1].textSpan)
						if command == "sshpass" {
							wrappedCommandAt = i + 2
						}
					} else if eq := strings.IndexByte(raw, '='); eq >= 0 && parameterEqual(raw[:eq], parameter) {
						secret = unquote(value, textSpan{body.start + eq + 1, body.end})
					}
					if secret.end <= secret.start || (rule.Type == "argument" && value[secret.start] == '-') {
						continue
					}
					if rule.Type == "credential" {
						colon := strings.IndexByte(value[secret.start:secret.end], ':')
						if colon < 0 {
							continue
						}
						secret = unquote(value, textSpan{secret.start + colon + 1, secret.end})
					}
					spans = append(spans, secret)
				}
			case "url":
				for _, location := range urlStart.FindAllStringIndex(raw, -1) {
					start := location[1]
					end := len(raw)
					// Quotes around a URL's value are outside its userinfo.
					for j := start; j < end; j++ {
						if raw[j] == '\'' || raw[j] == '"' {
							end = j
							break
						}
					}
					at := strings.LastIndexByte(raw[start:end], '@')
					if at < 0 {
						continue
					}
					at += start
					colon := strings.IndexByte(raw[start:at], ':')
					if colon >= 0 {
						spans = append(spans, textSpan{body.start + start + colon + 1, body.start + at})
					}
				}
			}
		}
	}
	for _, rule := range m.textRules {
		if rule.Type != "regex" || len(rule.Commands) != 0 {
			continue
		}
		group := rule.re.SubexpIndex("password") * 2
		for _, match := range rule.re.FindAllStringSubmatchIndex(value, -1) {
			if match[group] >= 0 {
				spans = append(spans, textSpan{match[group], match[group+1]})
			}
		}
	}
	return spans
}

var urlStart = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://`)

func unquote(value string, span textSpan) textSpan {
	if span.end-span.start >= 2 && (value[span.start] == '\'' || value[span.start] == '"') && value[span.end-1] == value[span.start] {
		span.start++
		span.end--
	}
	return span
}

// shellWords retains byte ranges rather than rebuilding commands, preserving
// whitespace, quotes, usernames, IPs and every byte outside a password.
func shellWords(value string) []shellWord {
	words := make([]shellWord, 0)
	for i := 0; i < len(value); {
		if strings.HasPrefix(value[i:], "：") {
			words = append(words, shellWord{textSpan{i, i + len("：")}, true})
			i += len("：")
			continue
		}
		if strings.ContainsRune("\n\r", rune(value[i])) || (strings.ContainsRune(";|&><()", rune(value[i])) && (i+1 == len(value) || strings.ContainsRune(" \t\n\r;|&><()", rune(value[i+1])))) {
			words = append(words, shellWord{textSpan{i, i + 1}, true})
			i++
			continue
		}
		if value[i] == ' ' || value[i] == '\t' {
			i++
			continue
		}
		start := i
		quote := byte(0)
		for i < len(value) {
			c := value[i]
			if c == '\\' && quote != '\'' && i+1 < len(value) {
				i += 2
				continue
			}
			if quote != 0 {
				if c == quote {
					quote = 0
				}
			} else if c == '\'' || c == '"' {
				quote = c
			} else if strings.ContainsRune(" \t\n\r", rune(c)) || (strings.HasPrefix(value[i:], "：") && !strings.ContainsAny(value[start:i], "=:")) {
				break
			}
			i++
		}
		words = append(words, shellWord{textSpan{start, i}, false})
	}
	return words
}
