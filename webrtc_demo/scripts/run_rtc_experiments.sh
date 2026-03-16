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
OPUS_PKG_CONFIG="${OPUS_PKG_CONFIG:-${ROOT_DIR}/../opus-install/lib/pkgconfig}"
WEIGHTS_PATH="${WEIGHTS_PATH:-${ROOT_DIR}/../weights_blob.bin}"
SIM_LOSS="${SIM_LOSS:-0.00}"
SIM_GE="${SIM_GE:-true}"
SIM_GE_P2B="${SIM_GE_P2B:-0.08}"
SIM_GE_B2G="${SIM_GE_B2G:-0.25}"
SIM_GE_BLOSS="${SIM_GE_BLOSS:-0.85}"
SIM_DELAY_MS="${SIM_DELAY_MS:-0}"
SIM_JITTER_MS="${SIM_JITTER_MS:-0}"
SIM_SEED="${SIM_SEED:-42}"
RECV_DURATION="${RECV_DURATION:-6s}"
AUDIO_PRESET="${AUDIO_PRESET:-representative}"
CLIP_SECONDS="${CLIP_SECONDS:-4}"
AUDIO_MANIFEST="${AUDIO_MANIFEST:-${TMP_DIR}/audio_manifest.txt}"
SUMMARY_CSV="${SUMMARY_CSV:-${TMP_DIR}/rtc_experiment_summary.csv}"

export PKG_CONFIG_PATH="${OPUS_PKG_CONFIG}:${PKG_CONFIG_PATH:-}"
export LD_LIBRARY_PATH="${ROOT_DIR}/../opus-install/lib:${LD_LIBRARY_PATH:-}"

cleanup() {
  if [[ -n "${SIGNAL_PID:-}" ]]; then
    kill "${SIGNAL_PID}" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

if [[ "${AUDIO_PRESET}" == "representative" ]]; then
  python3 "${ROOT_DIR}/scripts/prepare_representative_audio.py" \
    --out-dir "${TMP_DIR}/audio" \
    --manifest "${AUDIO_MANIFEST}" \
    --clip-seconds "${CLIP_SECONDS}"
else
  if [[ -z "${INPUT_WAV:-}" ]]; then
    echo "[exp] ERROR: when AUDIO_PRESET!=representative, INPUT_WAV must be set" >&2
    exit 1
  fi
  printf "custom|%s\n" "${INPUT_WAV}" > "${AUDIO_MANIFEST}"
fi

echo "audio_type,case,sim_mode,sim_loss,recovered_lbrr,recovered_dred,plc,decode_errors,recovery_rate,output_wav" > "${SUMMARY_CSV}"

echo "[exp] starting signaling server on ${SIGNAL_URL}"
(
  cd "${ROOT_DIR}"
  go run ./signaling -addr ":${SIGNAL_PORT}" >"${TMP_DIR}/signaling.log" 2>&1
) &
SIGNAL_PID=$!
sleep 1

run_case() {
  local audio_type="$1"
  local input_wav="$2"
  local case_name="$3"
  local sim_mode="$4"
  local sim_loss_value="$5"
  local sim_extra="$6"
  local sender_extra="$7"
  local receiver_extra="$8"
  local session="session-${audio_type}-${case_name}-$(date +%s%N)"
  local output_wav="${TMP_DIR}/${audio_type}_${case_name}.wav"
  local stats_json="${TMP_DIR}/${audio_type}_${case_name}.json"

  echo "[exp] running audio=${audio_type} case=${case_name}"
  (
    cd "${ROOT_DIR}"
    go run ./receiver \
      --signal "${SIGNAL_URL}" \
      --session "${session}" \
      --output "${output_wav}" \
      --stats-json "${stats_json}" \
      --sim-seed "${SIM_SEED}" \
      --weights "${WEIGHTS_PATH}" \
      --duration "${RECV_DURATION}" \
      ${sim_extra} \
      ${receiver_extra} >"${TMP_DIR}/${audio_type}_${case_name}_receiver.log" 2>&1
  ) &
  local receiver_pid=$!
  sleep 1

  (
    cd "${ROOT_DIR}"
    go run ./sender \
      --signal "${SIGNAL_URL}" \
      --session "${session}" \
      --input "${input_wav}" \
      --weights "${WEIGHTS_PATH}" ${sender_extra} >"${TMP_DIR}/${audio_type}_${case_name}_sender.log" 2>&1
  )

  wait "${receiver_pid}"

  if [[ ! -f "${stats_json}" ]]; then
    echo "[exp] case=${case_name} failed: missing ${stats_json}" >&2
    exit 1
  fi

  python3 - "${audio_type}" "${case_name}" "${sim_mode}" "${sim_loss_value}" "${stats_json}" "${output_wav}" "${SUMMARY_CSV}" <<'PY'
import json
import sys

audio_type, case_name, sim_mode, sim_loss, stats_path, output_wav, summary_csv = sys.argv[1:8]
with open(stats_path, "r", encoding="utf-8") as f:
    s = json.load(f)
row = ",".join([
    audio_type,
    case_name,
    sim_mode,
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

sim_mode="uniform"
sim_loss_value="${SIM_LOSS}"
sim_extra="--sim-loss ${SIM_LOSS} --sim-delay-ms ${SIM_DELAY_MS} --sim-jitter-ms ${SIM_JITTER_MS}"
if [[ "${SIM_GE}" == "true" ]]; then
  sim_mode="ge"
  sim_loss_value="p2b=${SIM_GE_P2B};b2g=${SIM_GE_B2G};bloss=${SIM_GE_BLOSS}"
  sim_extra="--sim-ge=true --sim-ge-p2b ${SIM_GE_P2B} --sim-ge-b2g ${SIM_GE_B2G} --sim-ge-bloss ${SIM_GE_BLOSS} --sim-delay-ms ${SIM_DELAY_MS} --sim-jitter-ms ${SIM_JITTER_MS}"
fi

while IFS='|' read -r audio_type input_wav; do
  [[ -z "${audio_type}" ]] && continue
  run_case "${audio_type}" "${input_wav}" "baseline_no_protection" "${sim_mode}" "${sim_loss_value}" "${sim_extra}" "--fec=false --dred=0 --plp=0" "--use-lbrr=false --use-dred=false"
  run_case "${audio_type}" "${input_wav}" "lbrr_only" "${sim_mode}" "${sim_loss_value}" "${sim_extra}" "--fec=true --dred=0 --plp=15" "--use-lbrr=true --use-dred=false"
  run_case "${audio_type}" "${input_wav}" "dred_only" "${sim_mode}" "${sim_loss_value}" "${sim_extra}" "--fec=false --dred=3 --plp=15" "--use-lbrr=false --use-dred=true"
  run_case "${audio_type}" "${input_wav}" "lbrr_dred" "${sim_mode}" "${sim_loss_value}" "${sim_extra}" "--fec=true --dred=3 --plp=15" "--use-lbrr=true --use-dred=true"
done < "${AUDIO_MANIFEST}"

echo "[exp] done"
echo "[exp] audio manifest: ${AUDIO_MANIFEST}"
echo "[exp] summary csv: ${SUMMARY_CSV}"
echo "[exp] temp logs dir: ${TMP_DIR}"
