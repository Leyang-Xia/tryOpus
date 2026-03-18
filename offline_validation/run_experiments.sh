#!/bin/bash
# run_experiments.sh - 批量运行 Opus 离线仿真实验
#
# 固定输入:
#   - representative_audio/news/news_30s_48k_mono.wav
#   - representative_audio/dialogue/dialogue_30s_48k_mono.wav
#
# 输出:
#   - results/offline_runs/<RUN_ID>/inputs
#   - results/offline_runs/<RUN_ID>/outputs
#   - results/offline_runs/<RUN_ID>/stats
#   - results/offline_runs/<RUN_ID>/logs
#   - results/offline_runs/<RUN_ID>/opus_experiment_summary.csv
#   - results/offline_runs/<RUN_ID>/opus_report.md

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

BUILD_DIR="$ROOT_DIR/build"
SIM="$BUILD_DIR/opus_sim"
BITRATE="${BITRATE:-32000}"
FRAMESIZE="${FRAMESIZE:-20}"
COMPLEXITY="${COMPLEXITY:-9}"
SIGNAL_HINT="${SIGNAL_HINT:-auto}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d_%H%M%S)}"
RUN_DIR="${RUN_DIR:-$ROOT_DIR/results/offline_runs/$RUN_ID}"
INPUT_DIR="$RUN_DIR/inputs"
OUTPUT_DIR="$RUN_DIR/outputs"
STATS_DIR="$RUN_DIR/stats"
LOG_DIR="$RUN_DIR/logs"
TRANSCRIPT_DIR="$RUN_DIR/transcripts"
SUMMARY_CSV="${SUMMARY_CSV:-$RUN_DIR/opus_experiment_summary.csv}"
REPORT_MD="${REPORT_MD:-$RUN_DIR/opus_report.md}"
LATEST_LINK="$ROOT_DIR/results/offline_latest"
LATEST_REPORT="$ROOT_DIR/results/offline_report.md"
REP_AUDIO_DIR="${REP_AUDIO_DIR:-$ROOT_DIR/representative_audio}"
REP_AUDIO_MANIFEST="$REP_AUDIO_DIR/manifest.txt"
AUDIO_MANIFEST="${AUDIO_MANIFEST:-$RUN_DIR/audio_manifest.txt}"
ASR_PYTHON="${ASR_PYTHON:-$ROOT_DIR/.venv_asr/bin/python}"
STT_MODEL="${STT_MODEL:-small.en}"

if [[ ! -x "${ASR_PYTHON}" ]]; then
    ASR_PYTHON="${ASR_PYTHON_FALLBACK:-python3}"
fi

mkdir -p "$INPUT_DIR" "$OUTPUT_DIR" "$STATS_DIR" "$LOG_DIR" "$TRANSCRIPT_DIR" "$(dirname "$LATEST_LINK")"

if [ ! -f "$SIM" ]; then
    echo "[错误] 找不到: $SIM"
    echo "请先编译: cd build && cmake .. && make -j\$(nproc)"
    exit 1
fi

if [ ! -f "$REP_AUDIO_MANIFEST" ]; then
    echo "[错误] 找不到代表性音频清单: $REP_AUDIO_MANIFEST"
    echo "请先执行: python3 tools/prepare_representative_audio.py --force"
    exit 1
fi

: > "$AUDIO_MANIFEST"
while IFS='|' read -r audio_type rel_input_wav; do
    [ -z "$audio_type" ] && continue
    input_wav="$REP_AUDIO_DIR/$rel_input_wav"
    if [ ! -f "$input_wav" ]; then
        echo "[错误] 缺少输入音频: $input_wav" >&2
        exit 1
    fi

    copied_input="$INPUT_DIR/${audio_type}.wav"
    cp "$input_wav" "$copied_input"
    sidecar_ref="$(dirname "$input_wav")/${audio_type}_reference.txt"
    if [ -f "$sidecar_ref" ]; then
        cp "$sidecar_ref" "$INPUT_DIR/${audio_type}_reference.txt"
    fi
    echo "${audio_type}|${copied_input}" >> "$AUDIO_MANIFEST"
done < "$REP_AUDIO_MANIFEST"

echo "audio_type,experiment,name,loss_model,loss_param,recovered_lbrr,recovered_dred,plc,recovery_rate,input_wav,output_wav,stats_csv" \
    > "$SUMMARY_CSV"

run_sim() {
    local audio_type="$1"
    local input_wav="$2"
    local experiment="$3"
    local name="$4"
    local loss_model="$5"
    local loss_param="$6"
    shift 6

    local full_name="${audio_type}_${name}"
    local out_wav="$OUTPUT_DIR/${audio_type}/${full_name}.wav"
    local out_csv="$STATS_DIR/${audio_type}/${full_name}.csv"
    local out_log="$LOG_DIR/${audio_type}/${full_name}.log"

    mkdir -p "$(dirname "$out_wav")" "$(dirname "$out_csv")" "$(dirname "$out_log")"

    "$SIM" \
        --bitrate "$BITRATE" \
        --framesize "$FRAMESIZE" \
        --complexity "$COMPLEXITY" \
        --signal "$SIGNAL_HINT" \
        --csv "$out_csv" \
        "$@" \
        "$input_wav" "$out_wav" \
        >"$out_log" 2>/dev/null
    local rc=$?

    if [ $rc -ne 0 ]; then
        printf "  %-40s  [错误]\n" "$full_name"
        return 1
    fi

    local cfg_loss actual_loss lbrr_cnt dred_cnt plc_cnt avg_br recov_rate
    cfg_loss=$(grep "^丢包率" "$out_log" | grep -oE '[0-9]+\.[0-9]+' | head -1 || true)
    actual_loss=$(grep "^丢包数" "$out_log" | grep -oE '\([0-9.]+%\)' | head -1 || true)
    lbrr_cnt=$(grep "^  - LBRR" "$out_log" | grep -oE '[0-9]+' | head -1 || true)
    dred_cnt=$(grep "^  - DRED" "$out_log" | grep -oE '[0-9]+' | head -1 || true)
    plc_cnt=$(grep "^  - PLC" "$out_log" | grep -oE '[0-9]+' | head -1 || true)
    avg_br=$(grep "^平均码率" "$out_log" | grep -oE '[0-9]+\.[0-9]+' | head -1 || true)
    recov_rate=$(grep "^丢包恢复率" "$out_log" | grep -oE '[0-9]+\.[0-9]+' | head -1 || true)

    [ -z "$recov_rate" ] && recov_rate="0.0"
    [ -z "$lbrr_cnt" ] && lbrr_cnt=0
    [ -z "$dred_cnt" ] && dred_cnt=0
    [ -z "$plc_cnt" ] && plc_cnt=0
    [ -z "$avg_br" ] && avg_br="?"

    printf "  %-40s  配置=%s%% 实际=%s  恢复率=%s%%  LBRR=%s DRED=%s PLC=%s  码率=%skbps\n" \
        "$full_name" "${cfg_loss:-?}" "${actual_loss:-(0.0%)}" \
        "$recov_rate" "$lbrr_cnt" "$dred_cnt" "$plc_cnt" "$avg_br"

    echo "${audio_type},${experiment},${name},${loss_model},${loss_param},${lbrr_cnt},${dred_cnt},${plc_cnt},${recov_rate},${input_wav},${out_wav},${out_csv}" \
        >> "$SUMMARY_CSV"
}

echo "========================================================"
echo " Opus 离线仿真实验套件"
echo "========================================================"
echo "码率     : ${BITRATE} bps"
echo "帧长     : ${FRAMESIZE} ms"
echo "复杂度   : ${COMPLEXITY}"
echo "Signal   : ${SIGNAL_HINT}"
echo "输入清单 : $REP_AUDIO_MANIFEST"
echo "运行目录 : $RUN_DIR"
echo ""
printf "  %-40s  %-12s %-10s %-8s %-20s %-10s\n" \
    "实验名称" "配置丢包率" "实际丢包" "恢复率" "LBRR/DRED/PLC帧数" "码率"
printf "  %s\n" "$(printf '%.0s-' {1..105})"

while IFS='|' read -r audio_type input_wav; do
    [ -z "$audio_type" ] && continue

    echo ""
    echo "============ 音频类型: $audio_type ($input_wav) ============"
    ref_wav="$INPUT_DIR/${audio_type}_reference.wav"
    cp "$input_wav" "$ref_wav"

    echo ""
    echo "--- 实验1: 均匀丢包 (0% / 5% / 10% / 20%) ---"
    for LOSS in 0.0 0.05 0.10 0.20; do
        case "$LOSS" in
            0.0) LOSS_PCT=0 ;;
            0.05) LOSS_PCT=5 ;;
            0.10) LOSS_PCT=10 ;;
            0.20) LOSS_PCT=20 ;;
        esac

        run_sim "$audio_type" "$input_wav" "均匀丢包" \
            "plc_loss${LOSS_PCT}pct" "uniform" "${LOSS_PCT}%" \
            --loss "$LOSS" --no-lbrr --no-dred

        run_sim "$audio_type" "$input_wav" "均匀丢包" \
            "lbrr_loss${LOSS_PCT}pct" "uniform" "${LOSS_PCT}%" \
            --loss "$LOSS" --lbrr --ploss "$LOSS_PCT" --no-dred

        run_sim "$audio_type" "$input_wav" "均匀丢包" \
            "dred3_loss${LOSS_PCT}pct" "uniform" "${LOSS_PCT}%" \
            --loss "$LOSS" -dred 3 --no-lbrr

        if [ "$LOSS_PCT" -gt 0 ]; then
            run_sim "$audio_type" "$input_wav" "均匀丢包" \
                "dred5_loss${LOSS_PCT}pct" "uniform" "${LOSS_PCT}%" \
                --loss "$LOSS" -dred 5 --no-lbrr
        fi
        echo ""
    done

    echo "--- 实验2: Gilbert-Elliott 突发丢包 ---"
    echo "    参数: p(G→B)=0.05, p(B→G)=0.3, BAD丢包率=0.8  (期望均值≈11%)"

    run_sim "$audio_type" "$input_wav" "GE突发丢包" \
        "ge_plc" "ge" "p2b=0.05;b2g=0.30;bloss=0.80" \
        -ge -ge-p2b 0.05 -ge-b2g 0.30 -ge-bloss 0.80 --no-lbrr --no-dred

    run_sim "$audio_type" "$input_wav" "GE突发丢包" \
        "ge_lbrr" "ge" "p2b=0.05;b2g=0.30;bloss=0.80" \
        -ge -ge-p2b 0.05 -ge-b2g 0.30 -ge-bloss 0.80 --lbrr --ploss 15 --no-dred

    run_sim "$audio_type" "$input_wav" "GE突发丢包" \
        "ge_dred3" "ge" "p2b=0.05;b2g=0.30;bloss=0.80" \
        -ge -ge-p2b 0.05 -ge-b2g 0.30 -ge-bloss 0.80 -dred 3 --no-lbrr

    run_sim "$audio_type" "$input_wav" "GE突发丢包" \
        "ge_dred5" "ge" "p2b=0.05;b2g=0.30;bloss=0.80" \
        -ge -ge-p2b 0.05 -ge-b2g 0.30 -ge-bloss 0.80 -dred 5 --no-lbrr

    run_sim "$audio_type" "$input_wav" "GE突发丢包" \
        "ge_dred10" "ge" "p2b=0.05;b2g=0.30;bloss=0.80" \
        -ge -ge-p2b 0.05 -ge-b2g 0.30 -ge-bloss 0.80 -dred 10 --no-lbrr

    echo ""
    echo "--- 实验3: 不同码率 + DRED（固定10%均匀丢包）---"
    for BR in 16000 32000 64000; do
        BITRATE_SAVED="$BITRATE"
        BITRATE="$BR"
        run_sim "$audio_type" "$input_wav" "不同码率" \
            "br${BR}_plc_10pct" "uniform" "10%" \
            --loss 0.10 --no-lbrr --no-dred

        run_sim "$audio_type" "$input_wav" "不同码率" \
            "br${BR}_dred3_10pct" "uniform" "10%" \
            --loss 0.10 -dred 3 --no-lbrr
        BITRATE="$BITRATE_SAVED"
    done

    echo ""
    echo "--- 实验4: 时延 + 抖动 + 10%丢包 ---"
    run_sim "$audio_type" "$input_wav" "延迟抖动" \
        "delay50_jitter20_plc" "uniform+delay" "10%+50ms/20ms" \
        --loss 0.10 -d 50 -j 20 --no-lbrr --no-dred

    run_sim "$audio_type" "$input_wav" "延迟抖动" \
        "delay50_jitter20_dred3" "uniform+delay" "10%+50ms/20ms" \
        --loss 0.10 -d 50 -j 20 -dred 3 --no-lbrr

    echo ""
    echo "--- 实验5: DTX 不连续发送 ---"
    run_sim "$audio_type" "$input_wav" "DTX" \
        "dtx_no_loss" "none" "0%" \
        --dtx --loss 0.0

    run_sim "$audio_type" "$input_wav" "DTX" \
        "dtx_10pct" "uniform" "10%" \
        --dtx --loss 0.10 -dred 3

    echo ""
done < "$AUDIO_MANIFEST"

echo ""
echo "[info] 生成 ASR Markdown 报告..."
"${ASR_PYTHON}" "$ROOT_DIR/tools/gen_rtc_report.py" \
    --csv "$SUMMARY_CSV" \
    --output "$REPORT_MD" \
    --transcript-dir "$TRANSCRIPT_DIR" \
    --stt-model "$STT_MODEL" \
    --mode opus

cp "$REPORT_MD" "$LATEST_REPORT"
ln -sfn "$RUN_DIR" "$LATEST_LINK"

echo ""
echo "========================================================"
echo "实验完成！"
echo ""
echo "Markdown 报告: $REPORT_MD"
echo "汇总 CSV: $SUMMARY_CSV"
echo "输入音频目录: $INPUT_DIR"
echo "输出音频目录: $OUTPUT_DIR"
echo "统计目录: $STATS_DIR"
echo "日志目录: $LOG_DIR"
echo "转写目录: $TRANSCRIPT_DIR"
echo "最新报告: $LATEST_REPORT"
echo "最新运行链接: $LATEST_LINK"
echo "========================================================"
