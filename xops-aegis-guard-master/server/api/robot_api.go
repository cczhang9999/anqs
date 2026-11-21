package api

import (
	"bytes"
	"context"
	stdjson "encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"xops-aegis-guard/config"
	"xops-aegis-guard/util/json"
)

// Notifier 按现有服务使用的 robot-new 接口发送通知。
// 按接口约定，不发送认证信息和 im_name。
type Notifier struct {
	cfg      config.Robot
	telegram config.Telegram
	client   *http.Client
}

const (
	// 为 robot-new 可能添加的内容预留空间，并按 UTF-16 单元保守计算长度。
	robotMessageLimit = 3500
	// 截断后保留的头部和尾部长度总和应低于消息上限。
	robotHeadUnits = 2400
	robotTailUnits = 700
	// Telegram echoes sent text in its response, including multibyte characters.
	notificationResponseLimit = 64 << 10
)

func NewNotifier(cfg config.Robot, telegram ...config.Telegram) *Notifier {
	var telegramCfg config.Telegram
	if len(telegram) > 0 {
		telegramCfg = telegram[0]
	}
	return &Notifier{cfg: cfg, telegram: telegramCfg, client: &http.Client{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second}}
}

func (n *Notifier) TelegramEnabled() bool {
	return n != nil && n.telegram.Enabled
}

func (n *Notifier) RobotEnabled() bool {
	return n != nil && n.cfg.Enabled
}

// NotifyTelegram sends plain text directly to the configured Telegram chat.
func (n *Notifier) NotifyTelegram(ctx context.Context, text string) error {
	if n == nil || !n.telegram.Enabled {
		return nil
	}
	token := strings.TrimSpace(n.telegram.BotToken)
	chatID := strings.TrimSpace(n.telegram.ChatID)
	if token == "" {
		return fmt.Errorf("telegram bot token is empty")
	}
	if chatID == "" {
		return fmt.Errorf("telegram chat id is empty")
	}
	endpoint := "https://api.telegram.org/bot" + token + "/sendMessage"
	body, err := stdjson.Marshal(map[string]any{
		"chat_id": chatID,
		"text":    truncateRobotMessage(text),
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("invalid telegram request")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.client.Do(req)
	if err != nil {
		// The request URL includes the bot token, so never propagate a transport
		// error that could print the URL into application logs.
		return fmt.Errorf("telegram request failed")
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, notificationResponseLimit))
	if err != nil {
		return err
	}
	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	decodeErr := stdjson.Unmarshal(respBody, &result)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if decodeErr == nil && result.Description != "" {
			return fmt.Errorf("telegram returned status %d: %s", resp.StatusCode, strings.ReplaceAll(result.Description, token, "[redacted]"))
		}
		return fmt.Errorf("telegram returned status %d", resp.StatusCode)
	}
	if decodeErr != nil {
		return fmt.Errorf("decode telegram response: %w", decodeErr)
	}
	if !result.OK {
		return fmt.Errorf("telegram rejected message: %s", strings.ReplaceAll(result.Description, token, "[redacted]"))
	}
	return nil
}

func (n *Notifier) Notify(ctx context.Context, robotID int, text string) error {
	if n == nil || !n.cfg.Enabled {
		return nil
	}
	if robotID == 0 {
		return fmt.Errorf("robot id is empty")
	}
	endpoint := strings.TrimRight(strings.TrimSpace(n.cfg.URL), "/")
	if endpoint == "" {
		return fmt.Errorf("robot URL is empty")
	}
	if !strings.HasSuffix(endpoint, "/v1/message") {
		endpoint += "/v1/message"
	}
	// 始终只发送一条消息；超长内容会截断，不会拆成多条。
	text = truncateRobotMessage(text)
	body, err := json.Marshal(map[string]any{"im_id": robotID, "source_name": n.cfg.SourceName, "text": text})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("robot-new returned status %d", resp.StatusCode)
	}
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, notificationResponseLimit))
	if err != nil {
		return fmt.Errorf("read robot-new response: %w", err)
	}
	if bytes.HasPrefix(bytes.TrimSpace(respBody), []byte("{")) {
		var result struct {
			Code *int `json:"code"`
		}
		if err := stdjson.Unmarshal(respBody, &result); err != nil {
			return fmt.Errorf("decode robot-new response: %w", err)
		}
		if result.Code != nil && *result.Code != 0 && *result.Code != http.StatusOK {
			return fmt.Errorf("robot-new rejected message (code %d)", *result.Code)
		}
	}
	return nil
}

func runeUnits(r rune) int {
	if r > 0xffff {
		return 2
	}
	return 1
}

func robotMessageUnits(text string) int {
	units := 0
	for _, r := range text {
		units += runeUnits(r)
	}
	return units
}

// truncateRobotMessage 将消息控制在 robotMessageLimit 以内，以单条请求发送。
// 函数保留开头（通常是告警摘要）和结尾（通常是结论），省略中间内容。
// 截断时优先选择换行处，并始终按完整 Unicode 字符切分。
func truncateRobotMessage(text string) string {
	if robotMessageUnits(text) <= robotMessageLimit {
		return text
	}
	runes := []rune(text)

	// 计算头部结束位置，头部长度不超过 robotHeadUnits。
	headEnd, units := 0, 0
	for headEnd < len(runes) {
		size := runeUnits(runes[headEnd])
		if units+size > robotHeadUnits {
			break
		}
		units += size
		headEnd++
	}
	// 计算尾部起始位置，尾部长度不超过 robotTailUnits。
	tailStart, units := len(runes), 0
	for tailStart > headEnd {
		size := runeUnits(runes[tailStart-1])
		if units+size > robotTailUnits {
			break
		}
		units += size
		tailStart--
	}

	// 若不会损失过多内容，优先在换行处截断。
	for i := headEnd - 1; i > headEnd/2; i-- {
		if runes[i] == '\n' {
			headEnd = i
			break
		}
	}
	for i := tailStart; i < len(runes) && i < tailStart+(len(runes)-tailStart)/2; i++ {
		if runes[i] == '\n' {
			tailStart = i + 1
			break
		}
	}

	marker := "\n……\n"
	return string(runes[:headEnd]) + marker + string(runes[tailStart:])
}
