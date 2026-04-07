# Opus 编解码实验框架

`opus_lab` 是一个围绕 **Opus 1.6.1** 的实验仓库，当前已经形成两条并行验证链路：

- `offline_validation/`：C 语言离线仿真和 UDP 回环验证
- `webrtc_demo/`：Go + Pion 的 RTC 端到端验证与实验矩阵

共享输入资产、分析脚本和实验报告都放在仓库顶层统一维护。

## 当前项目架构

| 层级 | 作用 | 关键目录 |
| --- | --- | --- |
| 本地 Opus 运行时 | 提供带 DRED / DeepPLC / OSCE / BWE 的 `libopus` | `opus-src/` `opus-install/` `weights_blob.bin` |
| 共享资产与报告链路 | 统一测试音频、分析脚本、报告生成 | `representative_audio/` `tools/` `results/` |
| 非 RTC 验证 | 离线仿真、UDP 回环、批量实验 | `offline_validation/` `build/` |
| RTC 验证 | Pion sender / receiver / signaling，自适应冗余控制 | `webrtc_demo/` |

当前的核心数据流可以概括为：

```text
representative_audio/ 或自定义 WAV
        │
        ├── offline_validation/
        │   ├── opus_sim         -> results/offline_runs/<RUN_ID>/
        │   └── opus_sender/receiver -> results/offline_runs/<RUN_ID>/
        │
        └── webrtc_demo/
            ├── signaling
            ├── sender -> receiver
            └── scripts/run_rtc_experiments.sh -> results/rtc_runs/<RUN_ID>/
```

## 目录结构

```text
.
├── CMakeLists.txt
├── offline_validation/          # C 侧离线/UDP 验证入口
│   ├── src/
│   ├── run_experiments.sh
│   ├── run_udp_test.sh
│   └── README.md
├── webrtc_demo/                 # Pion WebRTC 验证入口
│   ├── internal/
│   │   ├── adaptation/          # sender 侧动态冗余控制器
│   │   ├── opusx/               # 本地 libopus cgo 封装
│   │   ├── rtc/                 # Pion 封装与 receiver-side 丢包注入
│   │   ├── signal/              # 极简 HTTP 信令协议
│   │   └── wav/
│   ├── signaling/
│   ├── sender/
│   ├── receiver/
│   ├── scripts/
│   └── README.md
├── representative_audio/        # 标准基线音频与 manifest
├── tools/                       # 统一分析/报告/评估脚本
├── docs/                        # 技术分析与方案文档
├── results/                     # 运行产物、报告、缓存目录
├── opus-src/                    # Opus 1.6.1 源码（通常不纳入 git）
├── opus-install/                # 本地编译后的 Opus 运行时
└── weights_blob.bin             # DRED/DeepPLC 所需 DNN 权重
```

## 环境准备

### 1. 准备本地 Opus 1.6.1

系统自带的 `libopus` 往往是 1.4，不包含 DRED 相关 API，不能直接用于本项目。

如果仓库里还没有 `opus-install/`，可以按下面方式本地编译：

```bash
git clone --depth 1 --branch v1.6.1 https://github.com/xiph/opus.git opus-src
cd opus-src
./autogen.sh
./configure --prefix="$(pwd)/../opus-install" \
  --enable-dred \
  --enable-deep-plc \
  --enable-osce
make -j"$(nproc)"
make install
./dump_weights_blob weights_blob.bin
cp weights_blob.bin ..
cd ..
```

### 2. 构建 C 可执行文件

```bash
mkdir -p build
cd build
cmake ..
make -j"$(nproc)"
cd ..
```

构建完成后会生成：

- `build/opus_sim`
- `build/opus_sender`
- `build/opus_receiver`

### 3. 设置运行时库路径

离线工具和 RTC demo 都应优先链接项目内 `opus-install/lib`。

```bash
export LD_LIBRARY_PATH="$(pwd)/opus-install/lib:${LD_LIBRARY_PATH:-}"
export DYLD_LIBRARY_PATH="$(pwd)/opus-install/lib:${DYLD_LIBRARY_PATH:-}"
```

### 4. Go 环境

`webrtc_demo/` 需要 `Go >= 1.22`。运行前确认：

```bash
go version
```

## 快速开始

### 1. 刷新标准测试音频

```bash
python3 tools/prepare_representative_audio.py --force
```

仓库默认维护两类 30 秒基线输入：

- `representative_audio/news/news_30s_48k_mono.wav`
- `representative_audio/dialogue/dialogue_30s_48k_mono.wav`

其中 `dialogue` 还带有人工整理的参考文本：

- `representative_audio/dialogue/dialogue_reference.txt`

### 2. 离线仿真

手工运行一个 case：

```bash
./build/opus_sim \
  --loss 0.10 \
  --dred 3 \
  representative_audio/dialogue/dialogue_30s_48k_mono.wav \
  results/offline_runs/manual/dred3_10.wav \
  --csv results/offline_runs/manual/dred3_10.csv
```

跑完整离线实验矩阵：

```bash
bash offline_validation/run_experiments.sh
```

### 3. UDP 回环测试

```bash
bash offline_validation/run_udp_test.sh --loss 0.1 --dred 5
```

如果在 Linux 上想使用 `tc netem` 做真实网络损伤：

```bash
bash offline_validation/run_udp_test.sh --netem --loss 10 --delay 50 --jitter 20
```

### 4. RTC 冒烟测试

```bash
cd webrtc_demo
bash scripts/run_test.sh
```

这个脚本会自动：

- 编译 `signaling` / `sender` / `receiver`
- 校验它们实际链接到项目内 `libopus`
- 生成测试音频并建立 P2P 音频链路
- 输出接收端 WAV 和统计 JSON

### 5. RTC 实验矩阵

```bash
cd webrtc_demo

# 标准矩阵：2 音频 × 5 场景 × 4 固定策略
bash scripts/run_rtc_experiments.sh

# 快速回归：包含 adaptive_auto
EXPERIMENT_SUITE=quick bash scripts/run_rtc_experiments.sh

# 完整矩阵：额外加入 delay+jitter 场景
EXPERIMENT_SUITE=full bash scripts/run_rtc_experiments.sh
```

补充说明：

- `standard` / `full` 默认跑固定策略：`baseline`、`lbrr_only`、`dred_3`、`dred_5`
- `quick` 默认额外包含 `adaptive_auto`
- 可以通过 `STRATEGY_FILTER`、`SCENARIO_FILTER`、`EXTRA_DRED_VALUES` 缩小或扩展矩阵

## 结果目录约定

### 离线链路

- `results/offline_runs/<RUN_ID>/inputs/`
- `results/offline_runs/<RUN_ID>/outputs/`
- `results/offline_runs/<RUN_ID>/stats/`
- `results/offline_runs/<RUN_ID>/logs/`
- `results/offline_runs/<RUN_ID>/transcripts/`
- `results/offline_runs/<RUN_ID>/opus_experiment_summary.csv`
- `results/offline_runs/<RUN_ID>/opus_report.md`

便捷入口：

- `results/offline_report.md`
- `results/offline_latest`

### RTC 链路

- `results/rtc_runs/<RUN_ID>/inputs/`
- `results/rtc_runs/<RUN_ID>/outputs/`
- `results/rtc_runs/<RUN_ID>/stats/`
- `results/rtc_runs/<RUN_ID>/logs/`
- `results/rtc_runs/<RUN_ID>/transcripts/`
- `results/rtc_runs/<RUN_ID>/adaptation/`
- `results/rtc_runs/<RUN_ID>/rtc_experiment_summary.csv`
- `results/rtc_runs/<RUN_ID>/rtc_report.md`

便捷入口：

- `results/rtc_report.md`
- `results/rtc_latest`

### 其他产物

- `results/rtc_bin_cache/`：RTC 二进制缓存
- `results/rtc_go_cache/`：Go build cache
- `results/emotion_eval/`：情感评估相关实验产物

## 分析与报告工具

| 脚本 | 作用 |
| --- | --- |
| `tools/analyze.py` | 读取单个 CSV 或 WAV 对比，输出恢复情况、SNR / SegSNR、文本指标 |
| `tools/gen_rtc_report.py` | 从离线或 RTC 汇总 CSV 生成 Markdown 报告 |
| `tools/prepare_representative_audio.py` | 刷新仓库内标准 `news` / `dialogue` 测试集 |
| `tools/run_tess_emotion2vec.py` | 在 TESS 数据集上跑 emotion2vec 基线评估 |
| `tools/run_tess_offline_emotion_eval.py` | 先经 `opus_sim` 降质，再做情感识别评估 |

离线和 RTC 报告默认优先使用：

- `.venv_asr/`：ASR / WER / SER 报告环境
- `.venv_emotion/`：情感识别评估环境

如果 `.venv_asr/bin/python` 不存在，脚本会回退到系统 `python3`。

## 情绪识别实验

仓库里还有一条独立的情绪识别评估链路，用来回答两个问题：

- 干净语音上，`emotion2vec` 在标准情绪数据集上的基线效果如何
- 经过 `opus_sim` 降质后，弱网和恢复策略会对情绪分类造成多大退化

### 数据集

当前脚本默认使用 **TESS** 数据集，默认目录是：

- `.cache/datasets/tess/tess`

目录下应直接包含 `.wav` 文件，例如：

- `OAF_back_angry.wav`
- `OAF_back_happy.wav`

如果本地没有这套数据，按同样目录结构准备好即可：

```text
.cache/datasets/tess/tess/
├── MANIFEST.TXT
├── OAF_back_angry.wav
├── OAF_back_disgust.wav
└── ...
```

也可以通过 `--tess-dir /your/path/to/tess` 指向自定义位置。

### 模型与缓存

默认模型是：

- `iic/emotion2vec_plus_large`

脚本会自动把缓存放到：

- `.cache/modelscope`
- `.cache/huggingface`

如果本地已经存在：

- `.cache/modelscope/models/iic/emotion2vec_plus_large`

脚本会优先复用本地模型；否则首次运行会按 `funasr` / `modelscope` 的默认行为下载。

### 环境

建议使用仓库内的情感评估环境：

```bash
./.venv_emotion/bin/python -c "import funasr,librosa,soundfile,sklearn; print('emotion env ok')"
```

这条链路主要依赖：

- `funasr`
- `librosa`
- `soundfile`
- `scikit-learn`

### 1. 跑干净语音基线

```bash
./.venv_emotion/bin/python tools/run_tess_emotion2vec.py
```

常用参数：

- `--tess-dir`：指定数据集目录
- `--model`：指定模型名或本地模型目录
- `--batch-size`：批量推理大小
- `--limit`：只跑前 N 条样本
- `--run-name`：指定输出目录名

示例：

```bash
./.venv_emotion/bin/python tools/run_tess_emotion2vec.py \
  --limit 200 \
  --run-name emotion2vec_tess_smoke
```

输出目录默认在：

- `results/emotion_eval/<RUN_NAME>/`

主要产物：

- `predictions.csv`
- `summary.json`
- `report.md`

### 2. 跑 Opus 降质后的情绪识别退化实验

这个脚本会先调用 `build/opus_sim` 生成各类退化音频，再对每个场景做情绪分类。

运行前确保：

- `build/opus_sim` 已构建
- `opus-install/lib` 可被加载

```bash
export LD_LIBRARY_PATH="$(pwd)/opus-install/lib:${LD_LIBRARY_PATH:-}"
export DYLD_LIBRARY_PATH="$(pwd)/opus-install/lib:${DYLD_LIBRARY_PATH:-}"

./.venv_emotion/bin/python tools/run_tess_offline_emotion_eval.py
```

默认会覆盖这些场景：

- `clean`
- `uniform_10`
- `uniform_20`
- `ge_light`
- `ge_moderate`
- `ge_heavy`

常用参数：

- `--bitrate`
- `--framesize`
- `--complexity`
- `--signal`
- `--limit`
- `--run-name`

示例：

```bash
./.venv_emotion/bin/python tools/run_tess_offline_emotion_eval.py \
  --limit 200 \
  --bitrate 32000 \
  --framesize 20 \
  --signal voice \
  --run-name tess_offline_emotion_smoke
```

输出目录默认也在：

- `results/emotion_eval/<RUN_NAME>/`

主要产物：

- `generated/`：各场景生成的降质音频
- `prepared_inputs/`：重采样后的输入音频
- `emotion_predictions.csv`
- `scenario_summary.json`
- `report.md`

### 如何看结果

- `run_tess_emotion2vec.py` 主要看基线 `accuracy` 和各类别混淆矩阵
- `run_tess_offline_emotion_eval.py` 主要看各弱网场景相对 `clean` 的 `accuracy drop` 和 `macro F1`
- 如果只是想做快速检查，优先加 `--limit`

## 技术文档

- [LBRR 技术分析](docs/LBRR_技术分析.md)
- [DRED / PLC 深度分析](docs/DRED_PLC_深度分析.md)
- [RTC 动态冗余调控方案](docs/RTC_动态冗余调控方案.md)
- [DRED 源码实现分析](docs/Opus_DRED_源码实现分析.md)
- [FEC 新方案探索](docs/Opus_FEC_新方案探索报告.md)
- [端到端落地方案](docs/端到端落地方案.md)

## 当前实现重点

### `offline_validation/`

- `opus_sim`：单文件离线仿真，支持均匀丢包、Gilbert-Elliott、时延、抖动、LBRR、DRED、PLC、DTX
- `opus_sender` / `opus_receiver`：本地 UDP 回环测试，适合快速看实时收发路径
- `run_experiments.sh`：围绕 `representative_audio/manifest.txt` 批量跑统一矩阵并生成报告

### `webrtc_demo/`

- `internal/opusx`：直接绑定项目内 `libopus`，支持 DRED 相关 CTL / parse / decode
- `internal/rtc`：封装 Pion `PeerConnection`，并在 receiver 侧做 RTP 丢包注入
- `internal/adaptation`：基于 RR / REMB / TWCC / Stats 的 sender 自适应冗余控制
- `signaling`：内存态 HTTP 信令服务
- `sender` / `receiver`：P2P 音频收发、统计输出、实验矩阵接线

## 注意事项

- 必须优先使用项目内 `opus-install/lib`，否则很容易误链到系统旧版 `libopus`
- `weights_blob.bin` 对 DRED / DeepPLC 路径很重要，缺失时只能退化到无外部 DNN blob 的行为
- `representative_audio/manifest.txt` 是离线和 RTC 批量实验共用的输入清单
- `offline_validation/run_udp_test.sh --netem` 依赖 Linux、`tc` 和 root 权限
- `webrtc_demo/scripts/run_test.sh` 与 `run_rtc_experiments.sh` 会显式校验二进制链接到项目内 `libopus`
