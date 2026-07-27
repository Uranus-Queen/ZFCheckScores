package main

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"zfcheckscores/internal/config"
	"zfcheckscores/internal/push"
	"zfcheckscores/internal/semester"
	"zfcheckscores/internal/store"
	"zfcheckscores/internal/zfn"
)

const (
	pushTitle = "正方教务管理系统成绩推送"
	copyright = "Copyright © 2026 IKAROS. All rights reserved."
	divider   = "══════════════════════════"
	subdiv    = "──────────────────────────"

	firstRunMsg = "你的程序运行成功\n从现在开始,程序将会每隔 30 分钟自动检测一次成绩是否有更新\n若有更新,将通过微信推送及时通知你"
)

// The Liquid Glass CSS, JS runtime, and the HTML template now live in
// internal/push/templates and are embedded + rendered by internal/push/render.go
// (RenderGradeCard). This keeps the presentation layer out of the main logic
// with zero new dependencies. User data is HTML-escaped by html/template; the
// CSS/JS are injected as trusted embedded assets.

func main() {
	cfg := config.Load()
	st := store.New(store.DataDir)
	if err := st.EnsureDir(); err != nil {
		log.Printf("warn: data dir: %v", err)
	}

	// ── 1. Login ──
	client, err := zfn.NewClient(cfg.URL, cfg.TimeoutSec)
	if err != nil {
		log.Fatalf("client: %v", err)
	}
	if len(cfg.Cookies) > 0 {
		client.SetCookies(cfg.Cookies)
	} else {
		res := client.Login(cfg.Username, cfg.Password)
		if res.Code != 1000 {
			log.Fatalf("login: %s (code=%d)", res.Msg, res.Code)
		}
	}

	// ── 2. User info + GPA (all semesters) ──
	ui, allGrades := fetchUserInfo(client)

	// ── 3. Selected courses (single fetch, reused below) ──
	selRes, _ := client.GetSelectedCourses(0, 0)
	var selData *zfn.SelectedCoursesData
	if selRes != nil && selRes.Code == 1000 {
		selData = selRes.Data
	}

	// ── 4. Semester (uses already-fetched selected courses) ──
	sem := semester.ResolveFromData(selData)
	fmt.Printf("当前学期：%s\n", sem.Label())

	// ── 5. First-run detection ──
	infoHash := store.MD5(fmt.Sprintf("%s/%s/%s", ui.Name, ui.SID, ui.Class))
	firstRun := infoHash != "" && st.IsFirstRun(infoHash)

	// ── 6. Fetch current-semester grades ──
	runs := 1
	if firstRun {
		runs = 2
	}
	var curGrades *zfn.GradeData
	gradeEmpty, gradeErr := false, false

	for i := 0; i < runs; i++ {
		_ = st.SnapshotGrade()
		gr, _ := retryGrade(client, sem.Year, sem.Term)
		if gr == nil || gr.Data == nil || len(gr.Data.Courses) == 0 {
			if gr != nil && gr.Code == 1005 {
				gradeEmpty = true
			} else if gr == nil || gr.Code != 1000 {
				gradeErr = true
			}
			continue
		}
		curGrades = gr.Data
		text := rawGradeText(gr.Data)
		if !gradeErr && text != "" {
			if err := st.WriteGrade(text); err != nil {
				log.Printf("warn: write grade: %v", err)
			}
		}
	}

	// ── 7. GPA (cumulative, all semesters) ──
	gpa, pctGPA := "0.00", "0.00"
	if allGrades != nil && !gradeEmpty && !gradeErr {
		gpa, pctGPA = computeGPA(allGrades.Data)
	}

	// ── 8. Selected courses filtering (unpublished) ──
	selText := selectedCoursesText(selData, curGrades)

	// ── 9. Build push pages ──
	courses := gradeList(curGrades)
	pending := pendingCourses(selData, curGrades)
	fullPage := buildPage("📊 成绩已更新", ui.Name, ui.SID, sem.Label(), courses, gpa, pctGPA, selText, cfg)
	fullHTML, err := push.RenderGradeCard(push.GradeCardData{
		Title:     pushTitle,
		SemLabel:  sem.Label(),
		Courses:   courses,
		GPA:       gpa,
		PctGPA:    pctGPA,
		Pending:   pending,
		FirstRun:  firstRun,
		Copyright: copyright,
	})
	if err != nil {
		log.Fatalf("render grade card: %v", err)
	}

	// ── 10. Decision ──
	gc, _ := st.GradeContent()
	ogc, _ := st.OldGradeContent()
	var logLines []string

	switch {
	case gradeErr:
		logLines = append(logLines, "获取成绩时出错，运行失败")
	case firstRun:
		logLines = append(logLines, firstRunMsg)
		logLines = append(logLines, pushAndReport(cfg, pushTitle, fullHTML))
	case gc != ogc || cfg.ForcePush:
		logLines = append(logLines, "成绩已更新")
		logLines = append(logLines, pushAndReport(cfg, pushTitle, fullHTML))
	default:
		logLines = append(logLines, "成绩未更新")
		if last := lastSubmission(curGrades); last != "" {
			logLines = append(logLines, "最近一次: "+last)
		}
	}

	// 个人信息缺失不再阻断推送：只要成绩接口可用就照常推送（与 Python 原版
	// main.py 行为一致——原版在 info 为空时也只是记录错误、run_count 置 1，
	// 但仍会继续拉成绩并推送）。仅把失败原因写入日志，便于在 Actions 里区分
	// 是 WAF 拦截、IP 风控还是会话过期，而不是让整次运行空手而归。
	if ui.Name == "" {
		note := "个人信息为空（成绩推送仍照常）"
		if lastUserInfoErr != "" {
			note += "，最后一次错误: " + lastUserInfoErr
		}
		logLines = append(logLines, note)
	}

	// ── 11. Persist ──
	if firstRun && infoHash != "" && st.IsFirstRun(infoHash) {
		if err := st.SaveInfo(infoHash); err != nil {
			log.Printf("warn: save info: %v", err)
		}
	}

	// ── 12. Report ──
	runLog := strings.Join(logLines, "\n")
	if runLog == "" {
		return
	}
	// Plain-text card for local / console readability (Showdoc receives HTML).
	fmt.Println(fullPage)
	fmt.Println(runLog)
	if cfg.GitHubActions && cfg.StepSummary != "" {
		writeGitHubSummary(runLog, cfg)
	}
}

// pushAndReport calls Showdoc and returns the response for the run log.
func pushAndReport(cfg *config.Config, title, content string) string {
	resp, err := push.Showdoc(cfg.Token, title, content)
	if err != nil {
		return "push error: " + err.Error()
	}
	return resp
}

// ──────────────────────────────── data types ────────────────────────────────

type userInfo struct {
	Name, SID, Class string
}

// lastUserInfoErr keeps the最后一次个人信息获取失败的原因, surfaced in the run
// log so the GitHub Actions summary shows *why* instead of a bare
// "个人信息为空" (e.g. WAF challenge, session expiry, gateway error).
var lastUserInfoErr string

// ────────────────────────────── fetch helpers ───────────────────────────────

func fetchUserInfo(c *zfn.Client) (*userInfo, *zfn.GradeResult) {
	var result *zfn.UserInfoResult
	for i := 1; i <= 5; i++ {
		r, err := c.GetUserInfo()
		if err != nil {
			log.Printf("warn: user info attempt %d/5: %v", i, err)
			lastUserInfoErr = err.Error()
			zfn.Backoff(i, 10)
			continue
		}
		if r.Code == 1000 && r.Data != nil {
			result = r
			break
		}
		log.Printf("warn: user info attempt %d/5: code=%d msg=%s", i, r.Code, r.Msg)
		lastUserInfoErr = fmt.Sprintf("code=%d %s", r.Code, r.Msg)
		zfn.Backoff(i, 10)
	}
	if result == nil || result.Data == nil {
		return &userInfo{}, nil
	}
	ui := &userInfo{
		Name:  strVal(result.Data, "name", "xm"),
		SID:   strVal(result.Data, "sid", "xh"),
		Class: strVal(result.Data, "class_name", "bh_id", "xjztdm"),
	}
	gr, _ := retryGrade(c, 0, 0) // all semesters for cumulative GPA
	return ui, gr
}

func retryGrade(c *zfn.Client, year, term int) (*zfn.GradeResult, error) {
	var last *zfn.GradeResult
	for i := 1; i <= 5; i++ {
		gr, err := c.GetGrade(year, term)
		if err != nil {
			last = gr
			zfn.Backoff(i, 10)
			continue
		}
		last = gr
		if gr.Code == 1000 || gr.Code == 1005 {
			return gr, nil
		}
		zfn.Backoff(i, 10)
	}
	return last, nil
}

// ─────────────────────────── grade formatting ───────────────────────────────

// rawGradeText builds a deterministic text representation for MD5 comparison
// (keeps the legacy format to stay compatible with existing data/ files).
func rawGradeText(gd *zfn.GradeData) string {
	sorted := sortCourses(gd.Courses)
	if len(sorted) > 8 {
		sorted = sorted[:8]
	}
	var sb strings.Builder
	sb.WriteString("------\n成绩信息：")
	for _, c := range sorted {
		title := normalizeBrackets(c.Title)
		gradeStr := c.Grade
		if _, err := strconv.ParseFloat(c.Grade, 64); err != nil {
			gradeStr = c.Grade + " (" + c.PercentageGrades + ")"
		}
		sb.WriteString(fmt.Sprintf("\n教学班ID：%s\n课程名称：%s\n任课教师：%s\n成绩：%s\n提交时间：%s\n提交人姓名：%s\n------",
			c.ClassID, title, c.Teacher, gradeStr, c.SubmissionTime, c.Submitter))
	}
	return sb.String()
}

// gradeList returns compact grade rows for the push page (current semester only).
// It precomputes the display meta line and the Liquid Glass score class so the
// template stays free of presentation logic.
func gradeList(gd *zfn.GradeData) []push.Course {
	if gd == nil || len(gd.Courses) == 0 {
		return nil
	}
	var lines []push.Course
	for _, c := range sortCourses(gd.Courses) {
		title := normalizeBrackets(c.Title)
		gradeStr := c.Grade
		if _, err := strconv.ParseFloat(c.Grade, 64); err != nil {
			gradeStr = c.Grade + " (" + c.PercentageGrades + ")"
		}
		meta := c.Teacher
		if meta != "" && c.SubmissionTime != "" {
			meta = meta + " · " + shortTime(c.SubmissionTime)
		} else if c.SubmissionTime != "" {
			meta = shortTime(c.SubmissionTime)
		}
		lines = append(lines, push.Course{
			Course:     title,
			Grade:      gradeStr,
			Teacher:    c.Teacher,
			Time:       shortTime(c.SubmissionTime),
			Meta:       meta,
			ScoreClass: scoreClass(gradeStr),
		})
	}
	return lines
}

// shortTime extracts "MM-DD HH:MM" from a timestamp like "2024-01-15 10:30:00".
// Returns the original string if it doesn't match the expected format.
func shortTime(s string) string {
	if len(s) < 16 {
		return s
	}
	if s[4] != '-' || s[7] != '-' || s[10] != ' ' {
		return s
	}
	return s[5:16]
}

// normalizeBrackets converts full-width Chinese brackets to ASCII.
func normalizeBrackets(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "（", "("), "）", ")")
}

func sortCourses(courses []zfn.GradeCourse) []zfn.GradeCourse {
	sorted := make([]zfn.GradeCourse, len(courses))
	copy(sorted, courses)
	sort.Slice(sorted, func(i, j int) bool {
		a, b := sorted[i].SubmissionTime, sorted[j].SubmissionTime
		if a == "" {
			a = "1970-01-01 00:00:00"
		}
		if b == "" {
			b = "1970-01-01 00:00:00"
		}
		return a > b
	})
	return sorted
}

// ───────────────────────────── GPA ──────────────────────────────────────────

func computeGPA(gd *zfn.GradeData) (gpa, pct string) {
	if gd == nil {
		return "0.00", "0.00"
	}
	var credit, xfjd, pctCred float64
	for _, c := range gd.Courses {
		p, err := strconv.ParseFloat(c.PercentageGrades, 64)
		if err != nil || p < 60 {
			continue
		}
		cr, _ := strconv.ParseFloat(c.Credit, 64)
		xj, _ := strconv.ParseFloat(c.XFJD, 64)
		credit += cr
		xfjd += xj
		pctCred += p * cr
	}
	if credit > 0 {
		gpa = fmt.Sprintf("%.2f", xfjd/credit)
		pct = fmt.Sprintf("%.2f", pctCred/credit)
	} else {
		gpa, pct = "0.00", "0.00"
	}
	return
}

// ────────────────────────── selected courses ────────────────────────────────

// pendingCourses returns the enrolled courses that have no grade yet
// (i.e. courses in selData but not in curGrades). Returns nil if none.
func pendingCourses(selData *zfn.SelectedCoursesData, curGrades *zfn.GradeData) []push.PendingCourse {
	if selData == nil || len(selData.Courses) == 0 {
		return nil
	}
	// Build set of current-semester class IDs that already have a grade.
	graded := make(map[string]bool, len(curGrades.Courses))
	if curGrades != nil {
		for _, c := range curGrades.Courses {
			graded[c.ClassID] = true
		}
	}
	var out []push.PendingCourse
	for _, cour := range selData.Courses {
		if graded[cour.ClassID] {
			continue
		}
		out = append(out, push.PendingCourse{Name: normalizeBrackets(cour.Title), Teacher: cour.Teacher})
	}
	return out
}

// selectedCoursesText returns a plain-text list of enrolled courses that have
// no grade yet. Returns "" if none.
func selectedCoursesText(selData *zfn.SelectedCoursesData, curGrades *zfn.GradeData) string {
	pc := pendingCourses(selData, curGrades)
	if len(pc) == 0 {
		return ""
	}
	names := make([]string, 0, len(pc))
	for _, c := range pc {
		names = append(names, "  · "+c.Name+"  "+c.Teacher)
	}
	return subdiv + "\n  未公布成绩\n" + strings.Join(names, "\n")
}

// scoreClass maps a grade string to a Liquid Glass CSS class:
//   "g"   → green (good), "fail" → red (failing), "" → default white.
func scoreClass(g string) string {
	if f, err := strconv.ParseFloat(g, 64); err == nil {
		if f < 60 {
			return "fail"
		}
		if f >= 85 {
			return "g"
		}
		return ""
	}
	// Non-numeric grades: failure keywords → red, otherwise treat as passed.
	switch g {
	case "不及格", "挂科", "缺考", "作弊", "违纪", "缓考", "弃考", "差":
		return "fail"
	default:
		return "g"
	}
}

// ─────────────────────────── page builder ───────────────────────────────────

func buildPage(header, name, sid, semLabel string, courses []push.Course, gpa, pctGPA, selText string, cfg *config.Config) string {
	var b strings.Builder

	b.WriteString(divider + "\n")
	b.WriteString(fmt.Sprintf("  %s\n", header))
	b.WriteString(fmt.Sprintf("  %s\n", semLabel))
	b.WriteString(divider + "\n\n")

	if len(courses) > 0 {
		for _, c := range courses {
			b.WriteString(fmt.Sprintf("  %s    %s\n", padRight(c.Course, 16), c.Grade))
			b.WriteString(fmt.Sprintf("  %s · %s\n\n", c.Teacher, c.Time))
		}
	}

	b.WriteString(divider + "\n")
	b.WriteString(fmt.Sprintf("  📈 GPA %s    百分制 %s\n", gpa, pctGPA))
	b.WriteString(divider + "\n\n")

	b.WriteString(fmt.Sprintf("  %s · %s\n", name, sid))
	b.WriteString(fmt.Sprintf("  %s\n", semLabel))

	ts := time.Now().Format("2006-01-02 15:04")
	if cfg.BeijingTime != "" {
		ts = cfg.BeijingTime
	}
	b.WriteString(fmt.Sprintf("\n  %s\n", ts))

	if selText != "" {
		b.WriteString("\n" + selText + "\n")
	}

	b.WriteString("\n  " + copyright)

	return b.String()
}

// buildPageHTML was replaced by push.RenderGradeCard, which renders the same
// Liquid Glass layout from internal/push/templates via html/template. User data
// is auto-escaped (XSS-safe); CSS/JS are embedded as trusted assets. See
// internal/push/render.go.

// padRight right-pads s with spaces to the given visual width.
// CJK / full-width chars count as 2 columns.
func padRight(s string, width int) string {
	w := 0
	for _, r := range s {
		if isWide(r) {
			w += 2
		} else {
			w++
		}
	}
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// isWide reports whether r has a double-width rendering in fixed-width fonts.
// Covers CJK Unified Ideographs, Hiragana/Katakana, fullwidth forms, etc.
func isWide(r rune) bool {
	return r >= 0x1100 && (r <= 0x115f || // Hangul Jamo
		r == 0x2329 || r == 0x232a ||
		(0x2e80 <= r && r <= 0x303e) || // CJK Radicals + Symbols
		(0x3041 <= r && r <= 0x33ff) || // Hiragana/Katakana/CJK Symbols
		(0x3400 <= r && r <= 0x4dbf) || // CJK Ext A
		(0x4e00 <= r && r <= 0x9fff) || // CJK Unified
		(0xa000 <= r && r <= 0xa4cf) || // Yi
		(0xac00 <= r && r <= 0xd7a3) || // Hangul Syllables
		(0xf900 <= r && r <= 0xfaff) || // CJK Compat
		(0xfe30 <= r && r <= 0xfe4f) || // CJK Compat Forms
		(0xff00 <= r && r <= 0xff60) || // Fullwidth Forms
		(0xffe0 <= r && r <= 0xffe6))
}

// ────────────────────────────── utilities ───────────────────────────────────

func lastSubmission(gd *zfn.GradeData) string {
	if gd == nil || len(gd.Courses) == 0 {
		return ""
	}
	latest := gd.Courses[0].SubmissionTime
	for _, c := range gd.Courses[1:] {
		if c.SubmissionTime > latest {
			latest = c.SubmissionTime
		}
	}
	return latest
}

func strVal(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
			return fmt.Sprint(v)
		}
	}
	return ""
}

func writeGitHubSummary(runLog string, cfg *config.Config) {
	info := fmt.Sprintf("Force Push: %v | Branch: %s | Trigger: %s | Actor: %s | SHA: %s | Time: %s",
		cfg.ForcePush, cfg.RefName, cfg.EventName, cfg.Actor, cfg.SHA, cfg.BeijingTime)
	summary := fmt.Sprintf("# %s\n\n%s\n\n---\n%s\n\n%s", pushTitle, runLog, info, copyright)
	for strings.Contains(summary, "\n\n\n") {
		summary = strings.ReplaceAll(summary, "\n\n\n", "\n\n")
	}
	if cfg.StepSummary != "" {
		_ = os.WriteFile(cfg.StepSummary, []byte(summary), 0644)
	}
}
