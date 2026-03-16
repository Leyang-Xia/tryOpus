#!/usr/bin/env bash
set -euo pipefail

# ==========================================================================
# run_rtc_experiments.sh - RTC 传输模拟实验矩阵
#
# 实验矩阵:
#   音频类型  : music / news / dialogue（自动下载代表性音频）
#   丢包场景  : 均匀 5%/10%/20%, GE 中等/重度突发, 延迟+抖动+10%
#   保护策略  : baseline(无保护), LBRR, DRED-3, DRED-5, LBRR+DRED-3
#
# 输出:
#   - 汇总 CSV            → $SUMMARY_CSV (默认 /tmp 下)
#   - Markdown 报告       → $REPORT_MD  (默认 results/rtc_report.md)
#
# 环境变量:
#   EXPERIMENT_SUITE  quick|standard|full (默认 standard)
#   RECV_DURATION     接收时长 (默认 6s)
#   CLIP_SECONDS      音频剪辑时长 (默认 4)
#   SIM_SEED          随机种子 (默认 42)
#   REPORT_MD         报告输出路径
# ==========================================================================

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKSPACE_DIR="$(cd "${ROOT_DIR}/.." && pwd)"
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

OPUS_PKG_CONFIG="${OPUS_PKG_CONFIG:-${WORKSPACE_DIR}/opus-install/lib/pkgconfig}"
WEIGHTS_PATH="${WEIGHTS_PATH:-${WORKSPACE_DIR}/weights_blob.bin}"
SIM_SEED="${SIM_SEED:-42}"
RECV_DURATION="${RECV_DURATION:-6s}"
CLIP_SECONDS="${CLIP_SECONDS:-4}"
AUDIO_MANIFEST="${AUDIO_MANIFEST:-${TMP_DIR}/audio_manifest.txt}"
SUMMARY_CSV="${SUMMARY_CSV:-${TMP_DIR}/rtc_experiment_summary.csv}"
REPORT_MD="${REPORT_MD:-${WORKSPACE_DIR}/results/rtc_report.md}"
EXPERIMENT_SUITE="${EXPERIMENT_SUITE:-standard}"

export PKG_CONFIG_PATH="${OPUS_PKG_CONFIG}:${PKG_CONFIG_PATH:-}"
export LD_LIBRARY_PATH="${WORKSPACE_DIR}/opus-install/lib:${LD_LIBRARY_PATH:-}"

cleanup() {
  if [[ -n "${SIGNAL_PID:-}" ]]; then
    kill "${SIGNAL_PID}" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

# ---- 准备代表性音频 ----
echo "[exp] preparing representative audio (clip=${CLIP_SECONDS}s)"
python3 "${WORKSPACE_DIR}/tools/prepare_representative_audio.py" \
  --out-dir "${TMP_DIR}/audio" \
  --manifest "${AUDIO_MANIFEST}" \
  --clip-seconds "${CLIP_SECONDS}"

# ---- CSV header ----
echo "audio_type,scenario,case,sim_mode,sim_loss,recovered_lbrr,recovered_dred,plc,decode_errors,recovery_rate,input_wav,output_wav" \
  > "${SUMMARY_CSV}"

# ---- 启动信令服务 ----
echo "[exp] starting signaling server on ${SIGNAL_URL}"
(
  cd "${ROOT_DIR}"
  go run ./signaling -addr ":${SIGNAL_PORT}" >"${TMP_DIR}/signaling.log" 2>&1
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
  local output_wav="${TMP_DIR}/${audio_type}_${scenario_name}_${case_name}.wav"
  local stats_json="${TMP_DIR}/${audio_type}_${scenario_name}_${case_name}.json"

  echo "[exp] audio=${audio_type} scenario=${scenario_name} case=${case_name}"
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
      ${receiver_extra} >"${TMP_DIR}/${audio_type}_${scenario_name}_${case_name}_receiver.log" 2>&1
  ) &
  local receiver_pid=$!
  sleep 1

  (
    cd "${ROOT_DIR}"
    go run ./sender \
      --signal "${SIGNAL_URL}" \
      --session "${session}" \
      --input "${input_wav}" \
      --weights "${WEIGHTS_PATH}" ${sender_extra} \
      >"${TMP_DIR}/${audio_type}_${scenario_name}_${case_name}_sender.log" 2>&1
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
  "lbrr_dred_3|--fec=true --dred=3 --plp=15 --bitrate=64000 --vbr=true|--use-lbrr=true --use-dred=true"
)

declare -a STRATEGIES_FULL=(
  "baseline|--fec=false --dred=0 --plp=0|--use-lbrr=false --use-dred=false"
  "lbrr_only|--fec=true --dred=0 --plp=15|--use-lbrr=true --use-dred=false"
  "dred_3|--fec=false --dred=3 --plp=15|--use-lbrr=false --use-dred=true"
  "dred_5|--fec=false --dred=5 --plp=15|--use-lbrr=false --use-dred=true"
  "lbrr_dred_3|--fec=true --dred=3 --plp=15 --bitrate=64000 --vbr=true|--use-lbrr=true --use-dred=true"
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
echo " 音频类型   : ${n_audio}"
echo " 丢包场景   : ${n_scenarios}"
echo " 保护策略   : ${n_strategies}"
echo " 总实验数   : ${total}"
echo " 接收时长   : ${RECV_DURATION}"
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

# ---- 生成 Markdown 报告 ----
echo ""
echo "[exp] generating Markdown report..."
mkdir -p "$(dirname "${REPORT_MD}")"
python3 "${WORKSPACE_DIR}/tools/gen_rtc_report.py" \
  --csv "${SUMMARY_CSV}" \
  --output "${REPORT_MD}" \
  --mode rtc

echo ""
echo "========================================================"
echo " 实验完成！"
echo "========================================================"
echo " 总实验数    : ${count}"
echo " 汇总 CSV    : ${SUMMARY_CSV}"
echo " Markdown 报告: ${REPORT_MD}"
echo " 临时目录    : ${TMP_DIR}"
echo "========================================================"
