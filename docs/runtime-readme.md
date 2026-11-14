# tinyrill @VERSION@

平台：`@PLATFORM@`；源码提交：`@COMMIT@`。

## 开始使用

1. 在 `examples/` 内复制适合的 YAML 并编辑服务器、账户；EC 使用 `customer-a-ec.yaml`，aTrust 使用 `customer-c-atrust.yaml`。保持新配置在此目录中，相对状态路径以 YAML 所在目录为基准。
2. 校验配置，随后前台登录。Windows PowerShell 使用 `./tinyrill.exe`，Linux/macOS 使用 `./tinyrill`：

```powershell
./tinyrill.exe config-check --config examples/customer-a-ec.yaml
./tinyrill.exe run --config examples/customer-a-ec.yaml
```

```sh
./tinyrill config-check --config examples/customer-a-ec.yaml
./tinyrill run --config examples/customer-a-ec.yaml
```

3. 按提示隐藏输入密码、完成验证码。连接就绪后显示客户、代理端口与最多 20 条授权资源；添加 `--resources full` 显示全部，`--resources none` 关闭展示。`NAME` 列展示 EC 网关的资源名称；未取得名称时显示 `-`，aTrust 名称暂未接入。资源列表未探测可达性，代理只支持 TCP。SOCKS 客户端使用 `socks5h` 将域名交给 VPN 解析。Ctrl+C 退出。

需要记住密码时显式设置 `auth.remember_password: true`；默认原生凭据库为 Windows Credential Manager、macOS Keychain 或 Linux 已解锁的 Secret Service。不要在 YAML 写明文密码。EC 的旧 TLS 数据通道需按客户实际情况明确启用 `easyconnect.allow_legacy_tls`；私有证书使用 `tls.ca_file` 或完整配置指南中的 pin 设置。

多个客户各用一份配置、不同客户 ID、代理端口和状态目录。示例内服务器和用户名为占位符。stdout 为 JSON 状态，交互与资源列表在 stderr。`version` 查看版本；`credentials delete --config ...` 删除保存的密码，`session clear --config ...` 单独清除会话。

排查资源名称时可添加 `--resource-debug`，无需开启 YAML 的 `debug`。取得资源后，终端会提示该客户状态目录中的 `resource-fields.json` 路径；文件只保留有限的资源字段名、类型和层级，不包含字段原值。看到本次保存提示后，检查文件并提供给维护者即可。每次采集覆盖旧报告，普通运行不生成新报告。

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
