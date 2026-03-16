#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
SIGNAL_PORT="${SIGNAL_PORT:-18091}"
SIGNAL_URL="http://127.0.0.1:${SIGNAL_PORT}"
OPUS_PKG_CONFIG="${OPUS_PKG_CONFIG:-/workspace/opus-install/lib/pkgconfig}"
WEIGHTS_PATH="${WEIGHTS_PATH:-/workspace/weights_blob.bin}"
SIM_LOSS="${SIM_LOSS:-0.10}"
SIM_SEED="${SIM_SEED:-42}"
INPUT_WAV="${INPUT_WAV:-${TMP_DIR}/tone_48k_mono.wav}"
SUMMARY_CSV="${SUMMARY_CSV:-${TMP_DIR}/rtc_experiment_summary.csv}"

export PKG_CONFIG_PATH="${OPUS_PKG_CONFIG}:${PKG_CONFIG_PATH:-}"
export LD_LIBRARY_PATH="/workspace/opus-install/lib:${LD_LIBRARY_PATH:-}"

cleanup() {
  if [[ -n "${SIGNAL_PID:-}" ]]; then
    kill "${SIGNAL_PID}" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

if [[ ! -f "${INPUT_WAV}" ]]; then
  (
    cd "${TMP_DIR}"
    python3 "${ROOT_DIR}/scripts/gen_tone.py"
  )
fi

echo "case,loss,recovered_lbrr,recovered_dred,plc,decode_errors,recovery_rate,output_wav" > "${SUMMARY_CSV}"

echo "[exp] starting signaling server on ${SIGNAL_URL}"
(
  cd "${ROOT_DIR}"
  go run ./signaling -addr ":${SIGNAL_PORT}" >"${TMP_DIR}/signaling.log" 2>&1
) &
SIGNAL_PID=$!
sleep 1

run_case() {
  local case_name="$1"
  local sender_extra="$2"
  local receiver_extra="$3"
  local session="session-${case_name}-$(date +%s%N)"
  local output_wav="${TMP_DIR}/${case_name}.wav"
  local stats_json="${TMP_DIR}/${case_name}.json"

  echo "[exp] running case=${case_name}"
  (
    cd "${ROOT_DIR}"
    go run ./receiver \
      --signal "${SIGNAL_URL}" \
      --session "${session}" \
      --output "${output_wav}" \
      --stats-json "${stats_json}" \
      --sim-loss "${SIM_LOSS}" \
      --sim-seed "${SIM_SEED}" \
      --weights "${WEIGHTS_PATH}" \
      --duration 5s ${receiver_extra} >"${TMP_DIR}/${case_name}_receiver.log" 2>&1
  ) &
  local receiver_pid=$!
  sleep 1

  (
    cd "${ROOT_DIR}"
    go run ./sender \
      --signal "${SIGNAL_URL}" \
      --session "${session}" \
      --input "${INPUT_WAV}" \
      --weights "${WEIGHTS_PATH}" ${sender_extra} >"${TMP_DIR}/${case_name}_sender.log" 2>&1
  )

  wait "${receiver_pid}"

  if [[ ! -f "${stats_json}" ]]; then
    echo "[exp] case=${case_name} failed: missing ${stats_json}" >&2
    exit 1
  fi

  python3 - "${case_name}" "${SIM_LOSS}" "${stats_json}" "${output_wav}" "${SUMMARY_CSV}" <<'PY'
import json
import sys

case_name, sim_loss, stats_path, output_wav, summary_csv = sys.argv[1:6]
with open(stats_path, "r", encoding="utf-8") as f:
    s = json.load(f)
row = ",".join([
    case_name,
    sim_loss,
    str(s.get("recovered_lbrr", 0)),
    str(s.get("recovered_dred", 0)),
    str(s.get("plc_frames", 0)),
    str(s.get("decode_errors", 0)),
    f'{s.get("recovery_rate", 0.0):.4f}',
    output_wav,
])
with open(summary_csv, "a", encoding="utf-8") as out:
    out.write(row + "\n")
print(f"[exp] {row}")
PY
}

run_case "baseline_no_protection" "--fec=false --dred=0 --plp=0" "--use-lbrr=false --use-dred=false"
run_case "lbrr_only" "--fec=true --dred=0 --plp=15" "--use-lbrr=true --use-dred=false"
run_case "dred_only" "--fec=false --dred=3 --plp=15" "--use-lbrr=false --use-dred=true"
run_case "lbrr_dred" "--fec=true --dred=3 --plp=15" "--use-lbrr=true --use-dred=true"

echo "[exp] done"
echo "[exp] summary csv: ${SUMMARY_CSV}"
echo "[exp] temp logs dir: ${TMP_DIR}"
