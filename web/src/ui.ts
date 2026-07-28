import type { CourseRow, GradePayload, PendingRow } from "./types";

/* ────────────────────────── helpers ────────────────────────── */

export function esc(s: string): string {
  return s
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

/* ────────────────────────── icons（描边 SVG，不用 emoji） ────────────────────────── */

const SVG_ATTRS = `xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"`;

/** 旋转加载环（等待态）——motion-reduce 时静止 */
const ICON_SPINNER = `
  <svg ${SVG_ATTRS} class="h-7 w-7 animate-spin motion-reduce:animate-none" style="animation-duration:1.1s">
    <circle cx="12" cy="12" r="9" opacity="0.22"/>
    <path d="M21 12a9 9 0 0 0-9-9"/>
  </svg>`;

/** 锁（密钥态） */
const ICON_LOCK = `
  <svg ${SVG_ATTRS} class="h-7 w-7">
    <rect x="4.5" y="10.5" width="15" height="10" rx="2.5"/>
    <path d="M8 10.5V7.5a4 4 0 0 1 8 0v3"/>
    <circle cx="12" cy="15.5" r="1.1" fill="currentColor" stroke="none"/>
  </svg>`;

/** 警示三角（错误态） */
const ICON_ALERT = `
  <svg ${SVG_ATTRS} class="h-7 w-7">
    <path d="m21.73 18-8-14a2 2 0 0 0-3.46 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z"/>
    <path d="M12 9v4"/>
    <path d="M12 17h.01"/>
  </svg>`;

/** 液态玻璃圆形光环容器：与成绩卡片同语言的玻璃徽章 */
function orb(svg: string, tone: "blue" | "red" | "amber"): string {
  return `<div class="orb orb--${tone} mx-auto">${svg}</div>`;
}

/* ────────────────────────── static states ────────────────────────── */

export function renderWaiting(): string {
  return `
  <div class="fixed inset-0 z-[1] flex flex-col items-center justify-center overflow-hidden px-4 text-center">
    <div class="glass rise w-full max-w-sm px-7 py-10">
      ${orb(ICON_SPINNER, "blue")}
      <h1 class="mt-5 text-lg font-semibold tracking-tight">成绩页生成中</h1>
      <p class="mt-2 text-[13px] leading-relaxed text-[var(--c-text-2nd)]">
        首次成功运行 GitHub Actions 后，这里会自动生成本学期的成绩卡片，并实时随成绩更新。
      </p>
      <div class="mx-auto mt-6 h-px w-2/3 bg-gradient-to-r from-transparent via-white/15 to-transparent"></div>
      <p class="mt-4 text-[11px] tracking-wide text-[var(--c-text-2nd)]">数据端到端加密 · 仅持有密钥者可见</p>
    </div>
    <p class="mt-6 text-[11px] tracking-wide text-[var(--c-text-2nd)]/80">Powered by ZFCheckScores</p>
  </div>`;
}

export function renderKeyPrompt(wrongKey: boolean): string {
  return `
  <div class="fixed inset-0 z-[1] flex flex-col items-center justify-center overflow-hidden px-4 text-center">
    <div class="glass rise w-full max-w-sm px-7 py-9">
      ${orb(ICON_LOCK, wrongKey ? "red" : "blue")}
      <h1 class="mt-5 text-lg font-semibold tracking-tight">${wrongKey ? "解密失败" : "需要访问密钥"}</h1>
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
          class="shrink-0 cursor-pointer rounded-xl bg-[var(--c-blue)] px-4 py-2.5 text-[14px] font-medium text-white transition hover:brightness-110 active:scale-95">
          解锁
        </button>
      </form>
    </div>
    <p class="mt-6 text-[11px] tracking-wide text-[var(--c-text-2nd)]/80">端到端加密 · 密钥不会发送到服务器</p>
  </div>`;
}

export function renderError(msg: string): string {
  return `
  <div class="fixed inset-0 z-[1] flex flex-col items-center justify-center overflow-hidden px-4 text-center">
    <div class="glass rise w-full max-w-sm px-7 py-10">
      ${orb(ICON_ALERT, "amber")}
      <h1 class="mt-5 text-lg font-semibold tracking-tight">页面加载失败</h1>
      <p class="mt-2 text-[13px] text-[var(--c-text-2nd)]">${esc(msg)}</p>
    </div>
  </div>`;
}

/* ────────────────────────── main app（单卡片，与 preview_card.html 同源） ────────────────────────── */

export function renderApp(p: GradePayload, plaintextWarning: boolean): string {
  return `
  ${plaintextWarning ? `<div class="gc-plain">未设置 GRADES_KEY——本页为明文，公开可见。建议在 Secrets 配置密钥启用端到端加密。</div>` : ""}
  <header class="gc" data-glass style="--i:0">
    <div class="gc__shadow"></div><div class="gc__warp"></div><div class="gc__border"></div><div class="gc__border-2"></div>
    <div class="gc__content gc-title">
      <h1 class="sem">${esc(p.semester)}</h1>
      <p class="subtitle">成绩推送</p>
    </div>
  </header>

  <section class="gc" data-glass style="--i:1">
    <div class="gc__shadow"></div><div class="gc__warp"></div><div class="gc__border"></div><div class="gc__border-2"></div>
    <div class="gc__content">
      <h2 class="gc-header">已出成绩</h2>
      <ul class="gc-list">
        ${p.courses.map(courseRow).join("")}
      </ul>
    </div>
  </section>

  <section class="gc" data-glass style="--i:2">
    <div class="gc__shadow"></div><div class="gc__warp"></div><div class="gc__border"></div><div class="gc__border-2"></div>
    <div class="gc__content">
      <h2 class="gc-header">GPA 统计</h2>
      <dl class="gc-grid">
        <div class="gc-cell"><dd class="gc-val">${esc(p.gpa)}</dd><dt class="gc-lbl">累计 GPA</dt></div>
        <div class="gc-cell"><dd class="gc-val">${esc(p.pctGpa)}</dd><dt class="gc-lbl">百分制均分</dt></div>
      </dl>
    </div>
  </section>

  ${
    p.pending && p.pending.length
      ? `<section class="gc" data-glass style="--i:3">
    <div class="gc__shadow"></div><div class="gc__warp"></div><div class="gc__border"></div><div class="gc__border-2"></div>
    <div class="gc__content">
      <h2 class="gc-header">未公布成绩</h2>
      <ul class="gc-plist">
        ${p.pending.map(pendingItem).join("")}
      </ul>
    </div>
  </section>`
      : ""
  }

  <footer class="gc-footer">${esc(p.copyright)}</footer>`;
}

function courseRow(c: CourseRow): string {
  const cls = c.scoreClass === "fail" ? " fail" : "";
  return `
  <li class="gc-row">
    <div class="gc-info">
      <div class="gc-name">${esc(c.course)}</div>
      <div class="gc-meta">${esc(c.teacher)}${c.teacher && c.time ? " · " : ""}${esc(c.time)}</div>
    </div>
    <div class="gc-right">
      <span class="gc-score${cls}">${esc(c.grade)}</span>
    </div>
  </li>`;
}

function pendingItem(c: PendingRow): string {
  return `
  <li class="gc-pitem">
    <span class="dot"></span>
    <span class="cn">${esc(c.name)}</span>
    <span class="tn">${esc(c.teacher)}</span>
  </li>`;
}

/* ────────────────────────── 液态玻璃折射（与 preview_card.html 同源） ────────────────────────── */

/** 为成绩卡片套用 IKAROS 液态玻璃折射滤镜 + 鼠标光向跟随。 */
export function initLiquidGlass(): void {
  const F = "liquid-glass";
  const svg = `<svg style="position:absolute;width:0;height:0;overflow:hidden" aria-hidden="true"><defs><filter id="${F}" x="-30%" y="-30%" width="160%" height="160%" color-interpolation-filters="sRGB"><feImage id="disp-map" x="0" y="0" width="100%" height="100%" result="MAP" preserveAspectRatio="none" href=""/><feColorMatrix in="MAP" type="matrix" values="0.3 0.3 0.3 0 0 0.3 0.3 0.3 0 0 0.3 0.3 0.3 0 0 0 0 0 1 0" result="EI"/><feComponentTransfer in="EI" result="EM"><feFuncA type="discrete" tableValues="0 0.1 1"/></feComponentTransfer><feOffset in="SourceGraphic" dx="0" dy="0" result="C"/><feDisplacementMap in="SourceGraphic" in2="MAP" scale="-50" xChannelSelector="R" yChannelSelector="B" result="RD"/><feColorMatrix in="RD" type="matrix" values="1 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 1 0" result="R"/><feDisplacementMap in="SourceGraphic" in2="MAP" scale="-48" xChannelSelector="R" yChannelSelector="B" result="GD"/><feColorMatrix in="GD" type="matrix" values="0 0 0 0 0 0 1 0 0 0 0 0 0 0 0 0 0 1 0" result="G"/><feDisplacementMap in="SourceGraphic" in2="MAP" scale="-46" xChannelSelector="R" yChannelSelector="B" result="BD"/><feColorMatrix in="BD" type="matrix" values="0 0 0 0 0 0 0 0 0 0 0 1 0 0 0 0 0 1 0" result="B"/><feBlend in="G" in2="B" mode="screen" result="GB"/><feBlend in="R" in2="GB" mode="screen" result="RGB"/><feGaussianBlur in="RGB" stdDeviation="0.3" result="AB"/><feComposite in="AB" in2="EM" operator="in" result="EE"/><feComponentTransfer in="EM" result="IM"><feFuncA type="table" tableValues="1 0"/></feComponentTransfer><feComposite in="C" in2="IM" operator="in" result="CC"/><feComposite in="EE" in2="CC" operator="over"/></filter></defs></svg>`;

  const genMap = (): string => {
    const S = 256;
    const c = document.createElement("canvas");
    c.width = S;
    c.height = S;
    const ctx = c.getContext("2d");
    if (!ctx) return "";
    const img = ctx.createImageData(S, S);
    const d = img.data;
    for (let y = 0; y < S; y++) {
      for (let x = 0; x < S; x++) {
        const i = (y * S + x) * 4;
        const nx = (x / S - 0.5) * 2;
        const ny = (y / S - 0.5) * 2;
        const dist = Math.min(1, Math.sqrt(nx * nx + ny * ny));
        const edge = Math.pow(dist, 2.2);
        const ang = Math.atan2(ny, nx);
        const str = edge * 90;
        d[i] = 128 + Math.cos(ang) * str;
        d[i + 1] = 128;
        d[i + 2] = 128 + Math.sin(ang) * str;
        d[i + 3] = 255;
      }
    }
    ctx.putImageData(img, 0, 0);
    return c.toDataURL();
  };

  const setMap = (): void => {
    const fe = document.getElementById("disp-map");
    if (!fe) return;
    const u = genMap();
    fe.setAttribute("href", u);
    fe.setAttributeNS("http://www.w3.org/1999/xlink", "xlink:href", u);
  };

  const applyFilter = (): void => {
    const isChromium =
      (/Chrome/.test(navigator.userAgent) && !/Edg/.test(navigator.userAgent)) ||
      /Edg/.test(navigator.userAgent);
    if (!isChromium) return;
    document.querySelectorAll(".gc__warp").forEach((w) => {
      if (!w.classList.contains("has-filter")) w.classList.add("has-filter");
    });
  };

  const bindMouse = (): void => {
    if (window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    document.querySelectorAll("[data-glass]").forEach((el) => {
      const e = el as HTMLElement;
      if (e.getAttribute("data-glass-bound")) return;
      e.setAttribute("data-glass-bound", "1");
      let tx = 0.5,
        ty = 0.5,
        cx = 0.5,
        cy = 0.5,
        raf: number | null = null;
      const loop = (): void => {
        cx += (tx - cx) * 0.12;
        cy += (ty - cy) * 0.12;
        e.style.setProperty("--mx", cx.toFixed(3));
        e.style.setProperty("--my", cy.toFixed(3));
        if (Math.abs(tx - cx) > 0.001 || Math.abs(ty - cy) > 0.001) {
          raf = requestAnimationFrame(loop);
        } else {
          raf = null;
        }
      };
      const setTarget = (x: number, y: number): void => {
        const r = e.getBoundingClientRect();
        tx = Math.max(0, Math.min(1, (x - r.left) / r.width));
        ty = Math.max(0, Math.min(1, (y - r.top) / r.height));
        if (!raf) raf = requestAnimationFrame(loop);
      };
      const reset = (): void => {
        tx = 0.5;
        ty = 0.5;
        if (!raf) raf = requestAnimationFrame(loop);
      };
      e.addEventListener("mousemove", (ev) => setTarget(ev.clientX, ev.clientY));
      e.addEventListener("mouseleave", reset);
      e.addEventListener(
        "touchstart",
        (ev) => {
          if (ev.touches.length) setTarget(ev.touches[0].clientX, ev.touches[0].clientY);
        },
        { passive: true },
      );
      e.addEventListener(
        "touchmove",
        (ev) => {
          if (ev.touches.length) setTarget(ev.touches[0].clientX, ev.touches[0].clientY);
        },
        { passive: true },
      );
      e.addEventListener("touchend", reset, { passive: true });
      e.addEventListener("touchcancel", reset, { passive: true });
    });
  };

  if (!document.getElementById(F)) {
    const t = document.createElement("div");
    t.innerHTML = svg;
    const s = t.firstChild;
    if (s && s.nodeType === 1) document.body.appendChild(s);
  }
  setMap();
  applyFilter();
  bindMouse();
}
