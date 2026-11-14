---
name: Bug report
about: 报告 tinyrill 的认证、代理或运行问题
title: ''
labels: bug
assignees: ''
---

请先查看 README、docs/configuration.md 和已有 Issues。诊断材料按最小必要范围提供：请勿提交密码、短信/TOTP、令牌、Cookie、客户端证书、状态目录、凭据库、验证码原图/链接或未经处理的抓包；客户域名和 IP 可用一致的占位符替换。

**版本与环境**

- `tinyrill version` 输出（包含版本、提交、平台和 Go 版本）：
- 操作系统及架构：
- 原生 / rootless Podman（容器请补充 Podman 版本与镜像版本）：
- EC / aTrust 与网关版本（若已知，无需填写实际网关地址）：
- 认证方式：密码 / 短信 / 证书 / CAS / OAuth2；是否有验证码：

**问题与预期行为**

**最小复现步骤**

1. 使用的命令（例如 `tinyrill run --config customer.yaml`）：
2. 出错阶段、频率、退出码：
3. HTTP / SOCKS5 入口，使用 IP 或域名，资源列表是否包含目标：

**脱敏配置**

仅提供有关的 YAML 字段；保留字段名，用占位符替换客户信息。不要添加明文 password 字段。

```yaml
```

**脱敏日志或截图**

提供故障前后必要的 stdout 状态和 stderr 诊断，并再次检查客户资源与认证信息。需要关闭资源列表时使用 `--resources none`。

```text
```
