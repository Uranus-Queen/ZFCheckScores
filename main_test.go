package main

import (
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
			{ClassID: "A", Title: "金属腐蚀理论", Teacher: "汤涛", CourseYear: "2025-2026", CourseSemester: "2"},         // current, ungraded
			{ClassID: "B", Title: "材料科学前沿专题", Teacher: "丁某", CourseYear: "2025-2026", CourseSemester: "第二学期"}, // current, graded
			{ClassID: "C", Title: "旧课（往年）", Teacher: "某人", CourseYear: "2024-2025", CourseSemester: "2"},          // past, graded elsewhere
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
