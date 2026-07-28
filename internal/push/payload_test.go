package push

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// samplePayload mirrors a realistic run.
func samplePayload() GradePayload {
	return GradePayload{
		Semester:  "2025-2026 学年第2学期",
		GPA:       "3.45",
		PctGPA:    "88.20",
		FirstRun:  false,
		UpdatedAt: "2026-07-27 21:00",
		Courses: []PayloadCourse{
			{Course: "高等数学", Grade: "92", Teacher: "张三", Time: "07-20 10:30", ScoreClass: "g"},
			{Course: "大学英语", Grade: "55", Teacher: "李四", Time: "07-21 09:00", ScoreClass: "fail"},
		},
		Pending: []PayloadPending{{Name: "金属腐蚀理论", Teacher: "汤涛"}},
	}
}

// TestEnvelopeEncrypted locks the encrypted envelope shape consumed by
// web/src/main.ts: {v, ct, alg, updatedAt} with NO plaintext anywhere, and the
// ciphertext must decrypt (SHA-256 key, 12-byte nonce prefix) back to a JSON
// whose field names match web/src/types.ts exactly.
func TestEnvelopeEncrypted(t *testing.T) {
	out, err := BuildEnvelope(samplePayload(), "s3cr3t-🔑")
	if err != nil {
		t.Fatalf("BuildEnvelope: %v", err)
	}
	// Envelope must not leak any plaintext.
	for _, leak := range []string{"高等数学", "92", "张三", "plain"} {
		if strings.Contains(out, leak) {
			t.Fatalf("encrypted envelope leaks %q: %s", leak, out)
		}
	}
	var env struct {
		V         int    `json:"v"`
		CT        string `json:"ct"`
		Alg       string `json:"alg"`
		UpdatedAt string `json:"updatedAt"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("envelope not valid JSON: %v", err)
	}
	if env.V != 1 || env.Alg != "AES-256-GCM" || env.CT == "" || env.UpdatedAt == "" {
		t.Fatalf("bad envelope meta: %+v", env)
	}

	// Decrypt exactly the way the browser does.
	raw, err := base64.StdEncoding.DecodeString(env.CT)
	if err != nil {
		t.Fatalf("ct not base64: %v", err)
	}
	block, _ := aes.NewCipher(deriveKey("s3cr3t-🔑"))
	gcm, _ := cipher.NewGCM(block)
	ns := gcm.NonceSize()
	plain, err := gcm.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	// Field-name contract with web/src/types.ts.
	for _, field := range []string{`"semester"`, `"gpa"`, `"pctGpa"`, `"firstRun"`, `"updatedAt"`, `"courses"`, `"pending"`, `"scoreClass"`, `"course"`, `"grade"`, `"teacher"`, `"time"`, `"name"`} {
		if !strings.Contains(string(plain), field) {
			t.Errorf("decrypted payload missing field %s: %s", field, plain)
		}
	}
}

// TestEnvelopePlainFallback locks the GRADES_KEY-unset fallback: envelope
// carries `plain` (so the SPA can warn about public visibility) and arrays are
// never null (a nil slice would crash .length in the browser).
func TestEnvelopePlainFallback(t *testing.T) {
	p := samplePayload()
	p.Courses = nil
	p.Pending = nil
	out, err := BuildEnvelope(p, "")
	if err != nil {
		t.Fatalf("BuildEnvelope: %v", err)
	}
	if !strings.Contains(out, `"plain"`) {
		t.Fatalf("plain fallback envelope missing plain field: %s", out)
	}
	if strings.Contains(out, `"courses":null`) || strings.Contains(out, `"pending":null`) {
		t.Fatalf("arrays must never be null: %s", out)
	}
	if strings.Contains(out, `"ct"`) {
		t.Fatalf("plain envelope must not carry ct: %s", out)
	}
}
