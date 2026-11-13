# 来源与依赖

tinyrill 基于 zju-connect 的 Git 提交 `8576e7a16076c4175d6dbe276573a353cf7260be`；原仓库是 <https://github.com/Mythologyli/zju-connect>。本分支保留上游 Git 历史、源码版权声明以及根目录的 AGPL-3.0 许可证。EasierConnect 等协议实现的来源声明仍在相应源文件中。

模块路径暂时沿用 `github.com/mythologyli/zju-connect`，只为保持内部导入及上游补丁可比性。发布项目与二进制名称为 tinyrill，发布仓库为 <https://github.com/WhiteGreeny/tinyrill>。

`go.mod` / `go.sum` 固定依赖与校验值，其中 netaddr 显式替换到其维护仓库。发布脚本生成含依赖版本和许可证检测证据的 `sbom.cdx.json`、实际二进制的 `build-info.txt`，并打包对应提交的 `source.tar.gz`。`licenses/` 提供 Go 及模块原始许可证/NOTICE，`index.json` 记录模块、实际来源及版本。许可证自动检测结果可能不完整；对外分发前应检查 SBOM 中未识别项目并保留所需声明。

运行镜像采用固定 digest 的 Google Distroless static-debian13 nonroot，包含根证书等运行材料，未引入协议或认证的第二套实现。镜像来源和许可证材料参见 <https://github.com/GoogleContainerTools/distroless>。二进制 SBOM 不覆盖基础镜像的所有系统文件；镜像应另外按其 digest 扫描。
