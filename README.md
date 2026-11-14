# tinyrill

面向多客户场景的 EC / aTrust 精简客户端，fork 自 [zju-connect](https://github.com/Mythologyli/zju-connect)。每个进程连接一个客户，提供 HTTP 代理和 SOCKS5 TCP CONNECT；应用选择和系统流量接管交给上层代理软件。

当前已交付 `0.1.0-rc.1` 本地候选，包含 Windows/Linux amd64 原生包与 rootless Podman 镜像，见 [交付记录](docs/candidate-0.1.0-rc.1.md)。2026-11-14，用户确认 Windows 和 Linux 均通过本轮实测验收，已覆盖密码登录、SOCKS 代理，以及 Windows 凭据保存/记住密码和点选验证码；连接稳定。具体网关版本与部署方式继续在 [验收记录](docs/acceptance.md) 中登记。见 [实施记录](docs/implementation-status.md) 和 [完整方案](docs/ec-atrust-fork-plan.md)。

## 运行

复制并修改 [客户 A / EC](examples/customer-a-ec.yaml)、[客户 B / EC](examples/customer-b-ec.yaml)、[客户 C / aTrust](examples/customer-c-atrust.yaml)。配置中的服务器和用户名都是占位符。

```sh
tinyrill config-check --config customer.yaml
tinyrill auth-info --config customer.yaml
tinyrill run --config customer.yaml
```

`run` 前台运行，Ctrl+C 关闭当前客户连接。相对路径以 YAML 文件所在目录为基准。未知字段、重复键、类型不匹配、YAML 标签/别名和明文 password 字段都会被拒绝。旧 TOML、密码命令行参数和环境变量覆盖已删除。

aTrust 的 `auth-info` 查询网关支持的认证域和方式，不执行登录。EC 的该命令只列出本实现的主认证方式，并明确标记未进行网关发现。用户名/密码以外还保留短信、TOTP、图片/点选验证码、CAS/OAuth2 和 EC 客户端证书的协议路径；客户定制认证需实测。

默认监听回环地址。每个客户使用独立端口、状态目录和凭据标识；同一状态目录的第二个进程会被拒绝。标准输出为 JSON 状态事件，诊断和交互显示在标准错误。`ready` 表示认证、网络栈和监听器已完成初始化，实际业务权限仍由网关资源决定。

连接就绪后默认展示最多 20 条授权资源及代理监听地址。添加 `--resources full` 查看完整的域名/IP、端口和协议列表，或 `--resources none` 关闭展示。列表说明授权范围，未探测业务可达性；UDP/ICMP 条目不能通过当前 TCP 代理访问。

## 认证与密码

密码通过终端隐藏输入；无人值守时可设置 `auth.secret_file`，由部署方提供只读文件。文件包含一行密码，末尾换行会移除，密码本身的空格会保留。TOTP 种子和证书口令分别用 `totp_secret_file`、`certificate_password_file` 引用。

`auth.remember_password` 默认关闭，关闭时不读取或写入保存的密码。显式开启后，原生 `credentials.backend: auto` 使用当前用户的系统凭据库，不可用时明确报错。也可显式选择 `age-file` 并设置 `vault_file`；该方式需要单独的隐藏解锁口令，不用于无人值守。

```sh
tinyrill credentials status --config customer.yaml
tinyrill credentials set --config customer.yaml
tinyrill credentials delete --config customer.yaml
tinyrill session clear --config customer.yaml
```

凭据命令不会输出密码。忘记密码与清除会话是两个独立操作。保存记录绑定客户、协议、服务器、域和账户，换服务器或账户不会自动复用旧密码。自动保存发生在认证完成之后。

普通图片验证码支持 ANSI 彩色显示及 iTerm 内联图片，同时写入受保护的原图。点选验证码通过终端提供的一次性本机链接操作；可配置 `captcha_mode: file` 和 `captcha_file` 使用原图加离线坐标 JSON。后台遇到人工挑战返回 `waiting_auth`；重连只尝试已有会话，不自动反复提交密码或发送短信。

## 网络和 TLS

业务连接只经当前客户的 VPN。未授权资源、不支持的 IPv6、DNS 失败或会话断开都会返回错误，无本地直连或公共 DNS 兜底。业务域名使用网关资源及 VPN 内 DNS；可用 `dns.servers` 指定通过该 VPN 访问的解析器。网关控制连接单独使用基础网络。

SOCKS5 客户端应使用 `socks5h` 传递域名。仅支持 TCP CONNECT；没有 SOCKS5 UDP/BIND、系统 TUN、Shadowsocks、主机路由/DNS 修改或通用端口转发。保留 VPN L3 所需的 gVisor 用户态栈和内部 UDP/DNS。

默认验证 TLS 证书链与主机名。私有网关可配置 `tls.ca_file`，或使用从可信渠道取得的 `sha256/<base64 SPKI digest>` 配置 `tls.mode: pin` 与 `tls.pins`。不自动信任服务器从未验证连接下发的证书。

继承的 EC TCPP/L3 数据通道需要 TLS 1.1/RC4。只有明确设置 `easyconnect.allow_legacy_tls: true` 才会尝试，仍执行所选身份校验。`tls.mode: insecure` 是另一项显式例外，两者不互相隐含；这些例外不应成为所有客户的默认配置。

## 构建与验证

构建工具链固定为 Go 1.27.4。Windows / Linux 使用普通 Go 构建；macOS 系统凭据后端需要启用 CGO 并链接系统 Security Framework，不通过携带密码的子进程命令调用 Keychain。

```sh
go test -count=1 -timeout=120s ./...
go build -trimpath -o tinyrill .
```

Windows 输出文件名使用 `tinyrill.exe`。系统凭据集成测试默认跳过；设置 `TINYRILL_TEST_NATIVE_CREDENTIALS=1` 后只创建、更新和清理随机命名的虚构测试条目。该开关不包含秘密。

退出码：0 正常停止或成功；2 配置或监听冲突；3 需要人工认证；4 网关拒绝认证；5 连接失败；6 状态或凭据存储失败。进程管理器应对 3/4 停止自动重启，避免造成认证循环。

配置详解见 [YAML 与认证](docs/configuration.md)；容器见 [rootless Podman](docs/docker.md)；多客户分流见 [mihomo 示例](docs/mihomo.md)。[构建与候选发布](docs/releasing.md) 会生成对应源码、SBOM、扫描报告和 SHA256；[验收表](docs/acceptance.md) 用于用户安排的实际客户测试与回退。

## 来源与许可证

基于 zju-connect 提交 `8576e7a16076c4175d6dbe276573a353cf7260be`，保留其 Git 历史及 [AGPL-3.0 许可证](LICENSE)。协议代码还继承 EasierConnect 等上游贡献，保留相应源码中的版权声明。发布和分发时须一并提供对应源码、修改记录和许可证材料。
