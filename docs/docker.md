# rootless Podman

镜像直接复制 `scripts/release.ps1` 生成的 Linux 二进制，没有独立认证实现。当前为候选版；实际客户业务与长时间验收见 [验收表](acceptance.md)。需要已配置普通用户 user namespace 的 Linux Podman；Windows/macOS 上由 Linux 虚拟机承载运行。

## 构建

在干净提交上运行（PowerShell 7.3+，Windows 也可以交叉构建 Linux）：

```powershell
pwsh scripts/release.ps1 -TargetOS linux -TargetArch amd64 -Version candidate
```

然后在有 Podman 的 Linux 环境构建同架构镜像：

```sh
podman build --no-cache --build-arg TARGETARCH=amd64 \
  --build-arg VERSION=candidate --build-arg COMMIT="$(git rev-parse HEAD)" \
  -t localhost/tinyrill:candidate .
```

基础镜像已固定 digest；构建上下文只允许发布二进制与许可证。使用 `--no-cache` 避免部分 Podman/Buildah 版本跨构建参数复用旧 LABEL；版本与 revision 必须和二进制的 `version` 输出一致。arm64 使用对应发布目录和 TARGETARCH；本地架构必须匹配，或使用另行配置的模拟环境。源码、SBOM 与报告在发布目录，分发镜像时一起提供对应源码包。

## 前台运行

复制 `examples/customer-container.yaml` 为 `local/customer-a-container.yaml`，编辑网关与账户。该配置在容器内监听 0.0.0.0；主机仅发布回环端口。EC 旧 TLS 兼容按该客户明确配置，默认不开启。

```sh
podman volume create customer-a-state
podman run --rm -it --name tinyrill-a \
  --userns=keep-id:uid=65532,gid=65532 \
  --read-only --cap-drop=all --security-opt=no-new-privileges \
  --pids-limit=128 --tmpfs /tmp:rw,noexec,nosuid,nodev,size=16m,mode=1777 \
  -p 127.0.0.1:21080:1080 -p 127.0.0.1:21081:1081 \
  -p 127.0.0.1:21999:10999 \
  -v "$PWD/local/customer-a-container.yaml:/run/tinyrill/customer.yaml:ro,Z" \
  -v customer-a-state:/state:U \
  localhost/tinyrill:candidate
```

镜像默认非 root UID 65532。keep-id 映射当前用户，`:U` 只调整该命名卷的所有者；不要对任意主机目录使用这个选项。没有 host 网络、容器 socket、/dev/net/tun 或 NET_ADMIN。`:Z` 用于启用 SELinux 的主机。参考 [Podman run 的卷与用户映射](https://docs.podman.io/en/latest/markdown/podman-run.1.html)。

点选验证码使用固定的 10999 容器端口和 21999 主机回环端口，`callback_url` 必须与主机映射一致。B/C 使用各自 31999/41999 回调端口与独立卷，并修改客户 ID、协议、认证信息和 state_dir。图片 file 模式可省略回调端口，通过卷内原图与终端坐标 JSON 操作。

也提供 `docker-compose.yml` 作为 Podman Compose 示例；需前台交互时使用 `podman compose run --rm --service-ports tinyrill`，不要先后台启动再期待不可见终端收验证码。该文件使用 Podman 的 userns 扩展，未声明 Docker Compose 等价支持。

## 只读 secret

首次版本不连接宿主凭据库。可由部署方创建 Podman secret，并设置 `auth.secret_file: /run/secrets/customer-password`、`remember_password: false`。例如从已受保护的文件导入（这里不把密码放入 argv）：

```sh
podman secret create customer-a-password /private/path/customer-a-password.txt
```

在上述 run 参数中加入：

```text
--secret customer-a-password,target=customer-password,uid=65532,gid=65532,mode=0400
```

secret 的源文件、Podman 存储、更新与删除由部署方管理；Podman secret 本身不被本客户端当作系统加密凭据库。后台运行设 `auth.interactive: never`，不用 `-it`，人工挑战时返回退出码 3。默认不配置自动重启；避免持续重新发短信或尝试失效密码。参考 [Podman secrets](https://docs.podman.io/en/latest/markdown/podman-secret-create.1.html)。

`podman stop tinyrill-a` 只停止 A。状态卷保存敏感会话，不上传或与其他客户共享；删除保存的密码和清除会话是独立操作，详见 [配置参考](configuration.md)。
