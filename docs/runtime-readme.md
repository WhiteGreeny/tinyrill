# tinyrill @VERSION@

平台：`@PLATFORM@`；源码提交：`@COMMIT@`。

## 先选配置，再改必填项

普通用户名密码登录从以下示例开始。无需把完整参数表全部抄进 YAML，未填写的可选项会使用默认值。

| 客户端 | 复制哪个示例 | 首先确认什么 |
|---|---|---|
| EasyConnect（EC） | `examples/customer-a-ec.yaml` | 网关和用户名；按客户需要确认旧 TLS 通道兼容开关 |
| aTrust | `examples/customer-c-atrust.yaml` | 网关和用户名；用 `auth-info` 查询认证域/方式 |
| rootless Podman | 验证附件内 `examples/customer-container.yaml` | 容器端口映射、状态卷及回调地址，按附件 `docs/docker.md` 操作 |

复制示例为自己的文件，例如 `examples/customer.yaml`。示例中已用中文标明“必须修改／按需配置／保持默认”：

| 参数 | 普通用户应该怎么填 |
|---|---|
| `schema_version` | 保留 `1`，这是配置格式版本 |
| `id` | 设置客户独立标识，例如 `customer-a`；使用后保持稳定，最多 64 个英文字母/数字/点/下划线/连字符，首字符为字母或数字 |
| `protocol` | EC 用 `easyconnect`，aTrust 用 `atrust`；不是 `7.6.3` 等版本号 |
| `server` | **必须修改**为实际网关，如 `vpn.customer.example:443`，不带 `https://` 或路径；不写端口默认 443 |
| `auth.username` | **密码登录必须修改**为实际账户；密码本身在运行时隐藏输入，不能写 `password` 字段 |
| `auth.domain` | aTrust 使用 `auth-info` 输出中对应认证项的 `domain`；只有该域确实为空时才保留 `""` |
| `listen.socks5` / `listen.http` | 单客户可用示例端口；多客户或端口占用时修改。原生通常只监听 `127.0.0.1` |
| `session.state_dir` | 示例已显式指定目录；复制客户时同步修改，不能共用。它不会跟随 `id` 自动改写 |

路径相对于 **YAML 所在目录**。示例的 `../state/customer-a-ec` 表示 YAML 上一级目录中的状态子目录；移动 YAML 会改变其位置。默认保留示例的目录结构，更新程序时继续使用原客户配置和状态路径。

## 检查配置并登录

Windows PowerShell：

```powershell
./tinyrill.exe config-check --config examples/customer.yaml
./tinyrill.exe run --config examples/customer.yaml
```

Linux/macOS：

```sh
./tinyrill config-check --config examples/customer.yaml
./tinyrill run --config examples/customer.yaml
```

`config-check` 通过表示配置结构、必填条件及 TLS CA/pin 设置可接受，不代表网关、端口、密码或业务已经验证；它不占用监听端口，也不检查所有 secret/客户端证书文件。修改配置后，停止并重启对应客户实例。

aTrust 在登录前使用 `./tinyrill.exe auth-info --config examples/customer.yaml`（Linux 用 `./tinyrill`）。它查询认证方式与域，不执行登录。按同一项中的 `domain` 和下表选择配置；不要把显示名称 `name` 填进 `domain`：

| `auth-info` 的 `auth_type` | YAML 的 `auth.method` | 需要填写/操作 |
|---|---|---|
| `auth/psw` | `password` | `auth.username`，密码运行时输入 |
| `auth/smsCheckCode` | `sms` | `auth.phone` 填带引号的手机号，短信验证码运行时输入 |
| `auth/cas` | `cas` | 根据运行提示打开认证链接、回填响应 |
| `auth/httpsOauth2` | `oauth2` | 根据运行提示打开认证链接、回填响应 |

EC 的 `auth-info` 只列出本实现支持的 `password`/`certificate`，不查询网关能力。EC 证书主认证使用 `auth.method: certificate` 和 `auth.certificate_file`（PKCS#12 文件），按提示输入证书口令。所有非密码主认证均保持 `remember_password: false`。密码登录之后出现的短信、TOTP 或验证码按提示处理，不需要修改主认证方式。

连接就绪后，将应用代理指向本客户的监听端口。SOCKS 客户端使用 `socks5h`，例如 `socks5h://127.0.0.1:21080`，把域名交给本客户解析。代理只支持 TCP；没有 SOCKS5 UDP，也不会自动接管系统流量。Ctrl+C 退出。

## 有这些需求时再增加配置

| 需求 | 配置方法与条件 |
|---|---|
| 记住密码 | 在 `auth` 下将 `remember_password` 改为 `true`。Windows 使用当前用户 Credential Manager，Linux 使用已解锁的 Secret Service；macOS 原生构建使用 Keychain。没有可用系统凭据库时明确报错，不会自动保存为明文 |
| 固定业务域名的地址 | 在 `dns.hosts` 添加精确名称或 `+.` 后缀到 IPv4 的映射，见下面示例；仍需网关资源授权 |
| 指定 VPN 内 DNS | 在 `dns` 下加 `servers: ["10.0.0.53"]`，换成客户实际 IPv4，最多两个。显式列表替代网关 DNS，查询仍经 VPN；省略/`[]` 使用网关下发的 DNS，并非禁用 DNS |
| 信任私有网关证书 | 保持 `tls.mode: verify`，在 `tls` 下填写 `ca_file: "./customer-ca.pem"`；CA 文件从客户管理员取得，其 CA 集合替代系统根集合 |
| EC 旧 TLS 数据通道 | 确认该客户接受 TLS 1.1/RC4 后，设置 `easyconnect.allow_legacy_tls: true`。这不等于 `tls.mode: insecure`，仍按所选策略校验服务器身份 |
| 后台或容器注入密码 | 设置 `auth.secret_file` 指向受保护的单行密码文件；无人值守加 `auth.interactive: never`。文件优先于已保存密码；建议保持 `remember_password: false`。有人工挑战时退出 3，需前台完成 |
| 不使用系统凭据库、改用加密文件 | 设置 `credentials.backend: age-file`、独立 `credentials.vault_file`，并开启记住密码；需要额外的隐藏解锁口令，不适合无人值守 |
| 终端验证码显示不清 | 可查看输出中提示的原图路径；需要纯文件方式时设置 `auth.captcha_mode: file` 并显式填写 `auth.captcha_file`，点选改用原图坐标 JSON 输入 |
| 观察连接失败原因 | 临时设置顶层 `debug: true`，查看目标与 DNS 来源；日志含客户域名/IP，分享前筛选 |

下面是静态映射**片段**。将它合并进现有 `dns` 节，不要添加第二个 `dns`；地址仅为示例，应替换为实际客户资源。

```yaml
dns:
  hosts:
    "portal.apps.internal": "10.0.0.20"
    "+.apps.internal": "10.0.0.10"
```

精确名称优先，其次最长后缀。上例中 `portal.apps.internal` 解析为 `.20`；`apps.internal`、`dev.apps.internal` 和更深子域名解析为 `.10`，`notapps.internal` 不匹配。最多 256 条，仅支持 IPv4 值，大小写及一个结尾点不影响匹配。映射优先于缓存/网关/VPN DNS，仅作用于当前客户，修改后需重启，不修改主机 DNS。

## 通常可以省略的设置

| 参数 | 省略后的默认行为 |
|---|---|
| `auth.method` / `auth.interactive` / `auth.timeout` | `password` / `auto` / 单次交互 `5m` |
| `auth.remember_password` / `credentials.backend` | `false` / `auto`；选择后端本身不会开启记住密码 |
| `auth.captcha_mode` / `auth.captcha_file` | `auto` / 状态目录中的 `captcha.png`；file 模式仍要求显式路径 |
| `auth.callback_listen` / `auth.callback_url` | 本机临时端口 `127.0.0.1:0` / 使用实际地址；原生通常不改，容器按端口映射配套填写 |
| `listen.socks5` / `listen.http` | `127.0.0.1:1080` / `127.0.0.1:1081`，与示例显式填写的端口不同 |
| `dns.mode` / `dns.ttl` | 只支持 `vpn` / 缓存 `300` 秒；没有系统或公共 DNS 回退 |
| `session.state_dir` / `credentials.ref` | YAML 目录下 `state/<id>` / 客户 ID；与示例的显式状态路径不同 |
| `session.setup_timeout` / `session.dial_timeout` | 每次认证初始化 `10m` / 建立业务连接 `15s`；不是总在线或空闲时长 |
| `session.reconnect_attempts` | 整个进程生命周期内最多恢复 `3` 次；0–10，`0` 禁用；不自动重试首次失败的密码登录 |
| `tls.mode` / `easyconnect.multi_line` | `verify` / `false`；通常不改 |
| `atrust.session_refresh` / `atrust.nodes_refresh` | `30m` / `5m`；通常不改 |
| `debug` | `false`；普通失败日志和授权资源展示已可用 |

时间字段使用 `15s`、`5m` 等带单位值，允许 `1s`–`24h`；`dns.ttl` 单独使用 1–86400 的整数秒。布尔写 `true/false`，用户名/手机号建议加引号。Windows 路径可写 `C:/vpn/ca.pem` 或单引号 `'C:\vpn\ca.pem'`。不支持 `${VAR}` 展开、未知字段、重复键和 YAML 锚点/别名。

不用的字段直接省略，不要留空的 `auth:`、`dns:` 或 `hosts:`。显式空值不一定等于省略：`listen.http: ""` 会关闭 HTTP 入口，而省略则启用默认地址；至少保留一个代理入口。原生代理没有用户名/密码认证，通常只监听本机。

## 多客户、密码与会话管理

每客户一份 YAML，分别启动一个进程，使用独立 ID、监听端口和状态目录。切换应用代理端口就是选择客户；不同客户即使有相同 IP/域名，也不共享解析缓存或业务通道。复制示例时同步检查显式路径和端口，避免沿用另一个客户的目录。

`version` 查看版本；`credentials delete --config ...` 删除保存的密码，`session clear --config ...` 单独清除会话。关闭记住密码不会删除已保存记录，也不会禁用会话复用。更新程序前停止旧进程，保留自己的配置和状态；不要覆盖自定义 YAML。

## 资源和排障

连接就绪后默认显示最多 20 条授权资源。运行时添加 `--resources full` 显示全部，`--resources none` 隐藏；这些是命令行选项，不是 YAML 字段。`RESOURCE` 列分别显示 aTrust 应用名称/描述、分组名称/描述，以及 EC 名称/备注，名称为空也会保留非空描述。列表说明授权范围，没有主动探测业务可达性。

普通 stderr 日志现在区分解析、授权、会话和连接失败，并对相同失败限频；`debug: true` 可查看收到的目标和实际解析来源。启动时 `DNS ready` 显示配置的解析能力。遇到 IP 能访问、域名不能访问时，先用 `socks5h` 分别直接测试 tinyrill 的端口，再检查上层代理的域名传递；详细步骤和匿名案例在验证附件的 `docs/mihomo.md`。

stdout 输出 JSON 状态事件，stderr 用于交互和诊断。退出码 2 通常需修改配置或解决监听冲突；3 需要人工认证；4 认证被拒绝；5 连接失败；6 状态或凭据存储问题。`ready` 也不等于业务一定可达，应再访问实际资源。

排查资源名称时可添加 `--resource-debug`，无需开启 YAML 的 `debug`。取得资源后，终端会提示该客户状态目录中的 `resource-fields.json` 路径；文件只保留有限的资源字段名、类型和层级，不包含字段原值。看到本次保存提示后，检查文件并提供给维护者即可。每次采集覆盖旧报告，普通运行不生成新报告。

需要自己检查实际名称及其他字段值时，使用 `--resource-debug-values`，单独这个开关即可。状态目录额外生成 aTrust 的 `resource-values.json`（完整分组/应用结构在 `resources` 中）或 EC 的 `resource-values.xml`（完整 `Rcs` 子树），保留资源分支的全部字段原值。文件包含实际客户资源信息，先筛选必要片段再分享；终端只报告路径，普通 debug 不启用此导出。每次成功采集覆盖旧文件，检查本次保存提示和文件中的 `captured_at`。

## 文件用途

| 文件 | 用途 |
|---|---|
| `tinyrill` / `tinyrill.exe` | 主程序，无需旁置 DLL 或 Go 工具链 |
| `examples/*.yaml` | EC 和 aTrust 的配置起点 |
| `README.md` | 本说明 |
| `LICENSE` | 项目许可证 |
| `THIRD_PARTY_NOTICES.txt` | Go 与所用依赖的原始许可声明，合并保存 |
| `SHA256SUMS` | 包内文件校验清单 |

实际运行读取主程序、所选 YAML，以及该 YAML 明确引用的 CA/secret/证书等文件；状态目录由程序创建。许可声明与说明用于交付，不是运行时加载的依赖。

完整配置、容器说明、验收脚本、SBOM、扫描报告、逐依赖许可证和构建记录在同版本 `@VERIFICATION_ARCHIVE@`。对应源码为 `@SOURCE_ARCHIVE@`，与运行包通过验证包中的 `provenance.json`（提交号与 SHA256）关联。分发时同时提供对应源码和许可材料；无需把源码和扫描工具放在主程序旁运行。

Linux/macOS 可在本目录运行 `sha256sum -c SHA256SUMS`（macOS 使用 `shasum -a 256 -c SHA256SUMS`）；Windows 使用 `Get-FileHash -Algorithm SHA256` 对照清单。更新前停止旧进程、保留客户配置和受保护状态目录，再解压新包到独立目录；不要覆盖自定义配置。
