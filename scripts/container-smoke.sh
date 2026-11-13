#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
if [[ $(id -u) == 0 ]]; then echo 'Run this smoke test as a normal rootless user.' >&2; exit 1; fi
[[ $(podman info --format '{{.Host.Security.Rootless}}') == true ]]
test -x dist/tinyrill-linux-amd64/tinyrill
revision=$(git rev-parse HEAD)
podman build --build-arg TARGETARCH=amd64 --build-arg COMMIT="$revision" -t localhost/tinyrill:smoke .
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
