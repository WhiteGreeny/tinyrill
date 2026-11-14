# 实施记录

实施基线：`8576e7a16076c4175d6dbe276573a353cf7260be`。范围以 `ec-atrust-fork-plan.md` 为准。

## 基线验证

2026-11-13，Windows amd64，Go 1.27.2：依赖下载后运行 `go test -count=1 -timeout=120s ./...`。除 `internal/keylog/TestOpen` 与 `underlay/TestPCAPCaptureRecordsTCPInBothDirections` 外均通过。两项失败均为 Windows 文件 mode 返回 0666、测试要求 Unix 0600；安全文件实现和验收需改用 Windows ACL，不以跳过权限测试解决。

## 交付进度

- 已固定实施方案，首版容器为同一 Linux 二进制的轻量封装；宿主凭据联动延后。
- 已完成：业务拨号与 DNS 禁止直连回退，移除业务直连实现、HTTP 环境代理和 SOCKS5 UDP/BIND；`go test -count=1 -timeout=120s ./dial ./resolve ./service` 通过。
- 已完成：Windows 原生 ACL 与原子秘密文件；严格、版本化 YAML；默认 TLS 身份校验及显式 EC 旧 TLS 例外；有界隧道恢复、可关闭的 gVisor；状态目录独占锁和会话身份绑定。
- 已完成：带客户/代次/挑战 ID 的可取消认证接口；终端隐藏输入、ANSI/iTerm 图片、离线点选 JSON、受令牌/来源/期限约束的点选页面；后台短信发送前检查。
- 已完成：Windows Credential Manager、Linux Secret Service、macOS 原生 Keychain 适配与 age 后端。Windows 和隔离 Linux Secret Service 的增读改删已实际通过并清理测试记录；macOS 适配待原生 CI/环境验证。age 错误口令、密文截断、工作因子上限、身份改变和保存失败测试通过。
- 已接入：YAML CLI、独立实例运行、按需获取密码、认证成功后保存、关闭记住密码不读取后端、仅凭已有会话恢复；删除系统 TUN、Shadowsocks、通用转发、公共 DNS 监听、校园默认规则与旧 TOML/环境参数入口。
- 当前 Windows 全量 `go test -count=1 -timeout=120s ./...` 通过。三个模拟代理实例关闭隔离、HTTP CONNECT/未完成 SOCKS 握手关闭和启动冲突测试通过。
- 已完成：移除认证正文/数据包日志，统一控制字符与秘密字段保护；HTTP 响应、XML 深度/节点、RSA、资源数量和图片/坐标边界；认证重定向限制同源 HTTPS；畸形 XML/节点与地址池耗尽不再崩溃。新增解析 fuzz 入口。
- 已完成：生产 aTrust 隧道故障上报实例并关闭旧代理连接，EC 连续保活失败上报；aTrust 心跳只有收到响应才重置失败计数，增加读写截止时间。修复内部取消被误判为用户正常退出及认证 HTTP 连接未关闭的问题。
- Windows 全量测试已在上述修改后通过；Linux Secret Service 明确只允许本机 Unix D-Bus 传输。
- 已完成：Go 1.27.4、brotli v1.2.7、compress v1.20.2、x/crypto v0.59.0、x/net v0.62.0 等安全更新。govulncheck v1.9.0 最终 Linux 源码扫描无漏洞；已升级的 Debian 13 Distroless 镜像经 Trivy 0.76.2 扫描，系统和 Go 组件均为 0 条。详见 security-scans.md。
- 已完成：Windows 全量回归；Ubuntu 26.04 WSL 全量 race 回归；实际 Linux PTY 隐藏输入、取消和模式恢复；EC/aTrust 资源解析各 20 秒 fuzz（约 12 万/27 万次执行，无崩溃）。race 检查发现并修复了 aTrust 认证 HTTP 传输初始化与 Close 的竞争。
- 已完成：rootless Podman 5.7.0 的非 root、只读根、cap-drop=all 镜像启动、配置和私有状态写入；四个客户/容器配置均通过 config-check；验收探测脚本通过合成本机代理检查，无直连。
- 已完成：同一 Linux 二进制的轻量镜像、原生/容器/服务/mihomo 文档、用户验收表与回退步骤、固定 SHA 和只读权限的 CI，以及包含源码/SBOM/原始许可证/扫描报告/校验和的候选打包脚本。
- 已交付本地候选 `0.1.0-rc.1`：Windows amd64 ZIP、Linux amd64 tar.gz 和可直接 `podman load` 的镜像归档。程序与源码固定于 `f33da785624602f733e295e4974608f3cd5a90d3`；归档解压校验、原生 Linux 0755 执行权限、四份配置、镜像标签/导入/受限启动及内嵌二进制一致性均通过。两平台源码与二进制 govulncheck、最终镜像 Trivy 均为 0 条发现，详情见 [候选交付记录](candidate-0.1.0-rc.1.md)。
- 交付检查修复了跨平台 ZIP、只读依赖许可证复制和 Podman/Buildah 复用旧版本 LABEL 的问题。容器检查从实际二进制提取版本/源码提交，使用无缓存构建并断言标签。当前没有推送 Git、触发远端 CI、创建公开 release 或上传镜像。

## 本机验证环境

Windows 11 amd64 与 Ubuntu 26.04 WSL。Go 工具链固定 1.27.4；WSL 为验证准备了 GCC、Podman 和隔离 Keyring 环境。Windows 全量测试完成后，Defender 在 2026-11-13 17:44 拦截了自动下载工具链目录中的 go.exe，后续构建/扫描转到 WSL；未修改 Defender 设置或添加排除。该环境问题与候选程序的测试结果分别记录，后续直接在 Windows 执行 go 命令需先处理官方工具链的安全软件拦截。

## 外部验收

2026-11-14，用户确认 Windows 和 Linux 均已通过本轮实测验收：密码登录、SOCKS 代理，以及 Windows 密码凭据保存/记住密码和点选验证码均通过。用户报告连接稳定，Windows 任务管理器中内存约 10 MB；具体统计列、负载和持续时长未记录。详细记录见 [验收表](acceptance.md)。

本轮网关类型与版本、Linux 部署方式、实际运行版本/提交待补充，因此 EC 7.6.3、EC 7.6.7、aTrust 的逐项兼容矩阵仍需按实际场景登记。macOS 原生 Keychain、三个真实客户并行、8 小时业务与至少 60 分钟空闲恢复仍待验证。Linux Secret Service 和 rootless Podman 已有可控运行结果，容器客户业务验收另行记录。客户凭据不进入仓库或日志。
