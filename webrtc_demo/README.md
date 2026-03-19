# Pion WebRTC 方案 B：音频 P2P 轻量实现

对应 `docs/端到端落地方案.md` 的 **方案 B**。仓库顶层与之同级的 `offline_validation/` 负责非 RTC 验证；本目录只承接 RTC 验证，目录如下：

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
│   └── prepare_representative_audio.py # 代表性音频更新（委托 tools/ 版本）
```

## 依赖准备（本地 Opus + DRED）

```bash
ROOT_DIR=$(pwd)
export PKG_CONFIG_PATH=${ROOT_DIR}/../opus-install/lib/pkgconfig:${PKG_CONFIG_PATH}
export LD_LIBRARY_PATH=${ROOT_DIR}/../opus-install/lib:${LD_LIBRARY_PATH}
```

在 macOS 上，`go build` / `go run` 可能因为 cgo build cache 复用而错误链接到 Homebrew 的 `libopus`。当前 `scripts/run_test.sh` 和 `scripts/run_rtc_experiments.sh` 已经内置：

- `go clean -cache`
- 用项目内 `opus-install/lib/pkgconfig` 重建
- 用 `otool -L` 校验 sender/receiver 必须链接到项目内 `libopus.0.dylib`

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

# 标准实验矩阵（默认：news/dialogue + WER/SER）
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

#### 代表性音频

RTC 实验默认直接读取顶层 `representative_audio/manifest.txt` 中的两类基线语音音频：

- `news`：VOA Learning English 新闻播报片段（固定跳过片头音乐）
- `dialogue`：ELLLO 真实英文对话片段（附参考文本）

标准集固定为 30 秒，以便 `SER` 在对话与新闻样本上有足够多的句子可比较。

#### 实验报告

实验完成后自动生成 `results/rtc_report.md`，包含：

- **恢复策略对比表**：按音频类型分组，展示各保护方案的 LBRR/DRED/PLC 恢复帧数与恢复率
- **文本指标表 (WER/SER)**：对比各方案在语音可懂度上的相对退化
- **音频/转写工件表**：输入 WAV、输出 WAV、参考转写、输出转写、统计 JSON

输出路径可通过 `REPORT_MD` 环境变量自定义。

默认情况下，每次运行都会创建独立产物目录：

- `results/rtc_runs/<RUN_ID>/inputs/`：本次实验使用的代表性输入音频副本
- `results/rtc_runs/<RUN_ID>/outputs/`：每个音频/场景/策略对应的接收端输出 WAV
- `results/rtc_runs/<RUN_ID>/stats/`：每个 case 的统计 JSON
- `results/rtc_runs/<RUN_ID>/rtc_experiment_summary.csv`：本次实验汇总
- `results/rtc_runs/<RUN_ID>/rtc_report.md`：本次实验报告

同时会维护两个便捷入口：

- `results/rtc_report.md`：最新一次实验报告副本
- `results/rtc_latest`：指向最新一次实验目录的符号链接

代表性音频作为仓库内置资产保存在顶层 `representative_audio/`，RTC 实验会把它们复制到每次运行的 `inputs/` 目录。
WER/SER 报告默认使用仓库下 `.venv_asr/bin/python` 中的 ASR 环境，默认模型为 `small.en`。在 Apple Silicon macOS 上默认优先使用 `mlx-whisper`，其他环境默认使用 `faster-whisper`；也可以通过 `RTC_STT_BACKEND=mlx|faster|auto` 显式指定后端。报告会把干净输入音频的转写结果当作参考文本；若请求的模型在当前后端不可用，脚本会自动回退到可用后端或已缓存模型，避免整轮实验在报告阶段失败。
如果更关心回归速度而不是识别稳定性，可在运行前覆盖 `STT_MODEL=base.en`。

### 当前实现说明

- 音频链路固定为 48kHz，发送端将单声道扩展为 Opus 双声道，接收端下混为单声道 WAV。
- 接收端支持两类“传输损伤”来源：
  - RTP 序号缺口推断（真实网络丢包）
  - 内置仿真（均匀丢包 / Gilbert-Elliott / 延迟抖动）
- 当前实验重点是快速验证 Opus 编码与恢复策略，不包含 STT/LLM/TTS 业务层。
