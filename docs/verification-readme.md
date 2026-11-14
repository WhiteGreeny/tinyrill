# tinyrill 验证附件

本包无需放在主程序旁。与运行包、源码包一起保存，用于验收、排查和后续重新构建。

| 内容 | 用途 |
|---|---|
| `provenance.json` | 版本、平台、源码提交，以及运行归档、程序、源码归档的 SHA256；未签名 |
| `build-info.txt` | 实际可执行文件的 Go 构建信息 |
| `sbom.cdx.json` | 本平台的依赖版本与许可证检测结果 |
| `vulnerabilities-*.json` / `.txt` | 本平台源码与实际二进制的 govulncheck JSON 事件流及可读结果 |
| `licenses/`、`THIRD_PARTY.md`、`LICENSE` | 逐依赖的原始许可证/NOTICE、来源索引和项目许可证 |
| `docs/` | 配置、容器、多客户分流、验收与历史实施记录 |
| `examples/`、`docker-compose.yml` | 原生、多客户、容器配置与验收目标示例 |
| `scripts/soak.ps1` | 经明确代理进行持续业务探测的验收工具 |
| `SHA256SUMS` | 本验证包文件校验清单 |

在 `provenance.json` 找到同版本运行包和源码包名称，校验归档 SHA256 后再配对使用。源码包包含本次完整已提交源码、构建脚本和修改说明。镜像归档与镜像扫描报告另行提供；Go SBOM 不覆盖基础镜像中的全部系统组件。

持续验收从 `examples/acceptance-targets.json` 复制目标配置，再运行 `pwsh scripts/soak.ps1 -TargetsFile /path/to/targets.json`；目标配置涉及客户网络信息，保存在受控位置。具体参数和验收边界见 [验收说明](docs/acceptance.md)。
