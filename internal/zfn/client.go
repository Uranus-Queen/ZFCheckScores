package zfn

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// Client is a 正方教务管理系统 HTTP client. It manages an authenticated
// session (cookie jar) and exposes the key API methods: login, user info,
// grades, and selected courses.
type Client struct {
	baseURL *url.URL
	http    *http.Client
	cookies map[string]string // snapshot after successful login
}

// NewClient creates a Client for the given base URL (e.g. "https://jwgl.njtech.edu.cn").
// timeoutSec is the per-request HTTP timeout; pass 0 for the default (30s).
func NewClient(baseURL string, timeoutSec int) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(baseURL, "/") + "/")
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	jar, _ := cookiejar.New(nil)
	return &Client{
		baseURL: u,
		http: &http.Client{
			Jar:     jar,
			Timeout: time.Duration(timeoutSec) * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return http.ErrUseLastResponse
				}
				return nil
			},
		},
	}, nil
}

// SetCookies pre-populates the session jar with the given name→value map.
// Use this to reuse a browser session (cookie login) instead of username+password.
func (c *Client) SetCookies(cookies map[string]string) {
	if len(cookies) == 0 {
		return
	}
	var cs []*http.Cookie
	for k, v := range cookies {
		cs = append(cs, &http.Cookie{Name: k, Value: v})
	}
	c.http.Jar.SetCookies(c.baseURL, cs)
	c.cookies = cookies
}

// Cookies returns the cookies captured from the last successful login.
func (c *Client) Cookies() map[string]string { return c.cookies }

// ---------- helpers ----------

// resolveURL joins the base URL with a request path while PRESERVING any
// context path the base URL carries (e.g. "/jwglxt").
//
// NOTE: we must NOT use url.URL.ResolveReference with an absolute-path
// reference (path starting with "/"), because ResolveReference replaces the
// entire base path with the reference — dropping the "/jwglxt" context path.
// 正方 deployments are mounted under a context path; stripping it makes every
// request 404 with the "系统维护页面". We concatenate instead, which mirrors
// Python's urljoin(base, "xtgl/...") behavior and keeps the context path.
func (c *Client) resolveURL(path string) string {
	return c.baseURL.String() + strings.TrimPrefix(path, "/")
}

// isMaintenancePage reports whether the body looks like 正方's
// "系统维护页面" (system maintenance page) — returned as HTTP 404 when the
// request path is wrong (e.g. missing the /jwglxt context path).
func isMaintenancePage(b []byte) bool {
	s := string(b)
	return strings.Contains(s, "系统维护页面") || strings.Contains(s, "animationException")
}

// isMaintenancePageMsg is the message-string counterpart of isMaintenancePage,
// used to decide whether to retry login with the /jwglxt context path appended.
func isMaintenancePageMsg(msg string) bool {
	return strings.Contains(msg, "系统维护页面") || strings.Contains(msg, "animationException")
}

// withContextPath returns a copy of u whose path has seg appended as a context
// path (e.g. "/jwglxt"), preserving any existing context path. It is a no-op if
// the segment is already present.
func withContextPath(u *url.URL, seg string) *url.URL {
	cp := *u
	if strings.Contains(cp.Path, "/"+seg) {
		return &cp
	}
	if cp.Path == "" || cp.Path == "/" {
		cp.Path = "/" + seg + "/"
	} else {
		cp.Path = strings.TrimRight(cp.Path, "/") + "/" + seg + "/"
	}
	return &cp
}

func defaultHeaders(referer string) map[string]string {
	return map[string]string{
		"User-Agent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/56.0.2924.87 Safari/537.36",
		"Accept":     "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3",
		"Referer":    referer,
	}
}

func (c *Client) get(path string) (*http.Response, error) {
	req, _ := http.NewRequest("GET", c.resolveURL(path), nil)
	for k, v := range defaultHeaders(c.loginURL()) {
		req.Header.Set(k, v)
	}
	return c.http.Do(req)
}

func (c *Client) postForm(path string, data url.Values) (*http.Response, error) {
	body := strings.NewReader(data.Encode())
	req, _ := http.NewRequest("POST", c.resolveURL(path), body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range defaultHeaders(c.loginURL()) {
		req.Header.Set(k, v)
	}
	return c.http.Do(req)
}

// readBody reads the full response, replaces it with a fresh reader,
// and returns the bytes. This allows subsequent goquery parsing.
func readBody(r *http.Response) ([]byte, error) {
	b, err := io.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		return nil, err
	}
	r.Body = io.NopCloser(bytes.NewReader(b))
	return b, nil
}

// ---------- URLs ----------

func (c *Client) loginURL() string   { return c.resolveURL("/xtgl/login_slogin.html") }
func (c *Client) keyURL() string     { return c.resolveURL("/xtgl/login_getPublicKey.html") }
func (c *Client) kaptchaURL() string { return c.resolveURL("/kaptcha") }

// ---------- RSA encryption ----------

// encryptPassword encrypts |password| with RSA PKCS1v15 using the modulus
// and exponent returned by the 正方 public-key endpoint.
//
// The endpoint can return the values as either a hex string or a
// base64-encoded big-endian byte array (e.g. JSEncrypt-style "AQAB" for 65537).
// This function tries hex first, then falls back to base64.
func encryptPassword(password, modStr, expStr string) (string, error) {
	if modStr == "" || expStr == "" {
		return "", fmt.Errorf("missing public key (modulus/exponent empty)")
	}
	n, err := parseBigInt(modStr)
	if err != nil {
		return "", fmt.Errorf("invalid modulus (not hex or base64): %w", err)
	}
	e, err := parseExponent(expStr)
	if err != nil {
		return "", fmt.Errorf("invalid exponent: %w", err)
	}
	pub := &rsa.PublicKey{N: n, E: e}
	enc, err := rsa.EncryptPKCS1v15(rand.Reader, pub, []byte(password))
	if err != nil {
		return "", fmt.Errorf("rsa encrypt: %w", err)
	}
	return base64.StdEncoding.EncodeToString(enc), nil
}

// parseBigInt parses s as either a hex string or a base64-encoded
// big-endian byte array.
func parseBigInt(s string) (*big.Int, error) {
	if x, ok := new(big.Int).SetString(s, 16); ok {
		return x, nil
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	if len(b) == 0 {
		return nil, fmt.Errorf("empty modulus")
	}
	return new(big.Int).SetBytes(b), nil
}

// parseExponent parses the RSA public exponent. Some servers return it as
// a hex string ("10001"), others as base64 ("AQAB").
func parseExponent(s string) (int, error) {
	if e, err := strconv.ParseInt(s, 16, 32); err == nil {
		return int(e), nil
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return 0, fmt.Errorf("not hex or base64: %w", err)
	}
	if len(b) == 0 {
		return 0, fmt.Errorf("empty exponent")
	}
	e := 0
	for _, by := range b {
		e = e<<8 | int(by)
	}
	return e, nil
}

// Backoff sleeps with exponential delay: 1s, 2s, 4s, 8s, ... (capped at maxSec).
// attempt is 1-indexed: attempt=1 → 1s, attempt=2 → 2s, etc.
func Backoff(attempt, maxSec int) {
	if attempt < 1 {
		attempt = 1
	}
	d := time.Duration(1<<uint(attempt-1)) * time.Second
	if maxSec <= 0 {
		maxSec = 10
	}
	if max := time.Duration(maxSec) * time.Second; d > max {
		d = max
	}
	time.Sleep(d)
}

// ---------- result types ----------

// LoginResult is the structured result of a Login attempt.
type LoginResult struct {
	Code int
	Msg  string
	// Data carries extra fields (e.g. cookies on success, kaptcha info on 1001).
	Data map[string]interface{}
}

// GradeResult holds parsed grade data from get_grade API.
type GradeResult struct {
	Code    int
	Msg     string
	Data    *GradeData
	RawJSON []byte // original response for custom processing
}

// GradeData holds the top-level grade response.
type GradeData struct {
	SID     string        `json:"sid"`
	Name    string        `json:"name"`
	Year    int           `json:"year"`
	Term    int           `json:"term"`
	Count   int           `json:"count"`
	Courses []GradeCourse `json:"courses"`
}

// GradeCourse is a single course grade entry.
type GradeCourse struct {
	Title            string `json:"title"`
	Teacher          string `json:"teacher"`
	ClassName        string `json:"class_name"`
	ClassID          string `json:"class_id"`
	Credit           string `json:"credit"`
	Grade            string `json:"grade"`
	GradePoint       string `json:"grade_point"`
	SubmissionTime   string `json:"submission_time"`
	Submitter        string `json:"name_of_submitter"`
	XFJD             string `json:"xfjd"`
	PercentageGrades string `json:"percentage_grades"`
}

// UserInfoResult holds user profile data.
type UserInfoResult struct {
	Code int
	Msg  string
	Data map[string]interface{} // raw JSON fields from 正方
}

// SelectedCourse is a single course in the selected-courses list.
type SelectedCourse struct {
	ClassID        string `json:"class_id"`
	ClassName      string `json:"class_name"`
	Title          string `json:"title"`
	Teacher        string `json:"teacher"`
	CourseYear     string `json:"course_year"`
	CourseSemester string `json:"course_semester"`
}

// SelectedCoursesResult holds the selected-courses response.
type SelectedCoursesResult struct {
	Code int
	Msg  string
	Data *SelectedCoursesData
}

// SelectedCoursesData holds the courses list.
type SelectedCoursesData struct {
	Year    int              `json:"year"`
	Term    int              `json:"term"`
	Count   int              `json:"count"`
	Courses []SelectedCourse `json:"courses"`
}

// ---------- Login ----------

// Login authenticates with username and password.
// Flow: RSA-encrypt first → if server responds "用户名或密码", retry with raw password.
//
// 正方 deployments are typically mounted under a context path such as /jwglxt.
// If the configured base URL returns the "系统维护页面" (a 404-style maintenance
// page), we retry once with /jwglxt appended so that a base URL without the
// context path still works out of the box.
func (c *Client) Login(username, password string) *LoginResult {
	res := c.loginAttempt(username, password)
	if res != nil && res.Code == 2333 && isMaintenancePageMsg(res.Msg) {
		c.baseURL = withContextPath(c.baseURL, "jwglxt")
		res = c.loginAttempt(username, password)
	}
	return res
}

// loginAttempt performs a single username/password login using the current
// base URL. It returns a non-1000 result on any failure (the caller may retry
// with a corrected context path).
func (c *Client) loginAttempt(username, password string) *LoginResult {
	// 1) GET login page → csrf token
	resp, err := c.get("/xtgl/login_slogin.html")
	if err != nil {
		return &LoginResult{Code: 2333, Msg: "教务系统挂了（登录页无法访问）"}
	}
	body, _ := readBody(resp)
	// 正方 returns the "系统维护页面" as HTTP 404 when the request is missing
	// the context path (e.g. /jwglxt). Detect this BEFORE the status check so
	// the caller can retry with the context path appended.
	if isMaintenancePage(body) {
		return &LoginResult{Code: 2333, Msg: "访问教务系统返回『系统维护页面』，很可能 URL 缺少 /jwglxt 上下文路径（请确认 URL 为 https://jwgl.njtech.edu.cn/jwglxt）"}
	}
	if resp.StatusCode != 200 {
		return &LoginResult{Code: 2333, Msg: "教务系统挂了（登录页 HTTP " + strconv.Itoa(resp.StatusCode) + "）"}
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return &LoginResult{Code: 2333, Msg: "解析登录页失败"}
	}
	csrf, _ := doc.Find("#csrftoken").Attr("value")

	// 2) GET public key
	pkResp, err := c.get("/xtgl/login_getPublicKey.html")
	if err != nil {
		return &LoginResult{Code: 2333, Msg: "获取公钥失败"}
	}
	defer pkResp.Body.Close()
	var pk struct {
		Modulus  string `json:"modulus"`
		Exponent string `json:"exponent"`
	}
	if err := json.NewDecoder(pkResp.Body).Decode(&pk); err != nil {
		// Re-fetch the public key to capture the body for diagnosis.
		var body []byte
		if r2, _ := c.get("/xtgl/login_getPublicKey.html"); r2 != nil {
			body, _ = io.ReadAll(r2.Body)
			r2.Body.Close()
		}
		sample := string(body)
		if len(sample) > 200 {
			sample = sample[:200] + "..."
		}
		return &LoginResult{Code: 2333, Msg: fmt.Sprintf("公钥响应不是合法 JSON: %v; body=%q", err, sample)}
	}

	// 3) Captcha required?
	if doc.Find("input#yzm").Length() > 0 {
		kResp, _ := c.get("/kaptcha")
		kBytes, _ := io.ReadAll(kResp.Body)
		kResp.Body.Close()
		return &LoginResult{
			Code: 1001,
			Msg:  "获取验证码成功",
			Data: map[string]interface{}{
				"sid":         username,
				"csrf_token":  csrf,
				"password":    password,
				"modulus":     pk.Modulus,
				"exponent":    pk.Exponent,
				"kaptcha_pic": base64.StdEncoding.EncodeToString(kBytes),
				"timestamp":   float64(time.Now().Unix()),
			},
		}
	}

	// 4) RSA encrypt → POST login
	enc, err := encryptPassword(password, pk.Modulus, pk.Exponent)
	if err != nil {
		return &LoginResult{Code: 2333, Msg: "加密密码失败: " + err.Error()}
	}
	result := c.postLogin(csrf, username, enc, password)
	if result != nil {
		return result
	}

	// Success: capture cookies from jar
	c.cookies = c.jarCookies()
	return &LoginResult{Code: 1000, Msg: "登录成功", Data: map[string]interface{}{"cookies": c.cookies}}
}

// postLogin POSTs login form data. If the encrypted password fails with
// "用户名或密码", retries with the raw password (matching zfn_api.py behavior).
func (c *Client) postLogin(csrf, username, encrypted, rawPassword string) *LoginResult {
	data := url.Values{
		"csrftoken": {csrf},
		"yhm":       {username},
		"mm":        {encrypted},
	}
	resp, err := c.postForm("/xtgl/login_slogin.html", data)
	if err != nil {
		return &LoginResult{Code: 2333, Msg: "登录请求失败"}
	}
	body, _ := readBody(resp)
	if isMaintenancePage(body) {
		return &LoginResult{Code: 2333, Msg: "登录请求返回『系统维护页面』，很可能 URL 缺少 /jwglxt 上下文路径（请确认 URL 为 https://jwgl.njtech.edu.cn/jwglxt）"}
	}
	doc, _ := goquery.NewDocumentFromReader(bytes.NewReader(body))
	tips := strings.TrimSpace(doc.Find("p#tips").Text())

	if tips != "" {
		if strings.Contains(tips, "用户名或密码") {
			// Retry with raw password
			data.Set("mm", rawPassword)
			resp2, err := c.postForm("/xtgl/login_slogin.html", data)
			if err != nil {
				return &LoginResult{Code: 2333, Msg: "登录重试请求失败"}
			}
			body2, _ := readBody(resp2)
			doc2, _ := goquery.NewDocumentFromReader(bytes.NewReader(body2))
			tips2 := strings.TrimSpace(doc2.Find("p#tips").Text())
			if tips2 != "" {
				if strings.Contains(tips2, "用户名或密码") {
					return &LoginResult{Code: 1002, Msg: "用户名或密码不正确"}
				}
				return &LoginResult{Code: 998, Msg: tips2}
			}
			// Raw password succeeded
			return nil
		}
		return &LoginResult{Code: 998, Msg: tips}
	}
	// Encrypted succeeded
	return nil
}

func (c *Client) jarCookies() map[string]string {
	m := make(map[string]string)
	for _, ck := range c.http.Jar.Cookies(c.baseURL) {
		m[ck.Name] = ck.Value
	}
	return m
}

// ---------- User Info ----------

// GetUserInfo fetches personal information from the 正方 system.
// It first tries the JSON endpoint used by zfn_api; if that returns an HTML
// page, empty JSON, or a non-200 response, it falls back to parsing the
// student info maintenance HTML page (the _get_info behaviour of zfn_api).
func (c *Client) GetUserInfo() (*UserInfoResult, error) {
	resp, err := c.get("/xsxxxggl/xsxxwh_cxCkDgxsxx.html?gnmkdm=N100801")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := readBody(resp)
	sample := strings.TrimSpace(string(body))
	if len(sample) > 160 {
		sample = sample[:160] + "..."
	}

	// Check for session expiry (redirect to login page)
	doc, _ := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if strings.TrimSpace(doc.Find("h5").Text()) == "用户登录" {
		return &UserInfoResult{Code: 1006, Msg: "未登录或已过期，请重新登录"}, nil
	}
	// WAF / captcha challenge page (session rejected after login).
	if doc.Find("input#yzm").Length() > 0 {
		return &UserInfoResult{Code: 1001, Msg: "会话被 WAF 拦截（返回验证码页）"}, nil
	}

	// If the HTTP status is not 200, try the HTML fallback before giving up.
	if resp.StatusCode != 200 {
		fallback, fbErr := c.getUserInfoFromHTML()
		if fallback != nil && fallback.Code == 1000 {
			return fallback, nil
		}
		msg := fmt.Sprintf("教务系统挂了 (HTTP %d)", resp.StatusCode)
		if isMaintenancePage(body) {
			msg += "：返回『系统维护页面』，很可能 URL 缺少 /jwglxt 上下文路径（请确认 URL 为 https://jwgl.njtech.edu.cn/jwglxt）"
		} else {
			msg += fmt.Sprintf(" body=%q", sample)
		}
		msg += fmt.Sprintf("；备用 HTML 解析也失败: %v", fbErr)
		return &UserInfoResult{Code: 2333, Msg: msg}, nil
	}

	// Some deployments return an HTML page here even with HTTP 200.
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(sample, "<") || strings.Contains(ct, "text/html") {
		fallback, fbErr := c.getUserInfoFromHTML()
		if fallback != nil && fallback.Code == 1000 {
			return fallback, nil
		}
		return &UserInfoResult{Code: 2333, Msg: fmt.Sprintf("个人信息接口返回 HTML 而非 JSON; 备用 HTML 解析也失败: %v", fbErr)}, nil
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		fallback, fbErr := c.getUserInfoFromHTML()
		if fallback != nil && fallback.Code == 1000 {
			return fallback, nil
		}
		return &UserInfoResult{Code: 2333, Msg: fmt.Sprintf("解析个人信息失败（响应非 JSON）: %q; 备用 HTML 解析也失败: %v", sample, fbErr)}, nil
	}

	// Empty JSON object -> fallback to HTML page.
	if len(raw) == 0 {
		fallback, fbErr := c.getUserInfoFromHTML()
		if fallback != nil && fallback.Code == 1000 {
			return fallback, nil
		}
		return &UserInfoResult{Code: 1005, Msg: fmt.Sprintf("个人信息接口返回空 JSON; 备用 HTML 解析也失败: %v", fbErr)}, nil
	}

	return &UserInfoResult{Code: 1000, Msg: "获取个人信息成功", Data: raw}, nil
}

// getUserInfoFromHTML is the fallback used by zfn_api's _get_info().
// It parses the student information maintenance HTML page and returns the
// same field names as the JSON endpoint.
func (c *Client) getUserInfoFromHTML() (*UserInfoResult, error) {
	resp, err := c.get("/xsxxxggl/xsgrxxwh_cxXsgrxx.html?gnmkdm=N100801")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return &UserInfoResult{Code: 2333, Msg: fmt.Sprintf("备用个人信息页 HTTP %d", resp.StatusCode)}, nil
	}

	body, _ := readBody(resp)
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return &UserInfoResult{Code: 2333, Msg: "解析备用个人信息页失败"}, nil
	}
	if strings.TrimSpace(doc.Find("h5").Text()) == "用户登录" {
		return &UserInfoResult{Code: 1006, Msg: "未登录或已过期，请重新登录"}, nil
	}

	pending := make(map[string]string)
	// Student basic info and other sections use div.col-sm-6 / div.col-sm-4.
	doc.Find("div.col-sm-6 div.form-group, div.col-sm-4 div.form-group").Each(func(_ int, s *goquery.Selection) {
		key := strings.TrimSpace(s.Find("label.col-sm-4.control-label").Text())
		value := strings.TrimSpace(s.Find("div.col-sm-8 p.form-control-static").Text())
		if key != "" {
			pending[key] = value
		}
	})

	if pending["学号："] == "" {
		// The page may expose college/class on a separate tab; try to backfill.
		c.backfillCollegeInfo(pending)
		if pending["学号："] == "" {
			return &UserInfoResult{Code: 1014, Msg: "备用个人信息页未解析到学号，可能已毕业或无数据"}, nil
		}
	}

	result := map[string]interface{}{
		"sid":          pending["学号："],
		"name":         pending["姓名："],
		"class_name":   firstNonEmpty(pending["班级名称："], pending["班级："]),
		"college_name": firstNonEmpty(pending["学院名称："], pending["学院："]),
		"major_name":   firstNonEmpty(pending["专业名称："], pending["专业："]),
	}

	// If college/class are still missing, try the student-ID-card replacement entry.
	if result["class_name"] == "" || result["college_name"] == "" {
		c.backfillCollegeInfo(pending)
		if result["class_name"] == "" {
			result["class_name"] = firstNonEmpty(pending["班级："], pending["班级名称："])
		}
		if result["college_name"] == "" {
			result["college_name"] = firstNonEmpty(pending["学院："], pending["学院名称："])
		}
		if result["major_name"] == "" {
			result["major_name"] = firstNonEmpty(pending["专业："], pending["专业名称："])
		}
	}

	return &UserInfoResult{Code: 1000, Msg: "获取个人信息成功（HTML 备用）", Data: result}, nil
}

// backfillCollegeInfo mirrors zfn_api's behaviour of fetching the student-ID
// replacement application page when the main info page lacks college/major/class.
func (c *Client) backfillCollegeInfo(pending map[string]string) {
	resp, err := c.postForm("/xszbbgl/xszbbgl_cxXszbbsqIndex.html?doType=details&gnmkdm=N106005", url.Values{
		"offDetails": {"1"},
		"gnmkdm":     {"N106005"},
		"czdmKey":    {"00"},
	})
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return
	}
	body, _ := readBody(resp)
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return
	}
	if strings.TrimSpace(doc.Find("p.error_title").Text()) == "无功能权限，" {
		return
	}
	doc.Find("div.col-sm-6 div.form-group").Each(func(_ int, s *goquery.Selection) {
		key := strings.TrimSpace(s.Find("label.col-sm-4.control-label").Text())
		if key != "" && !strings.HasSuffix(key, "：") {
			key = key + "："
		}
		value := strings.TrimSpace(s.Find("div.col-sm-8 label.control-label").Text())
		if key != "" {
			pending[key] = value
		}
	})
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// ---------- Grade ----------

// GetGrade fetches grade data for the specified academic year and term.
// year=0 / term=0 means "all" (no filter). term is in "logical" form: 1=第一学期, 2=第二学期.
func (c *Client) GetGrade(year, term int) (*GradeResult, error) {
	// Convert logical term to 正方 encoding: 1→3, 2→12, 0→""
	xqm := 0
	if term == 1 {
		xqm = 3
	} else if term == 2 {
		xqm = 12
	}
	form := url.Values{
		"xnm":                    {strconv.Itoa(year)},
		"xqm":                    {strconv.Itoa(xqm)},
		"_search":                {"false"},
		"nd":                     {strconv.FormatInt(time.Now().UnixMilli(), 10)},
		"queryModel.showCount":   {"100"},
		"queryModel.currentPage": {"1"},
		"queryModel.sortName":    {""},
		"queryModel.sortOrder":   {"asc"},
		"time":                   {"0"},
	}
	if year == 0 {
		form.Set("xnm", "")
	}
	if xqm == 0 {
		form.Set("xqm", "")
	}

	resp, err := c.postForm("/cjcx/cjcx_cxXsgrcj.html?doType=query&gnmkdm=N305005", form)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := readBody(resp)
	if resp.StatusCode != 200 {
		msg := "教务系统挂了"
		if isMaintenancePage(body) {
			msg += "：返回『系统维护页面』，很可能 URL 缺少 /jwglxt 上下文路径（请确认 URL 为 https://jwgl.njtech.edu.cn/jwglxt）"
		}
		return &GradeResult{Code: 2333, Msg: msg}, nil
	}

	doc, _ := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if strings.TrimSpace(doc.Find("h5").Text()) == "用户登录" {
		return &GradeResult{Code: 1006, Msg: "未登录或已过期，请重新登录"}, nil
	}

	var raw struct {
		Items []struct {
			XH    string `json:"xh"`
			XM    string `json:"xm"`
			KCMC  string `json:"kcmc"`
			JSXM  string `json:"jsxm"`
			JXBMC string `json:"jxbmc"`
			JXBID string `json:"jxb_id"`
			XF    string `json:"xf"`
			CJ    string `json:"cj"`
			JD    string `json:"jd"`
			TJSJ  string `json:"tjsj"`
			TJRXM string `json:"tjrxm"`
			XFJD  string `json:"xfjd"`
			BFZCJ string `json:"bfzcj"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return &GradeResult{Code: 2333, Msg: "解析成绩失败"}, nil
	}
	if len(raw.Items) == 0 {
		return &GradeResult{Code: 1005, Msg: "获取内容为空"}, nil
	}

	gd := &GradeData{
		SID:   raw.Items[0].XH,
		Name:  raw.Items[0].XM,
		Year:  year,
		Term:  term,
		Count: len(raw.Items),
	}
	for _, it := range raw.Items {
		gd.Courses = append(gd.Courses, GradeCourse{
			Title:            it.KCMC,
			Teacher:          it.JSXM,
			ClassName:        it.JXBMC,
			ClassID:          it.JXBID,
			Credit:           it.XF,
			Grade:            it.CJ,
			GradePoint:       it.JD,
			SubmissionTime:   it.TJSJ,
			Submitter:        it.TJRXM,
			XFJD:             it.XFJD,
			PercentageGrades: it.BFZCJ,
		})
	}
	return &GradeResult{Code: 1000, Msg: "获取成绩成功", Data: gd, RawJSON: body}, nil
}

// ---------- Selected Courses ----------

// GetSelectedCourses fetches enrolled courses for the given year/term.
// year=0 / term=0 means "all".
func (c *Client) GetSelectedCourses(year, term int) (*SelectedCoursesResult, error) {
	xqm := 0
	if term == 1 {
		xqm = 3
	} else if term == 2 {
		xqm = 12
	}
	form := url.Values{
		"xnm":                    {strconv.Itoa(year)},
		"xqm":                    {strconv.Itoa(xqm)},
		"_search":                {"false"},
		"queryModel.showCount":   {"5000"},
		"queryModel.currentPage": {"1"},
		"queryModel.sortName":    {""},
		"queryModel.sortOrder":   {"asc"},
		"time":                   {"1"},
	}
	if year == 0 {
		form.Set("xnm", "")
	}
	if xqm == 0 {
		form.Set("xqm", "")
	}

	resp, err := c.postForm("/xsxxxggl/xsxxwh_cxXsxkxx.html?gnmkdm=N100801", form)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := readBody(resp)
	if resp.StatusCode != 200 {
		msg := "教务系统挂了"
		if isMaintenancePage(body) {
			msg += "：返回『系统维护页面』，很可能 URL 缺少 /jwglxt 上下文路径（请确认 URL 为 https://jwgl.njtech.edu.cn/jwglxt）"
		}
		return &SelectedCoursesResult{Code: 2333, Msg: msg}, nil
	}

	doc, _ := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if strings.TrimSpace(doc.Find("h5").Text()) == "用户登录" {
		return &SelectedCoursesResult{Code: 1006, Msg: "未登录或已过期，请重新登录"}, nil
	}

	var raw struct {
		Items []struct {
			JXBID string `json:"jxb_id"`
			JXBMC string `json:"jxbmc"`
			KCMC  string `json:"kcmc"`
			JSXM  string `json:"jsxm"`
			XNMC  string `json:"xnmc"`
			XQMMC string `json:"xqmmc"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return &SelectedCoursesResult{Code: 2333, Msg: "解析已选课程失败"}, nil
	}

	sd := &SelectedCoursesData{
		Year:  year,
		Term:  term,
		Count: len(raw.Items),
	}
	for _, it := range raw.Items {
		sd.Courses = append(sd.Courses, SelectedCourse{
			ClassID:        it.JXBID,
			ClassName:      it.JXBMC,
			Title:          it.KCMC,
			Teacher:        it.JSXM,
			CourseYear:     it.XNMC,
			CourseSemester: it.XQMMC,
		})
	}
	return &SelectedCoursesResult{Code: 1000, Msg: "获取已选课程成功", Data: sd}, nil
}

// ---------- GPA ----------

// GetGPA fetches GPA from the 正方 system.
func (c *Client) GetGPA() (*UserInfoResult, error) {
	// GPA is typically available via the user info or a separate endpoint.
	// For now, reuse GetUserInfo which includes GPA-related fields in some versions.
	return c.GetUserInfo()
}
