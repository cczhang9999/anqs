package http

import (
	"bytes"
	"context"
	"fmt"
	"io"
	stdhttp "net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"xops-aegis-guard/util/json"
)

const (
	Get    = "GET"
	Post   = "POST"
	Put    = "PUT"
	Patch  = "PATCH"
	Delete = "DELETE"
)

type HttpClient struct {
	client  *stdhttp.Client
	headers stdhttp.Header
}

func NewHttpClient() *HttpClient {
	return &HttpClient{client: &stdhttp.Client{}, headers: make(stdhttp.Header)}
}
func (c *HttpClient) SetHeader(name, value string) { c.headers.Set(name, value) }
func (c *HttpClient) SetTimeout(timeout time.Duration) {
	if timeout > 0 {
		c.client.Timeout = timeout
	}
}
func (c *HttpClient) ExecuteRequest(method, rawURL string, queryParams map[string]string, requestBody, responseBody interface{}) error {
	return c.ExecuteRequestContext(context.Background(), method, rawURL, queryParams, requestBody, responseBody)
}
func (c *HttpClient) ExecuteRequestContext(ctx context.Context, method, rawURL string, queryParams map[string]string, requestBody, responseBody interface{}) error {
	return c.ExecuteRequestContextWithHeaders(ctx, method, rawURL, queryParams, requestBody, responseBody, nil)
}
func (c *HttpClient) ExecuteRequestContextWithHeaders(ctx context.Context, method, rawURL string, queryParams map[string]string, requestBody, responseBody interface{}, headers map[string]string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	q := u.Query()
	for k, v := range queryParams {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	var body []byte
	if requestBody != nil {
		body, err = json.Marshal(requestBody)
		if err != nil {
			return err
		}
	}
	req, err := stdhttp.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	for k, v := range c.headers {
		req.Header[k] = append([]string(nil), v...)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// 将响应体带入错误信息。内部接口（例如扫描 Agent）会在 4xx 响应中返回有用的
		// 校验信息；如果丢弃响应体，服务端日志只能显示状态码，必须另外检查 Agent。
		errorBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		if readErr == nil {
			message := strings.TrimSpace(string(errorBody))
			if message != "" {
				return fmt.Errorf("request failed with status code: %d: %s", resp.StatusCode, message)
			}
		}
		return fmt.Errorf("request failed with status code: %d", resp.StatusCode)
	}
	if responseBody != nil {
		return json.NewDecoder(resp.Body).Decode(responseBody)
	}
	return nil
}
func (c *HttpClient) Download(rawURL, fileName string) {
	resp, err := c.client.Get(rawURL)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != stdhttp.StatusOK {
		return
	}
	f, err := os.Create(fileName)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = io.Copy(f, resp.Body)
}
