/**
 * Hono API · Cloudflare Pages Functions
 * 路由挂载在 /api/* 下。只暴露非敏感元信息——成绩本体始终是密文，
 * 解密只发生在浏览器端（密钥在 URL # 片段，永远不会到达这里）。
 */
import { Hono } from "hono";
import { handle } from "hono/cloudflare-pages";

interface EnvelopeMeta {
  v?: number;
  placeholder?: boolean;
  ct?: string;
  alg?: string;
  updatedAt?: string;
  plain?: unknown;
}

const app = new Hono().basePath("/api");

/** 健康检查：确认 Functions 在线。 */
app.get("/health", (c) =>
  c.json({ ok: true, service: "zfcheckscores-web", ts: new Date().toISOString() }),
);

/** 页面元信息：是否已有数据、是否加密、最近更新时间（不含任何成绩明文）。 */
app.get("/meta", async (c) => {
  const res = await fetch(new URL("/payload.json", c.req.url), {
    headers: { "cache-control": "no-store" },
  });
  if (!res.ok) return c.json({ ready: false }, 200);
  const env = (await res.json()) as EnvelopeMeta;
  return c.json({
    ready: !env.placeholder && (!!env.ct || !!env.plain),
    encrypted: !!env.ct,
    alg: env.alg ?? null,
    updatedAt: env.updatedAt ?? null,
  });
});

app.notFound((c) => c.json({ error: "not found" }, 404));

export const onRequest = handle(app);
