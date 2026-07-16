# 项目长期记忆 (ZFCheckScores)

## 环境与测试约束
- **njtech（南京工业大学）教务系统**：正确 base_url 为 `https://jwgl.njtech.edu.cn/`，**无需** `/jwglxt/`（带 jwglxt 会被 301 重定向回根路径的 `xtgl/login_slogin.html`）。
- 该校 **WAF 会对非校园网/非信任 IP 间歇性返回登录验证码页**（login 返回 code 1001，页面含 `input#yzm`），上游 `user_login.py` 遇到 1001 直接 `sys.exit`，**不支持验证码**。
- 因此本项目的端到端真机测试只能从「学校信任的 IP」（校园网 / 用户本机 / GitHub Actions 等）跑通；本沙箱云环境 IP 会被 WAF 验证码挑战，无法完成自动登录。
- **可绕过的本地验证方式**：用 Mock client 验证重构逻辑（见 2026-07-16 记录），无需真实登录。

## 关键架构事实
- `scripts/zfn_api.py` 的 `get_grade(year, term)` / `get_selected_courses(year, term)`：`year=0,term=0` 表示查**全部学年**；term=1→第一学期(xqm=3)，term=2→第二学期(xqm=12)。
- `data/` 目录**未被 .gitignore 忽略**，运行会改写 info.txt/grade.txt/old_grade.txt（workflow 会 force-push）。本地测试前务必备份。
- 运行方式：`python main.py`（非 `-m`），`scripts` 作为命名空间包；`get_selected_courses.py` 用相对导入 `from .get_grade`。

## 依赖
- 运行需 `requests`、`rsa`、`pyquery`（GitHub Actions 的 main.yml 已安装）。
