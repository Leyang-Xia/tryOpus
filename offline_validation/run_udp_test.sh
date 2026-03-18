#!/bin/bash
# run_udp_test.sh - UDP回环实时传输测试
#
# 在本机同时启动发送端和接收端，通过回环地址（127.0.0.1）传输。
# 可选：使用 Linux tc netem 在回环接口添加真实网络损伤。
#
# 用法:
#   bash offline_validation/run_udp_test.sh [--netem] [--loss 10] [--delay 50]

set -euo pipefail
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BUILD_DIR="$ROOT_DIR/build"
SENDER="$BUILD_DIR/opus_sender"
RECEIVER="$BUILD_DIR/opus_receiver"

INPUT="${INPUT:-$ROOT_DIR/representative_audio/dialogue/dialogue_30s_48k_mono.wav}"
RUN_ID="${RUN_ID:-udp_$(date +%Y%m%d_%H%M%S)}"
RUN_DIR="${RUN_DIR:-$ROOT_DIR/results/offline_runs/$RUN_ID}"
OUTPUT="${OUTPUT:-$RUN_DIR/udp_output.wav}"
PORT=5004
DURATION=15  # 录制秒数
LOSS=0.0
DELAY=0
JITTER=0
USE_NETEM=0
DRED=0
FEC=0

# 参数解析
while [[ $# -gt 0 ]]; do
    case "$1" in
        --netem)  USE_NETEM=1;;
        --loss)   LOSS="$2";   shift;;
        --delay)  DELAY="$2";  shift;;
        --jitter) JITTER="$2"; shift;;
        --dred)   DRED="$2";   shift;;
        --fec)    FEC=1;;
        --input)  INPUT="$2";  shift;;
        --out)    OUTPUT="$2"; shift;;
        --port)   PORT="$2";   shift;;
        --duration) DURATION="$2"; shift;;
    esac
    shift
done

mkdir -p "$RUN_DIR"

echo "========================================"
echo " Opus UDP 回环传输测试"
echo "========================================"
echo "输入文件 : $INPUT"
echo "输出文件 : $OUTPUT"
echo "端口     : $PORT"
echo "丢包率   : ${LOSS}"
echo "时延     : ${DELAY}ms"
echo "抖动     : ${JITTER}ms"
echo "DRED     : $DRED 帧"
echo "FEC/LBRR : $([ $FEC -eq 1 ] && echo 开 || echo 关)"
echo ""

# ---- 可选: 使用 tc netem 添加真实网络损伤 ----
cleanup_netem() {
    if [ "$USE_NETEM" -eq 1 ]; then
        echo "清理 tc netem..."
        sudo tc qdisc del dev lo root 2>/dev/null || true
    fi
}
trap cleanup_netem EXIT

if [ "$USE_NETEM" -eq 1 ]; then
    echo "配置 tc netem..."
    if ! command -v tc &>/dev/null; then
        echo "[警告] tc 命令不存在，回退到软件仿真"
        USE_NETEM=0
    else
        LOSS_PCT=$(echo "$LOSS * 100" | bc | cut -d. -f1)
        sudo tc qdisc add dev lo root netem \
            loss "${LOSS_PCT}%" \
            delay "${DELAY}ms" "${JITTER}ms" \
            2>/dev/null || \
        sudo tc qdisc change dev lo root netem \
            loss "${LOSS_PCT}%" \
            delay "${DELAY}ms" "${JITTER}ms"
        echo "tc netem 已配置: loss=${LOSS_PCT}% delay=${DELAY}ms±${JITTER}ms"
        # 如果使用netem，发送端不需要软件仿真
        LOSS=0.0
        DELAY=0
        JITTER=0
    fi
fi

# ---- 构造发送/接收参数 ----
SENDER_ARGS="-p $PORT"
[ "$LOSS" != "0" ] && [ "$LOSS" != "0.0" ] && \
    SENDER_ARGS="$SENDER_ARGS -l $LOSS"
[ "$DELAY" != "0" ] && SENDER_ARGS="$SENDER_ARGS -d $DELAY"
[ "$JITTER" != "0" ] && SENDER_ARGS="$SENDER_ARGS -j $JITTER"
[ "$DRED" != "0" ] && SENDER_ARGS="$SENDER_ARGS -dred $DRED"
[ "$FEC" -eq 1 ] && SENDER_ARGS="$SENDER_ARGS -fec"
SENDER_ARGS="$SENDER_ARGS -speed 1.0"

RECEIVER_ARGS="-p $PORT -t $DURATION"
[ "$DRED" -eq 0 ] && RECEIVER_ARGS="$RECEIVER_ARGS --no-dred"

echo "启动接收端..."
"$RECEIVER" $RECEIVER_ARGS "$OUTPUT" &
RECV_PID=$!
sleep 0.3  # 等接收端就绪

echo "启动发送端..."
"$SENDER" $SENDER_ARGS "$INPUT" &
SEND_PID=$!

echo "传输中（$DURATION 秒）..."
wait $RECV_PID
kill $SEND_PID 2>/dev/null

echo ""
echo "完成！输出: $OUTPUT"
echo ""
echo "分析结果:"
python3 "$ROOT_DIR/tools/analyze.py" \
    --ref "$INPUT" --deg "$OUTPUT" 2>/dev/null || true
