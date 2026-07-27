package zfn

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// newTestClient starts a mock server and returns a Client configured to hit it.
func newTestClient(handler http.HandlerFunc) (*Client, *httptest.Server) {
	server := httptest.NewServer(handler)
	client, _ := NewClient(server.URL, 5)
	return client, server
}

func TestGetUserInfo_JSONSuccess(t *testing.T) {
	client, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "xsxxwh_cxCkDgxsxx") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"xm":"张俊","xh":"230101","bh_id":"金属2301"}`)
			return
		}
		http.NotFound(w, r)
	})
	defer server.Close()

	res, err := client.GetUserInfo()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Code != 1000 {
		t.Fatalf("expected code 1000, got %d: %s", res.Code, res.Msg)
	}
	if res.Data["xm"] != "张俊" || res.Data["xh"] != "230101" {
		t.Fatalf("unexpected data: %+v", res.Data)
	}
}

func TestGetUserInfo_EmptyJSONFallsBackToHTML(t *testing.T) {
	client, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "xsxxwh_cxCkDgxsxx") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{}`)
			return
		}
		if strings.Contains(r.URL.Path, "xsgrxxwh_cxXsgrxx") {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, `<html><body>
				<div class="col-sm-6"><div class="form-group"><label class="col-sm-4 control-label">学号：</label><div class="col-sm-8"><p class="form-control-static">230101</p></div></div></div>
				<div class="col-sm-6"><div class="form-group"><label class="col-sm-4 control-label">姓名：</label><div class="col-sm-8"><p class="form-control-static">张俊</p></div></div></div>
				<div class="col-sm-6"><div class="form-group"><label class="col-sm-4 control-label">班级名称：</label><div class="col-sm-8"><p class="form-control-static">金属2301</p></div></div></div>
			</body></html>`)
			return
		}
		http.NotFound(w, r)
	})
	defer server.Close()

	res, err := client.GetUserInfo()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Code != 1000 {
		t.Fatalf("expected code 1000, got %d: %s", res.Code, res.Msg)
	}
	if res.Data["name"] != "张俊" {
		t.Fatalf("expected name 张俊, got %v", res.Data["name"])
	}
	if res.Data["class_name"] != "金属2301" {
		t.Fatalf("expected class_name 金属2301, got %v", res.Data["class_name"])
	}
}

func TestGetUserInfo_HTML200FallsBack(t *testing.T) {
	client, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "xsxxwh_cxCkDgxsxx") {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<html><body>unexpected html page</body></html>`)
			return
		}
		if strings.Contains(r.URL.Path, "xsgrxxwh_cxXsgrxx") {
			fmt.Fprint(w, `<html><body>
				<div class="col-sm-6"><div class="form-group"><label class="col-sm-4 control-label">学号：</label><div class="col-sm-8"><p class="form-control-static">230102</p></div></div></div>
				<div class="col-sm-6"><div class="form-group"><label class="col-sm-4 control-label">姓名：</label><div class="col-sm-8"><p class="form-control-static">李四</p></div></div></div>
			</body></html>`)
			return
		}
		http.NotFound(w, r)
	})
	defer server.Close()

	res, err := client.GetUserInfo()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Code != 1000 {
		t.Fatalf("expected code 1000, got %d: %s", res.Code, res.Msg)
	}
	if res.Data["name"] != "李四" {
		t.Fatalf("expected name 李四, got %v", res.Data["name"])
	}
}

func TestGetUserInfo_HTTP500FallsBack(t *testing.T) {
	client, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "xsxxwh_cxCkDgxsxx") {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "system maintenance")
			return
		}
		if strings.Contains(r.URL.Path, "xsgrxxwh_cxXsgrxx") {
			fmt.Fprint(w, `<html><body>
				<div class="col-sm-6"><div class="form-group"><label class="col-sm-4 control-label">学号：</label><div class="col-sm-8"><p class="form-control-static">230103</p></div></div></div>
				<div class="col-sm-6"><div class="form-group"><label class="col-sm-4 control-label">姓名：</label><div class="col-sm-8"><p class="form-control-static">王五</p></div></div></div>
			</body></html>`)
			return
		}
		http.NotFound(w, r)
	})
	defer server.Close()

	res, err := client.GetUserInfo()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Code != 1000 {
		t.Fatalf("expected code 1000, got %d: %s", res.Code, res.Msg)
	}
	if res.Data["name"] != "王五" {
		t.Fatalf("expected name 王五, got %v", res.Data["name"])
	}
}

func TestGetUserInfo_WAFChallenge(t *testing.T) {
	client, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body><h5></h5><form><input id="yzm" name="yzm"/></form></body></html>`)
	})
	defer server.Close()

	res, err := client.GetUserInfo()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Code != 1001 {
		t.Fatalf("expected code 1001 (WAF), got %d: %s", res.Code, res.Msg)
	}
}

func TestGetUserInfo_LoginPage(t *testing.T) {
	client, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body><h5>用户登录</h5></body></html>`)
	})
	defer server.Close()

	res, err := client.GetUserInfo()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Code != 1006 {
		t.Fatalf("expected code 1006 (not logged in), got %d: %s", res.Code, res.Msg)
	}
}

func TestGetUserInfo_BothEndpointsFail(t *testing.T) {
	client, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "not found")
	})
	defer server.Close()

	res, err := client.GetUserInfo()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Code != 2333 {
		t.Fatalf("expected code 2333, got %d: %s", res.Code, res.Msg)
	}
}

func TestGetUserInfo_BackfillCollegeInfo(t *testing.T) {
	client, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "xsxxwh_cxCkDgxsxx") {
			fmt.Fprint(w, `{}`)
			return
		}
		if strings.Contains(r.URL.Path, "xsgrxxwh_cxXsgrxx") {
			fmt.Fprint(w, `<html><body>
				<div class="col-sm-6"><div class="form-group"><label class="col-sm-4 control-label">学号：</label><div class="col-sm-8"><p class="form-control-static">230104</p></div></div></div>
				<div class="col-sm-6"><div class="form-group"><label class="col-sm-4 control-label">姓名：</label><div class="col-sm-8"><p class="form-control-static">赵六</p></div></div></div>
			</body></html>`)
			return
		}
		if strings.Contains(r.URL.Path, "xszbbgl") {
			fmt.Fprint(w, `<html><body>
				<div class="col-sm-6"><div class="form-group"><label class="col-sm-4 control-label">学院</label><div class="col-sm-8"><label class="control-label">材料科学与工程学院</label></div></div></div>
				<div class="col-sm-6"><div class="form-group"><label class="col-sm-4 control-label">专业</label><div class="col-sm-8"><label class="control-label">金属材料工程</label></div></div></div>
				<div class="col-sm-6"><div class="form-group"><label class="col-sm-4 control-label">班级</label><div class="col-sm-8"><label class="control-label">金属2301</label></div></div></div>
			</body></html>`)
			return
		}
		http.NotFound(w, r)
	})
	defer server.Close()

	res, err := client.GetUserInfo()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Code != 1000 {
		t.Fatalf("expected code 1000, got %d: %s", res.Code, res.Msg)
	}
	if res.Data["college_name"] != "材料科学与工程学院" {
		t.Fatalf("expected college backfilled, got %v", res.Data["college_name"])
	}
	if res.Data["class_name"] != "金属2301" {
		t.Fatalf("expected class backfilled, got %v", res.Data["class_name"])
	}
}

// TestResolveURLKeepsContextPath ensures resolveURL preserves any context
// path carried by the base URL (e.g. /jwglxt) instead of dropping it. This is
// the fix for njtech deployments where the whole app lives under /jwglxt.
func TestResolveURLKeepsContextPath(t *testing.T) {
	c, _ := NewClient("https://example.com/jwglxt", 5)
	if got := c.resolveURL("/xtgl/login_slogin.html"); got != "https://example.com/jwglxt/xtgl/login_slogin.html" {
		t.Fatalf("resolveURL with context = %q, want https://example.com/jwglxt/xtgl/login_slogin.html", got)
	}
	c2, _ := NewClient("https://example.com", 5)
	if got := c2.resolveURL("/xtgl/login_slogin.html"); got != "https://example.com/xtgl/login_slogin.html" {
		t.Fatalf("resolveURL root = %q, want https://example.com/xtgl/login_slogin.html", got)
	}
}

// TestWithContextPath verifies the context-path helper is idempotent and
// correctly appends /jwglxt.
func TestWithContextPath(t *testing.T) {
	u, _ := url.Parse("https://example.com")
	if got := withContextPath(u, "jwglxt").Path; got != "/jwglxt/" {
		t.Fatalf("path = %q, want /jwglxt/", got)
	}
	u2, _ := url.Parse("https://example.com/jwglxt/")
	if got := withContextPath(u2, "jwglxt").Path; got != "/jwglxt/" {
		t.Fatalf("idempotent path = %q, want /jwglxt/", got)
	}
}

// TestLoginContextPathFallback simulates njtech: requests WITHOUT the /jwglxt
// context path return the "系统维护页面" (HTTP 404), while the real endpoints
// live under /jwglxt. The client must PREFER the /jwglxt context path on the
// first attempt, so a base URL without the context path still works without
// wasting a root-path 404 round-trip.
func TestLoginContextPathFallback(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	mod := base64.StdEncoding.EncodeToString(priv.N.Bytes())
	exp := base64.StdEncoding.EncodeToString(big.NewInt(int64(priv.E)).Bytes())

	var mu sync.Mutex
	var paths []string
	record := func(p string) {
		mu.Lock()
		paths = append(paths, p)
		mu.Unlock()
	}

	mux := http.NewServeMux()
	// Catch-all: missing context path → 正方 "系统维护页面".
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		record(r.URL.Path)
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.WriteHeader(404)
		fmt.Fprint(w, `<!DOCTYPE html><html><head><title>系统维护页面</title><link rel="stylesheet" href="/jwglxt/css/animationException.css"></head></html>`)
	})
	// Real login page (GET) + login handler (POST) under /jwglxt.
	mux.HandleFunc("/jwglxt/xtgl/login_slogin.html", func(w http.ResponseWriter, r *http.Request) {
		record(r.URL.Path)
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		if r.Method == http.MethodPost {
			// No p#tips → login success.
			fmt.Fprint(w, `<html><body></body></html>`)
			return
		}
		fmt.Fprint(w, `<html><body><input id="csrftoken" value="tok"></body></html>`)
	})
	mux.HandleFunc("/jwglxt/xtgl/login_getPublicKey.html", func(w http.ResponseWriter, r *http.Request) {
		record(r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"modulus":%q,"exponent":%q}`, mod, exp)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Base URL WITHOUT the context path, as the user's URL secret currently is.
	c, _ := NewClient(srv.URL, 5)
	res := c.Login("student", "secret")
	if res.Code != 1000 {
		t.Fatalf("login code=%d msg=%s", res.Code, res.Msg)
	}
	if !strings.Contains(c.baseURL.Path, "/jwglxt") {
		t.Fatalf("base URL path not updated after fallback: %q", c.baseURL.Path)
	}
	// The first request must already target /jwglxt — no wasted root-path probe.
	if len(paths) < 1 || !strings.HasPrefix(paths[0], "/jwglxt/xtgl/login_slogin.html") {
		t.Fatalf("expected first request to prefer /jwglxt, got %v", paths)
	}
	// A base URL without the context path must NOT probe the bare root path first.
	for _, p := range paths {
		if p == "/xtgl/login_slogin.html" {
			t.Fatalf("unexpected root-path probe; should prefer /jwglxt: %v", paths)
		}
	}
}
