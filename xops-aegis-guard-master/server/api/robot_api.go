package api

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"xops-aegis-guard/config"
	"xops-aegis-guard/util/json"
)

// Notifier 按现有服务使用的 robot-new 接口发送通知。
// 按接口约定，不发送认证信息和 im_name。
type Notifier struct {
	cfg    config.Robot
	client *http.Client
}

const (
	// 为 robot-new 可能添加的内容预留空间，并按 UTF-16 单元保守计算长度。
	robotMessageLimit = 3500
	// 截断后保留的头部和尾部长度总和应低于消息上限。
	robotHeadUnits = 2400
	robotTailUnits = 700
)

func NewNotifier(cfg config.Robot) *Notifier {
	return &Notifier{cfg: cfg, client: &http.Client{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second}}
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
