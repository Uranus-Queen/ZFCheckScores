/**
 * Cloudflare Worker 入口（Hono）
 *
 * 架构：GitHub Actions（Go 抓成绩 → AES-256-GCM 加密 → wrangler kv put）
 *       与 Worker 部署完全解耦——数据更新只写 KV（免费 1k 写/天），
 *       Worker 壳只有代码变更时才重新 deploy，零构建额度消耗。
 *
 * 路由：
 *   /payload.json  → 从 KV 读加密信封（成绩本体始终是密文，解密只发生在
 *                    浏览器端，密钥在 URL # 片段，永远不会到达这里）
 *   /api/health    → 健康检查
 *   /api/meta      → 非敏感元信息（是否有数据、是否加密、更新时间）
 *   其余           → 静态资产（Vite 构建的 SPA，assets binding 直接处理）
 */
import { Hono } from "hono";

/** KV 中存放加密信封的键名。 */
const PAYLOAD_KEY = "payload";

/** KV 尚无数据时返回的占位信封（与 Go 侧 BuildEnvelope 契约一致）。 */
const PLACEHOLDER = JSON.stringify({ v: 1, placeholder: true });

interface EnvelopeMeta {
  v?: number;
  placeholder?: boolean;
  ct?: string;
  alg?: string;
  updatedAt?: string;
  plain?: unknown;
}

/* 最小运行时类型声明（避免额外引入 @cloudflare/workers-types 依赖）。 */
interface KVNamespaceLite {
  get(key: string): Promise<string | null>;
}
interface FetcherLite {
  fetch(request: Request): Promise<Response>;
}

type Bindings = {
  PAYLOAD_KV: KVNamespaceLite;
  ASSETS: FetcherLite;
};

const app = new Hono<{ Bindings: Bindings }>();

/** 加密信封：永远 no-store，保证成绩更新即时可见。 */
app.get("/payload.json", async (c) => {
  const body = (await c.env.PAYLOAD_KV.get(PAYLOAD_KEY)) ?? PLACEHOLDER;
  return c.body(body, 200, {
    "content-type": "application/json; charset=utf-8",
    "cache-control": "no-store",
  });
});

/** 健康检查：确认 Worker 在线。 */
app.get("/api/health", (c) =>
  c.json({ ok: true, service: "zfcheckscores-web", ts: new Date().toISOString() }),
);

/** 页面元信息：不含任何成绩明文。 */
app.get("/api/meta", async (c) => {
  const raw = await c.env.PAYLOAD_KV.get(PAYLOAD_KEY);
  if (!raw) return c.json({ ready: false });
  let env: EnvelopeMeta;
  try {
    env = JSON.parse(raw) as EnvelopeMeta;
  } catch {
    return c.json({ ready: false });
  }
  return c.json({
    ready: !env.placeholder && (!!env.ct || !!env.plain),
    encrypted: !!env.ct,
    alg: env.alg ?? null,
    updatedAt: env.updatedAt ?? null,
  });
});

app.all("/api/*", (c) => c.json({ error: "not found" }, 404));

/** 其余请求兜底转给静态资产（正常情况下 assets 已在 Worker 之前处理）。 */
app.all("*", (c) => c.env.ASSETS.fetch(c.req.raw));

export default app;
