#!/bin/bash
# run_experiments.sh - 批量运行 Opus 仿真实验
#
# 实验矩阵：
#   - 4种丢包率: 0%, 5%, 10%, 20%
#   - 4种保护方案: 无保护(PLC), LBRR, DRED, LBRR+DRED
#   - 2种丢包模型: 均匀, Gilbert突发
#
# 输出: results/ 目录下的WAV和CSV文件，以及汇总报告
#
# 用法:
#   bash scripts/run_experiments.sh [--input audio/speech_like.wav]
#                                   [--bitrate 32000]
#                                   [--quick]  # 仅运行关键实验

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

# 是否快速模式
QUICK=0
for arg in "$@"; do
    [[ "$arg" == "--quick" ]] && QUICK=1
    [[ "$arg" == "--input" ]] && INPUT="$2"
done

mkdir -p "$OUTDIR"

# ---- 检查输入文件 ----
if [ ! -f "$INPUT" ]; then
    echo "[错误] 输入文件不存在: $INPUT"
    echo "请先运行: python3 tools/gen_audio.py"
    exit 1
fi

# 复制参考文件
cp "$INPUT" "$OUTDIR/reference.wav"

echo "========================================"
echo " Opus 仿真实验套件"
echo "========================================"
echo "输入文件 : $INPUT"
echo "码率     : ${BITRATE} bps"
echo "帧长     : ${FRAMESIZE} ms"
echo "输出目录 : $OUTDIR"
echo ""

run_sim() {
    local name="$1"
    local extra_args="${@:2}"
    local out_wav="$OUTDIR/${name}.wav"
    local out_csv="$OUTDIR/${name}.csv"

    echo -n "  运行: $name ... "
    "$SIM" \
        --bitrate "$BITRATE" \
        --framesize "$FRAMESIZE" \
        --csv "$out_csv" \
        $extra_args \
        "$INPUT" "$out_wav" 2>/dev/null

    if [ $? -eq 0 ]; then
        echo "✓"
    else
        echo "✗ (失败)"
    fi
}

# ========== 实验1: 基础丢包率测试 ==========
echo "--- 实验1: 基础丢包（均匀分布） ---"
LOSS_RATES="0.0 0.05 0.10 0.20"

for LOSS in $LOSS_RATES; do
    LOSS_PCT=$(echo "$LOSS * 100" | bc | cut -d. -f1)
    echo "  丢包率: ${LOSS_PCT}%"

    # 1a. 仅PLC（基准）
    run_sim "plc_loss${LOSS_PCT}" \
        --loss "$LOSS" --no-lbrr --no-dred

    # 1b. LBRR保护
    run_sim "lbrr_loss${LOSS_PCT}" \
        --loss "$LOSS" --lbrr --ploss "$LOSS_PCT" --no-dred

    # 1c. DRED保护（3帧）
    run_sim "dred3_loss${LOSS_PCT}" \
        --loss "$LOSS" -dred 3 --no-lbrr

    if [ "$QUICK" -eq 0 ]; then
        # 1d. DRED保护（5帧）
        run_sim "dred5_loss${LOSS_PCT}" \
            --loss "$LOSS" -dred 5 --no-lbrr

        # 1e. LBRR + DRED联合保护
        run_sim "lbrr_dred3_loss${LOSS_PCT}" \
            --loss "$LOSS" --lbrr --ploss "$LOSS_PCT" -dred 3
    fi
done

# ========== 实验2: 突发丢包（Gilbert-Elliott）==========
echo ""
echo "--- 实验2: 突发丢包（Gilbert-Elliott模型）---"
echo "  参数: p(G→B)=0.05, p(B→G)=0.3, 突发丢包率=0.8"
echo "  等效平均丢包率 ≈ $(echo 'scale=1; 0.05/0.35*0.8*100' | bc)%"

if [ "$QUICK" -eq 0 ]; then
    run_sim "ge_plc" \
        -ge -ge-p2b 0.05 -ge-b2g 0.30 -ge-bloss 0.80 \
        --no-lbrr --no-dred

    run_sim "ge_lbrr" \
        -ge -ge-p2b 0.05 -ge-b2g 0.30 -ge-bloss 0.80 \
        --lbrr --ploss 15 --no-dred

    run_sim "ge_dred5" \
        -ge -ge-p2b 0.05 -ge-b2g 0.30 -ge-bloss 0.80 \
        -dred 5 --no-lbrr

    run_sim "ge_lbrr_dred5" \
        -ge -ge-p2b 0.05 -ge-b2g 0.30 -ge-bloss 0.80 \
        --lbrr --ploss 15 -dred 5
fi

# ========== 实验3: 不同码率下的DRED效果 ==========
echo ""
echo "--- 实验3: 不同码率 + DRED（固定10%丢包）---"
for BR in 16000 32000 64000; do
    run_sim "br${BR}_plc_loss10" \
        --bitrate "$BR" --loss 0.10 --no-lbrr --no-dred

    run_sim "br${BR}_dred3_loss10" \
        --bitrate "$BR" --loss 0.10 -dred 3 --no-lbrr
done

# ========== 实验4: 时延+抖动环境 ==========
echo ""
echo "--- 实验4: 时延 + 抖动 + 丢包 ---"
run_sim "delay100_jitter20_loss10_plc" \
    --loss 0.10 -d 100 -j 20 --no-lbrr --no-dred
run_sim "delay100_jitter20_loss10_dred3" \
    --loss 0.10 -d 100 -j 20 -dred 3 --no-lbrr

# ========== 实验5: DTX 效果 ==========
echo ""
echo "--- 实验5: DTX 不连续发送 ---"
run_sim "dtx_on_no_loss" \
    --dtx --loss 0.0
run_sim "dtx_on_loss10" \
    --dtx --loss 0.10 -dred 3

# ========== 分析报告 ==========
echo ""
echo "--- 生成分析报告 ---"
$ANALYZE --compare "$OUTDIR"

# 对某个具体文件做详细分析
if [ -f "$OUTDIR/lbrr_loss10.csv" ]; then
    echo ""
    echo "--- LBRR 10%丢包 详细统计 ---"
    $ANALYZE --csv "$OUTDIR/lbrr_loss10.csv"
fi

echo ""
echo "========================================"
echo "实验完成！结果保存在: $OUTDIR"
echo ""
echo "查看特定文件音质:"
echo "  python3 tools/analyze.py --ref $OUTDIR/reference.wav --deg $OUTDIR/dred3_loss10.wav"
echo ""
echo "播放音频（需要aplay）:"
echo "  aplay $OUTDIR/speech_like.wav"
echo "  aplay $OUTDIR/plc_loss10.wav"
echo "  aplay $OUTDIR/dred3_loss10.wav"
echo "========================================"
