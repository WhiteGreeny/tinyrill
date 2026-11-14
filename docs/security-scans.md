# Security scan record

工具链与依赖已固定，扫描结果按时间和目标记录，不承诺后续数据库更新后仍无新条目。

首次 govulncheck v1.9.0 在 Go 1.27.2 下发现 4 个可达问题：GO-2026-6888（brotli）、GO-2026-6865（compress）、GO-2026-6859（net/http）、GO-2026-6809（x509）。已升级 Go 1.27.4、brotli v1.2.7、compress v1.20.2。

复扫同时检查了未被程序调用的模块级问题，并升级 x/crypto v0.59.0、x/net v0.62.0、x/sys v0.50.0、x/term v0.48.0。保留 gVisor 和 uTLS 的协议适配边界，没有因扫描整体替换网络栈。

第一次镜像扫描使用 Trivy 0.76.2，发现 Debian 12 基础镜像的 ca-certificates/tzdata 更新提示，以及旧 x/crypto/x/net 条目。基础镜像已更新到固定 digest 的 Distroless static-debian13 nonroot。2026-11-13 最终 Linux 源码 govulncheck 无漏洞；更新后镜像 Trivy 的系统组件和 Go 二进制均为 0 条。

候选包的 `vulnerabilities-source.json` / `vulnerabilities-binary.json` 由固定版本 govulncheck 生成，分别检查目标平台源码和实际可执行文件。源码扫描可判断调用关系；二进制/镜像扫描的报告范围不同，应分别阅读，不能用一个结果覆盖其他结果。

`trivy-image.json` 记录基础镜像系统组件和内嵌 Go 二进制的版本匹配。CI 保留全部严重性条目，并阻止有 HIGH/CRITICAL 条目的候选镜像通过。非高危或未分级条目仍需记录和评估，不因门槛未阻止而称为不存在。

TLS 1.1/RC4 的 EC 数据通道和显式 insecure 配置是单客户兼容例外，不是扫描工具能够证明安全的模式。默认关闭，不能成为所有客户配置的模板。

2026-11-14，RC2（源码 `bf97632cea06989c8bd307c31907d100e588ff2d`）的 Windows/Linux 源码和实际二进制 govulncheck 均为 0 条发现；最终镜像归档经 Trivy 0.76.2 扫描，系统和 Go 组件均为 0 条。原生报告现位于各平台 `-verification` 附件，镜像报告位于 `dist/0.1.0-rc.2/tinyrill-0.1.0-rc.2-image-scan.json`。JSON 报告通过 `govulncheck -mode=convert` 应用可达漏洞失败门槛，已用合成报告验证退出码 3；不依赖 JSON 输出模式的退出码判断无漏洞。

2026-11-14，RC3（源码 `4b25fe2b6e9c607018c323bac8350e3a41889005`）的 Windows/Linux 源码和实际二进制 govulncheck v1.9.0 均为 0 条发现；最终镜像归档经 Trivy 0.76.2 扫描，全部严重性的系统与 Go 组件发现数为 0。报告位于 `dist/0.1.0-rc.3/` 的平台验证附件及 `tinyrill-0.1.0-rc.3-image-scan.json`，详见 [RC3 交付记录](candidate-0.1.0-rc.3.md)。

数据库与说明：[Go vulnerability management](https://go.dev/doc/security/vuln/)、[Trivy vulnerability scanning](https://trivy.dev/docs/v0.76/guide/scanner/vulnerability/)。
