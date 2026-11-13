# 构建与候选发布

当前交付为待实际客户验收的候选版，不创建正式兼容声明。用户安排客户验收，记录在 [验收表](acceptance.md)。本地 Git 提交不等于推送、发布 release 或上传镜像。

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

Linux/macOS 另运行 `go test -race -count=1 -timeout=180s ./...`。目标目录已存在时脚本拒绝覆盖；将先前产物移到独立目录再重建。Windows/Linux 可交叉构建 amd64/arm64，实际运行验收分别记录。macOS 发布必须使用本机编译器，不在 Windows 上伪造完成状态。

`dist/tinyrill-<os>-<arch>/` 包含可执行文件、对应 Git 提交的 source.tar.gz、LICENSE、第三方来源、示例、文档、CycloneDX SBOM、源码/二进制 govulncheck JSON、build-info、provenance.json 和 SHA256SUMS。工具固定为 govulncheck v1.9.0、cyclonedx-gomod v1.12.0；扫描发现漏洞时打包失败。SBOM 包含 Go 标准库和自动检测的许可证证据，`licenses/` 同时保留 Go 与模块原始许可证/NOTICE 和来源索引。provenance 是构建记录，未签名，不冒称 SLSA/签名证明。

Windows 输出 ZIP，Linux/macOS 输出 tar.gz；归档旁有单独 SHA256 文件。Unix 归档内的二进制显式保留 0755 执行权限，即使交叉构建主机为 Windows。

发布二进制采用 trimpath，显式版本和完整提交。时间戳位于构建记录和 SBOM；不承诺整个目录的字节级可复现。工具与模块下载由 Go 校验；扫描数据库是构建时获取的当前数据，报告记录其时间，后续发布需重扫。

可以使用 `sha256sum -c SHA256SUMS` 核验包内文件。Windows 可用 `Get-FileHash -Algorithm SHA256` 对照对应条目。发布源码包只包含已提交内容，不含 local/、state/、用户秘密或工作区未提交文件。

## 镜像与正式发布门槛

按 [Podman 指引](docker.md) 构建，运行 `bash scripts/container-smoke.sh` 验证 rootless、只读根、非 root 用户、配置解析与私有状态写入。此检查不连接客户网关。随后由客户验收覆盖认证、两个代理入口、三实例隔离、断线和长时间业务。

只有已验证的客户/平台写入支持列表；其他能力列为待验收。对外分发时一并提供对应源码、校验和、SBOM、扫描报告、许可证、修改记录及基础镜像 digest。基础镜像的系统文件不包含在 Go SBOM 中，需另外扫描；不要将 Go 扫描通过写成镜像完全无漏洞。

GitHub workflow 产物只是候选归档。当前没有自动公开发布、推送镜像或 AUR 更新。实际发布使用独立版本 tag，并与已验收提交和产物校验和对应。
