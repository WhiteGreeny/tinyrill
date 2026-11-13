#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
if [[ $(id -u) == 0 ]]; then echo 'Run this smoke test as a normal rootless user.' >&2; exit 1; fi
[[ $(podman info --format '{{.Host.Security.Rootless}}') == true ]]
test -x dist/tinyrill-linux-amd64/tinyrill
read -r _ version revision _ <<<"$(dist/tinyrill-linux-amd64/tinyrill version)"
revision=${revision#(}
revision=${revision%)}
[[ "$revision" =~ ^[0-9a-f]{40}$ ]]
[[ "$version" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$ ]]
# Some Podman/Buildah versions reuse LABEL layers across changed build arguments.
podman build --no-cache --build-arg TARGETARCH=amd64 --build-arg VERSION="$version" --build-arg COMMIT="$revision" -t localhost/tinyrill:smoke .
[[ $(podman image inspect localhost/tinyrill:smoke --format '{{index .Config.Labels "org.opencontainers.image.version"}}') == "$version" ]]
[[ $(podman image inspect localhost/tinyrill:smoke --format '{{index .Config.Labels "org.opencontainers.image.revision"}}') == "$revision" ]]
settings=(--rm --read-only --cap-drop=all --security-opt=no-new-privileges --userns=keep-id:uid=65532,gid=65532 --pids-limit=128)
podman run "${settings[@]}" localhost/tinyrill:smoke version
podman run "${settings[@]}" -v "$PWD/examples/customer-container.yaml:/run/tinyrill/customer.yaml:ro,Z" localhost/tinyrill:smoke config-check --config /run/tinyrill/customer.yaml
# Verify private state writes under the non-root identity and read-only root.
# Authenticated proxy and live state-lock isolation are separate acceptance checks.
volume="tinyrill-smoke-$RANDOM-$$"
podman volume create "$volume" >/dev/null
trap 'podman volume rm "$volume" >/dev/null' EXIT
podman run "${settings[@]}" -v "$volume:/state:U" -v "$PWD/examples/customer-container.yaml:/run/tinyrill/customer.yaml:ro,Z" localhost/tinyrill:smoke session clear --config /run/tinyrill/customer.yaml
podman image inspect localhost/tinyrill:smoke --format '{{.Id}} {{.Config.User}}'
