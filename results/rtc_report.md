# RTC 传输实验报告

> 自动生成于 2026-03-19 10:44:37

## 实验配置

### 音频类型

| 标识 | 说明 |
|------|------|
| `news` | 新闻播报 (VOA) |
| `dialogue` | 真实英文对话 (ELLLO) |

### 丢包场景

| 标识 | 说明 |
|------|------|
| `uniform_10` | 均匀 10% |
| `ge_moderate` | GE 中等突发 |

### 文本指标说明

- 当前报告 ASR 后端/模型：`mlx / mlx-community/whisper-small.en-mlx`。
- WER / SER 使用同一个 ASR 后端分别转写干净输入音频与 RTC 输出音频。
- 干净输入音频的转写结果作为参考文本，因此这里衡量的是相对可懂度退化，而不是人工标注基准上的绝对识别率。
- SER 只有在音频中包含足够多、边界清晰的句子时才更有解释力，因此默认代表性语音片段已提升到 30 秒。

## 恢复策略对比

### 新闻播报 (VOA)

| 丢包场景 | 保护策略 | LBRR 恢复 | DRED 恢复 | PLC 帧 | 解码错误 | 恢复率 |
|---------|---------|----------|----------|-------|---------|-------|
| 均匀 10% | adaptive_auto | 140 | 0 | 14 | 0 | 90.91% |
| GE 中等突发 | adaptive_auto | 62 | 0 | 72 | 0 | 46.27% |

### 真实英文对话 (ELLLO)

| 丢包场景 | 保护策略 | LBRR 恢复 | DRED 恢复 | PLC 帧 | 解码错误 | 恢复率 |
|---------|---------|----------|----------|-------|---------|-------|
| 均匀 10% | adaptive_auto | 145 | 0 | 9 | 0 | 94.16% |
| GE 中等突发 | adaptive_auto | 75 | 0 | 93 | 0 | 44.64% |

## 文本指标 (WER / SER)

| 音频类型 | 丢包场景 | 保护策略 | WER | SER |
|---------|---------|---------|:---:|:---:|
| 新闻播报 (VOA) | 均匀 10% | adaptive_auto | 0.00% | 50.00% |
| 新闻播报 (VOA) | GE 中等突发 | adaptive_auto | 3.92% | 50.00% |
| 真实英文对话 (ELLLO) | 均匀 10% | adaptive_auto | 1.54% | 27.27% |
| 真实英文对话 (ELLLO) | GE 中等突发 | adaptive_auto | 1.54% | 9.09% |

## 音频工件对照

| 音频类型 | 丢包场景 | 保护策略 | 输入音频 | 输出音频 | 参考转写 | 输出转写 | 统计 JSON |
|---------|---------|---------|---------|---------|---------|---------|---------|
| 新闻播报 (VOA) | 均匀 10% | adaptive_auto | [news.wav](inputs/news.wav) | [adaptive_auto.wav](outputs/news/uniform_10/adaptive_auto.wav) | [reference.txt](transcripts/news/reference.txt) | [adaptive_auto.txt](transcripts/news/uniform_10/adaptive_auto.txt) | [adaptive_auto.json](stats/news/uniform_10/adaptive_auto.json) |
| 新闻播报 (VOA) | GE 中等突发 | adaptive_auto | [news.wav](inputs/news.wav) | [adaptive_auto.wav](outputs/news/ge_moderate/adaptive_auto.wav) | [reference.txt](transcripts/news/reference.txt) | [adaptive_auto.txt](transcripts/news/ge_moderate/adaptive_auto.txt) | [adaptive_auto.json](stats/news/ge_moderate/adaptive_auto.json) |
| 真实英文对话 (ELLLO) | 均匀 10% | adaptive_auto | [dialogue.wav](inputs/dialogue.wav) | [adaptive_auto.wav](outputs/dialogue/uniform_10/adaptive_auto.wav) | [reference.txt](transcripts/dialogue/reference.txt) | [adaptive_auto.txt](transcripts/dialogue/uniform_10/adaptive_auto.txt) | [adaptive_auto.json](stats/dialogue/uniform_10/adaptive_auto.json) |
| 真实英文对话 (ELLLO) | GE 中等突发 | adaptive_auto | [dialogue.wav](inputs/dialogue.wav) | [adaptive_auto.wav](outputs/dialogue/ge_moderate/adaptive_auto.wav) | [reference.txt](transcripts/dialogue/reference.txt) | [adaptive_auto.txt](transcripts/dialogue/ge_moderate/adaptive_auto.txt) | [adaptive_auto.json](stats/dialogue/ge_moderate/adaptive_auto.json) |
