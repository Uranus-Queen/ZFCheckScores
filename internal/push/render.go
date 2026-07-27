package push

import (
	"bytes"
	_ "embed"
	"html/template"
)

//go:embed templates/glass.css
var glassCSSEmbed string

//go:embed templates/glass.js
var glassJSEmbed string

//go:embed templates/grade_card.html
var gradeCardHTML string

// gradeCardTemplate is parsed once at package init. It is a static, trusted
// template: the only dynamic values are user data (auto-escaped by
// html/template) plus the embedded CSS/JS (injected as trusted types).
var gradeCardTemplate = template.Must(template.New("grade_card").Parse(gradeCardHTML))

// Course is a single published grade row for the push card.
type Course struct {
	Course, Grade, Teacher, Time, Meta, ScoreClass string
}

// PendingCourse is a single enrolled course without a published grade yet.
type PendingCourse struct {
	Name, Teacher string
}

// GradeCardData is the full data model for the Liquid Glass push page.
// All string fields are HTML-escaped by html/template; CSS/JS are injected as
// trusted types from the embedded assets, so never place untrusted data there.
type GradeCardData struct {
	Title, SemLabel, GPA, PctGPA, Copyright string
	Courses                                 []Course
	Pending                                 []PendingCourse
	FirstRun                                bool
}

// RenderGradeCard returns a self-contained Liquid Glass HTML document for the
// Showdoc / WeChat push. User-supplied data is auto-escaped (XSS-safe); the
// glass effect is pure CSS, with the JS runtime as a progressive enhancement
// that some webviews strip.
func RenderGradeCard(d GradeCardData) (string, error) {
	data := struct {
		GradeCardData
		CSS template.CSS
		JS  template.JS
	}{
		GradeCardData: d,
		CSS:           template.CSS(glassCSSEmbed),
		JS:            template.JS(glassJSEmbed),
	}
	var buf bytes.Buffer
	if err := gradeCardTemplate.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
