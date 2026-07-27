package main

import (
	"strings"
	"testing"

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
