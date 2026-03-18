#!/usr/bin/env bash
set -euo pipefail

# ==========================================================================
# run_rtc_experiments.sh - RTC 传输模拟实验矩阵
#
# 实验矩阵:
#   音频类型  : 固定 news / dialogue（来自顶层 representative_audio/）
#   丢包场景  : 均匀 5%/10%/20%, GE 中等/重度突发, 延迟+抖动+10%
#   保护策略  : baseline(无保护), LBRR, DRED-3, DRED-5
#
# 输出:
#   - 汇总 CSV            → $SUMMARY_CSV
#   - Markdown 报告       → $REPORT_MD
#   - 输入/输出音频       → $RUN_DIR/inputs, $RUN_DIR/outputs
#   - 统计 JSON / 日志     → $RUN_DIR/stats, $RUN_DIR/logs
#   - 预编译二进制        → $BIN_DIR
#
# 环境变量:
#   EXPERIMENT_SUITE  quick|standard|full (默认 standard)
#   RECV_DURATION     接收时长 (默认 32s)
#   SIM_SEED          随机种子 (默认 42)
#   RUN_ID            实验目录名 (默认时间戳)
#   RUN_DIR           实验产物目录 (默认 results/rtc_runs/$RUN_ID)
#   REP_AUDIO_DIR     代表性音频根目录 (默认 <repo>/representative_audio)
#   BIN_DIR           sender/receiver/signaling 二进制缓存目录
#   REPORT_MD         报告输出路径
#   ASR_PYTHON        用于生成 WER/SER 报告的 Python (默认 .venv_asr/bin/python, 否则 python3)
#   STT_MODEL         Whisper 模型名 (默认 small.en)
#   RTC_STT_BACKEND   ASR 后端: mlx|faster|auto (默认 Apple Silicon 上为 mlx)
# ==========================================================================

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKSPACE_DIR="$(cd "${ROOT_DIR}/.." && pwd)"

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

OPUS_PKG_CONFIG="${OPUS_PKG_CONFIG:-${WORKSPACE_DIR}/opus-install/lib/pkgconfig}"
WEIGHTS_PATH="${WEIGHTS_PATH:-${WORKSPACE_DIR}/weights_blob.bin}"
SIM_SEED="${SIM_SEED:-42}"
REP_AUDIO_DIR="${REP_AUDIO_DIR:-${WORKSPACE_DIR}/representative_audio}"
REP_AUDIO_MANIFEST="${REP_AUDIO_DIR}/manifest.txt"
DEFAULT_RECV_DURATION="32s"
RECV_DURATION="${RECV_DURATION:-${DEFAULT_RECV_DURATION}}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d_%H%M%S)}"
RUN_DIR="${RUN_DIR:-${WORKSPACE_DIR}/results/rtc_runs/${RUN_ID}}"
INPUT_DIR="${RUN_DIR}/inputs"
OUTPUT_DIR="${RUN_DIR}/outputs"
STATS_DIR="${RUN_DIR}/stats"
LOG_DIR="${RUN_DIR}/logs"
TRANSCRIPT_DIR="${RUN_DIR}/transcripts"
AUDIO_MANIFEST="${AUDIO_MANIFEST:-${RUN_DIR}/audio_manifest.txt}"
SUMMARY_CSV="${SUMMARY_CSV:-${RUN_DIR}/rtc_experiment_summary.csv}"
REPORT_MD="${REPORT_MD:-${RUN_DIR}/rtc_report.md}"
LATEST_LINK="${WORKSPACE_DIR}/results/rtc_latest"
LATEST_REPORT="${WORKSPACE_DIR}/results/rtc_report.md"
BIN_DIR="${BIN_DIR:-${WORKSPACE_DIR}/results/rtc_bin_cache}"
ASR_PYTHON="${ASR_PYTHON:-${WORKSPACE_DIR}/.venv_asr/bin/python}"
STT_MODEL="${STT_MODEL:-small.en}"
SENDER_COMPLEXITY="${SENDER_COMPLEXITY:-9}"
SENDER_SIGNAL="${SENDER_SIGNAL:-auto}"
EXPERIMENT_SUITE="${EXPERIMENT_SUITE:-standard}"

if [[ ! -x "${ASR_PYTHON}" ]]; then
  ASR_PYTHON="${ASR_PYTHON_FALLBACK:-python3}"
fi

mkdir -p "${RUN_DIR}" "${INPUT_DIR}" "${OUTPUT_DIR}" "${STATS_DIR}" "${LOG_DIR}" "${TRANSCRIPT_DIR}" \
         "$(dirname "${LATEST_LINK}")" "$(dirname "${LATEST_REPORT}")" "${BIN_DIR}"

export PKG_CONFIG_PATH="${OPUS_PKG_CONFIG}:${PKG_CONFIG_PATH:-}"
export LD_LIBRARY_PATH="${WORKSPACE_DIR}/opus-install/lib:${LD_LIBRARY_PATH:-}"

cleanup() {
  if [[ -n "${SIGNAL_PID:-}" ]]; then
    kill "${SIGNAL_PID}" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

build_binaries() {
  echo "[exp] building rtc binaries into ${BIN_DIR}"
  (
    cd "${ROOT_DIR}"
    go build -o "${BIN_DIR}/signaling" ./signaling
    go build -o "${BIN_DIR}/receiver" ./receiver
    go build -o "${BIN_DIR}/sender" ./sender
  )
}

build_binaries

# ---- 检查代表性音频 ----
if [[ ! -f "${REP_AUDIO_MANIFEST}" ]]; then
  echo "[exp] missing representative audio manifest: ${REP_AUDIO_MANIFEST}" >&2
  echo "[exp] run: python3 tools/prepare_representative_audio.py --force" >&2
  exit 1
fi

: > "${AUDIO_MANIFEST}"
while IFS='|' read -r audio_type rel_wav; do
  [[ -z "${audio_type}" ]] && continue
  cached_wav="${REP_AUDIO_DIR}/${rel_wav}"
  if [[ ! -f "${cached_wav}" ]]; then
    echo "[exp] missing representative wav: ${cached_wav}" >&2
    exit 1
  fi
  input_wav="${INPUT_DIR}/${audio_type}.wav"
  cp "${cached_wav}" "${input_wav}"
  sidecar_ref="$(dirname "${cached_wav}")/${audio_type}_reference.txt"
  if [[ -f "${sidecar_ref}" ]]; then
    cp "${sidecar_ref}" "${INPUT_DIR}/${audio_type}_reference.txt"
  fi
  echo "${audio_type}|${input_wav}" >> "${AUDIO_MANIFEST}"
done < "${REP_AUDIO_MANIFEST}"

# ---- CSV header ----
echo "audio_type,scenario,case,sim_mode,sim_loss,recovered_lbrr,recovered_dred,plc,decode_errors,recovery_rate,input_wav,output_wav,stats_json" \
  > "${SUMMARY_CSV}"

# ---- 启动信令服务 ----
echo "[exp] starting signaling server on ${SIGNAL_URL}"
(
  cd "${ROOT_DIR}"
  "${BIN_DIR}/signaling" -addr ":${SIGNAL_PORT}" >"${LOG_DIR}/signaling.log" 2>&1
) &
SIGNAL_PID=$!
sleep 1

# ================================================================
# run_case <audio_type> <input_wav> <case_name> <scenario_name>
#          <sim_mode> <sim_loss_label> <sim_extra>
#          <sender_extra> <receiver_extra>
# ================================================================
run_case() {
  local audio_type="$1"
  local input_wav="$2"
  local case_name="$3"
  local scenario_name="$4"
  local sim_mode="$5"
  local sim_loss_label="$6"
  local sim_extra="$7"
  local sender_extra="$8"
  local receiver_extra="$9"
  local session="session-${audio_type}-${scenario_name}-${case_name}-$(date +%s%N)"
  local output_wav="${OUTPUT_DIR}/${audio_type}/${scenario_name}/${case_name}.wav"
  local stats_json="${STATS_DIR}/${audio_type}/${scenario_name}/${case_name}.json"

  mkdir -p "$(dirname "${output_wav}")" "$(dirname "${stats_json}")"

  echo "[exp] audio=${audio_type} scenario=${scenario_name} case=${case_name}"
  (
    cd "${ROOT_DIR}"
    "${BIN_DIR}/receiver" \
      --signal "${SIGNAL_URL}" \
      --session "${session}" \
      --output "${output_wav}" \
      --stats-json "${stats_json}" \
      --sim-seed "${SIM_SEED}" \
      --weights "${WEIGHTS_PATH}" \
      --duration "${RECV_DURATION}" \
      ${sim_extra} \
      ${receiver_extra} >"${LOG_DIR}/${audio_type}_${scenario_name}_${case_name}_receiver.log" 2>&1
  ) &
  local receiver_pid=$!
  sleep 1

  (
    cd "${ROOT_DIR}"
    "${BIN_DIR}/sender" \
      --signal "${SIGNAL_URL}" \
      --session "${session}" \
      --input "${input_wav}" \
      --complexity "${SENDER_COMPLEXITY}" \
      --signal-hint "${SENDER_SIGNAL}" \
      --weights "${WEIGHTS_PATH}" ${sender_extra} \
      >"${LOG_DIR}/${audio_type}_${scenario_name}_${case_name}_sender.log" 2>&1
  )

  wait "${receiver_pid}"

  if [[ ! -f "${stats_json}" ]]; then
    echo "[exp] WARNING: case=${case_name} missing stats, skipping" >&2
    return 0
  fi

  python3 - "${audio_type}" "${scenario_name}" "${case_name}" "${sim_mode}" "${sim_loss_label}" \
             "${stats_json}" "${input_wav}" "${output_wav}" "${SUMMARY_CSV}" <<'PY'
import json, sys

audio_type, scenario, case_name, sim_mode, sim_loss, stats_path, input_wav, output_wav, summary_csv = sys.argv[1:10]
with open(stats_path, "r", encoding="utf-8") as f:
    s = json.load(f)
row = ",".join([
    audio_type, scenario, case_name, sim_mode, sim_loss,
    str(s.get("recovered_lbrr", 0)),
    str(s.get("recovered_dred", 0)),
    str(s.get("plc_frames", 0)),
    str(s.get("decode_errors", 0)),
    f'{s.get("recovery_rate", 0.0):.4f}',
    input_wav, output_wav,
    stats_path,
])
with open(summary_csv, "a", encoding="utf-8") as out:
    out.write(row + "\n")
print(f"[exp]   -> {row}")
PY
}

# ================================================================
# 保护策略定义
#   name | sender_args | receiver_args
# ================================================================
declare -a STRATEGIES_QUICK=(
  "baseline|--fec=false --dred=0 --plp=0|--use-lbrr=false --use-dred=false"
  "lbrr_only|--fec=true --dred=0 --plp=15|--use-lbrr=true --use-dred=false"
  "dred_3|--fec=false --dred=3 --plp=15|--use-lbrr=false --use-dred=true"
)

declare -a STRATEGIES_FULL=(
  "baseline|--fec=false --dred=0 --plp=0|--use-lbrr=false --use-dred=false"
  "lbrr_only|--fec=true --dred=0 --plp=15|--use-lbrr=true --use-dred=false"
  "dred_3|--fec=false --dred=3 --plp=15|--use-lbrr=false --use-dred=true"
  "dred_5|--fec=false --dred=5 --plp=15|--use-lbrr=false --use-dred=true"
)

# ================================================================
# 丢包场景定义
#   name | sim_mode | loss_label | sim_extra (receiver flags)
# ================================================================
declare -a SCENARIOS_QUICK=(
  "uniform_10|uniform|10%|--sim-loss 0.10 --sim-delay-ms 0 --sim-jitter-ms 0"
  "ge_moderate|ge|p2b=0.05;b2g=0.30;bloss=0.80|--sim-ge=true --sim-ge-p2b 0.05 --sim-ge-b2g 0.30 --sim-ge-bloss 0.80 --sim-delay-ms 0 --sim-jitter-ms 0"
)

declare -a SCENARIOS_STANDARD=(
  "uniform_5|uniform|5%|--sim-loss 0.05 --sim-delay-ms 0 --sim-jitter-ms 0"
  "uniform_10|uniform|10%|--sim-loss 0.10 --sim-delay-ms 0 --sim-jitter-ms 0"
  "uniform_20|uniform|20%|--sim-loss 0.20 --sim-delay-ms 0 --sim-jitter-ms 0"
  "ge_moderate|ge|p2b=0.05;b2g=0.30;bloss=0.80|--sim-ge=true --sim-ge-p2b 0.05 --sim-ge-b2g 0.30 --sim-ge-bloss 0.80 --sim-delay-ms 0 --sim-jitter-ms 0"
  "ge_heavy|ge|p2b=0.10;b2g=0.15;bloss=0.90|--sim-ge=true --sim-ge-p2b 0.10 --sim-ge-b2g 0.15 --sim-ge-bloss 0.90 --sim-delay-ms 0 --sim-jitter-ms 0"
)

declare -a SCENARIOS_FULL=(
  "uniform_5|uniform|5%|--sim-loss 0.05 --sim-delay-ms 0 --sim-jitter-ms 0"
  "uniform_10|uniform|10%|--sim-loss 0.10 --sim-delay-ms 0 --sim-jitter-ms 0"
  "uniform_20|uniform|20%|--sim-loss 0.20 --sim-delay-ms 0 --sim-jitter-ms 0"
  "ge_moderate|ge|p2b=0.05;b2g=0.30;bloss=0.80|--sim-ge=true --sim-ge-p2b 0.05 --sim-ge-b2g 0.30 --sim-ge-bloss 0.80 --sim-delay-ms 0 --sim-jitter-ms 0"
  "ge_heavy|ge|p2b=0.10;b2g=0.15;bloss=0.90|--sim-ge=true --sim-ge-p2b 0.10 --sim-ge-b2g 0.15 --sim-ge-bloss 0.90 --sim-delay-ms 0 --sim-jitter-ms 0"
  "delay_jitter_10|uniform+delay|10%+50ms/20ms|--sim-loss 0.10 --sim-delay-ms 50 --sim-jitter-ms 20"
)

# ---- 根据 EXPERIMENT_SUITE 选择矩阵 ----
case "${EXPERIMENT_SUITE}" in
  quick)
    SCENARIOS=("${SCENARIOS_QUICK[@]}")
    STRATEGIES=("${STRATEGIES_QUICK[@]}")
    ;;
  standard)
    SCENARIOS=("${SCENARIOS_STANDARD[@]}")
    STRATEGIES=("${STRATEGIES_FULL[@]}")
    ;;
  full)
    SCENARIOS=("${SCENARIOS_FULL[@]}")
    STRATEGIES=("${STRATEGIES_FULL[@]}")
    ;;
  *)
    echo "[exp] unknown EXPERIMENT_SUITE=${EXPERIMENT_SUITE}, using standard" >&2
    SCENARIOS=("${SCENARIOS_STANDARD[@]}")
    STRATEGIES=("${STRATEGIES_FULL[@]}")
    ;;
esac

# ---- 打印实验矩阵 ----
n_audio=$(wc -l < "${AUDIO_MANIFEST}")
n_scenarios=${#SCENARIOS[@]}
n_strategies=${#STRATEGIES[@]}
total=$((n_audio * n_scenarios * n_strategies))

echo ""
echo "========================================================"
echo " RTC 传输实验矩阵"
echo "========================================================"
echo " 实验套件   : ${EXPERIMENT_SUITE}"
echo " 音频清单   : ${REP_AUDIO_MANIFEST}"
echo " 音频类型   : ${n_audio}"
echo " 丢包场景   : ${n_scenarios}"
echo " 保护策略   : ${n_strategies}"
echo " 总实验数   : ${total}"
echo " 接收时长   : ${RECV_DURATION}"
echo " 编码复杂度 : ${SENDER_COMPLEXITY}"
echo " Signal提示 : ${SENDER_SIGNAL}"
echo " 二进制缓存 : ${BIN_DIR}"
echo " 报告输出   : ${REPORT_MD}"
echo "========================================================"
echo ""

# ---- 执行实验矩阵 ----
count=0
while IFS='|' read -r audio_type input_wav; do
  [[ -z "${audio_type}" ]] && continue

  for scenario_def in "${SCENARIOS[@]}"; do
    IFS='|' read -r scenario_name sim_mode sim_loss_label sim_extra <<< "${scenario_def}"

    for strategy_def in "${STRATEGIES[@]}"; do
      IFS='|' read -r case_name sender_extra receiver_extra <<< "${strategy_def}"
      count=$((count + 1))
      echo ""
      echo "[exp] ---- (${count}/${total}) ----"
      run_case "${audio_type}" "${input_wav}" "${case_name}" \
               "${scenario_name}" "${sim_mode}" "${sim_loss_label}" \
               "${sim_extra}" "${sender_extra}" "${receiver_extra}"
    done
  done
done < "${AUDIO_MANIFEST}"

# Generate Markdown report.
echo ""
echo "[exp] generating Markdown report..."
mkdir -p "$(dirname "${REPORT_MD}")"
"${ASR_PYTHON}" "${WORKSPACE_DIR}/tools/gen_rtc_report.py" \
  --csv "${SUMMARY_CSV}" \
  --output "${REPORT_MD}" \
  --transcript-dir "${TRANSCRIPT_DIR}" \
  --stt-model "${STT_MODEL}" \
  --mode rtc

cp "${REPORT_MD}" "${LATEST_REPORT}"
ln -sfn "${RUN_DIR}" "${LATEST_LINK}"

echo ""
echo "========================================================"
echo " 实验完成！"
echo "========================================================"
echo " 总实验数    : ${count}"
echo " 汇总 CSV    : ${SUMMARY_CSV}"
echo " Markdown 报告: ${REPORT_MD}"
echo " 输入音频目录 : ${INPUT_DIR}"
echo " 输出音频目录 : ${OUTPUT_DIR}"
echo " 转写目录    : ${TRANSCRIPT_DIR}"
echo " 统计 JSON 目录: ${STATS_DIR}"
echo " 日志目录    : ${LOG_DIR}"
echo " 最新报告    : ${LATEST_REPORT}"
echo " 最新运行链接 : ${LATEST_LINK}"
echo "========================================================"
