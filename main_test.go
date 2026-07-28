package main

import (
	"strings"
	"testing"

	"zfcheckscores/internal/config"
	"zfcheckscores/internal/push"
	"zfcheckscores/internal/semester"
	"zfcheckscores/internal/zfn"
)

// TestPendingOnlyCurrentSemester reproduces the bug where "未公布成绩" listed
// courses from OTHER semesters. GetSelectedCourses(0,0) returns every enrolled
// semester; the pending computation must filter to the current semester first,
// otherwise past courses (whose grades live in other semesters and are absent
// from the current-semester grade set) wrongly appear as unpublished.
func TestPendingOnlyCurrentSemester(t *testing.T) {
	sem := semester.Semester{Year: 2025, Term: 2} // 2025-2026 学年第2学期

	sel := &zfn.SelectedCoursesData{
		Courses: []zfn.SelectedCourse{
			{ClassID: "A", Title: "金属腐蚀理论", Teacher: "汤涛", CourseYear: "2025-2026", CourseSemester: "2"},      // current, ungraded
			{ClassID: "B", Title: "材料科学前沿专题", Teacher: "丁某", CourseYear: "2025-2026", CourseSemester: "第二学期"}, // current, graded
			{ClassID: "C", Title: "旧课（往年）", Teacher: "某人", CourseYear: "2024-2025", CourseSemester: "2"},      // past, graded elsewhere
		},
	}

	// Current-semester grades only contain B.
	cur := &zfn.GradeData{
		Courses: []zfn.GradeCourse{
			{ClassID: "B", Title: "材料科学前沿专题", Grade: "90", Teacher: "丁某"},
		},
	}

	selCurrent := filterSelectedCoursesBySemester(sel, sem)
	if len(selCurrent.Courses) != 2 {
		t.Fatalf("filtered current-semester courses = %d, want 2 (A and B)", len(selCurrent.Courses))
	}

	pending := pendingCourses(selCurrent, cur)
	if len(pending) != 1 {
		t.Fatalf("pending courses = %d, want 1 (only A); got %+v", len(pending), pending)
	}
	if pending[0].Name != "金属腐蚀理论" {
		t.Fatalf("pending course = %q, want 金属腐蚀理论 (only the current, ungraded course)", pending[0].Name)
	}
}

// TestSemesterContainsCourse covers the term-name parsing used by
// filterSelectedCoursesBySemester (Arabic "2" and Chinese "第二学期" both match).
func TestSemesterContainsCourse(t *testing.T) {
	sem := semester.Semester{Year: 2025, Term: 2}
	if !sem.ContainsCourse(zfn.SelectedCourse{CourseYear: "2025-2026", CourseSemester: "2"}) {
		t.Fatal("Arabic term '2' should match")
	}
	if !sem.ContainsCourse(zfn.SelectedCourse{CourseYear: "2025-2026", CourseSemester: "第二学期"}) {
		t.Fatal("Chinese term '第二学期' should match")
	}
	if sem.ContainsCourse(zfn.SelectedCourse{CourseYear: "2024-2025", CourseSemester: "2"}) {
		t.Fatal("different academic year should NOT match")
	}
}

// TestClassifyFailure locks the failure-diagnosis mapping so the actionable
// reasons/suggestions stay consistent with the README 故障排查 section and
// never silently regress into a bare "运行失败".
func TestClassifyFailure(t *testing.T) {
	cases := []struct {
		name       string
		code       int
		msg        string
		wantReason string // substring that must appear in reason
		wantFix    string // substring that must appear in suggestion
	}{
		{"captcha_waf", 1001, "获取验证码成功", "验证码", "COOKIES"},
		{"bad_password", 1002, "用户名或密码不正确", "用户名或密码错误", "USERNAME"},
		{"session_expired", 1006, "未登录或已过期，请重新登录", "会话", "COOKIES"},
		{"missing_context_path", 2333, "访问教务系统返回『系统维护页面』，很可能 URL 缺少 /jwglxt", "系统维护页", "/jwglxt"},
		{"unreachable", 2333, "教务系统挂了", "不可达", "重试"},
		{"unknown_tip", 998, "未知提示xxx", "未知提示", "登录页"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reason, suggestion := classifyFailure(c.code, c.msg)
			if !strings.Contains(reason, c.wantReason) {
				t.Errorf("%s: reason %q does not contain %q", c.name, reason, c.wantReason)
			}
			if !strings.Contains(suggestion, c.wantFix) {
				t.Errorf("%s: suggestion %q does not contain %q", c.name, suggestion, c.wantFix)
			}
		})
	}
}

// TestClassifyGradeNil covers the nil-result branch (HTTP-level failure,
// distinct from a 正方-level error code).
func TestClassifyGradeNil(t *testing.T) {
	reason, suggestion := classifyGrade(nil)
	if !strings.Contains(reason, "网络请求失败") {
		t.Errorf("nil grade result reason = %q, want 网络请求失败", reason)
	}
	if !strings.Contains(suggestion, "重试") {
		t.Errorf("nil grade result suggestion = %q, want 重试", suggestion)
	}
}

// TestBuildNotify locks the Server酱 notification content: course count, GPA,
// and the self-hosted deep link. The link must be the unguessable path
// https://<domain>/<key>/ (so no login step is needed) and must fall back to a
// placeholder when GRADES_DOMAIN or GRADES_KEY is unset, so the notification
// never embeds a broken or publicly-rooted URL.
func TestBuildNotify(t *testing.T) {
	courses := []push.Course{{Course: "高等数学", Grade: "92"}, {Course: "大学英语", Grade: "85"}}

	// Domain + key -> keyed deep link.
	cfg := &config.Config{SiteDomain: "grades.example.com", SiteKey: "s3cr3t"}
	title, desp, short := buildNotify(cfg, "2025-2026 学年第2学期", courses, "3.45", "88.20", false)
	if title != "正方教务成绩更新" {
		t.Errorf("title = %q, want 正方教务成绩更新", title)
	}
	if !strings.Contains(desp, "本学期 **2** 门已出成绩") {
		t.Errorf("desp missing course count: %q", desp)
	}
	if !strings.Contains(desp, "https://grades.example.com/#s3cr3t") {
		t.Errorf("desp missing keyed deep link: %q", desp)
	}
	if short != "正方教务成绩更新 · 本学期2门 · GPA3.45" {
		t.Errorf("short = %q", short)
	}

	// Domain + key given with scheme + trailing slash must still normalize.
	cfg3 := &config.Config{SiteDomain: "https://grades.example.com/", SiteKey: "s3cr3t"}
	_, desp3, _ := buildNotify(cfg3, "x", courses, "3.45", "88.20", false)
	if !strings.Contains(desp3, "https://grades.example.com/#s3cr3t") {
		t.Errorf("desp3 missing keyed deep link: %q", desp3)
	}

	// Domain set but key missing -> no link (root would be public).
	cfgNoKey := &config.Config{SiteDomain: "grades.example.com"}
	_, despNoKey, _ := buildNotify(cfgNoKey, "x", courses, "3.45", "88.20", false)
	if strings.Contains(despNoKey, "https://") {
		t.Errorf("desp should not contain link when key missing: %q", despNoKey)
	}
	if !strings.Contains(despNoKey, "GRADES_KEY") {
		t.Errorf("desp missing GRADES_KEY hint: %q", despNoKey)
	}

	// No domain -> placeholder, never a broken https:// link.
	cfg2 := &config.Config{}
	_, desp2, _ := buildNotify(cfg2, "x", nil, "0.00", "0.00", true)
	if strings.Contains(desp2, "https://") {
		t.Errorf("desp should not contain link when domain empty: %q", desp2)
	}
	if !strings.Contains(desp2, "自托管成绩页部署中") {
		t.Errorf("desp missing placeholder: %q", desp2)
	}
}
