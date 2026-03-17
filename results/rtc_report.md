# RTC 传输实验报告

> 自动生成于 2026-03-17 14:59:43

## 实验配置

### 音频类型

| 标识 | 说明 |
|------|------|
| `music` | 音乐 (SoundHelix) |
| `news` | 新闻播报 (BBC) |
| `dialogue` | 对话场景 (餐厅) |

### 丢包场景

| 标识 | 说明 |
|------|------|
| `uniform_5` | 均匀 5% |
| `uniform_10` | 均匀 10% |
| `uniform_20` | 均匀 20% |
| `ge_moderate` | GE 中等突发 |
| `ge_heavy` | GE 重度突发 |

## 恢复策略对比

### 音乐 (SoundHelix)

| 丢包场景 | 保护策略 | LBRR 恢复 | DRED 恢复 | PLC 帧 | 解码错误 | 恢复率 |
|---------|---------|----------|----------|-------|---------|-------|
| 均匀 5% | 无保护 (PLC) | 0 | 0 | 16 | 0 | 0.00% |
| 均匀 5% | LBRR | 16 | 0 | 0 | 0 | 100.00% |
| 均匀 5% | DRED-3 | 0 | 16 | 0 | 0 | 100.00% |
| 均匀 5% | DRED-5 | 0 | 16 | 0 | 0 | 100.00% |
| 均匀 5% | LBRR + DRED-3 | 16 | 0 | 0 | 0 | 100.00% |
| 均匀 10% | 无保护 (PLC) | 0 | 0 | 43 | 0 | 0.00% |
| 均匀 10% | LBRR | 38 | 0 | 5 | 0 | 88.37% |
| 均匀 10% | DRED-3 | 0 | 43 | 0 | 0 | 100.00% |
| 均匀 10% | DRED-5 | 0 | 43 | 0 | 0 | 100.00% |
| 均匀 10% | LBRR + DRED-3 | 38 | 5 | 0 | 0 | 100.00% |
| 均匀 20% | 无保护 (PLC) | 0 | 0 | 108 | 0 | 0.00% |
| 均匀 20% | LBRR | 82 | 0 | 26 | 0 | 75.93% |
| 均匀 20% | DRED-3 | 0 | 108 | 0 | 0 | 100.00% |
| 均匀 20% | DRED-5 | 0 | 108 | 0 | 0 | 100.00% |
| 均匀 20% | LBRR + DRED-3 | 82 | 26 | 0 | 0 | 100.00% |
| GE 中等突发 | 无保护 (PLC) | 0 | 0 | 50 | 0 | 0.00% |
| GE 中等突发 | LBRR | 23 | 0 | 27 | 0 | 46.00% |
| GE 中等突发 | DRED-3 | 0 | 50 | 0 | 0 | 100.00% |
| GE 中等突发 | DRED-5 | 0 | 50 | 0 | 0 | 100.00% |
| GE 中等突发 | LBRR + DRED-3 | 23 | 27 | 0 | 0 | 100.00% |
| GE 重度突发 | 无保护 (PLC) | 0 | 0 | 161 | 0 | 0.00% |
| GE 重度突发 | LBRR | 39 | 0 | 122 | 0 | 24.22% |
| GE 重度突发 | DRED-3 | 0 | 161 | 0 | 0 | 100.00% |
| GE 重度突发 | DRED-5 | 0 | 161 | 0 | 0 | 100.00% |
| GE 重度突发 | LBRR + DRED-3 | 39 | 122 | 0 | 0 | 100.00% |

### 新闻播报 (BBC)

| 丢包场景 | 保护策略 | LBRR 恢复 | DRED 恢复 | PLC 帧 | 解码错误 | 恢复率 |
|---------|---------|----------|----------|-------|---------|-------|
| 均匀 5% | 无保护 (PLC) | 0 | 0 | 16 | 0 | 0.00% |
| 均匀 5% | LBRR | 16 | 0 | 0 | 0 | 100.00% |
| 均匀 5% | DRED-3 | 0 | 16 | 0 | 0 | 100.00% |
| 均匀 5% | DRED-5 | 0 | 16 | 0 | 0 | 100.00% |
| 均匀 5% | LBRR + DRED-3 | 16 | 0 | 0 | 0 | 100.00% |
| 均匀 10% | 无保护 (PLC) | 0 | 0 | 43 | 0 | 0.00% |
| 均匀 10% | LBRR | 38 | 0 | 5 | 0 | 88.37% |
| 均匀 10% | DRED-3 | 0 | 43 | 0 | 0 | 100.00% |
| 均匀 10% | DRED-5 | 0 | 43 | 0 | 0 | 100.00% |
| 均匀 10% | LBRR + DRED-3 | 38 | 5 | 0 | 0 | 100.00% |
| 均匀 20% | 无保护 (PLC) | 0 | 0 | 108 | 0 | 0.00% |
| 均匀 20% | LBRR | 82 | 0 | 26 | 0 | 75.93% |
| 均匀 20% | DRED-3 | 0 | 108 | 0 | 0 | 100.00% |
| 均匀 20% | DRED-5 | 0 | 108 | 0 | 0 | 100.00% |
| 均匀 20% | LBRR + DRED-3 | 82 | 26 | 0 | 0 | 100.00% |
| GE 中等突发 | 无保护 (PLC) | 0 | 0 | 50 | 0 | 0.00% |
| GE 中等突发 | LBRR | 23 | 0 | 27 | 0 | 46.00% |
| GE 中等突发 | DRED-3 | 0 | 50 | 0 | 0 | 100.00% |
| GE 中等突发 | DRED-5 | 0 | 50 | 0 | 0 | 100.00% |
| GE 中等突发 | LBRR + DRED-3 | 23 | 27 | 0 | 0 | 100.00% |
| GE 重度突发 | 无保护 (PLC) | 0 | 0 | 161 | 0 | 0.00% |
| GE 重度突发 | LBRR | 39 | 0 | 122 | 0 | 24.22% |
| GE 重度突发 | DRED-3 | 0 | 161 | 0 | 0 | 100.00% |
| GE 重度突发 | DRED-5 | 0 | 161 | 0 | 0 | 100.00% |
| GE 重度突发 | LBRR + DRED-3 | 39 | 122 | 0 | 0 | 100.00% |

### 对话场景 (餐厅)

| 丢包场景 | 保护策略 | LBRR 恢复 | DRED 恢复 | PLC 帧 | 解码错误 | 恢复率 |
|---------|---------|----------|----------|-------|---------|-------|
| 均匀 5% | 无保护 (PLC) | 0 | 0 | 16 | 0 | 0.00% |
| 均匀 5% | LBRR | 16 | 0 | 0 | 0 | 100.00% |
| 均匀 5% | DRED-3 | 0 | 16 | 0 | 0 | 100.00% |
| 均匀 5% | DRED-5 | 0 | 16 | 0 | 0 | 100.00% |
| 均匀 5% | LBRR + DRED-3 | 16 | 0 | 0 | 0 | 100.00% |
| 均匀 10% | 无保护 (PLC) | 0 | 0 | 43 | 0 | 0.00% |
| 均匀 10% | LBRR | 38 | 0 | 5 | 0 | 88.37% |
| 均匀 10% | DRED-3 | 0 | 43 | 0 | 0 | 100.00% |
| 均匀 10% | DRED-5 | 0 | 43 | 0 | 0 | 100.00% |
| 均匀 10% | LBRR + DRED-3 | 38 | 5 | 0 | 0 | 100.00% |
| 均匀 20% | 无保护 (PLC) | 0 | 0 | 108 | 0 | 0.00% |
| 均匀 20% | LBRR | 82 | 0 | 26 | 0 | 75.93% |
| 均匀 20% | DRED-3 | 0 | 108 | 0 | 0 | 100.00% |
| 均匀 20% | DRED-5 | 0 | 108 | 0 | 0 | 100.00% |
| 均匀 20% | LBRR + DRED-3 | 82 | 26 | 0 | 0 | 100.00% |
| GE 中等突发 | 无保护 (PLC) | 0 | 0 | 50 | 0 | 0.00% |
| GE 中等突发 | LBRR | 23 | 0 | 27 | 0 | 46.00% |
| GE 中等突发 | DRED-3 | 0 | 50 | 0 | 0 | 100.00% |
| GE 中等突发 | DRED-5 | 0 | 50 | 0 | 0 | 100.00% |
| GE 中等突发 | LBRR + DRED-3 | 23 | 27 | 0 | 0 | 100.00% |
| GE 重度突发 | 无保护 (PLC) | 0 | 0 | 161 | 0 | 0.00% |
| GE 重度突发 | LBRR | 39 | 0 | 122 | 0 | 24.22% |
| GE 重度突发 | DRED-3 | 0 | 161 | 0 | 0 | 100.00% |
| GE 重度突发 | DRED-5 | 0 | 161 | 0 | 0 | 100.00% |
| GE 重度突发 | LBRR + DRED-3 | 39 | 122 | 0 | 0 | 100.00% |

## 音质指标 (SNR / SegSNR)

| 音频类型 | 丢包场景 | 保护策略 | 全局 SNR (dB) | 分段 SegSNR (dB) |
|---------|---------|---------|:------------:|:---------------:|
| 音乐 (SoundHelix) | 均匀 5% | 无保护 (PLC) | -2.74 | -2.70 |
| 音乐 (SoundHelix) | 均匀 5% | LBRR | -2.13 | -2.12 |
| 音乐 (SoundHelix) | 均匀 5% | DRED-3 | -2.08 | -2.06 |
| 音乐 (SoundHelix) | 均匀 5% | DRED-5 | -2.08 | -2.06 |
| 音乐 (SoundHelix) | 均匀 5% | LBRR + DRED-3 | -2.20 | -2.19 |
| 音乐 (SoundHelix) | 均匀 10% | 无保护 (PLC) | -2.61 | -2.56 |
| 音乐 (SoundHelix) | 均匀 10% | LBRR | -2.12 | -2.10 |
| 音乐 (SoundHelix) | 均匀 10% | DRED-3 | -2.27 | -2.10 |
| 音乐 (SoundHelix) | 均匀 10% | DRED-5 | -2.27 | -2.10 |
| 音乐 (SoundHelix) | 均匀 10% | LBRR + DRED-3 | -2.21 | -2.19 |
| 音乐 (SoundHelix) | 均匀 20% | 无保护 (PLC) | -2.16 | -2.08 |
| 音乐 (SoundHelix) | 均匀 20% | LBRR | -2.08 | -2.07 |
| 音乐 (SoundHelix) | 均匀 20% | DRED-3 | -2.27 | -2.08 |
| 音乐 (SoundHelix) | 均匀 20% | DRED-5 | -2.27 | -2.08 |
| 音乐 (SoundHelix) | 均匀 20% | LBRR + DRED-3 | -2.20 | -2.18 |
| 音乐 (SoundHelix) | GE 中等突发 | 无保护 (PLC) | -2.61 | -2.55 |
| 音乐 (SoundHelix) | GE 中等突发 | LBRR | -2.09 | -2.06 |
| 音乐 (SoundHelix) | GE 中等突发 | DRED-3 | -2.11 | -2.08 |
| 音乐 (SoundHelix) | GE 中等突发 | DRED-5 | -2.11 | -2.08 |
| 音乐 (SoundHelix) | GE 中等突发 | LBRR + DRED-3 | -2.22 | -2.19 |
| 音乐 (SoundHelix) | GE 重度突发 | 无保护 (PLC) | -2.21 | -2.05 |
| 音乐 (SoundHelix) | GE 重度突发 | LBRR | -1.79 | -1.69 |
| 音乐 (SoundHelix) | GE 重度突发 | DRED-3 | -2.19 | -1.95 |
| 音乐 (SoundHelix) | GE 重度突发 | DRED-5 | -2.19 | -1.95 |
| 音乐 (SoundHelix) | GE 重度突发 | LBRR + DRED-3 | -2.07 | -1.97 |
| 新闻播报 (BBC) | 均匀 5% | 无保护 (PLC) | -3.68 | -2.91 |
| 新闻播报 (BBC) | 均匀 5% | LBRR | -3.66 | -2.91 |
| 新闻播报 (BBC) | 均匀 5% | DRED-3 | -3.58 | -2.85 |
| 新闻播报 (BBC) | 均匀 5% | DRED-5 | -3.58 | -2.85 |
| 新闻播报 (BBC) | 均匀 5% | LBRR + DRED-3 | -3.77 | -3.07 |
| 新闻播报 (BBC) | 均匀 10% | 无保护 (PLC) | -3.49 | -2.77 |
| 新闻播报 (BBC) | 均匀 10% | LBRR | -3.65 | -2.86 |
| 新闻播报 (BBC) | 均匀 10% | DRED-3 | -3.55 | -2.81 |
| 新闻播报 (BBC) | 均匀 10% | DRED-5 | -3.55 | -2.81 |
| 新闻播报 (BBC) | 均匀 10% | LBRR + DRED-3 | -3.77 | -3.02 |
| 新闻播报 (BBC) | 均匀 20% | 无保护 (PLC) | -2.82 | -2.39 |
| 新闻播报 (BBC) | 均匀 20% | LBRR | -3.38 | -2.70 |
| 新闻播报 (BBC) | 均匀 20% | DRED-3 | -3.14 | -2.61 |
| 新闻播报 (BBC) | 均匀 20% | DRED-5 | -3.14 | -2.61 |
| 新闻播报 (BBC) | 均匀 20% | LBRR + DRED-3 | -3.59 | -2.92 |
| 新闻播报 (BBC) | GE 中等突发 | 无保护 (PLC) | -3.39 | -2.70 |
| 新闻播报 (BBC) | GE 中等突发 | LBRR | -3.61 | -2.79 |
| 新闻播报 (BBC) | GE 中等突发 | DRED-3 | -3.39 | -2.75 |
| 新闻播报 (BBC) | GE 中等突发 | DRED-5 | -3.39 | -2.75 |
| 新闻播报 (BBC) | GE 中等突发 | LBRR + DRED-3 | -3.73 | -3.00 |
| 新闻播报 (BBC) | GE 重度突发 | 无保护 (PLC) | -2.79 | -2.15 |
| 新闻播报 (BBC) | GE 重度突发 | LBRR | -2.95 | -2.31 |
| 新闻播报 (BBC) | GE 重度突发 | DRED-3 | -2.92 | -2.33 |
| 新闻播报 (BBC) | GE 重度突发 | DRED-5 | -2.92 | -2.33 |
| 新闻播报 (BBC) | GE 重度突发 | LBRR + DRED-3 | -3.21 | -2.62 |
| 对话场景 (餐厅) | 均匀 5% | 无保护 (PLC) | -3.12 | -2.86 |
| 对话场景 (餐厅) | 均匀 5% | LBRR | -2.92 | -2.55 |
| 对话场景 (餐厅) | 均匀 5% | DRED-3 | -2.91 | -2.50 |
| 对话场景 (餐厅) | 均匀 5% | DRED-5 | -2.91 | -2.50 |
| 对话场景 (餐厅) | 均匀 5% | LBRR + DRED-3 | -3.02 | -2.66 |
| 对话场景 (餐厅) | 均匀 10% | 无保护 (PLC) | -3.01 | -2.78 |
| 对话场景 (餐厅) | 均匀 10% | LBRR | -2.90 | -2.50 |
| 对话场景 (餐厅) | 均匀 10% | DRED-3 | -2.85 | -2.48 |
| 对话场景 (餐厅) | 均匀 10% | DRED-5 | -2.85 | -2.48 |
| 对话场景 (餐厅) | 均匀 10% | LBRR + DRED-3 | -2.99 | -2.63 |
| 对话场景 (餐厅) | 均匀 20% | 无保护 (PLC) | -2.42 | -2.36 |
| 对话场景 (餐厅) | 均匀 20% | LBRR | -2.75 | -2.43 |
| 对话场景 (餐厅) | 均匀 20% | DRED-3 | -2.72 | -2.44 |
| 对话场景 (餐厅) | 均匀 20% | DRED-5 | -2.72 | -2.44 |
| 对话场景 (餐厅) | 均匀 20% | LBRR + DRED-3 | -2.90 | -2.60 |
| 对话场景 (餐厅) | GE 中等突发 | 无保护 (PLC) | -2.69 | -2.62 |
| 对话场景 (餐厅) | GE 中等突发 | LBRR | -2.71 | -2.44 |
| 对话场景 (餐厅) | GE 中等突发 | DRED-3 | -2.60 | -2.41 |
| 对话场景 (餐厅) | GE 中等突发 | DRED-5 | -2.60 | -2.41 |
| 对话场景 (餐厅) | GE 中等突发 | LBRR + DRED-3 | -2.89 | -2.61 |
| 对话场景 (餐厅) | GE 重度突发 | 无保护 (PLC) | -2.12 | -2.06 |
| 对话场景 (餐厅) | GE 重度突发 | LBRR | -2.11 | -2.02 |
| 对话场景 (餐厅) | GE 重度突发 | DRED-3 | -2.28 | -2.22 |
| 对话场景 (餐厅) | GE 重度突发 | DRED-5 | -2.28 | -2.22 |
| 对话场景 (餐厅) | GE 重度突发 | LBRR + DRED-3 | -2.47 | -2.37 |

## 音频工件对照

| 音频类型 | 丢包场景 | 保护策略 | 输入音频 | 输出音频 | 统计 JSON |
|---------|---------|---------|---------|---------|---------|
| 音乐 (SoundHelix) | 均匀 5% | 无保护 (PLC) | [music.wav](inputs/music.wav) | [baseline.wav](outputs/music/uniform_5/baseline.wav) | [baseline.json](stats/music/uniform_5/baseline.json) |
| 音乐 (SoundHelix) | 均匀 5% | LBRR | [music.wav](inputs/music.wav) | [lbrr_only.wav](outputs/music/uniform_5/lbrr_only.wav) | [lbrr_only.json](stats/music/uniform_5/lbrr_only.json) |
| 音乐 (SoundHelix) | 均匀 5% | DRED-3 | [music.wav](inputs/music.wav) | [dred_3.wav](outputs/music/uniform_5/dred_3.wav) | [dred_3.json](stats/music/uniform_5/dred_3.json) |
| 音乐 (SoundHelix) | 均匀 5% | DRED-5 | [music.wav](inputs/music.wav) | [dred_5.wav](outputs/music/uniform_5/dred_5.wav) | [dred_5.json](stats/music/uniform_5/dred_5.json) |
| 音乐 (SoundHelix) | 均匀 5% | LBRR + DRED-3 | [music.wav](inputs/music.wav) | [lbrr_dred_3.wav](outputs/music/uniform_5/lbrr_dred_3.wav) | [lbrr_dred_3.json](stats/music/uniform_5/lbrr_dred_3.json) |
| 音乐 (SoundHelix) | 均匀 10% | 无保护 (PLC) | [music.wav](inputs/music.wav) | [baseline.wav](outputs/music/uniform_10/baseline.wav) | [baseline.json](stats/music/uniform_10/baseline.json) |
| 音乐 (SoundHelix) | 均匀 10% | LBRR | [music.wav](inputs/music.wav) | [lbrr_only.wav](outputs/music/uniform_10/lbrr_only.wav) | [lbrr_only.json](stats/music/uniform_10/lbrr_only.json) |
| 音乐 (SoundHelix) | 均匀 10% | DRED-3 | [music.wav](inputs/music.wav) | [dred_3.wav](outputs/music/uniform_10/dred_3.wav) | [dred_3.json](stats/music/uniform_10/dred_3.json) |
| 音乐 (SoundHelix) | 均匀 10% | DRED-5 | [music.wav](inputs/music.wav) | [dred_5.wav](outputs/music/uniform_10/dred_5.wav) | [dred_5.json](stats/music/uniform_10/dred_5.json) |
| 音乐 (SoundHelix) | 均匀 10% | LBRR + DRED-3 | [music.wav](inputs/music.wav) | [lbrr_dred_3.wav](outputs/music/uniform_10/lbrr_dred_3.wav) | [lbrr_dred_3.json](stats/music/uniform_10/lbrr_dred_3.json) |
| 音乐 (SoundHelix) | 均匀 20% | 无保护 (PLC) | [music.wav](inputs/music.wav) | [baseline.wav](outputs/music/uniform_20/baseline.wav) | [baseline.json](stats/music/uniform_20/baseline.json) |
| 音乐 (SoundHelix) | 均匀 20% | LBRR | [music.wav](inputs/music.wav) | [lbrr_only.wav](outputs/music/uniform_20/lbrr_only.wav) | [lbrr_only.json](stats/music/uniform_20/lbrr_only.json) |
| 音乐 (SoundHelix) | 均匀 20% | DRED-3 | [music.wav](inputs/music.wav) | [dred_3.wav](outputs/music/uniform_20/dred_3.wav) | [dred_3.json](stats/music/uniform_20/dred_3.json) |
| 音乐 (SoundHelix) | 均匀 20% | DRED-5 | [music.wav](inputs/music.wav) | [dred_5.wav](outputs/music/uniform_20/dred_5.wav) | [dred_5.json](stats/music/uniform_20/dred_5.json) |
| 音乐 (SoundHelix) | 均匀 20% | LBRR + DRED-3 | [music.wav](inputs/music.wav) | [lbrr_dred_3.wav](outputs/music/uniform_20/lbrr_dred_3.wav) | [lbrr_dred_3.json](stats/music/uniform_20/lbrr_dred_3.json) |
| 音乐 (SoundHelix) | GE 中等突发 | 无保护 (PLC) | [music.wav](inputs/music.wav) | [baseline.wav](outputs/music/ge_moderate/baseline.wav) | [baseline.json](stats/music/ge_moderate/baseline.json) |
| 音乐 (SoundHelix) | GE 中等突发 | LBRR | [music.wav](inputs/music.wav) | [lbrr_only.wav](outputs/music/ge_moderate/lbrr_only.wav) | [lbrr_only.json](stats/music/ge_moderate/lbrr_only.json) |
| 音乐 (SoundHelix) | GE 中等突发 | DRED-3 | [music.wav](inputs/music.wav) | [dred_3.wav](outputs/music/ge_moderate/dred_3.wav) | [dred_3.json](stats/music/ge_moderate/dred_3.json) |
| 音乐 (SoundHelix) | GE 中等突发 | DRED-5 | [music.wav](inputs/music.wav) | [dred_5.wav](outputs/music/ge_moderate/dred_5.wav) | [dred_5.json](stats/music/ge_moderate/dred_5.json) |
| 音乐 (SoundHelix) | GE 中等突发 | LBRR + DRED-3 | [music.wav](inputs/music.wav) | [lbrr_dred_3.wav](outputs/music/ge_moderate/lbrr_dred_3.wav) | [lbrr_dred_3.json](stats/music/ge_moderate/lbrr_dred_3.json) |
| 音乐 (SoundHelix) | GE 重度突发 | 无保护 (PLC) | [music.wav](inputs/music.wav) | [baseline.wav](outputs/music/ge_heavy/baseline.wav) | [baseline.json](stats/music/ge_heavy/baseline.json) |
| 音乐 (SoundHelix) | GE 重度突发 | LBRR | [music.wav](inputs/music.wav) | [lbrr_only.wav](outputs/music/ge_heavy/lbrr_only.wav) | [lbrr_only.json](stats/music/ge_heavy/lbrr_only.json) |
| 音乐 (SoundHelix) | GE 重度突发 | DRED-3 | [music.wav](inputs/music.wav) | [dred_3.wav](outputs/music/ge_heavy/dred_3.wav) | [dred_3.json](stats/music/ge_heavy/dred_3.json) |
| 音乐 (SoundHelix) | GE 重度突发 | DRED-5 | [music.wav](inputs/music.wav) | [dred_5.wav](outputs/music/ge_heavy/dred_5.wav) | [dred_5.json](stats/music/ge_heavy/dred_5.json) |
| 音乐 (SoundHelix) | GE 重度突发 | LBRR + DRED-3 | [music.wav](inputs/music.wav) | [lbrr_dred_3.wav](outputs/music/ge_heavy/lbrr_dred_3.wav) | [lbrr_dred_3.json](stats/music/ge_heavy/lbrr_dred_3.json) |
| 新闻播报 (BBC) | 均匀 5% | 无保护 (PLC) | [news.wav](inputs/news.wav) | [baseline.wav](outputs/news/uniform_5/baseline.wav) | [baseline.json](stats/news/uniform_5/baseline.json) |
| 新闻播报 (BBC) | 均匀 5% | LBRR | [news.wav](inputs/news.wav) | [lbrr_only.wav](outputs/news/uniform_5/lbrr_only.wav) | [lbrr_only.json](stats/news/uniform_5/lbrr_only.json) |
| 新闻播报 (BBC) | 均匀 5% | DRED-3 | [news.wav](inputs/news.wav) | [dred_3.wav](outputs/news/uniform_5/dred_3.wav) | [dred_3.json](stats/news/uniform_5/dred_3.json) |
| 新闻播报 (BBC) | 均匀 5% | DRED-5 | [news.wav](inputs/news.wav) | [dred_5.wav](outputs/news/uniform_5/dred_5.wav) | [dred_5.json](stats/news/uniform_5/dred_5.json) |
| 新闻播报 (BBC) | 均匀 5% | LBRR + DRED-3 | [news.wav](inputs/news.wav) | [lbrr_dred_3.wav](outputs/news/uniform_5/lbrr_dred_3.wav) | [lbrr_dred_3.json](stats/news/uniform_5/lbrr_dred_3.json) |
| 新闻播报 (BBC) | 均匀 10% | 无保护 (PLC) | [news.wav](inputs/news.wav) | [baseline.wav](outputs/news/uniform_10/baseline.wav) | [baseline.json](stats/news/uniform_10/baseline.json) |
| 新闻播报 (BBC) | 均匀 10% | LBRR | [news.wav](inputs/news.wav) | [lbrr_only.wav](outputs/news/uniform_10/lbrr_only.wav) | [lbrr_only.json](stats/news/uniform_10/lbrr_only.json) |
| 新闻播报 (BBC) | 均匀 10% | DRED-3 | [news.wav](inputs/news.wav) | [dred_3.wav](outputs/news/uniform_10/dred_3.wav) | [dred_3.json](stats/news/uniform_10/dred_3.json) |
| 新闻播报 (BBC) | 均匀 10% | DRED-5 | [news.wav](inputs/news.wav) | [dred_5.wav](outputs/news/uniform_10/dred_5.wav) | [dred_5.json](stats/news/uniform_10/dred_5.json) |
| 新闻播报 (BBC) | 均匀 10% | LBRR + DRED-3 | [news.wav](inputs/news.wav) | [lbrr_dred_3.wav](outputs/news/uniform_10/lbrr_dred_3.wav) | [lbrr_dred_3.json](stats/news/uniform_10/lbrr_dred_3.json) |
| 新闻播报 (BBC) | 均匀 20% | 无保护 (PLC) | [news.wav](inputs/news.wav) | [baseline.wav](outputs/news/uniform_20/baseline.wav) | [baseline.json](stats/news/uniform_20/baseline.json) |
| 新闻播报 (BBC) | 均匀 20% | LBRR | [news.wav](inputs/news.wav) | [lbrr_only.wav](outputs/news/uniform_20/lbrr_only.wav) | [lbrr_only.json](stats/news/uniform_20/lbrr_only.json) |
| 新闻播报 (BBC) | 均匀 20% | DRED-3 | [news.wav](inputs/news.wav) | [dred_3.wav](outputs/news/uniform_20/dred_3.wav) | [dred_3.json](stats/news/uniform_20/dred_3.json) |
| 新闻播报 (BBC) | 均匀 20% | DRED-5 | [news.wav](inputs/news.wav) | [dred_5.wav](outputs/news/uniform_20/dred_5.wav) | [dred_5.json](stats/news/uniform_20/dred_5.json) |
| 新闻播报 (BBC) | 均匀 20% | LBRR + DRED-3 | [news.wav](inputs/news.wav) | [lbrr_dred_3.wav](outputs/news/uniform_20/lbrr_dred_3.wav) | [lbrr_dred_3.json](stats/news/uniform_20/lbrr_dred_3.json) |
| 新闻播报 (BBC) | GE 中等突发 | 无保护 (PLC) | [news.wav](inputs/news.wav) | [baseline.wav](outputs/news/ge_moderate/baseline.wav) | [baseline.json](stats/news/ge_moderate/baseline.json) |
| 新闻播报 (BBC) | GE 中等突发 | LBRR | [news.wav](inputs/news.wav) | [lbrr_only.wav](outputs/news/ge_moderate/lbrr_only.wav) | [lbrr_only.json](stats/news/ge_moderate/lbrr_only.json) |
| 新闻播报 (BBC) | GE 中等突发 | DRED-3 | [news.wav](inputs/news.wav) | [dred_3.wav](outputs/news/ge_moderate/dred_3.wav) | [dred_3.json](stats/news/ge_moderate/dred_3.json) |
| 新闻播报 (BBC) | GE 中等突发 | DRED-5 | [news.wav](inputs/news.wav) | [dred_5.wav](outputs/news/ge_moderate/dred_5.wav) | [dred_5.json](stats/news/ge_moderate/dred_5.json) |
| 新闻播报 (BBC) | GE 中等突发 | LBRR + DRED-3 | [news.wav](inputs/news.wav) | [lbrr_dred_3.wav](outputs/news/ge_moderate/lbrr_dred_3.wav) | [lbrr_dred_3.json](stats/news/ge_moderate/lbrr_dred_3.json) |
| 新闻播报 (BBC) | GE 重度突发 | 无保护 (PLC) | [news.wav](inputs/news.wav) | [baseline.wav](outputs/news/ge_heavy/baseline.wav) | [baseline.json](stats/news/ge_heavy/baseline.json) |
| 新闻播报 (BBC) | GE 重度突发 | LBRR | [news.wav](inputs/news.wav) | [lbrr_only.wav](outputs/news/ge_heavy/lbrr_only.wav) | [lbrr_only.json](stats/news/ge_heavy/lbrr_only.json) |
| 新闻播报 (BBC) | GE 重度突发 | DRED-3 | [news.wav](inputs/news.wav) | [dred_3.wav](outputs/news/ge_heavy/dred_3.wav) | [dred_3.json](stats/news/ge_heavy/dred_3.json) |
| 新闻播报 (BBC) | GE 重度突发 | DRED-5 | [news.wav](inputs/news.wav) | [dred_5.wav](outputs/news/ge_heavy/dred_5.wav) | [dred_5.json](stats/news/ge_heavy/dred_5.json) |
| 新闻播报 (BBC) | GE 重度突发 | LBRR + DRED-3 | [news.wav](inputs/news.wav) | [lbrr_dred_3.wav](outputs/news/ge_heavy/lbrr_dred_3.wav) | [lbrr_dred_3.json](stats/news/ge_heavy/lbrr_dred_3.json) |
| 对话场景 (餐厅) | 均匀 5% | 无保护 (PLC) | [dialogue.wav](inputs/dialogue.wav) | [baseline.wav](outputs/dialogue/uniform_5/baseline.wav) | [baseline.json](stats/dialogue/uniform_5/baseline.json) |
| 对话场景 (餐厅) | 均匀 5% | LBRR | [dialogue.wav](inputs/dialogue.wav) | [lbrr_only.wav](outputs/dialogue/uniform_5/lbrr_only.wav) | [lbrr_only.json](stats/dialogue/uniform_5/lbrr_only.json) |
| 对话场景 (餐厅) | 均匀 5% | DRED-3 | [dialogue.wav](inputs/dialogue.wav) | [dred_3.wav](outputs/dialogue/uniform_5/dred_3.wav) | [dred_3.json](stats/dialogue/uniform_5/dred_3.json) |
| 对话场景 (餐厅) | 均匀 5% | DRED-5 | [dialogue.wav](inputs/dialogue.wav) | [dred_5.wav](outputs/dialogue/uniform_5/dred_5.wav) | [dred_5.json](stats/dialogue/uniform_5/dred_5.json) |
| 对话场景 (餐厅) | 均匀 5% | LBRR + DRED-3 | [dialogue.wav](inputs/dialogue.wav) | [lbrr_dred_3.wav](outputs/dialogue/uniform_5/lbrr_dred_3.wav) | [lbrr_dred_3.json](stats/dialogue/uniform_5/lbrr_dred_3.json) |
| 对话场景 (餐厅) | 均匀 10% | 无保护 (PLC) | [dialogue.wav](inputs/dialogue.wav) | [baseline.wav](outputs/dialogue/uniform_10/baseline.wav) | [baseline.json](stats/dialogue/uniform_10/baseline.json) |
| 对话场景 (餐厅) | 均匀 10% | LBRR | [dialogue.wav](inputs/dialogue.wav) | [lbrr_only.wav](outputs/dialogue/uniform_10/lbrr_only.wav) | [lbrr_only.json](stats/dialogue/uniform_10/lbrr_only.json) |
| 对话场景 (餐厅) | 均匀 10% | DRED-3 | [dialogue.wav](inputs/dialogue.wav) | [dred_3.wav](outputs/dialogue/uniform_10/dred_3.wav) | [dred_3.json](stats/dialogue/uniform_10/dred_3.json) |
| 对话场景 (餐厅) | 均匀 10% | DRED-5 | [dialogue.wav](inputs/dialogue.wav) | [dred_5.wav](outputs/dialogue/uniform_10/dred_5.wav) | [dred_5.json](stats/dialogue/uniform_10/dred_5.json) |
| 对话场景 (餐厅) | 均匀 10% | LBRR + DRED-3 | [dialogue.wav](inputs/dialogue.wav) | [lbrr_dred_3.wav](outputs/dialogue/uniform_10/lbrr_dred_3.wav) | [lbrr_dred_3.json](stats/dialogue/uniform_10/lbrr_dred_3.json) |
| 对话场景 (餐厅) | 均匀 20% | 无保护 (PLC) | [dialogue.wav](inputs/dialogue.wav) | [baseline.wav](outputs/dialogue/uniform_20/baseline.wav) | [baseline.json](stats/dialogue/uniform_20/baseline.json) |
| 对话场景 (餐厅) | 均匀 20% | LBRR | [dialogue.wav](inputs/dialogue.wav) | [lbrr_only.wav](outputs/dialogue/uniform_20/lbrr_only.wav) | [lbrr_only.json](stats/dialogue/uniform_20/lbrr_only.json) |
| 对话场景 (餐厅) | 均匀 20% | DRED-3 | [dialogue.wav](inputs/dialogue.wav) | [dred_3.wav](outputs/dialogue/uniform_20/dred_3.wav) | [dred_3.json](stats/dialogue/uniform_20/dred_3.json) |
| 对话场景 (餐厅) | 均匀 20% | DRED-5 | [dialogue.wav](inputs/dialogue.wav) | [dred_5.wav](outputs/dialogue/uniform_20/dred_5.wav) | [dred_5.json](stats/dialogue/uniform_20/dred_5.json) |
| 对话场景 (餐厅) | 均匀 20% | LBRR + DRED-3 | [dialogue.wav](inputs/dialogue.wav) | [lbrr_dred_3.wav](outputs/dialogue/uniform_20/lbrr_dred_3.wav) | [lbrr_dred_3.json](stats/dialogue/uniform_20/lbrr_dred_3.json) |
| 对话场景 (餐厅) | GE 中等突发 | 无保护 (PLC) | [dialogue.wav](inputs/dialogue.wav) | [baseline.wav](outputs/dialogue/ge_moderate/baseline.wav) | [baseline.json](stats/dialogue/ge_moderate/baseline.json) |
| 对话场景 (餐厅) | GE 中等突发 | LBRR | [dialogue.wav](inputs/dialogue.wav) | [lbrr_only.wav](outputs/dialogue/ge_moderate/lbrr_only.wav) | [lbrr_only.json](stats/dialogue/ge_moderate/lbrr_only.json) |
| 对话场景 (餐厅) | GE 中等突发 | DRED-3 | [dialogue.wav](inputs/dialogue.wav) | [dred_3.wav](outputs/dialogue/ge_moderate/dred_3.wav) | [dred_3.json](stats/dialogue/ge_moderate/dred_3.json) |
| 对话场景 (餐厅) | GE 中等突发 | DRED-5 | [dialogue.wav](inputs/dialogue.wav) | [dred_5.wav](outputs/dialogue/ge_moderate/dred_5.wav) | [dred_5.json](stats/dialogue/ge_moderate/dred_5.json) |
| 对话场景 (餐厅) | GE 中等突发 | LBRR + DRED-3 | [dialogue.wav](inputs/dialogue.wav) | [lbrr_dred_3.wav](outputs/dialogue/ge_moderate/lbrr_dred_3.wav) | [lbrr_dred_3.json](stats/dialogue/ge_moderate/lbrr_dred_3.json) |
| 对话场景 (餐厅) | GE 重度突发 | 无保护 (PLC) | [dialogue.wav](inputs/dialogue.wav) | [baseline.wav](outputs/dialogue/ge_heavy/baseline.wav) | [baseline.json](stats/dialogue/ge_heavy/baseline.json) |
| 对话场景 (餐厅) | GE 重度突发 | LBRR | [dialogue.wav](inputs/dialogue.wav) | [lbrr_only.wav](outputs/dialogue/ge_heavy/lbrr_only.wav) | [lbrr_only.json](stats/dialogue/ge_heavy/lbrr_only.json) |
| 对话场景 (餐厅) | GE 重度突发 | DRED-3 | [dialogue.wav](inputs/dialogue.wav) | [dred_3.wav](outputs/dialogue/ge_heavy/dred_3.wav) | [dred_3.json](stats/dialogue/ge_heavy/dred_3.json) |
| 对话场景 (餐厅) | GE 重度突发 | DRED-5 | [dialogue.wav](inputs/dialogue.wav) | [dred_5.wav](outputs/dialogue/ge_heavy/dred_5.wav) | [dred_5.json](stats/dialogue/ge_heavy/dred_5.json) |
| 对话场景 (餐厅) | GE 重度突发 | LBRR + DRED-3 | [dialogue.wav](inputs/dialogue.wav) | [lbrr_dred_3.wav](outputs/dialogue/ge_heavy/lbrr_dred_3.wav) | [lbrr_dred_3.json](stats/dialogue/ge_heavy/lbrr_dred_3.json) |
