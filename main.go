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
	copyright = "Copyright © 2024 NianBroken. All rights reserved."
	divider   = "══════════════════════════"
	subdiv    = "──────────────────────────"

	firstRunMsg = "你的程序运行成功\n从现在开始,程序将会每隔 30 分钟自动检测一次成绩是否有更新\n若有更新,将通过微信推送及时通知你"
)

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
	fullPage := buildPage("📊 成绩已更新", ui.Name, ui.SID, sem.Label(), courses, gpa, pctGPA, selText, cfg)
	firstPage := firstRunMsg + "\n\n" + fullPage

	// ── 10. Decision ──
	gc, _ := st.GradeContent()
	ogc, _ := st.OldGradeContent()
	var logLines []string

	switch {
	case ui.Name == "":
		logLines = append(logLines, "个人信息为空，运行失败")
	case gradeErr:
		logLines = append(logLines, "获取成绩时出错，运行失败")
	case firstRun:
		logLines = append(logLines, firstRunMsg)
		logLines = append(logLines, pushAndReport(cfg, pushTitle, firstPage))
	case gc != ogc || cfg.ForcePush:
		logLines = append(logLines, "成绩已更新")
		logLines = append(logLines, pushAndReport(cfg, pushTitle, fullPage))
	default:
		logLines = append(logLines, "成绩未更新")
		if last := lastSubmission(curGrades); last != "" {
			logLines = append(logLines, "最近一次: "+last)
		}
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

// ────────────────────────────── fetch helpers ───────────────────────────────

func fetchUserInfo(c *zfn.Client) (*userInfo, *zfn.GradeResult) {
	var result *zfn.UserInfoResult
	for i := 1; i <= 5; i++ {
		r, err := c.GetUserInfo()
		if err != nil {
			zfn.Backoff(i, 10)
			continue
		}
		if r.Code == 1000 && r.Data != nil {
			result = r
			break
		}
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

// gradeList returns compact grade lines for the push page (current semester only).
func gradeList(gd *zfn.GradeData) []gradeLine {
	if gd == nil || len(gd.Courses) == 0 {
		return nil
	}
	var lines []gradeLine
	for _, c := range sortCourses(gd.Courses) {
		title := normalizeBrackets(c.Title)
		gradeStr := c.Grade
		if _, err := strconv.ParseFloat(c.Grade, 64); err != nil {
			gradeStr = c.Grade + " (" + c.PercentageGrades + ")"
		}
		lines = append(lines, gradeLine{
			Course:  title,
			Grade:   gradeStr,
			Teacher: c.Teacher,
			Time:    shortTime(c.SubmissionTime),
		})
	}
	return lines
}

type gradeLine struct {
	Course, Grade, Teacher, Time string
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

// selectedCoursesText returns a list of enrolled courses that have no grade yet
// (i.e. courses in selData but not in curGrades). Returns "" if none.
func selectedCoursesText(selData *zfn.SelectedCoursesData, curGrades *zfn.GradeData) string {
	if selData == nil || len(selData.Courses) == 0 {
		return ""
	}
	// Build set of current-semester class IDs that already have a grade.
	graded := make(map[string]bool, len(curGrades.Courses))
	if curGrades != nil {
		for _, c := range curGrades.Courses {
			graded[c.ClassID] = true
		}
	}
	var names []string
	for _, cour := range selData.Courses {
		if graded[cour.ClassID] {
			continue
		}
		names = append(names, "  · "+normalizeBrackets(cour.Title)+"  "+cour.Teacher)
	}
	if len(names) == 0 {
		return ""
	}
	return subdiv + "\n  未公布成绩\n" + strings.Join(names, "\n")
}

// ─────────────────────────── page builder ───────────────────────────────────

func buildPage(header, name, sid, semLabel string, courses []gradeLine, gpa, pctGPA, selText string, cfg *config.Config) string {
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
