# Opus 编解码实验框架

本项目提供一套完整的 Opus 音频编解码研究平台，用于研究和优化 Opus 的各类特性，包括：
- **LBRR** (Low Bit-Rate Redundancy)：SILK 层带内前向纠错
- **DRED** (Deep REDundancy)：基于深度学习的神经网络冗余恢复
- **PLC** (Packet Loss Concealment)：丢包隐藏
- **DTX** (Discontinuous Transmission)：不连续发送
- **网络信道仿真**：均匀/Gilbert-Elliott 丢包模型 + 时延/抖动

## 目录结构

```
.
├── opus-src/          # Opus 1.6.1 源码（含 DRED/OSCE/BWE 支持）
├── opus-install/      # 编译后的 Opus 库
├── src/
│   ├── common.h       # 公共类型定义（RTP、配置、统计）
│   ├── wav_io.h       # WAV 文件读写
│   ├── netsim.h       # 网络信道仿真（丢包/时延/抖动）
│   ├── opus_sim.c     # ★ 核心：离线仿真工具
│   ├── sender.c       # UDP 实时发送端
│   └── receiver.c     # UDP 实时接收端（含抖动缓冲）
├── tools/
│   ├── gen_audio.py                 # 生成合成测试音频（正弦波/语音/调频/噪声）
│   ├── prepare_representative_audio.py  # 下载代表性真实音频（music/news/dialogue）
│   ├── analyze.py                   # 分析仿真结果（SNR/SegSNR/丢包分布）
│   └── gen_rtc_report.py            # 从实验 CSV 生成 Markdown 报告
├── scripts/
│   ├── run_experiments.sh  # 批量离线仿真实验脚本
│   └── run_udp_test.sh     # UDP 回环测试脚本
├── webrtc_demo/       # Pion WebRTC 轻量 Demo（含 RTC 实验矩阵）
├── audio/             # 测试音频文件
├── results/           # 仿真输出 + Markdown 报告
├── weights_blob.bin   # DRED/DeepPLC 神经网络权重
└── CMakeLists.txt
```

## 技术文档

- [LBRR 技术分析](docs/LBRR_技术分析.md) — 带内 FEC 机制、激活条件、实测覆盖率、改进方向
- [DRED / PLC 技术分析](docs/DRED_PLC_深度分析.md) — 神经网络冗余恢复与丢包隐藏
- [FEC 新方案探索](docs/Opus_FEC_新方案探索报告.md) — 六个新方案方向的技术调研
- [端到端落地方案](docs/端到端落地方案.md) — AI 语音聊天 RTC 场景的优化呈现与集成方案

## 快速开始

### 1. 编译

```bash
mkdir build && cd build
cmake ..
make -j$(nproc)
cd ..
```

### 2. 准备测试音频

**方式 A：合成音频（离线快速测试）**

```bash
python3 tools/gen_audio.py
```

生成的文件：
- `audio/speech_like.wav` — 模拟语音（48kHz, 单声道, 10秒）
- `audio/sine_440hz.wav` — 纯正弦波
- `audio/chirp_200_4000.wav` — 线性调频信号（用于频响测试）
- `audio/speech_16k.wav` — 16kHz 语音（适合 LBRR 测试）

**方式 B：代表性真实音频（推荐，需联网 + ffmpeg）**

```bash
python3 tools/prepare_representative_audio.py \
    --out-dir audio --manifest audio/manifest.txt --clip-seconds 10
```

自动下载三类代表性音频并统一转码到 48kHz 单声道 WAV：

- `music`：音乐片段（SoundHelix）
- `news`：新闻播报片段（BBC podcast RSS 自动解析）
- `dialogue`：多人对话场景片段（餐厅会话环境音）

### 3. 运行仿真

```bash
export LD_LIBRARY_PATH=$(pwd)/opus-install/lib:$LD_LIBRARY_PATH

# 基础测试：无丢包
./build/opus_sim audio/speech_like.wav results/clean.wav

# 10% 均匀丢包 + PLC（基准）
./build/opus_sim -l 0.1 --no-lbrr --no-dred \
    audio/speech_like.wav results/plc_10.wav --csv results/plc_10.csv

# 10% 丢包 + DRED 3帧（推荐配置）
./build/opus_sim -l 0.1 -dred 3 \
    audio/speech_like.wav results/dred3_10.wav --csv results/dred3_10.csv

# 突发丢包（Gilbert-Elliott）+ DRED 5帧
./build/opus_sim -ge -ge-p2b 0.05 -ge-b2g 0.3 -ge-bloss 0.8 \
    -dred 5 audio/speech_like.wav results/ge_dred5.wav

# 批量实验（使用代表性真实音频，默认 music/news/dialogue）
bash scripts/run_experiments.sh

# 使用合成音频运行
AUDIO_MODE=synthetic bash scripts/run_experiments.sh
```

批量实验脚本会自动下载 music/news/dialogue 三类音频，对每种音频执行完整实验矩阵，
并在 `results/opus_report.md` 生成 Markdown 汇总报告。

---

## 主要特性说明

### DRED (Deep REDundancy)

DRED 是 Opus 1.5 引入、1.6 大幅改进的神经网络冗余机制，在每个包中附加过去 N 帧的 DNN 压缩冗余数据。
当包丢失时，接收端用后续包中的 DRED 数据恢复。

**关键参数：**
| 参数 | 说明 |
|------|------|
| `-dred <N>` | 冗余帧数（单位 10ms，推荐 3-10）|
| `-plp <PCT>` | 预期丢包率（必须设置，否则 DRED 不激活）|
| `weights_blob.bin` | 神经网络权重（编解码器自动加载）|

**重要说明：** DRED 要求 `OPUS_SET_PACKET_LOSS_PERC > 0`，编码器根据此值计算冗余预算。若设为 0，DRED 的比特分配为零，不产生冗余数据。

**实测效果（Opus 1.6.1，10% 均匀丢包）：**
```
DRED 3帧 VBR 32kbps → 100% 丢包恢复率
DRED 3帧 CBR 32kbps → 90-97%（简单信号）/ 13-34%（复杂语音，受 CBR 预算限制）
```

> **重要**：CBR 模式下 DRED 与主编码共享固定码率预算，复杂语音内容会挤占 DRED 预算。建议使用 **VBR 模式**或提高码率。

**原理（DRED 解码流程）：**
```
帧序列: [0, 1, 2(丢), 3(丢), 4(接收)]

1. 在包 4 上调用 opus_dred_parse() → 提取 DRED 数据（覆盖过去 5 帧）
2. 恢复帧 2: opus_decoder_dred_decode(dec, dred, offset=(4-2)*960, out, 960)
3. 恢复帧 3: opus_decoder_dred_decode(dec, dred, offset=(4-3)*960, out, 960)
4. 正常解码包 4: opus_decode(dec, pkt4, ...)
```

### LBRR (Low Bit-Rate Redundancy)

LBRR 是 SILK 层的带内 FEC，将前一帧的低质量副本打包在当前帧中。

**关键参数：**
| 参数 | 说明 |
|------|------|
| `-fec` / `--lbrr` | 启用 LBRR |
| `-plp <PCT>` | 声明给编码器的丢包率（必须 > 0，影响 FEC 开销） |

**实测效果（Opus 1.6.1，32kbps CBR，10% 均匀丢包，TTS 真实语音）：**
```
LBRR 生成率 → 80-91%（真实语音）
LBRR 恢复率 → 68-85%（单帧丢包）
LBRR 恢复率 → 36-45%（突发丢包，因仅恢复末尾帧）
```

**限制：**
- 仅适用于 **SILK 模式**（通常 ≤ 32kbps VOIP 语音）
- 一次只能恢复 **1 帧**（突发末尾的最近丢失帧）
- `plp ≤ 5%` 时在大多数码率下不激活
- CBR 模式下 LBRR 开销通过减小主流质量来补偿

**使用建议：**
- 单纯语音传输：使用 LBRR（低延迟，无 DNN 开销）
- 高丢包/突发丢包：使用 DRED（显著更高恢复率）
- 高质量音乐/语音混合：DRED（适用于 CELT 模式）
- 充足码率（≥ 48kbps）：可考虑 LBRR + DRED 组合

详见 [LBRR 技术分析](docs/LBRR_技术分析.md)。

### PLC (Packet Loss Concealment)

当 LBRR 和 DRED 均无法恢复时，解码器通过 PLC 生成近似音频（外插波形）。
调用方式：`opus_decode(dec, NULL, 0, out, frame_size, 0)`

### DTX (Discontinuous Transmission)

对静音/低能量段不发包，减少带宽消耗。
启用方式：`-dtx`

---

## opus_sim 完整参数说明

```
用法: opus_sim [选项] 输入.wav 输出.wav

编码器选项:
  -b,  --bitrate   <bps>     码率 (默认: 32000)
  -fs, --framesize <ms>      帧长 2.5/5/10/20/40/60 (默认: 20)
  -fec,--lbrr                开启LBRR带内FEC
  -plp,--ploss    <pct>      向编码器声明的丢包率(影响FEC/DRED强度)
  -dtx,--dtx                 开启DTX
  -vbr,--vbr                 开启VBR
  -dred <n>                  DRED冗余帧数 (单位:10ms, 推荐2-10)
  -cx, --complexity <0-10>   编码复杂度 (默认:9)
  -app <voip|audio|ll>       应用类型 (默认:voip)

解码器选项:
  --no-dred                  禁用DRED恢复
  --no-lbrr                  禁用LBRR恢复
  --no-plc                   禁用PLC（丢包填零）

网络仿真选项:
  -l,  --loss      <rate>    均匀丢包率 [0,1]
  -ge                        使用Gilbert-Elliott突发丢包模型
  -ge-p2b <p>                GE: GOOD→BAD转移概率 (默认:0.05)
  -ge-b2g <p>                GE: BAD→GOOD转移概率 (默认:0.3)
  -ge-bloss <p>              GE: BAD状态丢包率 (默认:0.8)
  -d,  --delay     <ms>      固定时延
  -j,  --jitter    <ms>      时延抖动标准差

输出选项:
  -v,  --verbose             打印每帧详情
  --csv <file>               输出统计CSV文件
```

---

## UDP 实时传输测试

### 基本回环测试

```bash
# 终端1：启动接收端（录制 15 秒）
./build/opus_receiver -p 5004 -t 15 results/udp_out.wav

# 终端2：启动发送端（含10%丢包仿真 + DRED）
./build/opus_sender -p 5004 -l 0.1 -dred 5 audio/speech_like.wav
```

### 使用脚本自动化

```bash
# 软件仿真丢包（推荐）
bash scripts/run_udp_test.sh --loss 0.1 --dred 5

# 使用 Linux tc netem 添加真实网络损伤（需要 root）
bash scripts/run_udp_test.sh --netem --loss 10 --delay 50 --jitter 20
```

---

## 结果分析

```bash
# 分析 CSV 统计文件
python3 tools/analyze.py --csv results/dred3_10.csv

# 比较原始音频与恢复后音频的 SNR
python3 tools/analyze.py --ref audio/speech_like.wav --deg results/dred3_10.wav

# 对比目录下所有方案（需要有 reference.wav）
python3 tools/analyze.py --compare results/
```

---

## 仿真实验关键结论

> 以下数据基于 Opus 1.6.1，32kbps，20ms 帧长。

| 场景 | 保护方案 | 丢包率 | 恢复率 | 说明 |
|------|---------|--------|--------|------|
| 均匀 10% | PLC only | 10% | 0% | 基线 |
| 均匀 10% | LBRR（CBR） | 10% | **68-85%** | TTS 真实语音 |
| 均匀 10% | DRED 3帧（VBR） | 10% | **100%** | VBR 模式最优 |
| 均匀 10% | DRED 3帧（CBR） | 10% | **13-97%** | CBR 受预算限制，复杂语音偏低 |
| 均匀 10% | LBRR + DRED 3帧（VBR） | 10% | **100%** | 组合最优 |
| Gilbert突发 | LBRR | ~13% | **42-45%** | 仅恢复突发末尾帧 |
| Gilbert突发 | DRED 5帧（CBR） | ~13% | **13-88%** | 与信号复杂度相关 |

> **关键发现**（Opus 1.6.1 vs 1.5.2）：1.6.1 的 DRED 在 VBR 模式下表现优异（100% 恢复率），但在 CBR 模式下对复杂语音内容恢复率显著下降（13-34%），因为主编码消耗更多比特导致 DRED 预算不足。建议使用 VBR 模式。
>
> LBRR 数据来自 TTS 语音实验（espeak-ng/flite）。合成测试信号因频谱稳定性导致 SILK VAD 噪声底适应，LBRR 生成率仅 ~15%，不代表真实语音表现。详见 [LBRR 技术分析](docs/LBRR_技术分析.md)。

---

## 扩展方向（WebRTC 集成）

后续集成 WebRTC 时，Opus 编码器通过以下接口插入：
1. **libwebrtc**：替换 `modules/audio_coding/codecs/opus/` 下的编解码模块
2. **DRED 参数传递**：通过 SDP 协商或 RTP extension 传递 DRED 配置
3. **NACK + DRED 协同**：WebRTC 的 NACK 机制（请求重传）与 DRED（前向冗余）可并行使用：
   - DRED 提供即时恢复（无需 RTT 等待）
   - NACK 在 DRED 覆盖范围外（突发 > dred_duration）提供补充恢复

---

## Pion WebRTC 轻量 Demo

仓库新增了独立的 `webrtc_demo/` 子目录，用于实现并验证 `docs/端到端落地方案.md` 的**方案 B（Pion 音频 P2P 轻量实现）**：

- `signaling`：轻量 HTTP 信令服务
- `sender`：WAV → 本地 Opus（支持 FEC/DRED）→ WebRTC 音频发送
- `receiver`：WebRTC 接收 → 本地 Opus（支持 DRED 恢复）→ WAV 输出 + 统计
- 支持 RTC 传输仿真（均匀丢包/GE/延迟抖动）用于快速回归

快速体验：

```bash
cd webrtc_demo
bash scripts/run_test.sh
```

快速实验矩阵：

```bash
cd webrtc_demo

# 标准实验矩阵（5 场景 × 5 策略 × 3 音频 = 75 组）
bash scripts/run_rtc_experiments.sh

# 快速回归（2 场景 × 4 策略 × 3 音频 = 24 组）
EXPERIMENT_SUITE=quick bash scripts/run_rtc_experiments.sh

# 完整矩阵（含延迟抖动，6 场景 × 5 策略 × 3 音频 = 90 组）
EXPERIMENT_SUITE=full bash scripts/run_rtc_experiments.sh
```

实验完成后自动生成 `results/rtc_report.md`，包含：
- **恢复策略表**：各音频类型下各保护方案的 LBRR/DRED/PLC 恢复帧数与恢复率
- **SNR/SegSNR 表**：全局信噪比与分段信噪比对比

每次 RTC 实验还会保留完整输入/输出音频与统计工件，默认目录为 `results/rtc_runs/<RUN_ID>/`：
- `inputs/`：本次使用的 `music/news/dialogue` 输入 WAV
- `outputs/`：每个场景与保护策略对应的接收端输出 WAV
- `stats/`：每个 case 的统计 JSON
- `rtc_experiment_summary.csv` / `rtc_report.md`：本次实验汇总与报告

为便于快速回归，代表性音频默认缓存到 `results/representative_audio_cache/clip_<N>s/`，重复执行时复用缓存；`results/rtc_latest` 始终指向最近一次 RTC 运行目录。
默认新闻样本使用稳定的小体积 fallback 片段；如需强制改回 BBC live RSS 源，可设置 `REP_AUDIO_PREFER_LIVE_NEWS=1`。

默认实验会自动联网下载三类代表性音频并统一转码到 48kHz 单声道：

- `music`：音乐片段（SoundHelix）
- `news`：新闻播报片段（BBC podcast RSS 自动解析）
- `dialogue`：多人对话场景片段（餐厅会话环境音）

默认每类截取 10 秒音频；如需覆盖，可设置环境变量 `CLIP_SECONDS`。

如果在 Cursor Cloud 中运行，建议在启动脚本执行：

```bash
bash scripts/cursor_cloud_startup.sh
```

该脚本会保证 Go `>=1.22`，并预热 `webrtc_demo` 的 Go 依赖。

---

## 依赖

- Opus 1.6.1（含 DRED/OSCE/BWE）
- GCC/Clang
- CMake ≥ 3.10
- Python 3.6+（测试工具）
- Linux（tc netem 用于真实网络仿真，可选）
