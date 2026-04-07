# 离线验证

`offline_validation/` 是仓库里的非 RTC 验证入口，负责三类事情：

- `opus_sim`：单机离线仿真
- `opus_sender` / `opus_receiver`：本地 UDP 回环传输
- 批量实验脚本：统一跑基线音频、汇总统计、生成报告

## 目录结构

```text
offline_validation/
├── src/
│   ├── opus_sim.c
│   ├── sender.c
│   ├── receiver.c
│   ├── common.h
│   ├── netsim.h
│   └── wav_io.h
├── run_experiments.sh
├── run_udp_test.sh
└── README.md
```

## 与顶层目录的关系

- 输入资产来自顶层 `representative_audio/`
- C 可执行文件由顶层 `CMakeLists.txt` 构建到 `build/`
- 报告由顶层 `tools/gen_rtc_report.py` 统一生成
- 产物默认写到顶层 `results/offline_runs/<RUN_ID>/`

## 构建

在仓库根目录执行：

```bash
mkdir -p build
cd build
cmake ..
make -j"$(nproc)"
cd ..
```

运行前建议显式设置：

```bash
export LD_LIBRARY_PATH="$(pwd)/opus-install/lib:${LD_LIBRARY_PATH:-}"
export DYLD_LIBRARY_PATH="$(pwd)/opus-install/lib:${DYLD_LIBRARY_PATH:-}"
```

## 主要入口

### 1. `opus_sim`

用途：

- WAV -> Opus 编码 -> 网络仿真 -> 解码 -> 输出 WAV
- 验证 LBRR / DRED / PLC / DTX 在离线条件下的效果
- 输出逐帧 CSV 和总览统计

常见命令：

```bash
# 10% 均匀丢包，仅 PLC
./build/opus_sim \
  --loss 0.10 \
  --no-lbrr --no-dred \
  representative_audio/dialogue/dialogue_30s_48k_mono.wav \
  results/offline_runs/manual/plc_10.wav \
  --csv results/offline_runs/manual/plc_10.csv

# 10% 均匀丢包，DRED 3
./build/opus_sim \
  --loss 0.10 \
  --dred 3 \
  representative_audio/dialogue/dialogue_30s_48k_mono.wav \
  results/offline_runs/manual/dred3_10.wav \
  --csv results/offline_runs/manual/dred3_10.csv

# GE 突发丢包，DRED 5
./build/opus_sim \
  -ge -ge-p2b 0.05 -ge-b2g 0.30 -ge-bloss 0.80 \
  --dred 5 \
  representative_audio/dialogue/dialogue_30s_48k_mono.wav \
  results/offline_runs/manual/ge_dred5.wav
```

### 2. `run_experiments.sh`

用途：

- 固定读取 `representative_audio/manifest.txt`
- 默认覆盖 `news` 和 `dialogue` 两类标准样本
- 跑统一离线矩阵并生成 Markdown 报告

```bash
bash offline_validation/run_experiments.sh
```

常用环境变量：

- `RUN_ID`
- `RUN_DIR`
- `BITRATE`
- `FRAMESIZE`
- `COMPLEXITY`
- `SIGNAL_HINT`
- `ASR_PYTHON`
- `STT_MODEL`

### 3. `opus_sender` / `opus_receiver`

用途：

- 验证本地 UDP 实时传输路径
- 发送端可注入软件丢包、时延、抖动
- 接收端输出恢复后的 WAV

手工运行：

```bash
# 终端 1
./build/opus_receiver -p 5004 -t 15 results/offline_runs/manual/udp_out.wav

# 终端 2
./build/opus_sender -p 5004 -l 0.1 -dred 5 representative_audio/dialogue/dialogue_30s_48k_mono.wav
```

脚本化运行：

```bash
bash offline_validation/run_udp_test.sh --loss 0.1 --dred 5
```

如果在 Linux 上有 root 权限，也可以：

```bash
bash offline_validation/run_udp_test.sh --netem --loss 10 --delay 50 --jitter 20
```

## 输出约定

默认输出到：

```text
results/offline_runs/<RUN_ID>/
├── inputs/
├── outputs/
├── stats/
├── logs/
├── transcripts/
├── opus_experiment_summary.csv
└── opus_report.md
```

便捷入口：

- `results/offline_report.md`
- `results/offline_latest`

## 分析

### 单个 CSV / WAV

```bash
python3 tools/analyze.py --csv results/offline_runs/manual/dred3_10.csv

python3 tools/analyze.py \
  --ref representative_audio/dialogue/dialogue_30s_48k_mono.wav \
  --deg results/offline_runs/manual/dred3_10.wav
```

### 批量报告

`run_experiments.sh` 结束后会自动调用顶层 `tools/gen_rtc_report.py` 生成离线报告。

## 注意事项

- `opus_sim` / `opus_sender` / `opus_receiver` 都依赖项目内 `opus-install/lib`
- DRED 路径默认会尝试读取顶层 `weights_blob.bin`
- 批量实验依赖 `representative_audio/manifest.txt`
- `run_udp_test.sh --netem` 是 Linux 专用方案，macOS 上通常只能用软件仿真
