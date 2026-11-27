#!/usr/bin/env bash
# Start the current PA#3 application locally, with synthetic E2E data.
set -Eeuo pipefail
umask 077

PROJECT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_DIR"

WITH_SEED=1
WITH_SMOKE=0
SEED_OPTIONS=()
for option in "$@"; do
  case "$option" in
    --no-seed) WITH_SEED=0 ;;
    --smoke) WITH_SMOKE=1 ;;
    --resubmit) SEED_OPTIONS+=(--resubmit) ;;
    --help|-h)
      echo 'Usage: ./scripts/run-local.sh [--no-seed] [--smoke] [--resubmit]'
      echo 'Runs Go API + worker + Next.js locally; Docker supplies PostgreSQL and isolated C++ execution.'
      exit 0 ;;
    *) echo "Unknown option: $option" >&2; exit 2 ;;
  esac
done
if (( (WITH_SMOKE || ${#SEED_OPTIONS[@]}) && ! WITH_SEED )); then
  echo '--smoke and --resubmit require the default seed; omit --no-seed.' >&2
  exit 2
fi

for required in node npm docker setsid flock sha256sum; do
  command -v "$required" >/dev/null || { echo "Required command missing: $required" >&2; exit 1; }
done
node -e 'const [a,b]=process.versions.node.split(".").map(Number);if(a<22||(a===22&&b<13))process.exit(1)' || {
  echo 'Node.js 22.13 or later is required.' >&2; exit 1;
}
GO_COMMAND="${LOCAL_GO_BIN:-$(command -v go || true)}"
if [[ ( -z "$GO_COMMAND" || "$("$GO_COMMAND" version 2>/dev/null || true)" != 'go version go1.27.4 '* ) &&
      -z "${LOCAL_GO_BIN:-}" && -x /snap/go/current/bin/go ]]; then
  GO_COMMAND=/snap/go/current/bin/go
fi
if [[ -z "$GO_COMMAND" || "$("$GO_COMMAND" version 2>/dev/null || true)" != 'go version go1.27.4 '* ]]; then
  echo 'Go 1.27.4 is required. Set LOCAL_GO_BIN to its executable.' >&2
  exit 1
fi

export LOCAL_WEB_PORT="${LOCAL_WEB_PORT:-3000}"
export LOCAL_API_PORT="${LOCAL_API_PORT:-8080}"
export LOCAL_POSTGRES_PORT="${LOCAL_POSTGRES_PORT:-5545}"
node -e 'const p=process.argv.slice(1).map(Number);if(p.some(v=>!Number.isInteger(v)||v<1025||v>65535)||new Set(p).size!==3)process.exit(1)' \
  "$LOCAL_WEB_PORT" "$LOCAL_API_PORT" "$LOCAL_POSTGRES_PORT" || {
  echo 'Local ports must be distinct integers between 1025 and 65535.' >&2; exit 2;
}

LOCAL_STATE="$PROJECT_DIR/.local-dev/v2"
mkdir -p "$LOCAL_STATE/bin" "$LOCAL_STATE/logs" "$LOCAL_STATE/work"
exec 9>"$LOCAL_STATE/run.lock"
flock -n 9 || { echo 'The local launcher is already running.' >&2; exit 1; }

# Generate only a synthetic local credential; never inherit an existing secret.
node --input-type=module - "$LOCAL_STATE/database-password" <<'NODE'
import { randomBytes } from 'node:crypto';
import { writeFileSync, existsSync } from 'node:fs';
const path = process.argv[2];
if (!existsSync(path)) writeFileSync(path, randomBytes(32).toString('base64url'), {mode: 0o600, flag: 'wx'});
NODE
LOCAL_DATABASE_PASSWORD="$(<"$LOCAL_STATE/database-password")"
[[ "$LOCAL_DATABASE_PASSWORD" =~ ^[A-Za-z0-9_-]{32,}$ ]] || { echo 'Invalid local database password file.' >&2; exit 1; }

compose() {
  env -i PATH="$PATH" LOCAL_POSTGRES_PORT="$LOCAL_POSTGRES_PORT" LOCAL_DATABASE_PASSWORD="$LOCAL_DATABASE_PASSWORD" \
    docker --host unix:///var/run/docker.sock compose --ansi never --progress plain --project-name leetkob-local-v2 \
    --env-file /dev/null -f "$PROJECT_DIR/scripts/local-compose.yml" "$@"
}
local_docker() {
  env -i PATH="$PATH" docker --host unix:///var/run/docker.sock "$@"
}
local_docker info >/dev/null || { echo 'Local Docker Engine must be running and accessible.' >&2; exit 1; }
node --input-type=module - "$LOCAL_API_PORT" "$LOCAL_WEB_PORT" <<'NODE'
import net from 'node:net';
for (const port of process.argv.slice(2).map(Number)) {
  await new Promise((resolve, reject) => {
    const server = net.createServer();
    server.once('error', () => reject(new Error(`Port ${port} is already in use; set LOCAL_API_PORT or LOCAL_WEB_PORT.`)));
    server.listen(port, '127.0.0.1', () => server.close(resolve));
  });
}
NODE

SERVICE_PIDS=()
DATABASE_STARTED=0
cleanup() {
  local result=$?
  trap - EXIT INT TERM
  for service_pid in "${SERVICE_PIDS[@]}"; do
    kill -TERM -- "-$service_pid" 2>/dev/null || true
  done
  for service_pid in "${SERVICE_PIDS[@]}"; do
    wait "$service_pid" 2>/dev/null || true
  done
  if (( DATABASE_STARTED )); then compose stop postgres >/dev/null 2>&1 || true; fi
  echo 'Local services stopped. Synthetic data is preserved.'
  exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

if [[ ! -x node_modules/.bin/next ]]; then npm ci --ignore-scripts; fi
if ! local_docker image inspect leetkob-cpp17:pa3 >/dev/null 2>&1; then
  local_docker build -t leetkob-cpp17:pa3 apps/backend/sandbox/cpp17
fi
ANALYZER_INPUTS="$(sha256sum apps/backend/checker/Dockerfile scripts/local-analyzer.Dockerfile \
  apps/backend/cmd/analyze-isolated/*.go apps/backend/internal/checker/hardrule/*.go \
  apps/backend/internal/checker/schema/*.go apps/backend/native/cpp-analyzer/analyzer.c \
  apps/backend/go.mod apps/backend/go.sum | sha256sum)"
ANALYZER_INPUTS="${ANALYZER_INPUTS%% *}"
if [[ "$(local_docker image inspect --format '{{index .Config.Labels "org.leetkob.local-analyzer.inputs"}}' leetkob-local-analyzer:pa3 2>/dev/null || true)" != "$ANALYZER_INPUTS" ]]; then
  local_docker build -f apps/backend/checker/Dockerfile -t leetkob-local-checker:pa3 apps/backend
  local_docker build -f scripts/local-analyzer.Dockerfile \
    --label "org.leetkob.local-analyzer.inputs=$ANALYZER_INPUTS" -t leetkob-local-analyzer:pa3 apps/backend
fi
echo 'Building local Go commands...'
for executable in api worker provision; do
  (cd apps/backend && CGO_ENABLED=0 GOTOOLCHAIN=local "$GO_COMMAND" build -o "$LOCAL_STATE/bin/$executable" "./cmd/$executable")
done
echo 'Starting the local database...'
DATABASE_STARTED=1
compose up -d --wait --wait-timeout 90 postgres

# Local socket access lets an existing synthetic volume adopt the persisted
# generated password as well; no application records are removed.
printf "ALTER ROLE leetkob_local PASSWORD '%s';\n" "$LOCAL_DATABASE_PASSWORD" | \
  compose exec -T postgres psql -v ON_ERROR_STOP=1 -U leetkob_local -d leetkob_local >/dev/null

LOCAL_DATABASE="postgres://leetkob_local:$LOCAL_DATABASE_PASSWORD@127.0.0.1:$LOCAL_POSTGRES_PORT/leetkob_local?sslmode=disable"
LOCAL_ORIGIN="http://127.0.0.1:$LOCAL_WEB_PORT"
start_service() {
  local name=$1
  shift
  (
    # The controller's private parent stays 0700; its source copy must be
    # readable by the container's uid 65534, as in the qualified sandbox.
    if [[ "$name" == worker ]]; then umask 022; fi
    exec setsid "$@"
  ) >"$LOCAL_STATE/logs/$name.log" 2>&1 9>&- &
  SERVICE_PIDS+=("$!")
}
wait_for() {
  local url=$1
  local attempt
  for (( attempt=0; attempt<120; attempt++ )); do
    if node -e 'fetch(process.argv[1],{signal:AbortSignal.timeout(1500)}).then(r=>process.exit(r.status===200?0:1)).catch(()=>process.exit(1))' "$url"; then return; fi
    for service_pid in "${SERVICE_PIDS[@]}"; do
      if ! kill -0 "$service_pid" 2>/dev/null; then
        echo "A service exited. See $LOCAL_STATE/logs/" >&2
        tail -n 15 "$LOCAL_STATE/logs/"*.log >&2
        return 1
      fi
    done
    sleep 1
  done
  echo "Timed out waiting for $url. See $LOCAL_STATE/logs/" >&2
  return 1
}

start_service api env -i PATH="$PATH" DATABASE_URL="$LOCAL_DATABASE" \
  API_HOST=127.0.0.1 PORT="$LOCAL_API_PORT" WEB_ORIGIN="$LOCAL_ORIGIN" SECURE_COOKIES=0 \
  "$LOCAL_STATE/bin/api"
wait_for "http://127.0.0.1:$LOCAL_API_PORT/healthz"
start_service worker env -i PATH="$PATH" DATABASE_URL="$LOCAL_DATABASE" MODEL_OFF=1 \
  WORKER_ID=local-worker WORKER_ISOLATED_ANALYZER=1 \
  TMPDIR="$LOCAL_STATE/work" \
  "$LOCAL_STATE/bin/worker"

echo 'Starting Next.js locally...'
start_service web env -i PATH="$PATH" PA3_API_ORIGIN="http://127.0.0.1:$LOCAL_API_PORT" NEXT_TELEMETRY_DISABLED=1 \
  node "$PROJECT_DIR/node_modules/next/dist/bin/next" dev "$PROJECT_DIR/apps/web" \
  --hostname 127.0.0.1 --port "$LOCAL_WEB_PORT"
wait_for "$LOCAL_ORIGIN/live"
if (( WITH_SEED )); then node scripts/e2e.mjs seed "${SEED_OPTIONS[@]}"; fi
if (( WITH_SMOKE )); then node scripts/e2e.mjs smoke; fi

echo
echo "Application ready: $LOCAL_ORIGIN/live"
echo "Synthetic accounts/passwords and lab links: $LOCAL_STATE/state.json"
echo "Logs: $LOCAL_STATE/logs/"
echo 'Press Ctrl+C to stop the application. The database volume is retained.'
# Monitor all services so an unexpected exit fails the launcher.
while true; do
  for service_pid in "${SERVICE_PIDS[@]}"; do
    kill -0 "$service_pid" 2>/dev/null || { echo 'A local service exited; inspect its log.' >&2; exit 1; }
  done
  sleep 2
done
