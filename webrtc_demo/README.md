# Pion WebRTC 方案 B：音频 P2P 轻量实现

对应 `docs/端到端落地方案.md` 的 **方案 B**，目录如下：

```
webrtc_demo/
├── internal/opusx/       # 本地 libopus(含DRED) cgo 封装
├── signaling/            # HTTP 信令服务
├── sender/               # WAV -> Opus -> WebRTC 发送
├── receiver/             # WebRTC 接收 -> Opus 解码 -> WAV
├── scripts/
│   ├── run_test.sh                     # 一键端到端测试
│   ├── run_rtc_experiments.sh          # RTC 传输模拟实验矩阵（含报告生成）
│   ├── gen_tone.py                     # 生成 48k 单声道测试音频
│   └── prepare_representative_audio.py # 代表性音频下载（委托 tools/ 版本）
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

### RTC 传输模拟实验矩阵

```bash
cd webrtc_demo

# 标准实验矩阵（默认）
bash scripts/run_rtc_experiments.sh

# 快速回归
EXPERIMENT_SUITE=quick bash scripts/run_rtc_experiments.sh

# 完整矩阵（含延迟抖动场景）
EXPERIMENT_SUITE=full bash scripts/run_rtc_experiments.sh
```

#### 实验矩阵

**丢包场景（standard 模式）：**

| 标识 | 说明 |
|------|------|
| `uniform_5` | 均匀 5% 丢包 |
| `uniform_10` | 均匀 10% 丢包 |
| `uniform_20` | 均匀 20% 丢包 |
| `ge_moderate` | GE 中等突发 (p2b=0.05, b2g=0.30, bloss=0.80, 期望≈11%) |
| `ge_heavy` | GE 重度突发 (p2b=0.10, b2g=0.15, bloss=0.90, 期望≈25%) |

full 模式额外增加 `delay_jitter_10`（均匀 10% + 50ms 延迟 + 20ms 抖动）。

**保护策略：**

| 标识 | 说明 |
|------|------|
| `baseline` | 无保护 (仅 PLC) |
| `lbrr_only` | LBRR 带内 FEC |
| `dred_3` | DRED 3 帧冗余 |
| `dred_5` | DRED 5 帧冗余（standard/full 模式） |
| `lbrr_dred_3` | LBRR + DRED-3 组合 (VBR 64kbps) |

#### 代表性音频

实验会自动联网下载三类代表性音频并统一转码到 48kHz 单声道：

- `music`：音乐片段（SoundHelix）
- `news`：新闻播报片段（BBC podcast RSS 自动解析）
- `dialogue`：多人对话场景片段（餐厅会话环境音）

#### 实验报告

实验完成后自动生成 `results/rtc_report.md`，包含：

- **恢复策略对比表**：按音频类型分组，展示各保护方案的 LBRR/DRED/PLC 恢复帧数与恢复率
- **音质指标表 (SNR/SegSNR)**：对比各方案的全局信噪比与分段信噪比

输出路径可通过 `REPORT_MD` 环境变量自定义。

### 当前实现说明

- 音频链路固定为 48kHz，发送端将单声道扩展为 Opus 双声道，接收端下混为单声道 WAV。
- 接收端支持两类“传输损伤”来源：
  - RTP 序号缺口推断（真实网络丢包）
  - 内置仿真（均匀丢包 / Gilbert-Elliott / 延迟抖动）
- 当前实验重点是快速验证 Opus 编码与恢复策略，不包含 STT/LLM/TTS 业务层。
