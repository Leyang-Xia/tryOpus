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
OPUS_PKG_CONFIG="${OPUS_PKG_CONFIG:-/workspace/opus-install/lib/pkgconfig}"
export PKG_CONFIG_PATH="${OPUS_PKG_CONFIG}:${PKG_CONFIG_PATH:-}"
export LD_LIBRARY_PATH="/workspace/opus-install/lib:${LD_LIBRARY_PATH:-}"
WEIGHTS_PATH="${WEIGHTS_PATH:-/workspace/weights_blob.bin}"
SIM_LOSS="${SIM_LOSS:-0.10}"
SIM_SEED="${SIM_SEED:-42}"

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
echo "[test] using libopus pkg-config: ${OPUS_PKG_CONFIG}"
echo "[test] using weights blob: ${WEIGHTS_PATH}"

(
  cd "${TMP_DIR}"
  python3 "${ROOT_DIR}/scripts/gen_tone.py"
)

echo "[test] starting signaling server on ${SIGNAL_URL}"
(
  cd "${ROOT_DIR}"
  go run ./signaling -addr ":${SIGNAL_PORT}" >"${TMP_DIR}/signaling.log" 2>&1
) &
SIGNAL_PID=$!
sleep 1

echo "[test] starting receiver session=${SESSION_ID}"
(
  cd "${ROOT_DIR}"
  go run ./receiver \
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
  go run ./sender \
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
