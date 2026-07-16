package semester

import (
	"strconv"
	"strings"
	"time"

	"zfcheckscores/internal/zfn"
)

// Semester represents an academic year and term (1=第一学期, 2=第二学期).
type Semester struct {
	Year int
	Term int
}

// Resolve determines the current semester for the given client.
// Strategy: prefer the highest year/term from selected courses;
// fall back to calendar-based inference.
func Resolve(c *zfn.Client) Semester {
	if s, ok := fromSelectedCourses(c); ok {
		return s
	}
	return fromCalendar(time.Now())
}

// ResolveFromData is like Resolve but accepts pre-fetched selected courses
// data to avoid duplicate API calls.
func ResolveFromData(sc *zfn.SelectedCoursesData) Semester {
	if sc == nil {
		return fromCalendar(time.Now())
	}
	var best Semester
	for _, cour := range sc.Courses {
		year := parseCourseYear(cour.CourseYear)
		term := parseCourseTerm(cour.CourseSemester)
		if year == 0 || term == 0 {
			continue
		}
		if year > best.Year || (year == best.Year && term > best.Term) {
			best.Year = year
			best.Term = term
		}
	}
	if best.Year == 0 {
		return fromCalendar(time.Now())
	}
	return best
}

// Label returns a human-readable Chinese label, e.g. "2025-2026 学年第1学期".
func (s Semester) Label() string {
	if s.Year == 0 {
		return "未知学期"
	}
	return strconv.Itoa(s.Year) + "-" + strconv.Itoa(s.Year+1) + " 学年第" + strconv.Itoa(s.Term) + "学期"
}

// fromSelectedCourses fetches all enrolled courses and extracts the
// highest academic year+term combination.
func fromSelectedCourses(c *zfn.Client) (Semester, bool) {
	res, err := c.GetSelectedCourses(0, 0)
	if err != nil || res.Code != 1000 || res.Data == nil {
		return Semester{}, false
	}
	var best Semester
	for _, cour := range res.Data.Courses {
		year := parseCourseYear(cour.CourseYear)
		term := parseCourseTerm(cour.CourseSemester)
		if year == 0 || term == 0 {
			continue
		}
		if year > best.Year || (year == best.Year && term > best.Term) {
			best.Year = year
			best.Term = term
		}
	}
	if best.Year == 0 {
		return Semester{}, false
	}
	return best, true
}

// fromCalendar infers semester from the current date.
// Aug–Dec → (current_year, 1); Jan–Jul → (current_year-1, 2).
func fromCalendar(t time.Time) Semester {
	y := t.Year()
	if t.Month() >= 8 {
		return Semester{Year: y, Term: 1}
	}
	return Semester{Year: y - 1, Term: 2}
}

// parseCourseYear extracts the starting year from a string like "2024-2025".
func parseCourseYear(s string) int {
	s = strings.TrimSpace(s)
	if idx := strings.Index(s, "-"); idx > 0 {
		s = s[:idx]
	}
	y, _ := strconv.Atoi(strings.TrimSpace(s))
	return y
}

// parseCourseTerm converts Chinese semester names to numeric term:
// "第一学期" → 1, "第二学期" → 2; returns 0 on failure.
func parseCourseTerm(s string) int {
	s = strings.TrimSpace(s)
	switch {
	case strings.Contains(s, "第一") || s == "1":
		return 1
	case strings.Contains(s, "第二") || s == "2":
		return 2
	}
	return 0
}
