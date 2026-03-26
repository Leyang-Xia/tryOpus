#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
if [[ -z "${SIGNAL_PORT:-}" ]]; then
  SIGNAL_PORT="$(python3 - <<'PY'
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
PY
)"
fi
SIGNAL_URL="http://127.0.0.1:${SIGNAL_PORT}"
SESSION_ID="session-$(date +%s)"
INPUT_WAV="${TMP_DIR}/tone_48k_mono.wav"
OUTPUT_WAV="${TMP_DIR}/received.wav"
STATS_JSON="${TMP_DIR}/stats.json"
OPUS_PKG_CONFIG="${OPUS_PKG_CONFIG:-${ROOT_DIR}/../opus-install/lib/pkgconfig}"
PROJECT_OPUS_DYLIB="${PROJECT_OPUS_DYLIB:-${ROOT_DIR}/../opus-install/lib/libopus.0.dylib}"
PROJECT_OPUS_SO="${PROJECT_OPUS_SO:-${ROOT_DIR}/../opus-install/lib/libopus.so.0}"
export PKG_CONFIG_PATH="${OPUS_PKG_CONFIG}:${PKG_CONFIG_PATH:-}"
export LD_LIBRARY_PATH="${ROOT_DIR}/../opus-install/lib:${LD_LIBRARY_PATH:-}"
export DYLD_LIBRARY_PATH="${ROOT_DIR}/../opus-install/lib:${DYLD_LIBRARY_PATH:-}"
WEIGHTS_PATH="${WEIGHTS_PATH:-${ROOT_DIR}/../weights_blob.bin}"
SIM_LOSS="${SIM_LOSS:-0.10}"
SIM_SEED="${SIM_SEED:-42}"
BIN_DIR="${TMP_DIR}/bin"

cleanup() {
  if [[ -n "${SIGNAL_PID:-}" ]]; then
    kill "${SIGNAL_PID}" >/dev/null 2>&1 || true
  fi
  if [[ -n "${RECEIVER_PID:-}" ]]; then
    kill "${RECEIVER_PID}" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

verify_binary_libopus() {
  local bin_path="$1"
  local linked_path
  local expected_path
  local os_name

  os_name="$(uname -s)"
  case "${os_name}" in
    Darwin)
      expected_path="$(python3 -c 'import os,sys; print(os.path.realpath(sys.argv[1]))' "${PROJECT_OPUS_DYLIB}")"
      linked_path="$(otool -L "${bin_path}" | awk '/libopus\.0\.dylib/ {print $1; exit}')"
      ;;
    Linux)
      expected_path="$(python3 -c 'import os,sys; print(os.path.realpath(sys.argv[1]))' "${PROJECT_OPUS_SO}")"
      linked_path="$(ldd "${bin_path}" | awk '/libopus\.so/ {print $3; exit}')"
      ;;
    *)
      echo "[test] WARN: skip libopus linkage validation on unsupported OS: ${os_name}" >&2
      return 0
      ;;
  esac

  if [[ -z "${linked_path}" || "${linked_path}" == "not" ]]; then
    echo "[test] ERROR: ${bin_path} is missing libopus dependency" >&2
    exit 1
  fi

  linked_path="$(python3 -c 'import os,sys; print(os.path.realpath(sys.argv[1]))' "${linked_path}")"
  if [[ "${linked_path}" != "${expected_path}" ]]; then
    echo "[test] ERROR: ${bin_path} linked unexpected libopus: ${linked_path}" >&2
    echo "[test] expected project libopus: ${expected_path}" >&2
    exit 1
  fi
}

echo "[test] tmp dir: ${TMP_DIR}"
echo "[test] using libopus pkg-config: ${OPUS_PKG_CONFIG}"
echo "[test] using weights blob: ${WEIGHTS_PATH}"

mkdir -p "${BIN_DIR}"
(
  cd "${ROOT_DIR}"
  go clean -cache
  PKG_CONFIG_PATH="${OPUS_PKG_CONFIG}" go build -o "${BIN_DIR}/signaling" ./signaling
  PKG_CONFIG_PATH="${OPUS_PKG_CONFIG}" go build -o "${BIN_DIR}/receiver" ./receiver
  PKG_CONFIG_PATH="${OPUS_PKG_CONFIG}" go build -o "${BIN_DIR}/sender" ./sender
)
verify_binary_libopus "${BIN_DIR}/receiver"
verify_binary_libopus "${BIN_DIR}/sender"

(
  cd "${TMP_DIR}"
  python3 "${ROOT_DIR}/scripts/gen_tone.py"
)

echo "[test] starting signaling server on ${SIGNAL_URL}"
(
  cd "${ROOT_DIR}"
  "${BIN_DIR}/signaling" -addr ":${SIGNAL_PORT}" >"${TMP_DIR}/signaling.log" 2>&1
) &
SIGNAL_PID=$!
sleep 1

echo "[test] starting receiver session=${SESSION_ID}"
(
  cd "${ROOT_DIR}"
  "${BIN_DIR}/receiver" \
    --signal "${SIGNAL_URL}" \
    --session "${SESSION_ID}" \
    --output "${OUTPUT_WAV}" \
    --stats-json "${STATS_JSON}" \
    --sim-loss "${SIM_LOSS}" \
    --sim-seed "${SIM_SEED}" \
    --use-dred=true \
    --use-lbrr=true \
    --weights "${WEIGHTS_PATH}" \
    --duration 4s >"${TMP_DIR}/receiver.log" 2>&1
) &
RECEIVER_PID=$!
sleep 1

echo "[test] starting sender"
(
  cd "${ROOT_DIR}"
  "${BIN_DIR}/sender" \
    --signal "${SIGNAL_URL}" \
    --session "${SESSION_ID}" \
    --input "${INPUT_WAV}" \
    --fec=true \
    --plp 15 \
    --dred 3 \
    --weights "${WEIGHTS_PATH}" >"${TMP_DIR}/sender.log" 2>&1
)

wait "${RECEIVER_PID}"

if [[ ! -f "${OUTPUT_WAV}" ]]; then
  echo "[test] ERROR: output wav not generated"
  exit 1
fi
if [[ "$(stat -c%s "${OUTPUT_WAV}")" -le 44 ]]; then
  echo "[test] ERROR: output wav too small"
  exit 1
fi
if [[ ! -f "${STATS_JSON}" ]]; then
  echo "[test] ERROR: stats json not generated"
  exit 1
fi

echo "[test] PASS"
echo "[test] input:  ${INPUT_WAV}"
echo "[test] output: ${OUTPUT_WAV}"
echo "[test] stats:  ${STATS_JSON}"
echo "[test] logs:   ${TMP_DIR}/sender.log ${TMP_DIR}/receiver.log ${TMP_DIR}/signaling.log"
