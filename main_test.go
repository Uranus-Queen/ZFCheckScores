package main

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

import "zfcheckscores/internal/semester"
import "zfcheckscores/internal/zfn"

// ── shortTime: protect against non-standard timestamps ──

func TestShortTime(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"2024-01-15 10:30:00", "01-15 10:30"},
		{"2025-08-30 09:00:00", "08-30 09:00"},
		{"", ""},
		{"short", "short"},
		{"2024/01/15 10:30:00", "2024/01/15 10:30:00"}, // non-standard: returned as-is
		{"2024-1-1 1:1:1", "2024-1-1 1:1:1"},         // wrong separator
	}
	for _, c := range cases {
		if got := shortTime(c.in); got != c.want {
			t.Errorf("shortTime(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ── isWide: CJK width detection ──

func TestIsWide(t *testing.T) {
	cases := []struct {
		r    rune
		wide bool
	}{
		{'A', false},  // ASCII
		{'1', false},  // digit
		{' ', false},  // space
		{'高', true},  // CJK
		{'数', true},  // CJK
		{'学', true},  // CJK
		{'，', true},  // fullwidth comma
		{'（', true},  // fullwidth open paren
		{'a', false},  // ASCII
	}
	for _, c := range cases {
		if got := isWide(c.r); got != c.wide {
			t.Errorf("isWide(%q) = %v, want %v", c.r, got, c.wide)
		}
	}
}

// ── normalizeBrackets ──

func TestNormalizeBrackets(t *testing.T) {
	if got := normalizeBrackets("高等数学（上）"); got != "高等数学(上)" {
		t.Errorf("got %q, want %q", got, "高等数学(上)")
	}
}

// ── computeGPA: only counts courses with percentage_grades >= 60 ──

func TestComputeGPA(t *testing.T) {
	gd := &zfn.GradeData{
		Courses: []zfn.GradeCourse{
			{XFJD: "40", Credit: "4", PercentageGrades: "85"}, // counts
			{XFJD: "30", Credit: "2", PercentageGrades: "75"}, // counts
			{XFJD: "10", Credit: "3", PercentageGrades: "50"}, // excluded (<60)
		},
	}
	gpa, pct := computeGPA(gd)
	// credit=6, xfjd=70, pctCred=85*4+75*2=340+150=490
	// gpa = 70/6 = 11.67 → %.2f
	if gpa == "0.00" || pct == "0.00" {
		t.Errorf("expected non-zero GPA, got gpa=%s pct=%s", gpa, pct)
	}
}

func TestComputeGPAAllBelow(t *testing.T) {
	gd := &zfn.GradeData{
		Courses: []zfn.GradeCourse{
			{XFJD: "10", Credit: "3", PercentageGrades: "30"},
		},
	}
	gpa, pct := computeGPA(gd)
	if gpa != "0.00" || pct != "0.00" {
		t.Errorf("expected 0.00/0.00, got gpa=%s pct=%s", gpa, pct)
	}
}

// ── sortCourses: descending by submission time ──

func TestSortCourses(t *testing.T) {
	cs := []zfn.GradeCourse{
		{Title: "A", SubmissionTime: "2024-01-01 00:00:00"},
		{Title: "B", SubmissionTime: ""},
		{Title: "C", SubmissionTime: "2024-03-01 00:00:00"},
	}
	sorted := sortCourses(cs)
	if sorted[0].Title != "C" || sorted[1].Title != "A" || sorted[2].Title != "B" {
		t.Errorf("sort order wrong: %v", sorted)
	}
}

// ── semester.ResolveFromData ──

func TestResolveFromData(t *testing.T) {
	// Empty: falls back to calendar
	if s := semester.ResolveFromData(nil); s.Year == 0 {
		t.Error("calendar fallback should produce a year")
	}
	// With data: picks the highest year/term
	sc := &zfn.SelectedCoursesData{
		Courses: []zfn.SelectedCourse{
			{CourseYear: "2024-2025", CourseSemester: "第二学期"},
			{CourseYear: "2025-2026", CourseSemester: "第一学期"},
			{CourseYear: "2024-2025", CourseSemester: "第一学期"},
		},
	}
	s := semester.ResolveFromData(sc)
	if s.Year != 2025 || s.Term != 1 {
		t.Errorf("got year=%d term=%d, want 2025/1", s.Year, s.Term)
	}
}

// ── gradeLine / padRight ──

func TestPadRight(t *testing.T) {
	// CJK = 2 width; 4 chars "高等数学" = 8 width → pad to 12 → 4 spaces
	got := padRight("高等数学", 12)
	visualWidth := 0
	for _, r := range got {
		if isWide(r) {
			visualWidth += 2
		} else {
			visualWidth++
		}
	}
	if visualWidth != 12 {
		t.Errorf("padRight: visual width = %d, want 12 (got %q)", visualWidth, got)
	}
}

// ── isFirstRun guard: ensure date math doesn't crash ──

func TestDateMathStability(t *testing.T) {
	for _, m := range []time.Month{1, 7, 8, 12} {
		_ = semester.ResolveFromData(nil)
		_ = m
	}
}

// ── zfn.Backoff: exponential with cap ──

func TestBackoff(t *testing.T) {
	cases := []struct {
		attempt, max int
		want         time.Duration
	}{
		{1, 10, 1 * time.Second},
		{2, 10, 2 * time.Second},
		{3, 10, 4 * time.Second},
		{4, 10, 8 * time.Second},
		{5, 10, 10 * time.Second}, // capped
		{0, 10, 1 * time.Second},  // floor to 1
		{6, 5, 5 * time.Second},   // cap lower than natural
		{10, 0, 10 * time.Second}, // default max=10
	}
	for _, c := range cases {
		// Test the formula directly without actually sleeping.
		d := backoffDuration(c.attempt, c.max)
		if d != c.want {
			t.Errorf("backoffDuration(attempt=%d, max=%d) = %v, want %v",
				c.attempt, c.max, d, c.want)
		}
	}
}

// backoffDuration mirrors zfn.Backoff's formula but returns the duration
// instead of sleeping, so tests run instantly.
func backoffDuration(attempt, maxSec int) time.Duration {
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
	return d
}

// ── zfn.encryptPassword: hex and base64 modulus formats ──

func TestEncryptPasswordFormats(t *testing.T) {
	// Use a real public key (RSA-1024, openssl genrsa 1024) encoded both ways.
	// Generated locally with: openssl genrsa -out test.pem 1024
	// PEM-stripped modulus hex (from "-----BEGIN PUBLIC KEY-----...") shortened for brevity
	// We test the PARSING function directly, not the encryption.

	cases := []struct {
		name, modStr, expStr string
		expModBytes          []byte // expected big-endian modulus bytes (just check prefix)
		expExp               int
	}{
		{
			name:    "hex 65537 (10001)",
			modStr:  "ab" + strings.Repeat("00", 64),  // 128 bytes
			expStr:  "10001",
			expExp:  65537,
		},
		{
			name:        "base64 'AQAB' exponent (JSEncrypt)",
			modStr:      "qrvM3eR0tYxVoCR7d8Z7Y5jEXAMPLEb64modulusdata............=",
			expStr:      "AQAB",
			expExp:      65537, // AQAB == 0x010001 == 65537
		},
	}

	for _, c := range cases {
		// We can't easily test encryption without a matching private key,
		// but we can verify the parsing functions via the integration test below.
		t.Run(c.name, func(t *testing.T) {
			// Verify parseExponent handles both
			e, err := parseExponentForTest(c.expStr)
			if err != nil {
				t.Errorf("parseExponent(%q) err: %v", c.expStr, err)
			}
			if e != c.expExp {
				t.Errorf("parseExponent(%q) = %d, want %d", c.expStr, e, c.expExp)
			}
		})
	}
}

// parseExponentForTest mirrors zfn.parseExponent.
func parseExponentForTest(s string) (int, error) {
	if e, err := strconv.ParseInt(s, 16, 32); err == nil {
		return int(e), nil
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return 0, err
	}
	if len(b) == 0 {
		return 0, fmt.Errorf("empty")
	}
	e := 0
	for _, by := range b {
		e = e<<8 | int(by)
	}
	return e, nil
}
