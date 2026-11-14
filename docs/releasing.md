# 构建与候选发布

当前交付为候选版，已收到 Windows/Linux 用户实测通过反馈；已测功能和待补充的场景信息记录在 [验收表](acceptance.md)。后续正式兼容声明按实际网关场景整理。本地 Git 提交不等于推送、发布 release 或上传镜像。

## 可复现输入

- `.go-version` 固定 Go 1.27.4，`go.mod` 要求至少该版本；`go.sum` 固定依赖校验值。
- GitHub Actions 使用完整提交 SHA，默认 contents:read，不使用上游发布凭据、AUR 更新或自动移动 tag。
- Dockerfile 固定 Distroless digest，只复制同一 Linux 发布二进制。容器没有 Go 编译器和额外认证脚本。
- Windows/Linux 静态 Go 构建；macOS 在原生 macOS 上启用 CGO，以编译并链接 Keychain Services。不能将 CGO=0 的 macOS 构建称为完整原生凭据支持。

## 本地验证与打包

使用 PowerShell 7.3+，在干净工作树运行：

```powershell
go mod verify
go vet ./...
go test -count=1 -timeout=120s ./...
pwsh scripts/release.ps1 -TargetOS windows -TargetArch amd64 -Version candidate
pwsh scripts/release.ps1 -TargetOS linux -TargetArch amd64 -Version candidate
```

Linux/macOS 另运行 `go test -race -count=1 -timeout=180s ./...`。输出位于 `dist/<version>/`，目标已存在时脚本拒绝覆盖；使用新版本号或移走不完整构建。同一版本不同平台必须来自同一提交，脚本检查 `release.json` 与源码归档 SHA256。Windows/Linux 可交叉构建 amd64/arm64，实际运行验收分别记录。macOS 发布必须使用本机编译器，不在 Windows 上伪造完成状态。

交付分为三类，文件名包含版本、操作系统和架构：

| 归档 | 内容与用途 |
|---|---|
| `tinyrill-<version>-<os>-<arch>.zip` / `.tar.gz` | 运行包，共 7 个文件：主程序、两份 YAML 示例、短说明、LICENSE、合并的第三方许可声明、SHA256SUMS |
| `tinyrill-<version>-<os>-<arch>-verification.zip` / `.tar.gz` | 同平台验证附件：完整文档、示例、验收脚本、SBOM、源码/二进制扫描、build-info、provenance、逐依赖原始许可证与校验清单 |
| `tinyrill-<version>-source.tar.gz` | 对应完整 Git 提交的源码和构建脚本，同一版本的所有平台共用 |

运行只需要可执行文件、所选配置及该配置明确引用的文件。文档、扫描报告和构建工具无需放在程序旁。运行包将 Go 与依赖的原始许可声明逐字节合并为 `THIRD_PARTY_NOTICES.txt`，验证包仍保留原文件及 `licenses/index.json` 的来源索引；精简目录不丢弃许可材料。

工具固定为 govulncheck v1.9.0、cyclonedx-gomod v1.12.0，工具缓存位于忽略的 `_build/tools/<host-os>-<host-arch>/`，不进入交付。SBOM 包含 Go 标准库及许可证检测证据。`provenance.json` 用源码提交、程序 SHA256、运行归档 SHA256、源码归档 SHA256 关联三类产物，是未签名构建记录。

govulncheck JSON 模式本身不会因发现漏洞返回失败；脚本对保存的 JSON 再执行 `-mode=convert`，以可达漏洞的非零退出码阻止打包，并保留可读文本报告。不为此重复联网扫描。

Windows 输出 ZIP，Linux/macOS 输出 tar.gz；归档旁有单独 SHA256 文件。Unix 归档内的二进制显式保留 0755 执行权限，即使交叉构建主机为 Windows。验证包解压到独立目录后，可使用其中的 `scripts/soak.ps1` 进行持续探测；重新构建的完整脚本在源码包和 Git 仓库中。

发布二进制采用 trimpath，显式版本和完整提交。时间戳位于构建记录和 SBOM；不承诺整个目录的字节级可复现。工具与模块下载由 Go 校验；扫描数据库是构建时获取的当前数据，报告记录其时间，后续发布需重扫。

可以使用 `sha256sum -c SHA256SUMS` 核验包内文件。Windows 可用 `Get-FileHash -Algorithm SHA256` 对照对应条目。发布源码包只包含已提交内容，不含 local/、state/、用户秘密或工作区未提交文件。

## 镜像与正式发布门槛

按 [Podman 指引](docker.md) 构建，运行 `bash scripts/container-smoke.sh candidate` 验证 rootless、只读根、非 root 用户、配置解析与私有状态写入；版本参数必须与发布目录一致。随后运行 `bash scripts/scan-image.sh candidate` 导出同版本镜像和 Trivy 报告到 `dist/candidate/`，保留全部严重度并对 HIGH/CRITICAL 设置失败门槛。Trivy 缓存在 `_build/scanner/`。此检查不连接客户网关。随后由客户验收覆盖认证、两个代理入口、三实例隔离、断线和长时间业务。

只有已验证的客户/平台写入支持列表；其他能力列为待验收。对外分发时一并提供对应源码、校验和、SBOM、扫描报告、许可证、修改记录及基础镜像 digest。基础镜像的系统文件不包含在 Go SBOM 中，需另外扫描；不要将 Go 扫描通过写成镜像完全无漏洞。

GitHub workflow 产物只是候选归档。当前没有自动公开发布、推送镜像或 AUR 更新。实际发布使用独立版本 tag，并与已验收提交和产物校验和对应。
