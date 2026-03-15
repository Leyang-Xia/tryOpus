#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
SIGNAL_PORT="${SIGNAL_PORT:-18090}"
SIGNAL_URL="http://127.0.0.1:${SIGNAL_PORT}"
SESSION_ID="session-$(date +%s)"
INPUT_WAV="${TMP_DIR}/tone_48k_mono.wav"
OUTPUT_WAV="${TMP_DIR}/received.wav"
OPUS_PKG_CONFIG="${OPUS_PKG_CONFIG:-/workspace/opus-install/lib/pkgconfig}"
export PKG_CONFIG_PATH="${OPUS_PKG_CONFIG}:${PKG_CONFIG_PATH:-}"
export LD_LIBRARY_PATH="/workspace/opus-install/lib:${LD_LIBRARY_PATH:-}"
GO_TAGS="${GO_TAGS:-nolibopusfile}"

cleanup() {
  if [[ -n "${SIGNAL_PID:-}" ]]; then
    kill "${SIGNAL_PID}" >/dev/null 2>&1 || true
  fi
  if [[ -n "${RECEIVER_PID:-}" ]]; then
    kill "${RECEIVER_PID}" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

echo "[test] tmp dir: ${TMP_DIR}"

(
  cd "${TMP_DIR}"
  python3 "${ROOT_DIR}/scripts/gen_tone.py"
)

echo "[test] starting signaling server on ${SIGNAL_URL}"
(
  cd "${ROOT_DIR}"
  go run -tags "${GO_TAGS}" ./signaling -addr ":${SIGNAL_PORT}" >"${TMP_DIR}/signaling.log" 2>&1
) &
SIGNAL_PID=$!
sleep 1

echo "[test] starting receiver session=${SESSION_ID}"
(
  cd "${ROOT_DIR}"
  go run -tags "${GO_TAGS}" ./receiver \
    --signal "${SIGNAL_URL}" \
    --session "${SESSION_ID}" \
    --output "${OUTPUT_WAV}" \
    --duration 4s >"${TMP_DIR}/receiver.log" 2>&1
) &
RECEIVER_PID=$!
sleep 1

echo "[test] starting sender"
(
  cd "${ROOT_DIR}"
  go run -tags "${GO_TAGS}" ./sender \
    --signal "${SIGNAL_URL}" \
    --session "${SESSION_ID}" \
    --input "${INPUT_WAV}" >"${TMP_DIR}/sender.log" 2>&1
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

echo "[test] PASS"
echo "[test] input:  ${INPUT_WAV}"
echo "[test] output: ${OUTPUT_WAV}"
echo "[test] logs:   ${TMP_DIR}/sender.log ${TMP_DIR}/receiver.log ${TMP_DIR}/signaling.log"
