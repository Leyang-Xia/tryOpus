# Pion WebRTC 方案 B：音频 P2P 轻量实现

对应 `docs/端到端落地方案.md` 的 **方案 B**，目录如下：

```
webrtc_demo/
├── internal/opusx/       # 本地 libopus(含DRED) cgo 封装
├── signaling/            # HTTP 信令服务
├── sender/               # WAV -> Opus -> WebRTC 发送
├── receiver/             # WebRTC 接收 -> Opus 解码 -> WAV
├── scripts/run_test.sh   # 一键端到端测试
├── scripts/run_rtc_experiments.sh # RTC传输模拟实验矩阵
└── scripts/gen_tone.py   # 生成 48k 单声道测试音频
```

## 依赖准备（本地 Opus + DRED）

```bash
ROOT_DIR=$(pwd)
export PKG_CONFIG_PATH=${ROOT_DIR}/../opus-install/lib/pkgconfig:${PKG_CONFIG_PATH}
export LD_LIBRARY_PATH=${ROOT_DIR}/../opus-install/lib:${LD_LIBRARY_PATH}
```

`sender` / `receiver` 会通过 `internal/opusx` 直接调用本地 `libopus`，并支持：

- `OPUS_SET_DRED_DURATION`
- `OPUS_SET_DNN_BLOB`
- `opus_dred_parse`
- `opus_decoder_dred_decode`

### 手动运行

终端 1：启动信令

```bash
cd webrtc_demo
go run ./signaling -addr :8090
```

终端 2：启动接收端

```bash
cd webrtc_demo
go run ./receiver \
  --signal http://127.0.0.1:8090 \
  --session demo-session \
  --weights ${PWD}/../weights_blob.bin \
  --output /tmp/received.wav \
  --duration 8s \
  --sim-loss 0.10 \
  --use-dred=true
```

终端 3：启动发送端

```bash
cd webrtc_demo
go run ./sender \
  --signal http://127.0.0.1:8090 \
  --session demo-session \
  --input /path/to/input_48k_mono.wav \
  --weights ${PWD}/../weights_blob.bin \
  --dred 3 --plp 15 --fec=true
```

### 一键端到端测试

```bash
cd webrtc_demo
bash scripts/run_test.sh
```

脚本会自动：

- 启动信令服务
- 生成测试音频
- 启动 receiver/sender 建链并传输
- 启用模拟丢包并验证 DRED/LBRR/PLC 恢复路径
- 校验输出 WAV 和统计 JSON

### RTC 传输模拟实验（快速回归）

```bash
cd webrtc_demo
bash scripts/run_rtc_experiments.sh
```

默认会在同一套输入音频和丢包条件下跑四组实验：

- `baseline_no_protection`
- `lbrr_only`
- `dred_only`
- `lbrr_dred`

并输出一份对比 CSV，便于本地 Opus 改动后的快速回归。

默认实验会自动联网下载三类代表性音频并统一转码到 48k 单声道：

- `music`：音乐片段（SoundHelix）
- `news`：新闻播报片段（BBC podcast RSS 自动解析）
- `dialogue`：多人对话场景片段（餐厅会话环境音）

### 当前实现说明

- 音频链路固定为 48kHz，发送端将单声道扩展为 Opus 双声道，接收端下混为单声道 WAV。
- 接收端支持两类“传输损伤”来源：
  - RTP 序号缺口推断（真实网络丢包）
  - 内置仿真（均匀丢包 / Gilbert-Elliott / 延迟抖动）
- 当前实验重点是快速验证 Opus 编码与恢复策略，不包含 STT/LLM/TTS 业务层。
