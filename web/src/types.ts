/** 与 Go 侧 internal/push/payload.go 对齐的数据契约。 */

/** dist 落盘的信封：仓库里只出现密文（或占位符）。 */
export interface Envelope {
  v: number;
  /** 初始占位：Actions 尚未生成任何成绩数据。 */
  placeholder?: boolean;
  /** 加密载荷：base64(nonce(12B) || AES-256-GCM ciphertext)。 */
  ct?: string;
  alg?: string;
  updatedAt?: string;
  /** 明文回退（GRADES_KEY 未配置时），仅用于本地调试。 */
  plain?: GradePayload;
}

export interface CourseRow {
  course: string;
  grade: string;
  teacher: string;
  time: string;
  /** "g" = 优秀(绿) · "" = 普通(白) · "fail" = 不及格(红)，Go 侧已算好。 */
  scoreClass: "g" | "" | "fail";
}

export interface PendingRow {
  name: string;
  teacher: string;
}

export interface GradePayload {
  semester: string;
  gpa: string;
  pctGpa: string;
  firstRun: boolean;
  updatedAt: string;
  courses: CourseRow[];
  pending: PendingRow[];
  copyright: string;
}

export type SortKey = "time" | "scoreDesc" | "scoreAsc";
export type FilterKey = "all" | "good" | "pass" | "fail";
