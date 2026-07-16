# 正方教务管理系统成绩推送

<img src="https://raw.githubusercontent.com/NianBroken/ZFCheckScores/main/img/7.jpg" style="zoom:60%;" />

## 简介

自动检测正方教务系统成绩更新，通过微信实时推送通知。

- **Go 实现**，编译为单二进制，CI 中无需安装依赖
- 每 30 分钟检测一次，仅在本学期成绩变化时推送
- 支持 **Cookie 登录**（复用浏览器会话，绕过验证码）

## 测试环境

正方教务管理系统 V8.0、V9.0。

<img src="https://raw.githubusercontent.com/NianBroken/ZFCheckScores/main/img/9.png" style="zoom:60%;" />

## 功能

- 成绩按提交时间排序，标注提交人
- 自动计算 GPA 和百分制 GPA
- 显示未公布成绩的课程
- 推送页面美观简洁，手机浏览友好
- 支持 Cookie 登录，绕过教务系统验证码

## 使用方法

### 1. Fork 仓库

`Fork` → `Create fork`

### 2. 开启工作流权限

`Settings` → `Actions` → `General` → `Workflow permissions` → `Read and write permissions` → `Save`

### 3. 添加 Secrets

`Settings` → `Secrets and variables` → `Actions` → `Secrets` → `Repository secrets`

| Name     | 例子                                | 说明                              |
| -------- | ----------------------------------- | --------------------------------- |
| URL      | https://jwgl.njtech.edu.cn/         | 教务系统地址（根路径，勿加 jwglxt）|
| USERNAME | 2023210333027                       | 学号                              |
| PASSWORD | Y3xhaCkb5PZ4                        | 密码                              |
| TOKEN    | J65KWMBfyDh3YPLpcvm8                | [Showdoc push token]              |
| COOKIES  | `{"JSESSIONID":"...","route":"..."}` | **可选**。浏览器 Cookie，跳过验证码 |

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

## 程序逻辑

1. 登录教务系统（Cookie 优先 → 账号密码 RSA 加密）
2. 判定当前学期（已选课程优先 → 日历兜底）
3. 抓取本学期成绩，MD5 哈希写入 `data/grade.txt`
4. 与上一次快照 `data/old_grade.txt` 比对
5. 成绩变化或首次运行时，通过 Showdoc 推送微信通知

## 本地运行

```bash
go run .
```

或编译后运行：

```bash
go build -ldflags="-s -w" -o zfcheckscores .
URL=... USERNAME=... PASSWORD=... TOKEN=... ./zfcheckscores
```

## 许可证

Apache-2.0

## 致谢

- [openschoolcn/zfn_api](https://github.com/openschoolcn/zfn_api) — 正方 API 参考
- [NianBroken/ZFCheckScores](https://github.com/NianBroken/ZFCheckScores) — 原始 Python 项目

---

Copyright © 2024 NianBroken. All rights reserved.
