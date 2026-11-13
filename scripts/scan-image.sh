#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p dist/scanner dist/verification
archive=dist/scanner/trivy_0.76.2_Linux-64bit.tar.gz
curl --fail --silent --show-error --location -o "$archive" https://github.com/aquasecurity/trivy/releases/download/v0.76.2/trivy_0.76.2_Linux-64bit.tar.gz
echo "bc0fc726e017b8927a62b2d3531d7a84ee01769daf5c99ed866dced54089fdbf  $archive" | sha256sum -c -
tar -xzf "$archive" -C dist/scanner trivy
podman save --format docker-archive -o dist/verification/tinyrill-image.tar localhost/tinyrill:smoke
dist/scanner/trivy image --input dist/verification/tinyrill-image.tar --scanners vuln --format json --output dist/verification/trivy-image.json
# Keep all severities in the report; HIGH/CRITICAL require resolution before release.
dist/scanner/trivy image --input dist/verification/tinyrill-image.tar --scanners vuln --skip-db-update --severity HIGH,CRITICAL --exit-code 1
