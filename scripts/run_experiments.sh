#!/bin/bash
# run_experiments.sh - 批量运行 Opus 仿真实验
#
# 实验矩阵：
#   - 3种音频类型: music / news / dialogue（自动下载代表性音频）
#   - 4种丢包率: 0%, 5%, 10%, 20%
#   - 多种保护方案: PLC(基准), LBRR, DRED-3帧, DRED-5帧, LBRR+DRED
#   - 2种丢包模型: 均匀, Gilbert突发
#
# 输出: results/ 目录下的WAV和CSV文件，以及 Markdown 汇总报告
#
# 用法:
#   bash scripts/run_experiments.sh
#   AUDIO_MODE=synthetic bash scripts/run_experiments.sh       # 仅用合成音频
#   bash scripts/run_experiments.sh audio/my_audio.wav         # 指定单个文件
#   BITRATE=64000 bash scripts/run_experiments.sh              # 指定码率
#
# 音频模式 (AUDIO_MODE):
#   representative  (默认) 自动下载 music / news / dialogue 三类代表性音频
#   synthetic       使用 gen_audio.py 生成的合成音频 (speech_like)
#   custom          使用命令行参数指定的单个文件

set -e
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

BUILD_DIR="$ROOT_DIR/build"
SIM="$BUILD_DIR/opus_sim"
ANALYZE="python3 $ROOT_DIR/tools/analyze.py"

# ---- 默认参数 ----
BITRATE="${BITRATE:-32000}"
FRAMESIZE="${FRAMESIZE:-20}"
OUTDIR="$ROOT_DIR/results"
AUDIO_MODE="${AUDIO_MODE:-representative}"
CLIP_SECONDS="${CLIP_SECONDS:-10}"
SUMMARY_CSV="$OUTDIR/opus_experiment_summary.csv"
REPORT_MD="$OUTDIR/opus_report.md"

mkdir -p "$OUTDIR"

if [ ! -f "$SIM" ]; then
    echo "[错误] 找不到: $SIM"
    echo "请先编译: cd build && cmake .. && make -j\$(nproc)"
    exit 1
fi

# ====================================================================
# 音频准备
# ====================================================================
AUDIO_MANIFEST="$OUTDIR/audio_manifest.txt"

if [ -n "${1:-}" ] && [ -f "${1:-}" ]; then
    AUDIO_MODE="custom"
    echo "custom|$1" > "$AUDIO_MANIFEST"
    echo "[info] 使用自定义音频: $1"
elif [ "$AUDIO_MODE" = "representative" ]; then
    echo "[info] 下载代表性音频 (music/news/dialogue, ${CLIP_SECONDS}s)..."
    python3 "$ROOT_DIR/tools/prepare_representative_audio.py" \
        --out-dir "$OUTDIR/audio_cache" \
        --manifest "$AUDIO_MANIFEST" \
        --clip-seconds "$CLIP_SECONDS"
elif [ "$AUDIO_MODE" = "synthetic" ]; then
    INPUT="$ROOT_DIR/audio/speech_like.wav"
    if [ ! -f "$INPUT" ]; then
        echo "[info] 生成合成音频..."
        python3 "$ROOT_DIR/tools/gen_audio.py"
    fi
    echo "synthetic|$INPUT" > "$AUDIO_MANIFEST"
else
    echo "[错误] 未知 AUDIO_MODE=$AUDIO_MODE" >&2
    exit 1
fi

# ---- 汇总 CSV header ----
echo "audio_type,experiment,name,loss_model,loss_param,recovered_lbrr,recovered_dred,plc,recovery_rate,input_wav,output_wav" \
    > "$SUMMARY_CSV"

# ====================================================================
# run_sim <audio_type> <input_wav> <experiment> <name> <loss_model>
#         <loss_param> [opus_sim选项...]
# ====================================================================
run_sim() {
    local audio_type="$1"
    local input_wav="$2"
    local experiment="$3"
    local name="$4"
    local loss_model="$5"
    local loss_param="$6"
    shift 6

    local full_name="${audio_type}_${name}"
    local out_wav="$OUTDIR/${full_name}.wav"
    local out_csv="$OUTDIR/${full_name}.csv"
    local out_log="$OUTDIR/${full_name}.log"

    "$SIM" \
        --bitrate  "$BITRATE"   \
        --framesize "$FRAMESIZE" \
        --csv  "$out_csv"        \
        "$@"                     \
        "$input_wav" "$out_wav"  \
        >"$out_log" 2>/dev/null
    local rc=$?

    if [ $rc -ne 0 ]; then
        printf "  %-40s  [错误]\n" "$full_name"
        return 1
    fi

    # 从 log 中提取关键数字
    local cfg_loss actual_loss lbrr_cnt dred_cnt plc_cnt avg_br recov_rate
    cfg_loss=$(  grep "^丢包率"    "$out_log" | grep -oE '[0-9]+\.[0-9]+' | head -1)
    actual_loss=$(grep "^丢包数"   "$out_log" | grep -oE '\([0-9.]+%\)'   | head -1)
    lbrr_cnt=$(  grep "^  - LBRR" "$out_log" | grep -oE '[0-9]+'          | head -1)
    dred_cnt=$(  grep "^  - DRED" "$out_log" | grep -oE '[0-9]+'          | head -1)
    plc_cnt=$(   grep "^  - PLC"  "$out_log" | grep -oE '[0-9]+'          | head -1)
    avg_br=$(    grep "^平均码率"  "$out_log" | grep -oE '[0-9]+\.[0-9]+'  | head -1)
    recov_rate=$(grep "^丢包恢复率" "$out_log" | grep -oE '[0-9]+\.[0-9]+' | head -1)

    [ -z "$recov_rate" ] && recov_rate="0.0"
    [ -z "$lbrr_cnt"  ] && lbrr_cnt=0
    [ -z "$dred_cnt"  ] && dred_cnt=0
    [ -z "$plc_cnt"   ] && plc_cnt=0
    [ -z "$avg_br"    ] && avg_br="?"

    printf "  %-40s  配置=%s%% 实际=%s  恢复率=%s%%  LBRR=%s DRED=%s PLC=%s  码率=%skbps\n" \
        "$full_name" "${cfg_loss:-?}" "${actual_loss:-(0.0%)}" \
        "$recov_rate" "$lbrr_cnt" "$dred_cnt" "$plc_cnt" "$avg_br"

    echo "${audio_type},${experiment},${name},${loss_model},${loss_param},${lbrr_cnt},${dred_cnt},${plc_cnt},${recov_rate},${input_wav},${out_wav}" \
        >> "$SUMMARY_CSV"
}

# ====================================================================
# 主循环：遍历每种音频类型
# ====================================================================
echo "========================================================"
echo " Opus 仿真实验套件"
echo "========================================================"
echo "码率     : ${BITRATE} bps"
echo "帧长     : ${FRAMESIZE} ms"
echo "输出目录 : $OUTDIR"
echo ""
printf "  %-40s  %-12s %-10s %-8s %-20s %-10s\n" \
    "实验名称" "配置丢包率" "实际丢包" "恢复率" "LBRR/DRED/PLC帧数" "码率"
printf "  %s\n" "$(printf '%.0s-' {1..105})"

while IFS='|' read -r audio_type input_wav; do
    [ -z "$audio_type" ] && continue

    echo ""
    echo "============ 音频类型: $audio_type ($input_wav) ============"

    # 复制参考文件
    cp "$input_wav" "$OUTDIR/${audio_type}_reference.wav"

    # ========== 实验1: 均匀丢包 × 保护方案 ==========
    echo ""
    echo "--- 实验1: 均匀丢包 (0% / 5% / 10% / 20%) ---"

    for LOSS in 0.0 0.05 0.10 0.20; do
        case "$LOSS" in
            0.0)  LOSS_PCT=0  ;;
            0.05) LOSS_PCT=5  ;;
            0.10) LOSS_PCT=10 ;;
            0.20) LOSS_PCT=20 ;;
            *)    LOSS_PCT=$(printf "%.0f" "$(echo "$LOSS * 100" | awk '{print $1*100}')") ;;
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

    # ========== 实验2: Gilbert-Elliott 突发丢包 ==========
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

    # ========== 实验3: 不同码率 ==========
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

    # ========== 实验4: 时延 + 抖动 ==========
    echo "--- 实验4: 时延 + 抖动 + 10%丢包 ---"
    run_sim "$audio_type" "$input_wav" "延迟抖动" \
        "delay50_jitter20_plc" "uniform+delay" "10%+50ms/20ms" \
        --loss 0.10 -d 50 -j 20 --no-lbrr --no-dred

    run_sim "$audio_type" "$input_wav" "延迟抖动" \
        "delay50_jitter20_dred3" "uniform+delay" "10%+50ms/20ms" \
        --loss 0.10 -d 50 -j 20 -dred 3 --no-lbrr

    echo ""

    # ========== 实验5: DTX ==========
    echo "--- 实验5: DTX 不连续发送 ---"
    run_sim "$audio_type" "$input_wav" "DTX" \
        "dtx_no_loss" "none" "0%" \
        --dtx --loss 0.0

    run_sim "$audio_type" "$input_wav" "DTX" \
        "dtx_10pct" "uniform" "10%" \
        --dtx --loss 0.10 -dred 3

    echo ""

done < "$AUDIO_MANIFEST"

# ====================================================================
# 综合分析报告 (SNR)
# ====================================================================
echo "========================================================"
echo " 音质对比报告 (SNR vs 原始音频)"
echo "========================================================"
printf "  %-40s  %8s  %10s\n" "实验名称" "全局SNR" "分段SNR"
printf "  %s\n" "$(printf '%.0s-' {1..65})"

while IFS='|' read -r audio_type input_wav; do
    [ -z "$audio_type" ] && continue
    ref_wav="$OUTDIR/${audio_type}_reference.wav"

    for wav in "$OUTDIR"/${audio_type}_*.wav; do
        name=$(basename "$wav" .wav)
        [ "$name" = "${audio_type}_reference" ] && continue
        py_out=$(python3 "$ROOT_DIR/tools/analyze.py" \
            --ref "$ref_wav" --deg "$wav" 2>/dev/null) || continue
        snr=$(   echo "$py_out" | grep "全局SNR" | grep -oE '[+-]?[0-9]+\.[0-9]+' | head -1)
        segsnr=$(echo "$py_out" | grep "分段SNR" | grep -oE '[+-]?[0-9]+\.[0-9]+' | head -1)
        [ -z "$snr"    ] && snr="-"
        [ -z "$segsnr" ] && segsnr="-"
        printf "  %-40s  %8s dB  %8s dB\n" "$name" "$snr" "$segsnr"
    done
done < "$AUDIO_MANIFEST"

# ====================================================================
# 生成 Markdown 报告
# ====================================================================
echo ""
echo "[info] 生成 Markdown 报告..."
python3 "$ROOT_DIR/tools/gen_rtc_report.py" \
    --csv "$SUMMARY_CSV" \
    --output "$REPORT_MD" \
    --mode opus

echo ""
echo "========================================================"
echo "实验完成！"
echo ""
echo "Markdown 报告: $REPORT_MD"
echo "汇总 CSV: $SUMMARY_CSV"
echo ""
echo "查看单个实验的详细日志:"
echo "  cat $OUTDIR/<实验名>.log"
echo ""
echo "查看 CSV 统计:"
echo "  python3 tools/analyze.py --csv $OUTDIR/music_dred3_loss10pct.csv"
echo "========================================================"
