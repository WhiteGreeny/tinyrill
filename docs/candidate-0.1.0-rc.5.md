# 0.1.0-rc.5 交付记录

日期：2026-11-16。程序与对应源码固定于 `b87ab2553fb37d3f86bd33fb78e8e6aef5923f36`；后续提交补充本记录。Windows/Linux amd64 运行包、验证附件、源码和 rootless Podman 镜像已在本地生成并验证。

## 本次变更

- 普通 stderr 日志区分 DNS、资源授权、会话、VPN 拨号及 HTTP 转发失败。同一实例、阶段、目标和错误在 30 秒内限频；debug 展示目标、解析来源/地址和拨号结果。启动时显示已配置的 DNS 能力，stdout 保持 JSON 状态事件。
- 每客户可选 `dns.hosts`，精确名称优先，其次最长 `+.` 后缀，包含基域名与多层子域名；最多 256 条 IPv4 映射。忽略大小写及一个结尾点，拒绝规范化后重复键、非法主机名和非 IPv4 值。
- 显式映射优先于本实例缓存、网关资源映射和 VPN DNS；域名资源元数据保留，连接继续经过原有授权检查。每客户独立，修改后重启该实例生效，不改变系统 DNS。`dns.mode` 仍只接受 `vpn`。
- 补充 [配置参考](configuration.md)、示例及 [匿名 mihomo 排障案例](mihomo.md)。清理上次排障临时文件 18 个、112,895 字节，另清理已完成的 Windows 测试与解压核验临时文件约 189 MiB；旧交付和可复用工具保留。

保留 RC4 的 aTrust 分组/应用名称与描述、EC 名称与备注，以及默认关闭的资源结构/原值诊断功能。

## 产物

目录为 `dist/0.1.0-rc.5/`。入口 `DELIVERY.md`、`verification.json` 和顶层 `SHA256SUMS` 记录使用方式与核验结果；各归档另有 `.sha256`。

| 文件 | 字节数 | 用途 |
|---|---:|---|
| `tinyrill-0.1.0-rc.5-windows-amd64.zip` | 7,055,066 | Windows 运行包 |
| `tinyrill-0.1.0-rc.5-linux-amd64.tar.gz` | 6,979,472 | Linux 运行包 |
| `tinyrill-0.1.0-rc.5-windows-amd64-verification.zip` | 284,579 | Windows 验证附件 |
| `tinyrill-0.1.0-rc.5-linux-amd64-verification.tar.gz` | 245,415 | Linux 验证附件 |
| `tinyrill-0.1.0-rc.5-source.tar.gz` | 269,368 | 对应源码 |
| `tinyrill-0.1.0-rc.5-linux-amd64-image.tar` | 21,178,880 | rootless Podman 镜像 |

运行包仍各为 7 个文件。主要归档 SHA256：

```text
28843906e83e0a632a5cc7e482f985e11d829c484d6eb9208a131f39dfda4083  tinyrill-0.1.0-rc.5-windows-amd64.zip
be093fc5ee8faab730a4e3a83f8632a3a5eca6061ece9b4ed8e7ab61dd429c02  tinyrill-0.1.0-rc.5-linux-amd64.tar.gz
142315d5de603aa622222eab10aef46e9faa20dc9f197ffc0c5b2cb9fb2e0ee8  tinyrill-0.1.0-rc.5-source.tar.gz
e661a2cb78005ea707e44456880b7ffaa04d9a689094727080c71c26683e90af  tinyrill-0.1.0-rc.5-linux-amd64-image.tar
```

镜像 ID：`54c4b40de7bcc75e2fab8d7e8af1a75fa99a35ed077346f3589fd5c43e2b464a`。通过 `podman load -i tinyrill-0.1.0-rc.5-linux-amd64-image.tar` 载入后使用 `localhost/tinyrill:0.1.0-rc.5`。

## 验证与范围

- Windows 全量回归通过：WSL Go 1.27.4 交叉编译后在 Windows 实际执行。Linux 全量 `go test -race -count=1 -timeout=180s ./...` 和 `go vet ./...` 通过。
- 新增回归通过本机真实 HTTP/SOCKS 入口，将相同域名映射到两个同时存活实例各自的模拟 VPN 地址，验证原始 HTTP Host 保留、未授权映射拒绝；另覆盖精确/最长后缀、域名边界、配置副本、严格 YAML、缓存/网关覆盖、授权元数据保留、关闭后的拒绝和普通失败日志限频/脱敏。
- 两平台实际归档的内部校验、源码提交、provenance、二进制和原始许可证核对通过。Windows 解压运行，Linux 原生文件系统解压后 0755 权限、版本/帮助/示例，以及有效/无效静态映射配置检查通过。
- Windows/Linux 源码和实际二进制 govulncheck v1.9.0 均为 0 条发现。Trivy 0.76.2 本次更新漏洞库后检查最终镜像，全部严重性系统与 Go 组件发现数为 0；结论仅对应本次目标和扫描时间。
- rootless Podman 的非 root、只读根、cap-drop=all、配置读取及私有状态写入通过。实际镜像归档重新导入后，禁网受限启动和静态映射配置检查通过；镜像内程序及许可声明与 Linux 运行包一致。

停止旧进程，将新包解压到独立目录，继续使用自己的 YAML、状态与凭据。旧 RC4 及更早归档可用于回退。本轮未连接客户网关；新映射的实际客户验收由用户安排。此前业务反馈按 [验收记录](acceptance.md) 保存，不扩展为本版本或其他网关场景的实测。当前仅本地交付，没有推送 Git、公开发布或上传镜像。
