#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
version=${1:?Usage: scan-image.sh VERSION}
[[ "$version" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$ ]]
image="localhost/tinyrill:$version"
image_archive="dist/$version/tinyrill-$version-linux-amd64-image.tar"
report="dist/$version/tinyrill-$version-image-scan.json"
test -f "dist/$version/release.json"
if [[ -e "$image_archive" || -e "$report" ]]; then echo 'Image outputs already exist; refusing to overwrite.' >&2; exit 1; fi
mkdir -p _build/scanner
archive=_build/scanner/trivy_0.76.2_Linux-64bit.tar.gz
if [[ ! -f "$archive" ]]; then
  curl --fail --silent --show-error --location -o "$archive" https://github.com/aquasecurity/trivy/releases/download/v0.76.2/trivy_0.76.2_Linux-64bit.tar.gz
fi
echo "bc0fc726e017b8927a62b2d3531d7a84ee01769daf5c99ed866dced54089fdbf  $archive" | sha256sum -c -
tar -xzf "$archive" -C _build/scanner trivy
podman save --format docker-archive -o "$image_archive" "$image"
_build/scanner/trivy image --input "$image_archive" --scanners vuln --format json --output "$report"
# Keep all severities in the report; HIGH/CRITICAL require resolution before release.
_build/scanner/trivy image --input "$image_archive" --scanners vuln --skip-db-update --severity HIGH,CRITICAL --exit-code 1
(cd "dist/$version" && sha256sum "$(basename "$image_archive")" >"$(basename "$image_archive").sha256")
