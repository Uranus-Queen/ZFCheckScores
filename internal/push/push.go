package push

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const serverChanBase = "https://sctapi.ftqq.com"

// ServerChan sends a grade-update alert via Server酱 (ServerChan).
// sendKey is the SCT... SendKey; title is the message title (≤32 chars),
// desp is Markdown content (≤32KB), and short is the card preview (≤64 chars,
// optional). The self-hosted glassmorphism page is the canonical view; this
// call only carries a short summary + a deep link to it, because WeChat
// templates/markdown cannot render the card itself.
func ServerChan(sendKey, title, desp, short string) (string, error) {
	form := url.Values{}
	form.Set("title", title)
	form.Set("desp", desp)
	if short != "" {
		form.Set("short", short)
	}
	body := strings.NewReader(form.Encode())
	req, err := http.NewRequest("POST", serverChanBase+"/"+sendKey+".send", body)
	if err != nil {
		return "", fmt.Errorf("serverchan: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("serverchan: %w", err)
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	return string(rb), nil
}
