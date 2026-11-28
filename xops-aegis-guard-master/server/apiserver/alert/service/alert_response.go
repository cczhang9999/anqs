package service

import (
	stdhtml "html"
	"io"
	"net/url"
	"strconv"
	"strings"

	"xops-aegis-guard/desensitize"
	"xops-aegis-guard/logger"
	alertmodel "xops-aegis-guard/server/apiserver/alert/model"
	alerttools "xops-aegis-guard/server/apiserver/alert/tools"
	jsonutil "xops-aegis-guard/util/json"

	htmlparser "golang.org/x/net/html"
)

// sanitizeAlertRows 仅清理接口返回值，旧记录无需回写也不会在确认页暴露密码。
func sanitizeAlertRows(rows []alertmodel.SecurityAlert) []alertmodel.SecurityAlert {
	for i := range rows {
		sanitizeAlertRow(&rows[i])
	}
	return rows
}

func sanitizeAlertRow(row *alertmodel.SecurityAlert) {
	maskAlertContent(row)
}

// maskAlertContent covers manual API writes as well as snapshots read before
// confirming an alert. Collection uses the same Masker before saving/routing.
func maskAlertContent(row *alertmodel.SecurityAlert) {
	if row == nil {
		return
	}
	defer func() { row.ContentHash = alerttools.ContentHash(row.LastMessage) }()
	var masker *desensitize.Masker
	if instance != nil {
		masker = instance.masker
	}
	row.LastMessage, _ = masker.Apply(row.LastMessage).(string)
	if row == nil || row.LastPayload == "" {
		return
	}
	var payload any
	if err := jsonutil.Unmarshal([]byte(row.LastPayload), &payload); err != nil {
		row.LastPayload = `"******"`
		if logger.GetLogger() != nil {
			logger.Errorf("password masking failed: invalid alert payload; content replaced with fixed mask")
		}
		return
	}
	cleaned := masker.Apply(sanitizeHTMLFields(payload))
	data, err := jsonutil.Marshal(cleaned)
	if err == nil {
		row.LastPayload = string(data)
	} else {
		row.LastPayload = `"******"`
	}
}

func sanitizeHTMLFields(value any) any {
	switch item := value.(type) {
	case []any:
		for i := range item {
			item[i] = sanitizeHTMLFields(item[i])
		}
		return item
	case []map[string]any:
		for i := range item {
			item[i] = sanitizeHTMLFields(item[i]).(map[string]any)
		}
		return item
	case string:
		if looksLikeHTML(item) {
			return htmlToPlainText(item)
		}
		return item
	case map[string]any:
		isHTML := false
		for key, child := range item {
			if strings.EqualFold(key, "type") && strings.EqualFold(strings.TrimSpace(stringValue(child)), "html") {
				isHTML = true
				break
			}
		}
		for key, child := range item {
			if isHTML && (strings.EqualFold(key, "value") || strings.EqualFold(key, "valueDisplay")) {
				if text, ok := child.(string); ok {
					item[key] = htmlToPlainText(text)
					continue
				}
			}
			if strings.EqualFold(key, "value") || strings.EqualFold(key, "valueDisplay") || strings.EqualFold(key, "markMisRules") {
				if text, ok := child.(string); ok {
					if looksLikeHTML(text) {
						item[key] = htmlToPlainText(text)
					} else {
						item[key] = normalizeAlertText(text)
					}
					continue
				}
			}
			item[key] = sanitizeHTMLFields(child)
		}
		return item
	default:
		return value
	}
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func looksLikeHTML(value string) bool {
	if strings.Contains(value, "<") && strings.Contains(value, ">") {
		return true
	}
	for _, entity := range []string{"&nbsp", "&lt;", "&gt;", "&amp;", "&quot;", "&#"} {
		if strings.Contains(strings.ToLower(value), entity) {
			return true
		}
	}
	return false
}

func htmlToPlainText(value string) string {
	if value == "" {
		return value
	}
	// 部分云厂商字段会把换行再次转义为两个字符 "\\n"。
	value = strings.ReplaceAll(value, `\n`, "\n")
	value = strings.ReplaceAll(value, `\r`, "\r")
	var builder strings.Builder
	tokenizer := htmlparser.NewTokenizer(strings.NewReader(value))
	listOrdered := make([]bool, 0, 2)
	listCounters := make([]int, 0, 2)
	anchorURLs := make([]string, 0, 1)
	ensureNewline := func() {
		if builder.Len() == 0 || strings.HasSuffix(builder.String(), "\n") {
			return
		}
		builder.WriteByte('\n')
	}
	for {
		tokenType := tokenizer.Next()
		switch tokenType {
		case htmlparser.ErrorToken:
			if tokenizer.Err() == io.EOF {
				return collapsePlainText(builder.String())
			}
			return stdhtml.UnescapeString(value)
		case htmlparser.TextToken:
			builder.WriteString(stdhtml.UnescapeString(string(tokenizer.Raw())))
		case htmlparser.StartTagToken, htmlparser.EndTagToken, htmlparser.SelfClosingTagToken:
			token := tokenizer.Token()
			tagName := strings.ToLower(token.Data)
			if tokenType == htmlparser.StartTagToken {
				switch tagName {
				case "a":
					for _, attr := range token.Attr {
						if strings.EqualFold(attr.Key, "href") {
							if href := safeHTTPURL(attr.Val); href != "" {
								builder.WriteByte('[')
								anchorURLs = append(anchorURLs, href)
							}
							break
						}
					}
				case "ol", "ul":
					ensureNewline()
					listOrdered = append(listOrdered, tagName == "ol")
					listCounters = append(listCounters, 0)
				case "li":
					ensureNewline()
					if len(listOrdered) > 0 {
						last := len(listOrdered) - 1
						if listOrdered[last] {
							listCounters[last]++
							builder.WriteString(strconv.Itoa(listCounters[last]))
							builder.WriteString(". ")
						} else {
							builder.WriteString("- ")
						}
					}
				case "br":
					ensureNewline()
				default:
					if isBlockHTMLTag(tagName) {
						ensureNewline()
					}
				}
			} else {
				switch tagName {
				case "a":
					if len(anchorURLs) > 0 {
						last := len(anchorURLs) - 1
						builder.WriteString("](")
						builder.WriteString(anchorURLs[last])
						builder.WriteByte(')')
						anchorURLs = anchorURLs[:last]
					}
				case "ol", "ul":
					ensureNewline()
					if len(listOrdered) > 0 {
						listOrdered = listOrdered[:len(listOrdered)-1]
						listCounters = listCounters[:len(listCounters)-1]
					}
				case "li":
					ensureNewline()
				default:
					if isBlockHTMLTag(tagName) {
						ensureNewline()
					}
				}
			}
		}
	}
}

// safeHTTPURL accepts only absolute HTTP(S) URLs before they are embedded in a
// Markdown link. Provider values must never be allowed to inject arbitrary
// Markdown or a non-web URI into robot notifications.
func safeHTTPURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(stdhtml.UnescapeString(value)))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	return parsed.String()
}

func isBlockHTMLTag(tag string) bool {
	switch strings.ToLower(tag) {
	case "br", "p", "div", "li", "ul", "ol", "section", "article", "h1", "h2", "h3", "h4", "h5", "h6", "pre", "tr":
		return true
	default:
		return false
	}
}

func normalizeAlertText(value string) string {
	value = strings.ReplaceAll(value, "\u00a0", " ")
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	lines := strings.Split(value, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n")
}

func notificationPlainText(value string) string {
	if looksLikeHTML(value) {
		return htmlToPlainText(value)
	}
	return normalizeAlertText(value)
}

func collapsePlainText(value string) string {
	return normalizeAlertText(value)
}
