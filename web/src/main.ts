/**
 * 入口：加载 payload.json → 判定状态（占位 / 明文 / 加密）→ 解密 → 渲染富 UI。
 * 密钥来源优先级：URL # 片段 > sessionStorage > 手动输入。
 */
import "./style.css";
import { decryptPayload } from "./crypto";
import type { Envelope, GradePayload } from "./types";
import {
  initLiquidGlass,
  renderApp,
  renderError,
  renderKeyPrompt,
  renderWaiting,
} from "./ui";

const app = document.getElementById("app")!;
const KEY_CACHE = "zfcs.key";

async function boot(): Promise<void> {
  let env: Envelope;
  try {
    const res = await fetch(`/payload.json?t=${Date.now()}`, {
      cache: "no-store",
    });
    if (!res.ok) throw new Error(`payload.json HTTP ${res.status}`);
    env = (await res.json()) as Envelope;
  } catch (e) {
    app.innerHTML = renderError(e instanceof Error ? e.message : String(e));
    return;
  }

  // ① Actions 尚未生成数据 → 占位
  if (env.placeholder || (!env.ct && !env.plain)) {
    app.innerHTML = renderWaiting();
    return;
  }

  // ② 明文回退（GRADES_KEY 未配置）
  if (env.plain) {
    mount(env.plain, true);
    return;
  }

  // ③ 加密载荷：取密钥并解密
  const fromHash = decodeURIComponent(location.hash.slice(1));
  const cached = sessionStorage.getItem(KEY_CACHE) ?? "";
  const key = fromHash || cached;

  if (!key) {
    promptKey(env, false);
    return;
  }
  await tryDecrypt(env, key, fromHash !== "");
}

async function tryDecrypt(
  env: Envelope,
  key: string,
  fresh: boolean,
): Promise<void> {
  try {
    const json = await decryptPayload(env.ct!, key);
    sessionStorage.setItem(KEY_CACHE, key);
    mount(JSON.parse(json) as GradePayload, false);
  } catch {
    if (!fresh) sessionStorage.removeItem(KEY_CACHE);
    promptKey(env, true);
  }
}

function promptKey(env: Envelope, wrongKey: boolean): void {
  app.innerHTML = renderKeyPrompt(wrongKey);
  const form = document.getElementById("key-form") as HTMLFormElement;
  const input = document.getElementById("key-input") as HTMLInputElement;
  form.addEventListener("submit", (ev) => {
    ev.preventDefault();
    const k = input.value.trim();
    if (k) void tryDecrypt(env, k, true);
  });
  input.focus();
}

/* ────────────────────────── mount（单卡片，无交互） ────────────────────────── */

function mount(payload: GradePayload, plaintextWarning: boolean): void {
  app.innerHTML = renderApp(payload, plaintextWarning);
  initLiquidGlass();
}

void boot();
