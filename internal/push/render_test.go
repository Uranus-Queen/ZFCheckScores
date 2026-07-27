package push

import (
	"strings"
	"testing"
)

// TestRenderGradeCardEscapes verifies that user-supplied data is HTML-escaped
// (XSS-safe) while the embedded Liquid Glass runtime script is still present.
func TestRenderGradeCardEscapes(t *testing.T) {
	d := GradeCardData{
		Title:     "正方教务管理系统成绩推送",
		SemLabel:  "2025-2026 学年第2学期",
		GPA:       "3.70",
		PctGPA:    "88.50",
		Copyright: "Copyright © 2026 IKAROS. All rights reserved.",
		Courses: []Course{
			{Course: "高等数学<script>", Grade: "92", Teacher: "张三", Time: "2026-01-02 10:30", Meta: "张三 · 2026-01-02 10:30", ScoreClass: "g"},
			{Course: "物理", Grade: "45", Teacher: "李四", Time: "2026-01-03 09:00", Meta: "李四 · 2026-01-03 09:00", ScoreClass: "fail"},
		},
		Pending: []PendingCourse{{Name: "化学", Teacher: "王五"}},
	}
	html, err := RenderGradeCard(d)
	if err != nil {
		t.Fatalf("RenderGradeCard: %v", err)
	}
	checks := []string{
		"GPA 统计",
		"已出成绩",
		"未公布成绩",
		"Copyright © 2026 IKAROS",
		"&lt;script&gt;", // injected markup must be escaped
		`class="course-score g"`,   // 92 → green
		`class="course-score fail"`, // 45 → red
		`id="disp-map"`, // static Liquid Glass runtime present
	}
	for _, c := range checks {
		if !strings.Contains(html, c) {
			t.Errorf("HTML missing %q", c)
		}
	}
	if strings.Contains(html, "<script>高等数学") {
		t.Error("user-supplied markup was not escaped")
	}
	// Removed sections must stay removed.
	for _, gone := range []string{"preview-badge", "meta-bar", "推送时间", "NianBroken"} {
		if strings.Contains(html, gone) {
			t.Errorf("HTML should no longer contain %q", gone)
		}
	}
}

// TestRenderGradeCardFirstRun verifies the first-run banner only appears when
// FirstRun is true.
func TestRenderGradeCardFirstRun(t *testing.T) {
	d := GradeCardData{
		SemLabel:  "2025-2026 学年第2学期",
		FirstRun:  true,
		Copyright: "Copyright © 2026 IKAROS. All rights reserved.",
	}
	html, err := RenderGradeCard(d)
	if err != nil {
		t.Fatalf("RenderGradeCard: %v", err)
	}
	if !strings.Contains(html, "first-run") {
		t.Error("first-run banner missing when FirstRun=true")
	}
	if !strings.Contains(html, "每隔") {
		t.Error("first-run message missing")
	}
}
