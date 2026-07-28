package push

import (
	"encoding/json"
)

// ─────────────────────────────────────────────────────────────────────────────
// 自托管成绩页（web/ SPA）的数据契约。
//
// Go 侧只产出「加密 JSON 信封」写入 web/public/payload.json；浏览器端
// （web/src/main.ts）读取信封、用 URL # 片段里的密钥做 AES-256-GCM 解密，
// 再由 TypeScript 渲染富 UI。字段名必须与 web/src/types.ts 严格一致。
// ─────────────────────────────────────────────────────────────────────────────

// Course is a single published grade row (Go 侧工作类型，由 main.go 组装；
// Meta 仅供纯文本页使用，不进 JSON payload)。
type Course struct {
	Course, Grade, Teacher, Time, Meta, ScoreClass string
}

// PendingCourse is a single enrolled course without a published grade yet.
type PendingCourse struct {
	Name, Teacher string
}

// PayloadCourse 是单门已出成绩（浏览器端渲染用）。
type PayloadCourse struct {
	Course  string `json:"course"`
	Grade   string `json:"grade"`
	Teacher string `json:"teacher"`
	Time    string `json:"time"`
	// ScoreClass: "g" = 优秀(绿) · "" = 普通(白) · "fail" = 不及格(红)。
	ScoreClass string `json:"scoreClass"`
}

// PayloadPending 是单门未公布成绩的已选课程。
type PayloadPending struct {
	Name    string `json:"name"`
	Teacher string `json:"teacher"`
}

// GradePayload 是成绩页的完整明文数据模型（加密前）。
type GradePayload struct {
	Semester  string           `json:"semester"`
	GPA       string           `json:"gpa"`
	PctGPA    string           `json:"pctGpa"`
	FirstRun  bool             `json:"firstRun"`
	UpdatedAt string           `json:"updatedAt"`
	Courses   []PayloadCourse  `json:"courses"`
	Pending   []PayloadPending `json:"pending"`
	Copyright string           `json:"copyright"`
}

// envelope 是落盘到仓库的信封：公开仓库里只出现密文（或明文回退）。
type envelope struct {
	V         int           `json:"v"`
	CT        string        `json:"ct,omitempty"`
	Alg       string        `json:"alg,omitempty"`
	UpdatedAt string        `json:"updatedAt,omitempty"`
	Plain     *GradePayload `json:"plain,omitempty"`
}

// BuildEnvelope serializes the payload into the on-disk envelope JSON.
//
// key != ""  → AES-256-GCM 加密（密钥 SHA-256 派生，密文 = base64(nonce||ct)），
//
//	仓库里只存密文；解密只发生在浏览器端（密钥在 URL # 片段）。
//
// key == ""  → 明文回退（信封带 plain 字段），SPA 会显示「明文公开」警告；
//
//	仅用于未配置 GRADES_KEY 的本地调试。
func BuildEnvelope(p GradePayload, key string) (string, error) {
	// json 里 null 会让前端 .length 崩溃；保证数组字段非 nil。
	if p.Courses == nil {
		p.Courses = []PayloadCourse{}
	}
	if p.Pending == nil {
		p.Pending = []PayloadPending{}
	}

	if key == "" {
		out, err := json.Marshal(envelope{V: 1, UpdatedAt: p.UpdatedAt, Plain: &p})
		return string(out), err
	}

	plainJSON, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	ct, err := EncryptHTML(string(plainJSON), key)
	if err != nil {
		return "", err
	}
	out, err := json.Marshal(envelope{V: 1, CT: ct, Alg: "AES-256-GCM", UpdatedAt: p.UpdatedAt})
	return string(out), err
}
