# 正方教务管理系统成绩推送

<img src="https://raw.githubusercontent.com/NianBroken/ZFCheckScores/main/img/7.jpg" style="zoom:60%;" />

## 简介

自动检测正方教务系统成绩更新，通过微信实时推送通知，并把完整成绩卡片托管到 Cloudflare Workers 自托管页面。

- **Go 实现**，编译为单二进制，CI 中无需安装依赖
- 期末季（12/1/2、6/7/8 月）每 30 分钟检测一次，仅在本学期成绩变化时推送；平时可手动触发
- 通知走 **Server酱**（仅摘要 + 链接），完整成绩页自托管在 **Cloudflare Workers**（Hono + 静态资产 + KV）
- **数据与部署解耦**：成绩更新只写 1 次 Workers KV，不触发任何站点构建——免费额度完全无压力
- 成绩页是 **Hono + TypeScript + Tailwind（Vite）** 单页应用：苹果液态玻璃风格的单卡片，成绩仅白 / 红两色（挂科标红），无搜索 / 排序 / 筛选等复杂交互
- **端到端加密**：仓库里只有 AES-256-GCM 密文，解密只发生在浏览器（密钥在链接 `#` 片段）
- 支持 **Cookie 登录**（复用浏览器会话，绕过验证码）

## 测试环境

正方教务管理系统 V8.0、V9.0。

<img src="https://raw.githubusercontent.com/NianBroken/ZFCheckScores/main/img/9.png" style="zoom:60%;" />

## 功能

- 成绩卡片展示课程、任课教师、时间与分数，挂科标红
- 自动计算 GPA 和百分制 GPA
- 显示未公布成绩的课程
- 自托管成绩页：苹果液态玻璃暗色风格，单卡片展示已出成绩、GPA 统计、未公布成绩，手机浏览友好
- 支持 Cookie 登录，绕过教务系统验证码

## 使用方法

### 1. Fork 仓库

`Fork` → `Create fork`

### 2. 开启工作流权限

`Settings` → `Actions` → `General` → `Workflow permissions` → `Read and write permissions` → `Save`

### 3. 添加 Secrets

`Settings` → `Secrets and variables` → `Actions` → `Secrets` → `Repository secrets`

| Name               | 例子                                | 说明                              |
| ------------------ | ----------------------------------- | --------------------------------- |
| URL                | https://jwgl.njtech.edu.cn/jwglxt   | 教务系统地址（建议带 `/jwglxt` 上下文路径）|
| USERNAME           | 2023210333027                       | 学号                              |
| PASSWORD           | Y3xhaCkb5PZ4                        | 密码                              |
| SERVERCHAN_SENDKEY | SCT386139Txxxxxxxxxxxxxxxxxxxxxxxxx | [Server酱 SendKey]，成绩更新通知。**必填**（替代原 Showdoc）|
| GRADES_DOMAIN      | grades.example.com                  | 自托管成绩页的自定义域名。**与 `GRADES_KEY` 成对设置**（两者都填或都留空）；通知里的「查看完整卡片」链接指向 `https://<域名>/#<密钥>` |
| GRADES_KEY         | `s3cr3t-9x2k`                        | 成绩页**端到端加密密钥**（AES-256-GCM 密钥，由它派生）。成对设置后，成绩卡片在仓库里是**密文**，浏览器用链接里的 `#<密钥>` 片段实时解密显示；片段不上服务器、不进仓库，公开仓库也泄露不了明文。链接即凭证，请勿外泄 |
| COOKIES            | `{"JSESSIONID":"...","route":"..."}` | **可选**。浏览器 Cookie，跳过验证码 |
| CLOUDFLARE_API_TOKEN | `xxxxxxxx`                        | 启用自托管成绩页时**必填**：Actions 用它把加密成绩信封写入 Workers KV（`wrangler kv key put`）。创建：Cloudflare 控制台 → `My Profile` → `API Tokens` → `Create Token`，权限给 **Account / Workers KV Storage / Edit** 即可 |

### Cookie 登录

若账号密码受验证码限制，可使用已登录浏览器的 Cookie 直接复用会话：

1. 浏览器登录教务系统。
2. `F12` → `Application` → `Cookies`，复制 `JSESSIONID`（及 `route`）。
3. 填入仓库 Secrets 的 `COOKIES`，格式：`{"JSESSIONID":"xxx","route":"yyy"}` 或 `JSESSIONID=xxx; route=yyy`。
4. 设置 `COOKIES` 后无需再填 `USERNAME` / `PASSWORD`。

### 4. 启用 Actions

`Actions` → `CheckScores` → `Enable workflow`

### 5. 运行

`Actions` → `CheckScores` → `Run workflow`。定时任务只在期末季（12/1/2、6/7/8 月）每 30 分钟运行；其余月份需要时手动触发，或修改 `.github/workflows/main.yml` 里的 cron。

### 6. 部署成绩页到 Cloudflare Workers（自托管）

成绩页是 `web/` 下的 **Hono + TypeScript + Tailwind（Vite）单页应用**，部署为一个 **Cloudflare Worker**（静态资产 + KV）。核心设计：**数据更新与站点部署完全解耦**——

- **Worker 壳**（页面代码）只有代码变更时才 `wrangler deploy` 一次；
- **成绩数据**由 Actions 每轮 `wrangler kv key put` 写进 **Workers KV**（AES-256-GCM 密文），Worker 运行时从 KV 读取。
- 免费额度账：Workers 10 万请求/天、KV 10 万读 + 1000 写/天，30 分钟一轮也只 48 写/天，**零构建额度消耗**。

部署步骤（本地一次性）：

1. **创建 KV namespace**：`cd web && npx wrangler kv namespace create PAYLOAD_KV`，把输出的 `id` 填进 `web/wrangler.jsonc` 的 `kv_namespaces[0].id`；同时把你的 `account_id` 填进去。
2. **部署 Worker**：`cd web && npm install && npm run build && npx wrangler deploy`（首次会引导浏览器登录）。
3. **自定义域名**：`web/wrangler.jsonc` 的 `routes` 已配置 route 模式（`grades.example.com/*` + `zone_name`），改成你的域名后重新 deploy。要求该域名的 DNS 记录在同一 Cloudflare 账号下且为**橙云代理**状态。若域名此前绑定在某个 Pages 项目上，需先在 Pages 后台解绑。
4. **CI 数据投递**：在 GitHub Secrets 添加 `CLOUDFLARE_API_TOKEN`（Workers KV Edit 权限），并把 `.github/workflows/main.yml` 中 KV 上传步骤的 `--namespace-id` 与 `CLOUDFLARE_ACCOUNT_ID` 改成你自己的。
5. **访问方式（免登录、端到端加密）**：成绩数据以 **AES-256-GCM 密文**存于 KV。解密密钥只存在于通知链接的片段里：`https://<域名>/#<GRADES_KEY>`。浏览器从 `#` 片段取出密钥、本地解密渲染——片段**不会发到服务器、不进仓库、不进 KV**。「链接即凭证」，请只发给自己。没带片段直接打开页面时，会显示密钥输入框（密钥同样只留在浏览器）。
   > 为什么不用「密码查询参数 `?key=` 只做访问校验」？那种做法 JS 只是校验口令、明文仍在页面里，看源码即可绕过。这里是**端到端加密**：明文只在浏览器用密钥解出，服务端只有密文，安全得多。

> KV 无数据时 Worker 返回占位信封（页面显示"成绩页生成中"），首次成功运行 Actions 后被加密信封覆盖、**即时生效**（`/payload.json` 由 Worker 返回并带 `Cache-Control: no-store`，无需任何重新部署）。Worker 还暴露 `/api/health`、`/api/meta`（Hono），只含"是否有数据、何时更新"等非敏感元信息。

## 程序逻辑

1. 登录教务系统（Cookie 优先 → 账号密码 RSA 加密）
2. 判定当前学期（已选课程优先 → 日历兜底）
3. 抓取本学期成绩，MD5 哈希写入 `data/grade.txt`
4. 与上一次快照 `data/old_grade.txt` 比对
5. 成绩变化或首次运行时，通过 **Server酱** 推送微信通知（摘要 + 自托管页链接），并把结构化成绩 JSON **AES-256-GCM 加密**后写入 `data/payload.json`（密钥在链接 `#<GRADES_KEY>` 片段），随后 workflow 用 `wrangler kv key put` 把它推进 **Workers KV**，成绩页即时更新（无需重新构建部署）

## 本地运行

```bash
go run .
```

或编译后运行：

```bash
go build -ldflags="-s -w" -o zfcheckscores .
URL=... USERNAME=... PASSWORD=... SERVERCHAN_SENDKEY=... GRADES_DOMAIN=grades.example.com GRADES_KEY=s3cr3t-9x2k ./zfcheckscores
```

成绩页前端（`web/`）本地开发：

```bash
cd web
npm install
npm run dev            # Vite 开发服务器
npm run build          # tsc 类型检查 + 产出 web/dist
npx wrangler dev       # 本地完整模拟 Worker（静态资产 + KV + /api/*）
npx wrangler deploy    # 部署到 Cloudflare Workers
```

## 故障排查

程序运行失败时（Actions 显示红色 ✗，或推送内容出现「FATAL」），失败原因与建议会同时写入 **Actions 运行摘要（Step Summary）**，无需翻原始日志。常见分类：

| 现象 / 原因 | 诊断提示关键词 | 处理建议 |
| --- | --- | --- |
| 登录被验证码 / WAF 拦截（GitHub IP 偶发） | `验证码 / WAF 拦截` | 在 Secrets 设置 `COOKIES`（浏览器登录后复制 `JSESSIONID`、`route`）复用会话；或改从校园网 / 信任 IP 触发 |
| 用户名或密码错误 | `用户名或密码错误` | 检查 `USERNAME` / `PASSWORD` Secret |
| 会话过期 / 未登录 | `会话已过期` | 设置 `COOKIES` 复用会话，或重新运行刷新登录 |
| URL 缺少 `/jwglxt` 上下文路径 | `系统维护页面` | 将 `URL` 改为 `https://jwgl.njtech.edu.cn/jwglxt`（代码已兜底，但显式带上更稳） |
| 教务系统维护 / 网络不可达 | `不可达` | 稍后重试；持续则确认 URL 与系统开放状态 |
| 成绩接口网络请求失败 | `网络请求失败` | 网络抖动，下一轮通常自愈；持续则检查 URL |

> 提示：验证码 / WAF 是**非校园网 IP 的常态限制**，不是程序 bug。配置 `COOKIES` 后基本可长期稳定运行。

## 许可证

Apache-2.0

## 致谢

- [openschoolcn/zfn_api](https://github.com/openschoolcn/zfn_api) — 正方 API 参考
- [NianBroken/ZFCheckScores](https://github.com/NianBroken/ZFCheckScores) — 原始 Python 项目

---

Copyright © 2026 IKAROS. All rights reserved.
