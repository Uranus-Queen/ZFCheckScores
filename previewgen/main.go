// previewgen 是本地预览工具：用真实的 push.BuildEnvelope 生成一份演示成绩的
// AES-256-GCM 加密信封（密钥 "demo"），写到 web/.local-preview/payload.json，
// 配合 `wrangler kv key put --local` + `wrangler dev --local` 在本地预览成绩页。
// 不参与生产构建；产物 web/.local-preview/ 已被 .gitignore 忽略。
package main

import (
	"fmt"
	"os"

	"zfcheckscores/internal/push"
)

func main() {
	p := push.GradePayload{
		Semester:  "2025-2026 学年第 2 学期",
		GPA:       "3.82",
		PctGPA:    "4.12",
		FirstRun:  false,
		UpdatedAt: "2026-07-28 13:55",
		Copyright: "Copyright © 2026 IKAROS. All rights reserved.",
		Courses: []push.PayloadCourse{
			{Course: "高等数学（下）", Grade: "95", Teacher: "王老师", Time: "2025-2026-2", ScoreClass: "g"},
			{Course: "程序设计基础", Grade: "92", Teacher: "赵老师", Time: "2025-2026-2", ScoreClass: "g"},
			{Course: "大学英语（二）", Grade: "88", Teacher: "李老师", Time: "2025-2026-2", ScoreClass: ""},
			{Course: "线性代数", Grade: "76", Teacher: "钱老师", Time: "2025-2026-2", ScoreClass: ""},
			{Course: "大学物理", Grade: "58", Teacher: "孙老师", Time: "2025-2026-2", ScoreClass: "fail"},
			{Course: "体育（二）", Grade: "优秀", Teacher: "周老师", Time: "2025-2026-2", ScoreClass: "g"},
			{Course: "形势与政策", Grade: "合格", Teacher: "吴老师", Time: "2025-2026-2", ScoreClass: ""},
		},
		Pending: []push.PayloadPending{
			{Name: "概率论与数理统计", Teacher: "郑老师"},
			{Name: "军事理论", Teacher: "冯老师"},
		},
	}

	env, err := push.BuildEnvelope(p, "demo")
	if err != nil {
		fmt.Fprintln(os.Stderr, "BuildEnvelope:", err)
		os.Exit(1)
	}
	if err := os.MkdirAll("web/.local-preview", 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	out := "web/.local-preview/payload.json"
	if err := os.WriteFile(out, []byte(env), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote", out, len(env), "bytes (key=demo)")
}
