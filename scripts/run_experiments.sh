#!/bin/bash
# run_experiments.sh - 批量运行 Opus 仿真实验
#
# 实验矩阵：
#   - 4种丢包率: 0%, 5%, 10%, 20%
#   - 多种保护方案: PLC(基准), LBRR, DRED-3帧, DRED-5帧, LBRR+DRED
#   - 2种丢包模型: 均匀, Gilbert突发
#
# 输出: results/ 目录下的WAV和CSV文件，以及汇总报告
#
# 用法:
#   bash scripts/run_experiments.sh
#   bash scripts/run_experiments.sh audio/my_audio.wav   # 指定输入文件
#   BITRATE=64000 bash scripts/run_experiments.sh        # 指定码率

set -e
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

BUILD_DIR="$ROOT_DIR/build"
SIM="$BUILD_DIR/opus_sim"
ANALYZE="python3 $ROOT_DIR/tools/analyze.py"

# ---- 默认参数 ----
INPUT="${1:-$ROOT_DIR/audio/speech_like.wav}"
BITRATE="${BITRATE:-32000}"
FRAMESIZE="${FRAMESIZE:-20}"
OUTDIR="$ROOT_DIR/results"

mkdir -p "$OUTDIR"

# ---- 检查输入文件 ----
if [ ! -f "$INPUT" ]; then
    echo "[错误] 输入文件不存在: $INPUT"
    echo "请先运行: python3 tools/gen_audio.py"
    exit 1
fi

if [ ! -f "$SIM" ]; then
    echo "[错误] 找不到: $SIM"
    echo "请先编译: cd build && make -j\$(nproc)"
    exit 1
fi

# ---- 复制参考文件 ----
cp "$INPUT" "$OUTDIR/reference.wav"

# ====================================================================
# run_sim <name> [opus_sim选项...]
#   - 抑制 opus_sim 的冗长输出（保存到 .log 文件）
#   - 从输出中提取关键数字，打印一行摘要
# ====================================================================
run_sim() {
    local name="$1"
    shift                                  # 剩余参数全部传给 opus_sim
    local out_wav="$OUTDIR/${name}.wav"
    local out_csv="$OUTDIR/${name}.csv"
    local out_log="$OUTDIR/${name}.log"

    # 运行仿真（stdout 保存到 log，stderr 丢弃）
    "$SIM" \
        --bitrate  "$BITRATE"   \
        --framesize "$FRAMESIZE" \
        --csv  "$out_csv"        \
        "$@"                     \
        "$INPUT" "$out_wav"      \
        >"$out_log" 2>/dev/null
    local rc=$?

    if [ $rc -ne 0 ]; then
        printf "  %-35s  [错误]\n" "$name"
        return 1
    fi

    # 从 log 中提取关键数字
    local cfg_loss  actual_loss  lbrr_cnt  dred_cnt  plc_cnt  avg_br
    cfg_loss=$(  grep "^丢包率"    "$out_log" | grep -oE '[0-9]+\.[0-9]+' | head -1)
    actual_loss=$(grep "^丢包数"   "$out_log" | grep -oE '\([0-9.]+%\)'   | head -1)
    lbrr_cnt=$(  grep "^  - LBRR" "$out_log" | grep -oE '[0-9]+'          | head -1)
    dred_cnt=$(  grep "^  - DRED" "$out_log" | grep -oE '[0-9]+'          | head -1)
    plc_cnt=$(   grep "^  - PLC"  "$out_log" | grep -oE '[0-9]+'          | head -1)
    avg_br=$(    grep "^平均码率"  "$out_log" | grep -oE '[0-9]+\.[0-9]+'  | head -1)
    local recov_rate
    recov_rate=$(grep "^丢包恢复率" "$out_log" | grep -oE '[0-9]+\.[0-9]+' | head -1)

    # 若没有丢包恢复行，恢复率设为 0.0
    [ -z "$recov_rate" ] && recov_rate="0.0"
    [ -z "$lbrr_cnt"  ] && lbrr_cnt=0
    [ -z "$dred_cnt"  ] && dred_cnt=0
    [ -z "$plc_cnt"   ] && plc_cnt=0
    [ -z "$avg_br"    ] && avg_br="?"

    printf "  %-35s  配置=%s%% 实际=%s  恢复率=%s%%  LBRR=%s DRED=%s PLC=%s  码率=%skbps\n" \
        "$name" "${cfg_loss:-?}" "${actual_loss:-(0.0%)}" \
        "$recov_rate" "$lbrr_cnt" "$dred_cnt" "$plc_cnt" "$avg_br"
}

# ====================================================================
# 打印表头
# ====================================================================
echo "========================================================"
echo " Opus 仿真实验套件"
echo "========================================================"
echo "输入文件 : $INPUT"
echo "码率     : ${BITRATE} bps"
echo "帧长     : ${FRAMESIZE} ms"
echo "输出目录 : $OUTDIR"
echo ""
printf "  %-35s  %-12s %-10s %-8s %-20s %-10s\n" \
    "实验名称" "配置丢包率" "实际丢包" "恢复率" "LBRR/DRED/PLC帧数" "码率"
printf "  %s\n" "$(printf '%.0s-' {1..100})"

# ========== 实验1: 均匀丢包 × 保护方案 ==========
echo ""
echo "--- 实验1: 均匀丢包 (0% / 5% / 10% / 20%) ---"

for LOSS in 0.0 0.05 0.10 0.20; do
    # 根据 LOSS 计算整数百分比（避免依赖 bc）
    case "$LOSS" in
        0.0)  LOSS_PCT=0  ;;
        0.05) LOSS_PCT=5  ;;
        0.10) LOSS_PCT=10 ;;
        0.20) LOSS_PCT=20 ;;
        *)    LOSS_PCT=$(printf "%.0f" "$(echo "$LOSS * 100" | awk '{print $1*100}')") ;;
    esac

    # PLC（基准）
    run_sim "plc_loss${LOSS_PCT}pct" \
        --loss "$LOSS" --no-lbrr --no-dred

    # LBRR/FEC
    run_sim "lbrr_loss${LOSS_PCT}pct" \
        --loss "$LOSS" --lbrr --ploss "$LOSS_PCT" --no-dred

    # DRED 3帧
    run_sim "dred3_loss${LOSS_PCT}pct" \
        --loss "$LOSS" -dred 3 --no-lbrr

    # DRED 5帧（丢包 > 0 时才有意义）
    if [ "$LOSS_PCT" -gt 0 ]; then
        run_sim "dred5_loss${LOSS_PCT}pct" \
            --loss "$LOSS" -dred 5 --no-lbrr
    fi

    echo ""  # 每个丢包率后空行分隔
done

# ========== 实验2: Gilbert-Elliott 突发丢包 ==========
echo "--- 实验2: Gilbert-Elliott 突发丢包 ---"
echo "    参数: p(G→B)=0.05, p(B→G)=0.3, BAD丢包率=0.8  (期望均值≈11%)"

run_sim "ge_plc"        -ge -ge-p2b 0.05 -ge-b2g 0.30 -ge-bloss 0.80 --no-lbrr --no-dred
run_sim "ge_lbrr"       -ge -ge-p2b 0.05 -ge-b2g 0.30 -ge-bloss 0.80 --lbrr --ploss 15 --no-dred
run_sim "ge_dred3"      -ge -ge-p2b 0.05 -ge-b2g 0.30 -ge-bloss 0.80 -dred 3 --no-lbrr
run_sim "ge_dred5"      -ge -ge-p2b 0.05 -ge-b2g 0.30 -ge-bloss 0.80 -dred 5 --no-lbrr
run_sim "ge_dred10"     -ge -ge-p2b 0.05 -ge-b2g 0.30 -ge-bloss 0.80 -dred 10 --no-lbrr

echo ""

# ========== 实验3: 不同码率 ==========
echo "--- 实验3: 不同码率 + DRED（固定10%均匀丢包）---"
for BR in 16000 32000 64000; do
    BITRATE_SAVED="$BITRATE"
    BITRATE="$BR"
    run_sim "br${BR}_plc_10pct"   --loss 0.10 --no-lbrr --no-dred
    run_sim "br${BR}_dred3_10pct" --loss 0.10 -dred 3 --no-lbrr
    BITRATE="$BITRATE_SAVED"
done

echo ""

# ========== 实验4: 时延 + 抖动 ==========
echo "--- 实验4: 时延 + 抖动 + 10%丢包 ---"
run_sim "delay50_jitter20_plc"   --loss 0.10 -d 50 -j 20 --no-lbrr --no-dred
run_sim "delay50_jitter20_dred3" --loss 0.10 -d 50 -j 20 -dred 3 --no-lbrr

echo ""

# ========== 实验5: DTX ==========
echo "--- 实验5: DTX 不连续发送 ---"
run_sim "dtx_no_loss"  --dtx --loss 0.0
run_sim "dtx_10pct"    --dtx --loss 0.10 -dred 3

echo ""

# ====================================================================
# 综合分析报告
# ====================================================================
echo "========================================================"
echo " 音质对比报告 (SNR vs 原始音频)"
echo "========================================================"
printf "  %-35s  %8s  %10s\n" "实验名称" "全局SNR" "分段SNR"
printf "  %s\n" "$(printf '%.0s-' {1..60})"

for wav in "$OUTDIR"/*.wav; do
    name=$(basename "$wav" .wav)
    [ "$name" = "reference" ] && continue
    # 直接从analyze.py提取数值（格式如 "全局SNR  : 12.34 dB"）
    py_out=$(python3 "$ROOT_DIR/tools/analyze.py" \
        --ref "$OUTDIR/reference.wav" --deg "$wav" 2>/dev/null)
    snr=$(   echo "$py_out" | grep "全局SNR" | grep -oE '[+-]?[0-9]+\.[0-9]+' | head -1)
    segsnr=$(echo "$py_out" | grep "分段SNR" | grep -oE '[+-]?[0-9]+\.[0-9]+' | head -1)
    [ -z "$snr"    ] && snr="-"
    [ -z "$segsnr" ] && segsnr="-"
    printf "  %-35s  %8s dB  %8s dB\n" "$name" "$snr" "$segsnr"
done

echo ""
echo "========================================================"
echo "实验完成！"
echo ""
echo "查看单个实验的详细日志:"
echo "  cat $OUTDIR/<实验名>.log"
echo ""
echo "查看 CSV 统计:"
echo "  python3 tools/analyze.py --csv $OUTDIR/dred3_loss10pct.csv"
echo "========================================================"
