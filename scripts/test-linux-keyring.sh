#!/usr/bin/env bash
# A disposable D-Bus/keyring session containing synthetic credentials only.
set -euo pipefail
cd "$(dirname "$0")/.."
test_state=$(mktemp -d /tmp/tinyrill-keyring.XXXXXX)
trap 'rm -rf -- "$test_state"' EXIT
export XDG_DATA_HOME="$test_state/data"
export XDG_CONFIG_HOME="$test_state/config"
export XDG_CACHE_HOME="$test_state/cache"
export GNOME_KEYRING_CONTROL="$test_state/control"
mkdir -p "$XDG_DATA_HOME" "$XDG_CONFIG_HOME" "$XDG_CACHE_HOME" "$GNOME_KEYRING_CONTROL"
export TINYRILL_TEST_NATIVE_CREDENTIALS=1
dbus-run-session -- bash -euo pipefail -c '
  gnome-keyring-daemon --foreground --unlock --components=secrets < <(printf "%s\n" synthetic-test-keyring-passphrase) &
  daemon_pid=$!
  trap "kill $daemon_pid 2>/dev/null || true; wait $daemon_pid 2>/dev/null || true" EXIT
  for attempt in {1..50}; do
    owner=$(dbus-send --session --print-reply --dest=org.freedesktop.DBus / org.freedesktop.DBus.NameHasOwner string:org.freedesktop.secrets)
    if [[ $owner == *"boolean true"* ]]; then break; fi
    sleep 0.1
  done
  go test -count=1 -timeout=60s ./internal/credentials -run TestNativeCredentialRoundTrip -v
'
