package push

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const showdocURL = "https://push.showdoc.com.cn/server/api/push"

// Showdoc sends a WeChat notification via Showdoc push.
// token is the Showdoc push key; title and content are the notification fields.
func Showdoc(token, title, content string) (string, error) {
	body := map[string]string{
		"title":   title,
		"content": content,
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", showdocURL+"/"+token, bytes.NewReader(b))
	if err != nil {
		return "", fmt.Errorf("push: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("push: %w", err)
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	return string(rb), nil
}
