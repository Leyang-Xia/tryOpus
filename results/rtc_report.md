# RTC 传输实验报告

> 自动生成于 2026-03-17 00:25:45

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
| `uniform_10` | 均匀 10% |
| `ge_moderate` | GE 中等突发 |

## 恢复策略对比

### 音乐 (SoundHelix)

| 丢包场景 | 保护策略 | LBRR 恢复 | DRED 恢复 | PLC 帧 | 解码错误 | 恢复率 |
|---------|---------|----------|----------|-------|---------|-------|
| 均匀 10% | 无保护 (PLC) | 0 | 0 | 23 | 0 | 0.00% |
| 均匀 10% | LBRR | 20 | 0 | 1 | 0 | 95.24% |
| 均匀 10% | DRED-3 | 0 | 23 | 0 | 0 | 100.00% |
| 均匀 10% | LBRR + DRED-3 | 22 | 1 | 0 | 0 | 100.00% |
| GE 中等突发 | 无保护 (PLC) | 0 | 0 | 22 | 0 | 0.00% |
| GE 中等突发 | LBRR | 10 | 0 | 12 | 0 | 45.45% |
| GE 中等突发 | DRED-3 | 0 | 22 | 0 | 0 | 100.00% |
| GE 中等突发 | LBRR + DRED-3 | 10 | 12 | 0 | 0 | 100.00% |

### 新闻播报 (BBC)

| 丢包场景 | 保护策略 | LBRR 恢复 | DRED 恢复 | PLC 帧 | 解码错误 | 恢复率 |
|---------|---------|----------|----------|-------|---------|-------|
| 均匀 10% | 无保护 (PLC) | 0 | 0 | 23 | 0 | 0.00% |
| 均匀 10% | LBRR | 21 | 0 | 2 | 0 | 91.30% |
| 均匀 10% | DRED-3 | 0 | 23 | 0 | 0 | 100.00% |
| 均匀 10% | LBRR + DRED-3 | 22 | 1 | 0 | 0 | 100.00% |
| GE 中等突发 | 无保护 (PLC) | 0 | 0 | 22 | 0 | 0.00% |
| GE 中等突发 | LBRR | 10 | 0 | 12 | 0 | 45.45% |
| GE 中等突发 | DRED-3 | 0 | 22 | 0 | 0 | 100.00% |
| GE 中等突发 | LBRR + DRED-3 | 10 | 12 | 0 | 0 | 100.00% |

### 对话场景 (餐厅)

| 丢包场景 | 保护策略 | LBRR 恢复 | DRED 恢复 | PLC 帧 | 解码错误 | 恢复率 |
|---------|---------|----------|----------|-------|---------|-------|
| 均匀 10% | 无保护 (PLC) | 0 | 0 | 23 | 0 | 0.00% |
| 均匀 10% | LBRR | 20 | 0 | 1 | 0 | 95.24% |
| 均匀 10% | DRED-3 | 0 | 21 | 0 | 0 | 100.00% |
| 均匀 10% | LBRR + DRED-3 | 20 | 1 | 0 | 0 | 100.00% |
| GE 中等突发 | 无保护 (PLC) | 0 | 0 | 22 | 0 | 0.00% |
| GE 中等突发 | LBRR | 10 | 0 | 12 | 0 | 45.45% |
| GE 中等突发 | DRED-3 | 0 | 22 | 0 | 0 | 100.00% |
| GE 中等突发 | LBRR + DRED-3 | 10 | 12 | 0 | 0 | 100.00% |

## 音质指标 (SNR / SegSNR)

| 音频类型 | 丢包场景 | 保护策略 | 全局 SNR (dB) | 分段 SegSNR (dB) |
|---------|---------|---------|:------------:|:---------------:|
| 音乐 (SoundHelix) | 均匀 10% | 无保护 (PLC) | -2.75 | -2.62 |
| 音乐 (SoundHelix) | 均匀 10% | LBRR | -2.33 | -2.26 |
| 音乐 (SoundHelix) | 均匀 10% | DRED-3 | -2.28 | -2.20 |
| 音乐 (SoundHelix) | 均匀 10% | LBRR + DRED-3 | -2.41 | -2.33 |
| 音乐 (SoundHelix) | GE 中等突发 | 无保护 (PLC) | -2.90 | -2.75 |
| 音乐 (SoundHelix) | GE 中等突发 | LBRR | -2.26 | -2.20 |
| 音乐 (SoundHelix) | GE 中等突发 | DRED-3 | -2.26 | -2.20 |
| 音乐 (SoundHelix) | GE 中等突发 | LBRR + DRED-3 | -2.41 | -2.32 |
| 新闻播报 (BBC) | 均匀 10% | 无保护 (PLC) | -2.77 | -2.56 |
| 新闻播报 (BBC) | 均匀 10% | LBRR | -2.72 | -2.59 |
| 新闻播报 (BBC) | 均匀 10% | DRED-3 | -2.74 | -2.55 |
| 新闻播报 (BBC) | 均匀 10% | LBRR + DRED-3 | -2.88 | -2.74 |
| 新闻播报 (BBC) | GE 中等突发 | 无保护 (PLC) | -2.76 | -2.57 |
| 新闻播报 (BBC) | GE 中等突发 | LBRR | -2.78 | -2.60 |
| 新闻播报 (BBC) | GE 中等突发 | DRED-3 | -2.71 | -2.56 |
| 新闻播报 (BBC) | GE 中等突发 | LBRR + DRED-3 | -2.91 | -2.79 |
| 对话场景 (餐厅) | 均匀 10% | 无保护 (PLC) | -2.79 | -2.67 |
| 对话场景 (餐厅) | 均匀 10% | LBRR | -2.60 | -2.45 |
| 对话场景 (餐厅) | 均匀 10% | DRED-3 | -2.63 | -2.48 |
| 对话场景 (餐厅) | 均匀 10% | LBRR + DRED-3 | -2.70 | -2.57 |
| 对话场景 (餐厅) | GE 中等突发 | 无保护 (PLC) | -2.55 | -2.64 |
| 对话场景 (餐厅) | GE 中等突发 | LBRR | -2.47 | -2.43 |
| 对话场景 (餐厅) | GE 中等突发 | DRED-3 | -2.37 | -2.41 |
| 对话场景 (餐厅) | GE 中等突发 | LBRR + DRED-3 | -2.65 | -2.59 |

## 音频工件对照

| 音频类型 | 丢包场景 | 保护策略 | 输入音频 | 输出音频 | 统计 JSON |
|---------|---------|---------|---------|---------|---------|
| 音乐 (SoundHelix) | 均匀 10% | 无保护 (PLC) | [music.wav](inputs/music.wav) | [baseline.wav](outputs/music/uniform_10/baseline.wav) | [baseline.json](stats/music/uniform_10/baseline.json) |
| 音乐 (SoundHelix) | 均匀 10% | LBRR | [music.wav](inputs/music.wav) | [lbrr_only.wav](outputs/music/uniform_10/lbrr_only.wav) | [lbrr_only.json](stats/music/uniform_10/lbrr_only.json) |
| 音乐 (SoundHelix) | 均匀 10% | DRED-3 | [music.wav](inputs/music.wav) | [dred_3.wav](outputs/music/uniform_10/dred_3.wav) | [dred_3.json](stats/music/uniform_10/dred_3.json) |
| 音乐 (SoundHelix) | 均匀 10% | LBRR + DRED-3 | [music.wav](inputs/music.wav) | [lbrr_dred_3.wav](outputs/music/uniform_10/lbrr_dred_3.wav) | [lbrr_dred_3.json](stats/music/uniform_10/lbrr_dred_3.json) |
| 音乐 (SoundHelix) | GE 中等突发 | 无保护 (PLC) | [music.wav](inputs/music.wav) | [baseline.wav](outputs/music/ge_moderate/baseline.wav) | [baseline.json](stats/music/ge_moderate/baseline.json) |
| 音乐 (SoundHelix) | GE 中等突发 | LBRR | [music.wav](inputs/music.wav) | [lbrr_only.wav](outputs/music/ge_moderate/lbrr_only.wav) | [lbrr_only.json](stats/music/ge_moderate/lbrr_only.json) |
| 音乐 (SoundHelix) | GE 中等突发 | DRED-3 | [music.wav](inputs/music.wav) | [dred_3.wav](outputs/music/ge_moderate/dred_3.wav) | [dred_3.json](stats/music/ge_moderate/dred_3.json) |
| 音乐 (SoundHelix) | GE 中等突发 | LBRR + DRED-3 | [music.wav](inputs/music.wav) | [lbrr_dred_3.wav](outputs/music/ge_moderate/lbrr_dred_3.wav) | [lbrr_dred_3.json](stats/music/ge_moderate/lbrr_dred_3.json) |
| 新闻播报 (BBC) | 均匀 10% | 无保护 (PLC) | [news.wav](inputs/news.wav) | [baseline.wav](outputs/news/uniform_10/baseline.wav) | [baseline.json](stats/news/uniform_10/baseline.json) |
| 新闻播报 (BBC) | 均匀 10% | LBRR | [news.wav](inputs/news.wav) | [lbrr_only.wav](outputs/news/uniform_10/lbrr_only.wav) | [lbrr_only.json](stats/news/uniform_10/lbrr_only.json) |
| 新闻播报 (BBC) | 均匀 10% | DRED-3 | [news.wav](inputs/news.wav) | [dred_3.wav](outputs/news/uniform_10/dred_3.wav) | [dred_3.json](stats/news/uniform_10/dred_3.json) |
| 新闻播报 (BBC) | 均匀 10% | LBRR + DRED-3 | [news.wav](inputs/news.wav) | [lbrr_dred_3.wav](outputs/news/uniform_10/lbrr_dred_3.wav) | [lbrr_dred_3.json](stats/news/uniform_10/lbrr_dred_3.json) |
| 新闻播报 (BBC) | GE 中等突发 | 无保护 (PLC) | [news.wav](inputs/news.wav) | [baseline.wav](outputs/news/ge_moderate/baseline.wav) | [baseline.json](stats/news/ge_moderate/baseline.json) |
| 新闻播报 (BBC) | GE 中等突发 | LBRR | [news.wav](inputs/news.wav) | [lbrr_only.wav](outputs/news/ge_moderate/lbrr_only.wav) | [lbrr_only.json](stats/news/ge_moderate/lbrr_only.json) |
| 新闻播报 (BBC) | GE 中等突发 | DRED-3 | [news.wav](inputs/news.wav) | [dred_3.wav](outputs/news/ge_moderate/dred_3.wav) | [dred_3.json](stats/news/ge_moderate/dred_3.json) |
| 新闻播报 (BBC) | GE 中等突发 | LBRR + DRED-3 | [news.wav](inputs/news.wav) | [lbrr_dred_3.wav](outputs/news/ge_moderate/lbrr_dred_3.wav) | [lbrr_dred_3.json](stats/news/ge_moderate/lbrr_dred_3.json) |
| 对话场景 (餐厅) | 均匀 10% | 无保护 (PLC) | [dialogue.wav](inputs/dialogue.wav) | [baseline.wav](outputs/dialogue/uniform_10/baseline.wav) | [baseline.json](stats/dialogue/uniform_10/baseline.json) |
| 对话场景 (餐厅) | 均匀 10% | LBRR | [dialogue.wav](inputs/dialogue.wav) | [lbrr_only.wav](outputs/dialogue/uniform_10/lbrr_only.wav) | [lbrr_only.json](stats/dialogue/uniform_10/lbrr_only.json) |
| 对话场景 (餐厅) | 均匀 10% | DRED-3 | [dialogue.wav](inputs/dialogue.wav) | [dred_3.wav](outputs/dialogue/uniform_10/dred_3.wav) | [dred_3.json](stats/dialogue/uniform_10/dred_3.json) |
| 对话场景 (餐厅) | 均匀 10% | LBRR + DRED-3 | [dialogue.wav](inputs/dialogue.wav) | [lbrr_dred_3.wav](outputs/dialogue/uniform_10/lbrr_dred_3.wav) | [lbrr_dred_3.json](stats/dialogue/uniform_10/lbrr_dred_3.json) |
| 对话场景 (餐厅) | GE 中等突发 | 无保护 (PLC) | [dialogue.wav](inputs/dialogue.wav) | [baseline.wav](outputs/dialogue/ge_moderate/baseline.wav) | [baseline.json](stats/dialogue/ge_moderate/baseline.json) |
| 对话场景 (餐厅) | GE 中等突发 | LBRR | [dialogue.wav](inputs/dialogue.wav) | [lbrr_only.wav](outputs/dialogue/ge_moderate/lbrr_only.wav) | [lbrr_only.json](stats/dialogue/ge_moderate/lbrr_only.json) |
| 对话场景 (餐厅) | GE 中等突发 | DRED-3 | [dialogue.wav](inputs/dialogue.wav) | [dred_3.wav](outputs/dialogue/ge_moderate/dred_3.wav) | [dred_3.json](stats/dialogue/ge_moderate/dred_3.json) |
| 对话场景 (餐厅) | GE 中等突发 | LBRR + DRED-3 | [dialogue.wav](inputs/dialogue.wav) | [lbrr_dred_3.wav](outputs/dialogue/ge_moderate/lbrr_dred_3.wav) | [lbrr_dred_3.json](stats/dialogue/ge_moderate/lbrr_dred_3.json) |
