package system

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type SSEClient struct {
	url     string
	headers map[string]string
	timeout time.Duration
}

func NewSSEClient(url string, headers map[string]string, timeout time.Duration) *SSEClient {
	return &SSEClient{
		url:     url,
		headers: headers,
		timeout: timeout,
	}
}

func (c *SSEClient) Connect(ctx context.Context, onMessage func(string)) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.url, nil)
	if err != nil {
		return err
	}

	// 设置SSE必要的头部
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "keep-alive")

	// 添加自定义头部
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: c.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SSE connection failed with status: %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data != "[DONE]" {
				onMessage(data)
			}
		}
	}

	return scanner.Err()
}
