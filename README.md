# 正方教务管理系统成绩推送

<img src="https://raw.githubusercontent.com/NianBroken/ZFCheckScores/main/img/7.jpg" style="zoom:60%;" />

## 简介

自动检测正方教务系统成绩更新，通过微信实时推送通知，并把完整成绩卡片托管到 Cloudflare Pages 自托管页面。

- **Go 实现**，编译为单二进制，CI 中无需安装依赖
- 每 30 分钟检测一次，仅在本学期成绩变化时推送
- 通知走 **Server酱**（仅摘要 + 链接），完整成绩页自托管在 **Cloudflare Pages**
- 成绩页是 **Hono + TypeScript + Tailwind（Vite）** 单页应用：毛玻璃风格，支持搜索、排序、筛选、分数分布图
- **端到端加密**：仓库里只有 AES-256-GCM 密文，解密只发生在浏览器（密钥在链接 `#` 片段）
- 支持 **Cookie 登录**（复用浏览器会话，绕过验证码）

## 测试环境

正方教务管理系统 V8.0、V9.0。

<img src="https://raw.githubusercontent.com/NianBroken/ZFCheckScores/main/img/9.png" style="zoom:60%;" />

## 功能

- 成绩按提交时间排序，标注提交人
- 自动计算 GPA 和百分制 GPA
- 显示未公布成绩的课程
- 自托管成绩页：毛玻璃暗色风格，统计卡片、分数分布、课程搜索 / 排序 / 筛选，手机浏览友好
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

### Cookie 登录

若账号密码受验证码限制，可使用已登录浏览器的 Cookie 直接复用会话：

1. 浏览器登录教务系统。
2. `F12` → `Application` → `Cookies`，复制 `JSESSIONID`（及 `route`）。
3. 填入仓库 Secrets 的 `COOKIES`，格式：`{"JSESSIONID":"xxx","route":"yyy"}` 或 `JSESSIONID=xxx; route=yyy`。
4. 设置 `COOKIES` 后无需再填 `USERNAME` / `PASSWORD`。

### 4. 启用 Actions

`Actions` → `CheckScores` → `Enable workflow`

### 5. 运行

`Actions` → `CheckScores` → `Run workflow`，之后每 30 分钟自动运行。

### 6. 部署成绩页到 Cloudflare Pages（自托管）

成绩页是 `web/` 下的 **Hono + TypeScript + Tailwind（Vite）单页应用**，由 Cloudflare Pages 通过 **Git 集成**自动构建部署——无需在仓库里放任何 API 令牌。Actions 每次成绩更新时把 **AES-256-GCM 加密**后的成绩 JSON 写入 `web/public/payload.json` 并 push，Cloudflare 随之重新部署；浏览器端用链接 `#` 片段里的密钥本地解密渲染。

1. **连接仓库**：Cloudflare 控制台 → `Workers & Pages` → `Create` → `Pages` → `Connect to Git` → 选择本仓库。
2. **构建设置**：
   - `Root directory` 填 **`web`**
   - `Build command` 填 **`npm run build`**
   - `Build output directory` 填 **`dist`**（即 `web/dist`）
   - 框架预设选 `None`（或 `Vite`），环境变量不需要填。保存后每次 push 到 `main` 自动构建部署。
3. **自定义域名**（成对设置 `GRADES_DOMAIN` + `GRADES_KEY`）：Pages 项目 → `Custom domains` → 添加你的域名（如 `grades.example.com`），按提示在域名服务商处加一条 **CNAME** 记录指向 `*.pages.dev` 提供的目标。然后把域名填到 Secrets 的 `GRADES_DOMAIN`，并设置一个任意长随机串作为 `GRADES_KEY`。
4. **访问方式（免登录、端到端加密）**：成绩数据在仓库里以 **AES-256-GCM 密文**形式存于 `web/public/payload.json`。解密密钥只存在于通知链接的片段里：`https://<域名>/#<GRADES_KEY>`。点开链接后，浏览器从 `#` 片段取出密钥、本地解密并渲染——**无需任何登录步骤**，且片段**不会发到 Cloudflare 服务器、也不会进仓库**，所以即便仓库是公开的，他人读到的也只是密文。「链接即凭证」，请只发给自己，不要公开。没带片段直接打开页面时，会显示密钥输入框（密钥同样只留在浏览器）。
   > 为什么不用「密码查询参数 `?key=` 只做访问校验」？那种做法 JS 只是校验口令、明文仍在页面里，看源码即可绕过。这里是**端到端加密**：明文只在浏览器用密钥解出，仓库里只有密文，安全得多。

> 首跑前 `web/public/payload.json` 是占位信封（页面显示"成绩页生成中"），首次成功运行 Actions 后被加密信封覆盖并自动部署。`web/public/_headers` 已将 `payload.json` 设为 `no-store`，保证成绩更新即时可见。页面还带一个 Hono API（`/api/health`、`/api/meta`，Cloudflare Pages Functions），只暴露"是否有数据、何时更新"等非敏感元信息。

## 程序逻辑

1. 登录教务系统（Cookie 优先 → 账号密码 RSA 加密）
2. 判定当前学期（已选课程优先 → 日历兜底）
3. 抓取本学期成绩，MD5 哈希写入 `data/grade.txt`
4. 与上一次快照 `data/old_grade.txt` 比对
5. 成绩变化或首次运行时，通过 **Server酱** 推送微信通知（摘要 + 自托管页链接），并把结构化成绩 JSON **AES-256-GCM 加密**后写入 `web/public/payload.json`（密钥在链接 `#<GRADES_KEY>` 片段），由 Cloudflare Pages 随 `web/` SPA 一起构建部署

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
npm run dev    # Vite 开发服务器（读取 public/payload.json）
npm run build  # tsc 类型检查 + 产出 web/dist（与 Cloudflare 构建一致）
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

Copyright © 2024 NianBroken. All rights reserved.
