# 0.1.0-rc.1 候选交付记录

交付日期：2026-11-13。交付时已完成可自主执行的实现、构建和本地验证。2026-11-14，用户反馈 Windows 和 Linux 均已通过本轮实测验收，详见下文及 [验收记录](acceptance.md)。产物保持候选版本。

程序、对应源码及镜像中的程序固定于 `f33da785624602f733e295e4974608f3cd5a90d3`，Go 1.27.4。后续提交补齐容器标签检查与本交付记录，程序代码未改变。分支为 `codex/ec-atrust-client`。

## 产物

文件位于仓库 `dist/`，不提交二进制到 Git。

| 文件 | 用途 | 字节数 |
|---|---|---:|
| `tinyrill-windows-amd64.zip` | Windows 原生程序及配套材料 | 7,972,015 |
| `tinyrill-linux-amd64.tar.gz` | Linux 原生程序及配套材料 | 7,904,088 |
| `tinyrill-0.1.0-rc.1-linux-amd64-image.tar` | rootless Podman 镜像归档 | 21,685,760 |

SHA256：

```text
6c3b802de9317a5bb2ac1a5e79c03a35a9d5b6cf5264e4b37b3663a567d9bb15  tinyrill-windows-amd64.zip
88071472ee58afb7be8ba9409dc3521c38570614ef95d8ab1bc3d66355d7b7d6  tinyrill-linux-amd64.tar.gz
fcf9770dc586ee631aebf6b61d2b6da547b7a2550868581b8041fdda784cd0b4  tinyrill-0.1.0-rc.1-linux-amd64-image.tar
```

原生包内有配置示例、文档、`scripts/soak.ps1`、相同提交的 `source.tar.gz`、AGPL 许可证、依赖原始许可证/NOTICE、SBOM、源码/二进制扫描报告、构建记录和文件校验和。Go 标准库的许可证检测未自动给出 ID，已核对并保留其原始 BSD 文案于 `licenses/Go-LICENSE`。这些构建记录未签名。

## 开始验收

原生部署解压对应平台归档，复制 `examples/` 下的客户配置，填写网关、账户及实际信任配置，再执行 `config-check` 和前台 `run`。密码通过终端或配置引用的只读 secret 文件输入。EC 旧 TLS 例外需逐客户明确开启；不改为所有客户默认配置。

容器在 Linux Podman 环境中导入：

```sh
podman load -i tinyrill-0.1.0-rc.1-linux-amd64-image.tar
podman tag localhost/tinyrill:0.1.0-rc.1 localhost/tinyrill:candidate
```

然后按 [rootless Podman 指引](docker.md) 提供单客户配置、独立状态卷与回环端口。原始导入标签为 `localhost/tinyrill:0.1.0-rc.1`，第二条命令用于匹配文档和 Compose 中的 `candidate` 示例名。

镜像 ID 为 `6c1a7d3de43fb9f71bcf6f9f33b9360db761b9119f8b451bd1909422a4ba5108`。镜像内程序与 Linux 发布程序逐字节相同，SHA256 均为 `b6f0adaf4066f21c42614ec5e888dfb96887f0ee8ec8bf8e4c358e758c630746`。最终导出归档已经重新加载并启动验证。

## 已完成的验证

| 检查 | 结果 |
|---|---|
| Windows 全量测试及本机 Credential Manager 增读改删 | 通过 |
| Linux 全量 race、静态检查、隔离 Secret Service、PTY 隐藏输入/取消/恢复 | 通过 |
| EC/aTrust 资源解析 fuzz，各 20 秒 | 无崩溃；约 12 万/27 万次执行 |
| 无业务直连/DNS 回退、严格 YAML、默认 TLS、会话/监听隔离、挑战/凭据错误路径 | 可控回归通过 |
| Windows ZIP 与 Linux tar.gz 解压后的文件校验和、版本、四份配置、验收脚本 | 通过；Linux 原生文件系统中的程序权限为 0755 |
| Podman 5.7.0，UID 65532、只读根、删除全部 capabilities、禁止提权、独立状态卷 | 启动、配置和状态写入通过 |
| 镜像版本/提交标签、导出标签、内嵌程序、保存/加载 | 全部核对一致 |
| govulncheck v1.9.0，两平台源码及实际二进制 | 均为 0 条发现 |
| Trivy 0.76.2，最终导出镜像的 Debian 13.7 与 Go 程序 | 全部严重性均为 0 条发现 |

镜像报告为 `dist/tinyrill-0.1.0-rc.1-image-scan.json`；原生报告在各包内。扫描结果只对应本次目标及数据库时间，后续发布需重扫。

## 用户实测与后续验收

2026-11-14，用户报告 Windows 和 Linux 均验收通过；已测试密码登录、SOCKS 代理，以及 Windows 密码凭据保存/记住密码和点选验证码。实测连接稳定，Windows 任务管理器显示内存约 10 MB。该数值为本轮观察值，具体统计列、负载与持续时长未记录。

本轮反馈的网关类型/版本、Linux 部署方式和实际运行版本/提交待补充。EC 7.6.3、EC 7.6.7、aTrust 的逐项客户兼容记录，三客户真实并行、8 小时业务、至少 60 分钟空闲，以及与原 docker-easyconnect 的性能比较，继续使用 [验收表](acceptance.md) 记录。原客户端配置和状态保留用于回退。

macOS 原生构建/Keychain 和 arm64 实机尚未验证，本次不提供这些平台的已验收包。多平台 CI 已配置，尚未推送或触发远端运行。

本机 Windows 的 Go 1.27.4 工具链在全量测试完成后被 Defender 拦截，后续打包转到 WSL；未修改安全软件设置。候选 Windows 程序已从最终 ZIP 解压并实际运行验证，工具链环境限制另见 [实施记录](implementation-status.md)。
