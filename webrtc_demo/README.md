# WebRTC 验证

`webrtc_demo/` 是仓库里的 RTC 验证入口，基于 **Go + Pion WebRTC** 实现。它不是浏览器 Demo，而是一套用于研究 Opus 冗余保护的可控实验链路：

- `signaling`：极简 HTTP 信令
- `sender`：WAV -> 本地 `libopus` 编码 -> WebRTC 发送
- `receiver`：WebRTC 接收 -> 本地 `libopus` 解码 -> WAV 输出
- `scripts/run_rtc_experiments.sh`：批量跑 RTC 弱网实验并生成报告

## 当前架构

```text
webrtc_demo/
├── internal/
│   ├── adaptation/   # sender 侧动态冗余控制器
│   ├── opusx/        # cgo 封装项目内 libopus
│   ├── rtc/          # Pion 封装与 receiver-side 丢包注入
│   ├── signal/       # 信令客户端/协议
│   └── wav/          # 48k PCM16 mono WAV 读写
├── signaling/
├── sender/
├── receiver/
├── scripts/
│   ├── run_test.sh
│   ├── run_rtc_experiments.sh
│   ├── gen_tone.py
│   └── prepare_representative_audio.py
├── go.mod
└── README.md
```

## 关键实现边界

### `internal/opusx`

直接绑定项目内 `libopus`，支持：

- `OPUS_SET_INBAND_FEC`
- `OPUS_SET_PACKET_LOSS_PERC`
- `OPUS_SET_DRED_DURATION`
- `OPUS_SET_DNN_BLOB`
- `opus_dred_parse`
- `opus_decoder_dred_decode`

### `internal/rtc`

- 初始化 Pion `PeerConnection`
- 注册默认 codec / interceptor
- 为 sender 打开 TWCC header extension
- 支持在 receiver 侧用 interceptor 注入均匀丢包或 GE 突发丢包

### `internal/adaptation`

sender 侧根据以下反馈做冗余切换：

- RTCP Receiver Report
- REMB
- Pion `GetStats()`
- TWCC 派生的 burst 指标

控制结果会落到：

- `FEC`
- `PLP`
- `DRED duration`
- `adaptation.json`

## 依赖准备

### 1. Go 版本

```bash
go version
```

要求 `>= 1.22`。

### 2. 使用项目内 Opus 运行时

```bash
cd webrtc_demo
export PKG_CONFIG_PATH="$(pwd)/../opus-install/lib/pkgconfig:${PKG_CONFIG_PATH:-}"
export LD_LIBRARY_PATH="$(pwd)/../opus-install/lib:${LD_LIBRARY_PATH:-}"
export DYLD_LIBRARY_PATH="$(pwd)/../opus-install/lib:${DYLD_LIBRARY_PATH:-}"
```

### 3. 权重文件

默认使用：

```text
../weights_blob.bin
```

## 冒烟测试

```bash
cd webrtc_demo
bash scripts/run_test.sh
```

这个脚本会：

- `go clean -cache`
- 编译 `signaling` / `sender` / `receiver`
- 检查二进制是否实际链接到项目内 `libopus`
- 生成测试音频
- 建立 sender -> receiver 的 P2P 会话
- 输出接收端 WAV 和统计 JSON

## 手动运行

### 1. 启动信令

```bash
cd webrtc_demo
go run ./signaling -addr :8090
```

### 2. 启动接收端

```bash
cd webrtc_demo
go run ./receiver \
  --signal http://127.0.0.1:8090 \
  --session demo-session \
  --output /tmp/received.wav \
  --stats-json /tmp/received_stats.json \
  --weights ../weights_blob.bin \
  --duration 8s \
  --sim-loss 0.10 \
  --use-lbrr=true \
  --use-dred=true
```

### 3. 启动发送端

```bash
cd webrtc_demo
go run ./sender \
  --signal http://127.0.0.1:8090 \
  --session demo-session \
  --input ../representative_audio/dialogue/dialogue_30s_48k_mono.wav \
  --weights ../weights_blob.bin \
  --bitrate 32000 \
  --plp 15 \
  --fec=true \
  --dred 3
```

## RTC 实验矩阵

```bash
cd webrtc_demo

# 标准矩阵
bash scripts/run_rtc_experiments.sh

# 快速回归
EXPERIMENT_SUITE=quick bash scripts/run_rtc_experiments.sh

# 完整矩阵
EXPERIMENT_SUITE=full bash scripts/run_rtc_experiments.sh
```

### 默认输入

脚本固定读取顶层：

- `../representative_audio/manifest.txt`

当前标准集包含：

- `news`
- `dialogue`

### 场景

`standard` 默认场景：

- `uniform_5`
- `uniform_10`
- `uniform_20`
- `ge_moderate`
- `ge_heavy`

`full` 在此基础上额外增加：

- `delay_jitter_10`

### 策略

`standard` / `full` 默认策略：

- `baseline`
- `lbrr_only`
- `dred_3`
- `dred_5`

`quick` 默认额外包含：

- `adaptive_auto`

### 常用环境变量

- `EXPERIMENT_SUITE=quick|standard|full`
- `RUN_ID`
- `RUN_DIR`
- `RECV_DURATION`
- `SIM_SEED`
- `SENDER_BITRATE`
- `SENDER_COMPLEXITY`
- `SENDER_SIGNAL`
- `SENDER_ADAPTIVE_REDUNDANCY`
- `SENDER_FEEDBACK_INTERVAL`
- `SENDER_ADAPT_WINDOW`
- `EXTRA_DRED_VALUES`
- `STRATEGY_FILTER`
- `SCENARIO_FILTER`
- `ASR_PYTHON`
- `STT_MODEL`
- `RTC_STT_BACKEND`

### 典型命令

只跑固定策略里的 `dred_5`：

```bash
cd webrtc_demo
EXPERIMENT_SUITE=standard \
STRATEGY_FILTER=dred_5 \
SCENARIO_FILTER=uniform_10,ge_moderate \
bash scripts/run_rtc_experiments.sh
```

只跑自适应策略：

```bash
cd webrtc_demo
EXPERIMENT_SUITE=quick \
STRATEGY_FILTER=adaptive_auto \
SCENARIO_FILTER=uniform_10,ge_moderate \
bash scripts/run_rtc_experiments.sh
```

添加额外 DRED 档位：

```bash
cd webrtc_demo
EXPERIMENT_SUITE=standard \
EXTRA_DRED_VALUES=10 \
bash scripts/run_rtc_experiments.sh
```

## 输出目录

默认输出到：

```text
results/rtc_runs/<RUN_ID>/
├── inputs/
├── outputs/
├── stats/
├── logs/
├── transcripts/
├── adaptation/
├── rtc_experiment_summary.csv
└── rtc_report.md
```

同时维护：

- `results/rtc_report.md`
- `results/rtc_latest`

缓存目录：

- `results/rtc_bin_cache/`
- `results/rtc_go_cache/`

## 报告链路

`run_rtc_experiments.sh` 结束后会调用顶层 `tools/gen_rtc_report.py`，输出：

- 恢复帧统计
- 恢复率
- WER / SER
- 音频 / 转写 / JSON 工件链接

默认优先使用顶层 `.venv_asr/bin/python`。在 Apple Silicon macOS 上，ASR 后端默认优先 `mlx-whisper`，其他环境默认优先 `faster-whisper`。

## 当前实现特征

- 输入 WAV 约定为 `48kHz / PCM16 / mono`
- sender 内部会按 Opus 双声道编码链路处理，receiver 最终下混输出单声道 WAV
- 丢包注入发生在 receiver-side RTP interceptor，而不是 sender 侧伪造“未发送”
- receiver 同时支持：
  - RTP 序号缺口推断真实丢包
  - 内置均匀丢包 / GE / 延迟 / 抖动仿真

## 注意事项

- 如果 `PKG_CONFIG_PATH` 没有指向项目内 `opus-install/lib/pkgconfig`，Go 二进制很容易误链到系统 `libopus`
- `run_test.sh` 和 `run_rtc_experiments.sh` 会显式校验 sender / receiver 的 `libopus` 链接目标
- `adaptive_auto` 默认只在 `quick` 套件里出现；如果在 `standard` / `full` 下直接过滤它，结果会是空矩阵
