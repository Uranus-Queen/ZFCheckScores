import type { CourseRow, FilterKey, GradePayload, SortKey } from "./types";

/* ────────────────────────── helpers ────────────────────────── */

export function esc(s: string): string {
  return s
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

/** 从成绩字符串提取数值分数："92" → 92，"优秀 (90)" → 90，提取不到 → null。 */
export function scoreOf(grade: string): number | null {
  const m = grade.match(/\d+(?:\.\d+)?/);
  return m ? parseFloat(m[0]) : null;
}

function scoreCls(c: CourseRow): string {
  if (c.scoreClass === "g") return "score-g";
  if (c.scoreClass === "fail") return "score-fail";
  return "text-white";
}

/* ────────────────────────── static states ────────────────────────── */

export function renderWaiting(): string {
  return `
  <div class="glass rise mx-auto my-auto w-full max-w-sm px-7 py-10 text-center">
    <div class="text-4xl">⏳</div>
    <h1 class="mt-4 text-lg font-semibold">成绩页生成中</h1>
    <p class="mt-2 text-[13px] leading-relaxed text-[var(--c-text-2nd)]">
      首次成功运行 GitHub Actions 后，这里会自动生成本学期的毛玻璃成绩卡片，并实时随成绩更新。
    </p>
  </div>
  <footer class="mt-auto pt-6 text-center text-[11px] text-[var(--c-text-2nd)]">Powered by ZFCheckScores</footer>`;
}

export function renderKeyPrompt(wrongKey: boolean): string {
  return `
  <div class="glass rise mx-auto my-auto w-full max-w-sm px-7 py-9 text-center">
    <div class="text-4xl">${wrongKey ? "🔐" : "🔑"}</div>
    <h1 class="mt-4 text-lg font-semibold">${wrongKey ? "解密失败" : "需要访问密钥"}</h1>
    <p class="mt-2 text-[13px] leading-relaxed text-[var(--c-text-2nd)]">
      ${
        wrongKey
          ? "密钥不正确或页面已更新，请核对后重试。"
          : "本页内容已端到端加密。请使用带 <code class=\"text-white\">#密钥</code> 的完整链接打开，或在下方输入密钥。"
      }
    </p>
    <form id="key-form" class="mt-5 flex gap-2">
      <input id="key-input" type="password" autocomplete="off" placeholder="访问密钥"
        class="ctl min-w-0 flex-1 px-4 py-2.5 text-[14px]" />
      <button type="submit"
        class="shrink-0 rounded-xl bg-[var(--c-blue)] px-4 py-2.5 text-[14px] font-medium text-white transition active:scale-95">
        解锁
      </button>
    </form>
  </div>
  <footer class="mt-auto pt-6 text-center text-[11px] text-[var(--c-text-2nd)]">端到端加密 · 密钥不会发送到服务器</footer>`;
}

export function renderError(msg: string): string {
  return `
  <div class="glass rise mx-auto my-auto w-full max-w-sm px-7 py-10 text-center">
    <div class="text-4xl">⚠️</div>
    <h1 class="mt-4 text-lg font-semibold">页面加载失败</h1>
    <p class="mt-2 text-[13px] text-[var(--c-text-2nd)]">${esc(msg)}</p>
  </div>`;
}

/* ────────────────────────── main app ────────────────────────── */

const FILTERS: { key: FilterKey; label: string }[] = [
  { key: "all", label: "全部" },
  { key: "good", label: "优秀 ≥85" },
  { key: "pass", label: "合格" },
  { key: "fail", label: "不及格" },
];

export function renderApp(p: GradePayload, plaintextWarning: boolean): string {
  const n = p.courses.length;
  const failN = p.courses.filter((c) => c.scoreClass === "fail").length;
  const goodN = p.courses.filter((c) => c.scoreClass === "g").length;
  const scores = p.courses
    .map((c) => scoreOf(c.grade))
    .filter((v): v is number => v !== null);
  const maxScore = scores.length ? Math.max(...scores) : null;

  return `
  ${
    plaintextWarning
      ? `<div class="glass glass--sm rise mb-4 px-4 py-3 text-[12px] text-amber-300/90">⚠️ 未设置 GRADES_KEY——本页为明文，公开可见。建议在 Secrets 配置密钥启用端到端加密。</div>`
      : ""
  }
  <header class="rise">
    <p class="text-[12px] font-medium tracking-wide text-[var(--c-blue)]">${p.firstRun ? "🎉 首次运行 · 本学期全部已出成绩" : "📊 成绩已更新"}</p>
    <h1 class="mt-1 text-[22px] font-bold tracking-tight">${esc(p.semester)}</h1>
    <p class="mt-0.5 text-[12px] text-[var(--c-text-2nd)]">更新于 ${esc(p.updatedAt)}</p>
  </header>

  <!-- 统计 -->
  <section class="rise mt-5 grid grid-cols-2 gap-3" style="animation-delay:.05s">
    ${statCard("累计 GPA", p.gpa, "text-[var(--c-green)]")}
    ${statCard("百分制均分", p.pctGpa, "text-white")}
    ${statCard("已出成绩", `${n} <span class="text-[13px] font-normal text-[var(--c-text-2nd)]">门</span>`, "text-white")}
    ${
      failN > 0
        ? statCard("不及格", `${failN} <span class="text-[13px] font-normal text-[var(--c-text-2nd)]">门</span>`, "text-[var(--c-red)]")
        : statCard(
            maxScore !== null ? "最高分" : "优秀课程",
            maxScore !== null ? String(maxScore) : `${goodN} <span class="text-[13px] font-normal text-[var(--c-text-2nd)]">门</span>`,
            "text-[var(--c-green)]",
          )
    }
  </section>

  <!-- 分数分布 -->
  ${distSection(scores)}

  <!-- 控件 -->
  <section class="rise mt-5 space-y-3" style="animation-delay:.12s">
    <div class="flex gap-2">
      <input id="search" type="search" placeholder="搜索课程 / 教师…"
        class="ctl min-w-0 flex-1 px-4 py-2.5 text-[14px]" />
      <select id="sort" class="ctl shrink-0 px-3 py-2.5 text-[13px]">
        <option value="time">最新提交</option>
        <option value="scoreDesc">分数 高→低</option>
        <option value="scoreAsc">分数 低→高</option>
      </select>
    </div>
    <div id="filters" class="flex flex-wrap gap-2">
      ${FILTERS.map(
        (f, i) =>
          `<button data-filter="${f.key}" class="chip px-3.5 py-1.5 text-[12px] ${i === 0 ? "chip--on" : ""}">${f.label}</button>`,
      ).join("")}
    </div>
  </section>

  <!-- 课程列表（动态区） -->
  <section id="course-list" class="mt-4 space-y-3"></section>

  <!-- 待公布 -->
  ${pendingSection(p)}

  <footer class="mt-auto pt-8 text-center text-[11px] text-[var(--c-text-2nd)]">
    <p>端到端加密 · 仅持有密钥者可见</p>
    <p class="mt-1">${esc(p.copyright)}</p>
  </footer>`;
}

function statCard(label: string, valueHTML: string, valueCls: string): string {
  return `
  <div class="glass glass--sm px-4 py-3.5">
    <p class="text-[11px] text-[var(--c-text-2nd)]">${label}</p>
    <p class="mt-1 text-[22px] font-bold tabular-nums tracking-tight ${valueCls}">${valueHTML}</p>
  </div>`;
}

function distSection(scores: number[]): string {
  if (scores.length === 0) return "";
  const buckets = [
    { label: "90+", min: 90, max: 101, color: "var(--c-green)" },
    { label: "80-89", min: 80, max: 90, color: "var(--c-blue)" },
    { label: "70-79", min: 70, max: 80, color: "#bf5af2" },
    { label: "60-69", min: 60, max: 70, color: "#ff9f0a" },
    { label: "<60", min: -1, max: 60, color: "var(--c-red)" },
  ];
  const rows = buckets
    .map((b) => ({ ...b, n: scores.filter((s) => s >= b.min && s < b.max).length }))
    .filter((b) => b.n > 0);
  if (rows.length === 0) return "";
  const maxN = Math.max(...rows.map((r) => r.n));
  return `
  <section class="glass glass--sm rise mt-3 px-4 py-3.5" style="animation-delay:.08s">
    <p class="text-[11px] text-[var(--c-text-2nd)]">分数分布</p>
    <div class="mt-2.5 space-y-2">
      ${rows
        .map(
          (r) => `
      <div class="flex items-center gap-2.5">
        <span class="w-11 shrink-0 text-right text-[11px] tabular-nums text-[var(--c-text-2nd)]">${r.label}</span>
        <div class="bar flex-1"><i style="width:${Math.round((r.n / maxN) * 100)}%;background:${r.color}"></i></div>
        <span class="w-5 shrink-0 text-[11px] tabular-nums text-[var(--c-text-2nd)]">${r.n}</span>
      </div>`,
        )
        .join("")}
    </div>
  </section>`;
}

function pendingSection(p: GradePayload): string {
  if (!p.pending || p.pending.length === 0) return "";
  return `
  <section class="rise mt-6" style="animation-delay:.18s">
    <h2 class="px-1 text-[12px] font-medium text-[var(--c-text-2nd)]">未公布成绩 · ${p.pending.length} 门</h2>
    <div class="glass glass--sm mt-2 divide-y divide-white/[0.06] px-4">
      ${p.pending
        .map(
          (c) => `
      <div class="flex items-center justify-between py-3">
        <span class="text-[14px] text-white/80">${esc(c.name)}</span>
        <span class="ml-3 shrink-0 text-[12px] text-[var(--c-text-2nd)]">${esc(c.teacher)}</span>
      </div>`,
        )
        .join("")}
    </div>
  </section>`;
}

/* ────────────────────────── course list（动态刷新） ────────────────────────── */

export function renderCourseList(
  courses: CourseRow[],
  q: string,
  sort: SortKey,
  filter: FilterKey,
): string {
  let rows = courses.slice();

  const query = q.trim().toLowerCase();
  if (query) {
    rows = rows.filter(
      (c) =>
        c.course.toLowerCase().includes(query) ||
        c.teacher.toLowerCase().includes(query),
    );
  }
  if (filter === "good") rows = rows.filter((c) => c.scoreClass === "g");
  else if (filter === "fail") rows = rows.filter((c) => c.scoreClass === "fail");
  else if (filter === "pass") rows = rows.filter((c) => c.scoreClass !== "fail");

  if (sort === "scoreDesc" || sort === "scoreAsc") {
    const dir = sort === "scoreDesc" ? -1 : 1;
    rows.sort((a, b) => {
      const sa = scoreOf(a.grade) ?? -1;
      const sb = scoreOf(b.grade) ?? -1;
      return (sa - sb) * dir;
    });
  } // "time"：保持 Go 侧按提交时间降序的原始顺序

  if (rows.length === 0) {
    return `<div class="glass glass--sm px-4 py-8 text-center text-[13px] text-[var(--c-text-2nd)]">没有匹配的课程</div>`;
  }

  return rows
    .map(
      (c) => `
  <article class="glass px-5 py-4">
    <div class="flex items-start justify-between gap-3">
      <div class="min-w-0">
        <h3 class="truncate text-[15px] font-semibold leading-snug">${esc(c.course)}</h3>
        <p class="mt-1 text-[12px] text-[var(--c-text-2nd)]">${esc(c.teacher)}${c.teacher && c.time ? " · " : ""}${esc(c.time)}</p>
      </div>
      <span class="shrink-0 text-[22px] font-bold tabular-nums tracking-tight ${scoreCls(c)}">${esc(c.grade)}</span>
    </div>
  </article>`,
    )
    .join("");
}
