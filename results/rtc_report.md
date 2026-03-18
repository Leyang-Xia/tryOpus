# RTC 传输实验报告

> 自动生成于 2026-03-18 16:45:36

## 实验配置

### 音频类型

| 标识 | 说明 |
|------|------|
| `news` | 新闻播报 (VOA) |
| `dialogue` | 真实英文对话 (ELLLO) |

### 丢包场景

| 标识 | 说明 |
|------|------|
| `uniform_5` | 均匀 5% |
| `uniform_10` | 均匀 10% |
| `uniform_20` | 均匀 20% |
| `ge_moderate` | GE 中等突发 |
| `ge_heavy` | GE 重度突发 |

### 文本指标说明

- 当前报告 ASR 后端/模型：`mlx / mlx-community/whisper-small.en-mlx`。
- WER / SER 使用同一个 ASR 后端分别转写干净输入音频与 RTC 输出音频。
- 干净输入音频的转写结果作为参考文本，因此这里衡量的是相对可懂度退化，而不是人工标注基准上的绝对识别率。
- SER 只有在音频中包含足够多、边界清晰的句子时才更有解释力，因此默认代表性语音片段已提升到 30 秒。

## 恢复策略对比

### 新闻播报 (VOA)

| 丢包场景 | 保护策略 | LBRR 恢复 | DRED 恢复 | PLC 帧 | 解码错误 | 恢复率 |
|---------|---------|----------|----------|-------|---------|-------|
| 均匀 5% | 无保护 (PLC) | 0 | 0 | 74 | 0 | 0.00% |
| 均匀 5% | LBRR | 70 | 0 | 3 | 0 | 95.89% |
| 均匀 5% | DRED-3 | 0 | 73 | 0 | 0 | 100.00% |
| 均匀 5% | DRED-5 | 0 | 73 | 0 | 0 | 100.00% |
| 均匀 10% | 无保护 (PLC) | 0 | 0 | 144 | 0 | 0.00% |
| 均匀 10% | LBRR | 125 | 0 | 15 | 0 | 89.29% |
| 均匀 10% | DRED-3 | 0 | 141 | 0 | 0 | 100.00% |
| 均匀 10% | DRED-5 | 0 | 141 | 0 | 0 | 100.00% |
| 均匀 20% | 无保护 (PLC) | 0 | 0 | 308 | 0 | 0.00% |
| 均匀 20% | LBRR | 243 | 0 | 58 | 0 | 80.73% |
| 均匀 20% | DRED-3 | 0 | 301 | 0 | 0 | 100.00% |
| 均匀 20% | DRED-5 | 0 | 301 | 0 | 0 | 100.00% |
| GE 中等突发 | 无保护 (PLC) | 0 | 0 | 184 | 0 | 0.00% |
| GE 中等突发 | LBRR | 77 | 0 | 103 | 0 | 42.78% |
| GE 中等突发 | DRED-3 | 0 | 180 | 0 | 0 | 100.00% |
| GE 中等突发 | DRED-5 | 0 | 180 | 0 | 0 | 100.00% |
| GE 重度突发 | 无保护 (PLC) | 0 | 0 | 531 | 0 | 0.00% |
| GE 重度突发 | LBRR | 112 | 0 | 397 | 0 | 22.00% |
| GE 重度突发 | DRED-3 | 0 | 509 | 0 | 0 | 100.00% |
| GE 重度突发 | DRED-5 | 0 | 509 | 1 | 0 | 99.80% |

### 真实英文对话 (ELLLO)

| 丢包场景 | 保护策略 | LBRR 恢复 | DRED 恢复 | PLC 帧 | 解码错误 | 恢复率 |
|---------|---------|----------|----------|-------|---------|-------|
| 均匀 5% | 无保护 (PLC) | 0 | 0 | 73 | 0 | 0.00% |
| 均匀 5% | LBRR | 70 | 0 | 3 | 0 | 95.89% |
| 均匀 5% | DRED-3 | 0 | 68 | 5 | 0 | 93.15% |
| 均匀 5% | DRED-5 | 0 | 68 | 5 | 0 | 93.15% |
| 均匀 10% | 无保护 (PLC) | 0 | 0 | 136 | 0 | 0.00% |
| 均匀 10% | LBRR | 127 | 0 | 17 | 0 | 88.19% |
| 均匀 10% | DRED-3 | 0 | 132 | 9 | 0 | 93.62% |
| 均匀 10% | DRED-5 | 0 | 132 | 9 | 0 | 93.62% |
| 均匀 20% | 无保护 (PLC) | 0 | 0 | 304 | 0 | 0.00% |
| 均匀 20% | LBRR | 244 | 0 | 58 | 0 | 80.79% |
| 均匀 20% | DRED-3 | 0 | 285 | 16 | 0 | 94.68% |
| 均匀 20% | DRED-5 | 0 | 285 | 16 | 0 | 94.68% |
| GE 中等突发 | 无保护 (PLC) | 0 | 0 | 180 | 0 | 0.00% |
| GE 中等突发 | LBRR | 77 | 0 | 103 | 0 | 42.78% |
| GE 中等突发 | DRED-3 | 0 | 171 | 9 | 0 | 95.00% |
| GE 中等突发 | DRED-5 | 0 | 171 | 9 | 0 | 95.00% |
| GE 重度突发 | 无保护 (PLC) | 0 | 0 | 533 | 0 | 0.00% |
| GE 重度突发 | LBRR | 119 | 0 | 414 | 0 | 22.33% |
| GE 重度突发 | DRED-3 | 0 | 504 | 29 | 0 | 94.56% |
| GE 重度突发 | DRED-5 | 0 | 480 | 31 | 0 | 93.93% |

## 文本指标 (WER / SER)

| 音频类型 | 丢包场景 | 保护策略 | WER | SER |
|---------|---------|---------|:---:|:---:|
| 新闻播报 (VOA) | 均匀 5% | 无保护 (PLC) | 3.92% | 50.00% |
| 新闻播报 (VOA) | 均匀 5% | LBRR | 1.96% | 50.00% |
| 新闻播报 (VOA) | 均匀 5% | DRED-3 | 3.92% | 50.00% |
| 新闻播报 (VOA) | 均匀 5% | DRED-5 | 3.92% | 50.00% |
| 新闻播报 (VOA) | 均匀 10% | 无保护 (PLC) | 3.92% | 50.00% |
| 新闻播报 (VOA) | 均匀 10% | LBRR | 1.96% | 25.00% |
| 新闻播报 (VOA) | 均匀 10% | DRED-3 | 1.96% | 50.00% |
| 新闻播报 (VOA) | 均匀 10% | DRED-5 | 1.96% | 50.00% |
| 新闻播报 (VOA) | 均匀 20% | 无保护 (PLC) | 3.92% | 50.00% |
| 新闻播报 (VOA) | 均匀 20% | LBRR | 3.92% | 50.00% |
| 新闻播报 (VOA) | 均匀 20% | DRED-3 | 3.92% | 50.00% |
| 新闻播报 (VOA) | 均匀 20% | DRED-5 | 3.92% | 50.00% |
| 新闻播报 (VOA) | GE 中等突发 | 无保护 (PLC) | 5.88% | 50.00% |
| 新闻播报 (VOA) | GE 中等突发 | LBRR | 5.88% | 50.00% |
| 新闻播报 (VOA) | GE 中等突发 | DRED-3 | 3.92% | 50.00% |
| 新闻播报 (VOA) | GE 中等突发 | DRED-5 | 3.92% | 50.00% |
| 新闻播报 (VOA) | GE 重度突发 | 无保护 (PLC) | 21.57% | 100.00% |
| 新闻播报 (VOA) | GE 重度突发 | LBRR | 29.41% | 75.00% |
| 新闻播报 (VOA) | GE 重度突发 | DRED-3 | 11.76% | 100.00% |
| 新闻播报 (VOA) | GE 重度突发 | DRED-5 | 11.76% | 100.00% |
| 真实英文对话 (ELLLO) | 均匀 5% | 无保护 (PLC) | 6.15% | 90.91% |
| 真实英文对话 (ELLLO) | 均匀 5% | LBRR | 9.23% | 90.91% |
| 真实英文对话 (ELLLO) | 均匀 5% | DRED-3 | 6.15% | 90.91% |
| 真实英文对话 (ELLLO) | 均匀 5% | DRED-5 | 6.15% | 90.91% |
| 真实英文对话 (ELLLO) | 均匀 10% | 无保护 (PLC) | 6.15% | 90.91% |
| 真实英文对话 (ELLLO) | 均匀 10% | LBRR | 1.54% | 9.09% |
| 真实英文对话 (ELLLO) | 均匀 10% | DRED-3 | 1.54% | 27.27% |
| 真实英文对话 (ELLLO) | 均匀 10% | DRED-5 | 1.54% | 27.27% |
| 真实英文对话 (ELLLO) | 均匀 20% | 无保护 (PLC) | 3.08% | 27.27% |
| 真实英文对话 (ELLLO) | 均匀 20% | LBRR | 1.54% | 9.09% |
| 真实英文对话 (ELLLO) | 均匀 20% | DRED-3 | 3.08% | 36.36% |
| 真实英文对话 (ELLLO) | 均匀 20% | DRED-5 | 3.08% | 36.36% |
| 真实英文对话 (ELLLO) | GE 中等突发 | 无保护 (PLC) | 6.15% | 100.00% |
| 真实英文对话 (ELLLO) | GE 中等突发 | LBRR | 7.69% | 90.91% |
| 真实英文对话 (ELLLO) | GE 中等突发 | DRED-3 | 3.08% | 18.18% |
| 真实英文对话 (ELLLO) | GE 中等突发 | DRED-5 | 3.08% | 18.18% |
| 真实英文对话 (ELLLO) | GE 重度突发 | 无保护 (PLC) | 16.92% | 81.82% |
| 真实英文对话 (ELLLO) | GE 重度突发 | LBRR | 15.38% | 54.55% |
| 真实英文对话 (ELLLO) | GE 重度突发 | DRED-3 | 6.15% | 18.18% |
| 真实英文对话 (ELLLO) | GE 重度突发 | DRED-5 | 10.77% | 27.27% |

## 音频工件对照

| 音频类型 | 丢包场景 | 保护策略 | 输入音频 | 输出音频 | 参考转写 | 输出转写 | 统计 JSON |
|---------|---------|---------|---------|---------|---------|---------|---------|
| 新闻播报 (VOA) | 均匀 5% | 无保护 (PLC) | [news.wav](inputs/news.wav) | [baseline.wav](outputs/news/uniform_5/baseline.wav) | [reference.txt](transcripts/news/reference.txt) | [baseline.txt](transcripts/news/uniform_5/baseline.txt) | [baseline.json](stats/news/uniform_5/baseline.json) |
| 新闻播报 (VOA) | 均匀 5% | LBRR | [news.wav](inputs/news.wav) | [lbrr_only.wav](outputs/news/uniform_5/lbrr_only.wav) | [reference.txt](transcripts/news/reference.txt) | [lbrr_only.txt](transcripts/news/uniform_5/lbrr_only.txt) | [lbrr_only.json](stats/news/uniform_5/lbrr_only.json) |
| 新闻播报 (VOA) | 均匀 5% | DRED-3 | [news.wav](inputs/news.wav) | [dred_3.wav](outputs/news/uniform_5/dred_3.wav) | [reference.txt](transcripts/news/reference.txt) | [dred_3.txt](transcripts/news/uniform_5/dred_3.txt) | [dred_3.json](stats/news/uniform_5/dred_3.json) |
| 新闻播报 (VOA) | 均匀 5% | DRED-5 | [news.wav](inputs/news.wav) | [dred_5.wav](outputs/news/uniform_5/dred_5.wav) | [reference.txt](transcripts/news/reference.txt) | [dred_5.txt](transcripts/news/uniform_5/dred_5.txt) | [dred_5.json](stats/news/uniform_5/dred_5.json) |
| 新闻播报 (VOA) | 均匀 10% | 无保护 (PLC) | [news.wav](inputs/news.wav) | [baseline.wav](outputs/news/uniform_10/baseline.wav) | [reference.txt](transcripts/news/reference.txt) | [baseline.txt](transcripts/news/uniform_10/baseline.txt) | [baseline.json](stats/news/uniform_10/baseline.json) |
| 新闻播报 (VOA) | 均匀 10% | LBRR | [news.wav](inputs/news.wav) | [lbrr_only.wav](outputs/news/uniform_10/lbrr_only.wav) | [reference.txt](transcripts/news/reference.txt) | [lbrr_only.txt](transcripts/news/uniform_10/lbrr_only.txt) | [lbrr_only.json](stats/news/uniform_10/lbrr_only.json) |
| 新闻播报 (VOA) | 均匀 10% | DRED-3 | [news.wav](inputs/news.wav) | [dred_3.wav](outputs/news/uniform_10/dred_3.wav) | [reference.txt](transcripts/news/reference.txt) | [dred_3.txt](transcripts/news/uniform_10/dred_3.txt) | [dred_3.json](stats/news/uniform_10/dred_3.json) |
| 新闻播报 (VOA) | 均匀 10% | DRED-5 | [news.wav](inputs/news.wav) | [dred_5.wav](outputs/news/uniform_10/dred_5.wav) | [reference.txt](transcripts/news/reference.txt) | [dred_5.txt](transcripts/news/uniform_10/dred_5.txt) | [dred_5.json](stats/news/uniform_10/dred_5.json) |
| 新闻播报 (VOA) | 均匀 20% | 无保护 (PLC) | [news.wav](inputs/news.wav) | [baseline.wav](outputs/news/uniform_20/baseline.wav) | [reference.txt](transcripts/news/reference.txt) | [baseline.txt](transcripts/news/uniform_20/baseline.txt) | [baseline.json](stats/news/uniform_20/baseline.json) |
| 新闻播报 (VOA) | 均匀 20% | LBRR | [news.wav](inputs/news.wav) | [lbrr_only.wav](outputs/news/uniform_20/lbrr_only.wav) | [reference.txt](transcripts/news/reference.txt) | [lbrr_only.txt](transcripts/news/uniform_20/lbrr_only.txt) | [lbrr_only.json](stats/news/uniform_20/lbrr_only.json) |
| 新闻播报 (VOA) | 均匀 20% | DRED-3 | [news.wav](inputs/news.wav) | [dred_3.wav](outputs/news/uniform_20/dred_3.wav) | [reference.txt](transcripts/news/reference.txt) | [dred_3.txt](transcripts/news/uniform_20/dred_3.txt) | [dred_3.json](stats/news/uniform_20/dred_3.json) |
| 新闻播报 (VOA) | 均匀 20% | DRED-5 | [news.wav](inputs/news.wav) | [dred_5.wav](outputs/news/uniform_20/dred_5.wav) | [reference.txt](transcripts/news/reference.txt) | [dred_5.txt](transcripts/news/uniform_20/dred_5.txt) | [dred_5.json](stats/news/uniform_20/dred_5.json) |
| 新闻播报 (VOA) | GE 中等突发 | 无保护 (PLC) | [news.wav](inputs/news.wav) | [baseline.wav](outputs/news/ge_moderate/baseline.wav) | [reference.txt](transcripts/news/reference.txt) | [baseline.txt](transcripts/news/ge_moderate/baseline.txt) | [baseline.json](stats/news/ge_moderate/baseline.json) |
| 新闻播报 (VOA) | GE 中等突发 | LBRR | [news.wav](inputs/news.wav) | [lbrr_only.wav](outputs/news/ge_moderate/lbrr_only.wav) | [reference.txt](transcripts/news/reference.txt) | [lbrr_only.txt](transcripts/news/ge_moderate/lbrr_only.txt) | [lbrr_only.json](stats/news/ge_moderate/lbrr_only.json) |
| 新闻播报 (VOA) | GE 中等突发 | DRED-3 | [news.wav](inputs/news.wav) | [dred_3.wav](outputs/news/ge_moderate/dred_3.wav) | [reference.txt](transcripts/news/reference.txt) | [dred_3.txt](transcripts/news/ge_moderate/dred_3.txt) | [dred_3.json](stats/news/ge_moderate/dred_3.json) |
| 新闻播报 (VOA) | GE 中等突发 | DRED-5 | [news.wav](inputs/news.wav) | [dred_5.wav](outputs/news/ge_moderate/dred_5.wav) | [reference.txt](transcripts/news/reference.txt) | [dred_5.txt](transcripts/news/ge_moderate/dred_5.txt) | [dred_5.json](stats/news/ge_moderate/dred_5.json) |
| 新闻播报 (VOA) | GE 重度突发 | 无保护 (PLC) | [news.wav](inputs/news.wav) | [baseline.wav](outputs/news/ge_heavy/baseline.wav) | [reference.txt](transcripts/news/reference.txt) | [baseline.txt](transcripts/news/ge_heavy/baseline.txt) | [baseline.json](stats/news/ge_heavy/baseline.json) |
| 新闻播报 (VOA) | GE 重度突发 | LBRR | [news.wav](inputs/news.wav) | [lbrr_only.wav](outputs/news/ge_heavy/lbrr_only.wav) | [reference.txt](transcripts/news/reference.txt) | [lbrr_only.txt](transcripts/news/ge_heavy/lbrr_only.txt) | [lbrr_only.json](stats/news/ge_heavy/lbrr_only.json) |
| 新闻播报 (VOA) | GE 重度突发 | DRED-3 | [news.wav](inputs/news.wav) | [dred_3.wav](outputs/news/ge_heavy/dred_3.wav) | [reference.txt](transcripts/news/reference.txt) | [dred_3.txt](transcripts/news/ge_heavy/dred_3.txt) | [dred_3.json](stats/news/ge_heavy/dred_3.json) |
| 新闻播报 (VOA) | GE 重度突发 | DRED-5 | [news.wav](inputs/news.wav) | [dred_5.wav](outputs/news/ge_heavy/dred_5.wav) | [reference.txt](transcripts/news/reference.txt) | [dred_5.txt](transcripts/news/ge_heavy/dred_5.txt) | [dred_5.json](stats/news/ge_heavy/dred_5.json) |
| 真实英文对话 (ELLLO) | 均匀 5% | 无保护 (PLC) | [dialogue.wav](inputs/dialogue.wav) | [baseline.wav](outputs/dialogue/uniform_5/baseline.wav) | [reference.txt](transcripts/dialogue/reference.txt) | [baseline.txt](transcripts/dialogue/uniform_5/baseline.txt) | [baseline.json](stats/dialogue/uniform_5/baseline.json) |
| 真实英文对话 (ELLLO) | 均匀 5% | LBRR | [dialogue.wav](inputs/dialogue.wav) | [lbrr_only.wav](outputs/dialogue/uniform_5/lbrr_only.wav) | [reference.txt](transcripts/dialogue/reference.txt) | [lbrr_only.txt](transcripts/dialogue/uniform_5/lbrr_only.txt) | [lbrr_only.json](stats/dialogue/uniform_5/lbrr_only.json) |
| 真实英文对话 (ELLLO) | 均匀 5% | DRED-3 | [dialogue.wav](inputs/dialogue.wav) | [dred_3.wav](outputs/dialogue/uniform_5/dred_3.wav) | [reference.txt](transcripts/dialogue/reference.txt) | [dred_3.txt](transcripts/dialogue/uniform_5/dred_3.txt) | [dred_3.json](stats/dialogue/uniform_5/dred_3.json) |
| 真实英文对话 (ELLLO) | 均匀 5% | DRED-5 | [dialogue.wav](inputs/dialogue.wav) | [dred_5.wav](outputs/dialogue/uniform_5/dred_5.wav) | [reference.txt](transcripts/dialogue/reference.txt) | [dred_5.txt](transcripts/dialogue/uniform_5/dred_5.txt) | [dred_5.json](stats/dialogue/uniform_5/dred_5.json) |
| 真实英文对话 (ELLLO) | 均匀 10% | 无保护 (PLC) | [dialogue.wav](inputs/dialogue.wav) | [baseline.wav](outputs/dialogue/uniform_10/baseline.wav) | [reference.txt](transcripts/dialogue/reference.txt) | [baseline.txt](transcripts/dialogue/uniform_10/baseline.txt) | [baseline.json](stats/dialogue/uniform_10/baseline.json) |
| 真实英文对话 (ELLLO) | 均匀 10% | LBRR | [dialogue.wav](inputs/dialogue.wav) | [lbrr_only.wav](outputs/dialogue/uniform_10/lbrr_only.wav) | [reference.txt](transcripts/dialogue/reference.txt) | [lbrr_only.txt](transcripts/dialogue/uniform_10/lbrr_only.txt) | [lbrr_only.json](stats/dialogue/uniform_10/lbrr_only.json) |
| 真实英文对话 (ELLLO) | 均匀 10% | DRED-3 | [dialogue.wav](inputs/dialogue.wav) | [dred_3.wav](outputs/dialogue/uniform_10/dred_3.wav) | [reference.txt](transcripts/dialogue/reference.txt) | [dred_3.txt](transcripts/dialogue/uniform_10/dred_3.txt) | [dred_3.json](stats/dialogue/uniform_10/dred_3.json) |
| 真实英文对话 (ELLLO) | 均匀 10% | DRED-5 | [dialogue.wav](inputs/dialogue.wav) | [dred_5.wav](outputs/dialogue/uniform_10/dred_5.wav) | [reference.txt](transcripts/dialogue/reference.txt) | [dred_5.txt](transcripts/dialogue/uniform_10/dred_5.txt) | [dred_5.json](stats/dialogue/uniform_10/dred_5.json) |
| 真实英文对话 (ELLLO) | 均匀 20% | 无保护 (PLC) | [dialogue.wav](inputs/dialogue.wav) | [baseline.wav](outputs/dialogue/uniform_20/baseline.wav) | [reference.txt](transcripts/dialogue/reference.txt) | [baseline.txt](transcripts/dialogue/uniform_20/baseline.txt) | [baseline.json](stats/dialogue/uniform_20/baseline.json) |
| 真实英文对话 (ELLLO) | 均匀 20% | LBRR | [dialogue.wav](inputs/dialogue.wav) | [lbrr_only.wav](outputs/dialogue/uniform_20/lbrr_only.wav) | [reference.txt](transcripts/dialogue/reference.txt) | [lbrr_only.txt](transcripts/dialogue/uniform_20/lbrr_only.txt) | [lbrr_only.json](stats/dialogue/uniform_20/lbrr_only.json) |
| 真实英文对话 (ELLLO) | 均匀 20% | DRED-3 | [dialogue.wav](inputs/dialogue.wav) | [dred_3.wav](outputs/dialogue/uniform_20/dred_3.wav) | [reference.txt](transcripts/dialogue/reference.txt) | [dred_3.txt](transcripts/dialogue/uniform_20/dred_3.txt) | [dred_3.json](stats/dialogue/uniform_20/dred_3.json) |
| 真实英文对话 (ELLLO) | 均匀 20% | DRED-5 | [dialogue.wav](inputs/dialogue.wav) | [dred_5.wav](outputs/dialogue/uniform_20/dred_5.wav) | [reference.txt](transcripts/dialogue/reference.txt) | [dred_5.txt](transcripts/dialogue/uniform_20/dred_5.txt) | [dred_5.json](stats/dialogue/uniform_20/dred_5.json) |
| 真实英文对话 (ELLLO) | GE 中等突发 | 无保护 (PLC) | [dialogue.wav](inputs/dialogue.wav) | [baseline.wav](outputs/dialogue/ge_moderate/baseline.wav) | [reference.txt](transcripts/dialogue/reference.txt) | [baseline.txt](transcripts/dialogue/ge_moderate/baseline.txt) | [baseline.json](stats/dialogue/ge_moderate/baseline.json) |
| 真实英文对话 (ELLLO) | GE 中等突发 | LBRR | [dialogue.wav](inputs/dialogue.wav) | [lbrr_only.wav](outputs/dialogue/ge_moderate/lbrr_only.wav) | [reference.txt](transcripts/dialogue/reference.txt) | [lbrr_only.txt](transcripts/dialogue/ge_moderate/lbrr_only.txt) | [lbrr_only.json](stats/dialogue/ge_moderate/lbrr_only.json) |
| 真实英文对话 (ELLLO) | GE 中等突发 | DRED-3 | [dialogue.wav](inputs/dialogue.wav) | [dred_3.wav](outputs/dialogue/ge_moderate/dred_3.wav) | [reference.txt](transcripts/dialogue/reference.txt) | [dred_3.txt](transcripts/dialogue/ge_moderate/dred_3.txt) | [dred_3.json](stats/dialogue/ge_moderate/dred_3.json) |
| 真实英文对话 (ELLLO) | GE 中等突发 | DRED-5 | [dialogue.wav](inputs/dialogue.wav) | [dred_5.wav](outputs/dialogue/ge_moderate/dred_5.wav) | [reference.txt](transcripts/dialogue/reference.txt) | [dred_5.txt](transcripts/dialogue/ge_moderate/dred_5.txt) | [dred_5.json](stats/dialogue/ge_moderate/dred_5.json) |
| 真实英文对话 (ELLLO) | GE 重度突发 | 无保护 (PLC) | [dialogue.wav](inputs/dialogue.wav) | [baseline.wav](outputs/dialogue/ge_heavy/baseline.wav) | [reference.txt](transcripts/dialogue/reference.txt) | [baseline.txt](transcripts/dialogue/ge_heavy/baseline.txt) | [baseline.json](stats/dialogue/ge_heavy/baseline.json) |
| 真实英文对话 (ELLLO) | GE 重度突发 | LBRR | [dialogue.wav](inputs/dialogue.wav) | [lbrr_only.wav](outputs/dialogue/ge_heavy/lbrr_only.wav) | [reference.txt](transcripts/dialogue/reference.txt) | [lbrr_only.txt](transcripts/dialogue/ge_heavy/lbrr_only.txt) | [lbrr_only.json](stats/dialogue/ge_heavy/lbrr_only.json) |
| 真实英文对话 (ELLLO) | GE 重度突发 | DRED-3 | [dialogue.wav](inputs/dialogue.wav) | [dred_3.wav](outputs/dialogue/ge_heavy/dred_3.wav) | [reference.txt](transcripts/dialogue/reference.txt) | [dred_3.txt](transcripts/dialogue/ge_heavy/dred_3.txt) | [dred_3.json](stats/dialogue/ge_heavy/dred_3.json) |
| 真实英文对话 (ELLLO) | GE 重度突发 | DRED-5 | [dialogue.wav](inputs/dialogue.wav) | [dred_5.wav](outputs/dialogue/ge_heavy/dred_5.wav) | [reference.txt](transcripts/dialogue/reference.txt) | [dred_5.txt](transcripts/dialogue/ge_heavy/dred_5.txt) | [dred_5.json](stats/dialogue/ge_heavy/dred_5.json) |
