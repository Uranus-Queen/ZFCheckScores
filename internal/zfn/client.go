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

func (c *Client) resolveURL(path string) string {
	return c.baseURL.ResolveReference(&url.URL{Path: path}).String()
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

func (c *Client) loginURL() string  { return c.resolveURL("/xtgl/login_slogin.html") }
func (c *Client) keyURL() string    { return c.resolveURL("/xtgl/login_getPublicKey.html") }
func (c *Client) kaptchaURL() string { return c.resolveURL("/kaptcha") }

// ---------- RSA encryption ----------

// encryptPassword encrypts |password| with RSA PKCS1v15 using the hex-encoded
// modulus and exponent returned by the 正方 public-key endpoint.
func encryptPassword(password, modHex, expHex string) (string, error) {
	if modHex == "" || expHex == "" {
		return "", fmt.Errorf("missing public key (modulus/exponent empty)")
	}
	n := new(big.Int)
	if _, ok := n.SetString(modHex, 16); !ok {
		// Most likely the school's WAF returned a non-RSA response
		// (e.g. captcha challenge) on the public-key endpoint.
		// Include a sample of the actual value to aid diagnosis.
		sample := modHex
		if len(sample) > 80 {
			sample = sample[:40] + "..." + sample[len(sample)-40:]
		}
		return "", fmt.Errorf("invalid modulus hex (len=%d, sample=%q) — server may be returning WAF/captcha response; try Cookie login via COOKIES env", len(modHex), sample)
	}
	e, err := strconv.ParseInt(expHex, 16, 32)
	if err != nil {
		return "", fmt.Errorf("bad exponent %q: %w", expHex, err)
	}
	pub := &rsa.PublicKey{N: n, E: int(e)}
	enc, err := rsa.EncryptPKCS1v15(rand.Reader, pub, []byte(password))
	if err != nil {
		return "", fmt.Errorf("rsa encrypt: %w", err)
	}
	return base64.StdEncoding.EncodeToString(enc), nil
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
	SID    string        `json:"sid"`
	Name   string        `json:"name"`
	Year   int           `json:"year"`
	Term   int           `json:"term"`
	Count  int           `json:"count"`
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
	Code    int
	Msg     string
	Data    *SelectedCoursesData
}

// SelectedCoursesData holds the courses list.
type SelectedCoursesData struct {
	Year    int             `json:"year"`
	Term    int             `json:"term"`
	Count   int             `json:"count"`
	Courses []SelectedCourse `json:"courses"`
}

// ---------- Login ----------

// Login authenticates with username and password.
// Flow: RSA-encrypt first → if server responds "用户名或密码", retry with raw password.
func (c *Client) Login(username, password string) *LoginResult {
	// 1) GET login page → csrf token
	resp, err := c.get("/xtgl/login_slogin.html")
	if err != nil || resp.StatusCode != 200 {
		return &LoginResult{Code: 2333, Msg: "教务系统挂了"}
	}
	body, _ := readBody(resp)
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
// Returns the raw JSON data which includes fields like xm (name), xh (student ID), etc.
func (c *Client) GetUserInfo() (*UserInfoResult, error) {
	resp, err := c.get("/xsxxxggl/xsxxwh_cxCkDgxsxx.html?gnmkdm=N100801")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return &UserInfoResult{Code: 2333, Msg: "教务系统挂了"}, nil
	}
	body, _ := readBody(resp)

	// Check for session expiry (redirect to login page)
	doc, _ := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if strings.TrimSpace(doc.Find("h5").Text()) == "用户登录" {
		return &UserInfoResult{Code: 1006, Msg: "未登录或已过期，请重新登录"}, nil
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return &UserInfoResult{Code: 2333, Msg: "解析个人信息失败"}, nil
	}
	return &UserInfoResult{Code: 1000, Msg: "获取个人信息成功", Data: raw}, nil
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
	if resp.StatusCode != 200 {
		return &GradeResult{Code: 2333, Msg: "教务系统挂了"}, nil
	}
	body, _ := readBody(resp)

	doc, _ := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if strings.TrimSpace(doc.Find("h5").Text()) == "用户登录" {
		return &GradeResult{Code: 1006, Msg: "未登录或已过期，请重新登录"}, nil
	}

	var raw struct {
		Items []struct {
			XH   string `json:"xh"`
			XM   string `json:"xm"`
			KCMC string `json:"kcmc"`
			JSXM string `json:"jsxm"`
			JXBMC string `json:"jxbmc"`
			JXBID string `json:"jxb_id"`
			XF   string `json:"xf"`
			CJ   string `json:"cj"`
			JD   string `json:"jd"`
			TJSJ string `json:"tjsj"`
			TJRXM string `json:"tjrxm"`
			XFJD string `json:"xfjd"`
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
	if resp.StatusCode != 200 {
		return &SelectedCoursesResult{Code: 2333, Msg: "教务系统挂了"}, nil
	}
	body, _ := readBody(resp)

	doc, _ := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if strings.TrimSpace(doc.Find("h5").Text()) == "用户登录" {
		return &SelectedCoursesResult{Code: 1006, Msg: "未登录或已过期，请重新登录"}, nil
	}

	var raw struct {
		Items []struct {
			JXBID  string `json:"jxb_id"`
			JXBMC  string `json:"jxbmc"`
			KCMC   string `json:"kcmc"`
			JSXM   string `json:"jsxm"`
			XNMC   string `json:"xnmc"`
			XQMMC  string `json:"xqmmc"`
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
