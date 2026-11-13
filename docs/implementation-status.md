# 实施记录

实施基线：`8576e7a16076c4175d6dbe276573a353cf7260be`。范围以 `ec-atrust-fork-plan.md` 为准。

## 基线验证

2026-11-13，Windows amd64，Go 1.27.2：依赖下载后运行 `go test -count=1 -timeout=120s ./...`。除 `internal/keylog/TestOpen` 与 `underlay/TestPCAPCaptureRecordsTCPInBothDirections` 外均通过。两项失败均为 Windows 文件 mode 返回 0666、测试要求 Unix 0600；安全文件实现和验收需改用 Windows ACL，不以跳过权限测试解决。

## 交付进度

- 已固定实施方案，首版容器为同一 Linux 二进制的轻量封装；宿主凭据联动延后。
- 已完成：业务拨号与 DNS 禁止直连回退，移除业务直连实现、HTTP 环境代理和 SOCKS5 UDP/BIND；`go test -count=1 -timeout=120s ./dial ./resolve ./service` 通过。
- 已完成：Windows 原生 ACL 与原子秘密文件；严格、版本化 YAML；默认 TLS 身份校验及显式 EC 旧 TLS 例外；有界隧道恢复、可关闭的 gVisor；状态目录独占锁和会话身份绑定。
- 已完成：带客户/代次/挑战 ID 的可取消认证接口；终端隐藏输入、ANSI/iTerm 图片、离线点选 JSON、受令牌/来源/期限约束的点选页面；后台短信发送前检查。
- 已完成：Windows Credential Manager、Linux Secret Service、macOS 原生 Keychain 适配与 age 后端。Windows 系统库的增读改删实际通过并清理测试记录；Linux 后端交叉编译通过。age 错误口令、密文截断、工作因子上限、身份改变和保存失败测试通过。
- 已接入：YAML CLI、独立实例运行、按需获取密码、认证成功后保存、关闭记住密码不读取后端、仅凭已有会话恢复；删除系统 TUN、Shadowsocks、通用转发、公共 DNS 监听、校园默认规则与旧 TOML/环境参数入口。
- 当前 Windows 全量 `go test -count=1 -timeout=120s ./...` 通过。三个模拟代理实例关闭隔离、HTTP CONNECT/未完成 SOCKS 握手关闭和启动冲突测试通过。
- 待完成：日志敏感内容与网关输入全面收口、CLI/恢复补充验收、轻量镜像与部署文档、固定 CI/发布材料、漏洞扫描及外部环境验收。

## 外部验收

以下尚未验证，不能作为已支持能力发布：实际 EC 7.6.3、EC 7.6.7、aTrust 客户网关与业务；Linux/macOS 系统凭据库；rootless Podman；三个客户并行与 8 小时稳定性。继续完成可控测试，客户凭据不进入仓库或日志。
