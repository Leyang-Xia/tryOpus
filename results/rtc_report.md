# RTC 传输实验报告

> 自动生成于 2026-03-24 22:00:06

## 实验配置

### 音频类型

| 标识 | 说明 |
|------|------|
| `news` | 新闻播报 (VOA) |
| `dialogue` | 真实英文对话 (ELLLO) |

### 丢包场景

| 标识 | 说明 |
|------|------|
| `ge_heavy` | GE 重度突发 |

### 文本指标说明

- 当前报告 ASR 后端/模型：`mlx / mlx-community/whisper-small.en-mlx`。
- `WER` 是主评估指标。
- WER 使用同一个 ASR 后端分别转写干净输入音频与 RTC 输出音频。
- 干净输入音频的转写结果作为参考文本，因此这里衡量的是相对可懂度退化，而不是人工标注基准上的绝对识别率。
- `ASR unstable` 表示输出文本出现明显插入膨胀或重复 n-gram 循环，这类 case 的 WER 仍保留，但应结合原始 transcript 复核。
- `发送载荷字节` 基于 sender `sender_stats.json`，表示实验期间 sender 实际写出的 Opus payload 总字节数。
- `平均载荷/包` 与 `有效载荷码率(kbps)` 也都来自 sender 侧统计。

## 恢复策略对比

### 新闻播报 (VOA)

| 丢包场景 | 保护策略 | LBRR 恢复 | DRED 恢复 | PLC 帧 | 解码错误 | 恢复率 |
|---------|---------|----------|----------|-------|---------|-------|
| GE 重度突发 | DRED-3 | 0 | 521 | 0 | 0 | 99.62% |
| GE 重度突发 | DRED-5 | 0 | 581 | 0 | 0 | 99.66% |
| GE 重度突发 | dred_10 | 0 | 582 | 0 | 0 | 100.00% |
| GE 重度突发 | dred_20 | 0 | 583 | 0 | 0 | 99.32% |
| GE 重度突发 | dred_50 | 0 | 513 | 0 | 0 | 99.81% |
| GE 重度突发 | dred_100 | 0 | 565 | 0 | 0 | 98.95% |

### 真实英文对话 (ELLLO)

| 丢包场景 | 保护策略 | LBRR 恢复 | DRED 恢复 | PLC 帧 | 解码错误 | 恢复率 |
|---------|---------|----------|----------|-------|---------|-------|
| GE 重度突发 | DRED-3 | 0 | 478 | 48 | 0 | 90.87% |
| GE 重度突发 | DRED-5 | 0 | 609 | 21 | 0 | 94.86% |
| GE 重度突发 | dred_10 | 0 | 452 | 19 | 0 | 95.97% |
| GE 重度突发 | dred_20 | 0 | 504 | 30 | 0 | 94.38% |
| GE 重度突发 | dred_50 | 0 | 442 | 24 | 0 | 94.85% |
| GE 重度突发 | dred_100 | 0 | 501 | 20 | 0 | 96.16% |

## 带宽 / WER 综合对比

| 音频类型 | 丢包场景 | 保护策略 | 发送载荷字节 | 平均载荷/包(bytes) | 有效载荷码率(kbps) | WER | 恢复率 |
|---------|---------|---------|-------------:|-------------------:|-------------------:|:---:|-------|
| 新闻播报 (VOA) | GE 重度突发 | DRED-3 | 130039 | 86.7 | 34.68 | 9.80% | 99.62% |
| 新闻播报 (VOA) | GE 重度突发 | DRED-5 | 130039 | 86.7 | 34.68 | 5.88% | 99.66% |
| 新闻播报 (VOA) | GE 重度突发 | dred_10 | 129003 | 86.0 | 34.40 | 11.76% | 100.00% |
| 新闻播报 (VOA) | GE 重度突发 | dred_20 | 122413 | 81.6 | 32.64 | 7.84% | 99.32% |
| 新闻播报 (VOA) | GE 重度突发 | dred_50 | 122413 | 81.6 | 32.64 | 3.92% | 99.81% |
| 新闻播报 (VOA) | GE 重度突发 | dred_100 | 122413 | 81.6 | 32.64 | 7.84% | 98.95% |
| 真实英文对话 (ELLLO) | GE 重度突发 | DRED-3 | 111146 | 74.1 | 29.64 | 7.69% | 90.87% |
| 真实英文对话 (ELLLO) | GE 重度突发 | DRED-5 | 111146 | 74.1 | 29.64 | 23.08% | 94.86% |
| 真实英文对话 (ELLLO) | GE 重度突发 | dred_10 | 111999 | 74.7 | 29.87 | 1.54% | 95.97% |
| 真实英文对话 (ELLLO) | GE 重度突发 | dred_20 | 106601 | 71.1 | 28.43 | 4.62% | 94.38% |
| 真实英文对话 (ELLLO) | GE 重度突发 | dred_50 | 106601 | 71.1 | 28.43 | 7.69% | 94.85% |
| 真实英文对话 (ELLLO) | GE 重度突发 | dred_100 | 106601 | 71.1 | 28.43 | 3.08% | 96.16% |

## 文本指标 (WER)

| 音频类型 | 丢包场景 | 保护策略 | WER | ASR稳定性 |
|---------|---------|---------|:---:|:---:|
| 新闻播报 (VOA) | GE 重度突发 | DRED-3 | 9.80% | ok |
| 新闻播报 (VOA) | GE 重度突发 | DRED-5 | 5.88% | ok |
| 新闻播报 (VOA) | GE 重度突发 | dred_10 | 11.76% | ok |
| 新闻播报 (VOA) | GE 重度突发 | dred_20 | 7.84% | ok |
| 新闻播报 (VOA) | GE 重度突发 | dred_50 | **3.92%** | ok |
| 新闻播报 (VOA) | GE 重度突发 | dred_100 | 7.84% | ok |
| 真实英文对话 (ELLLO) | GE 重度突发 | DRED-3 | 7.69% | ok |
| 真实英文对话 (ELLLO) | GE 重度突发 | DRED-5 | 23.08% | ok |
| 真实英文对话 (ELLLO) | GE 重度突发 | dred_10 | **1.54%** | ok |
| 真实英文对话 (ELLLO) | GE 重度突发 | dred_20 | 4.62% | ok |
| 真实英文对话 (ELLLO) | GE 重度突发 | dred_50 | 7.69% | ok |
| 真实英文对话 (ELLLO) | GE 重度突发 | dred_100 | 3.08% | ok |

## 音频工件对照

| 音频类型 | 丢包场景 | 保护策略 | 输入音频 | 输出音频 | 参考转写 | 输出转写 | 统计 JSON |
|---------|---------|---------|---------|---------|---------|---------|---------|
| 新闻播报 (VOA) | GE 重度突发 | DRED-3 | [news.wav](inputs/news.wav) | [dred_3.wav](outputs/news/ge_heavy/dred_3.wav) | [reference.txt](transcripts/news/reference.txt) | [dred_3.txt](transcripts/news/ge_heavy/dred_3.txt) | [dred_3.json](stats/news/ge_heavy/dred_3.json) |
| 新闻播报 (VOA) | GE 重度突发 | DRED-5 | [news.wav](inputs/news.wav) | [dred_5.wav](outputs/news/ge_heavy/dred_5.wav) | [reference.txt](transcripts/news/reference.txt) | [dred_5.txt](transcripts/news/ge_heavy/dred_5.txt) | [dred_5.json](stats/news/ge_heavy/dred_5.json) |
| 新闻播报 (VOA) | GE 重度突发 | dred_10 | [news.wav](inputs/news.wav) | [dred_10.wav](outputs/news/ge_heavy/dred_10.wav) | [reference.txt](transcripts/news/reference.txt) | [dred_10.txt](transcripts/news/ge_heavy/dred_10.txt) | [dred_10.json](stats/news/ge_heavy/dred_10.json) |
| 新闻播报 (VOA) | GE 重度突发 | dred_20 | [news.wav](inputs/news.wav) | [dred_20.wav](outputs/news/ge_heavy/dred_20.wav) | [reference.txt](transcripts/news/reference.txt) | [dred_20.txt](transcripts/news/ge_heavy/dred_20.txt) | [dred_20.json](stats/news/ge_heavy/dred_20.json) |
| 新闻播报 (VOA) | GE 重度突发 | dred_50 | [news.wav](inputs/news.wav) | [dred_50.wav](outputs/news/ge_heavy/dred_50.wav) | [reference.txt](transcripts/news/reference.txt) | [dred_50.txt](transcripts/news/ge_heavy/dred_50.txt) | [dred_50.json](stats/news/ge_heavy/dred_50.json) |
| 新闻播报 (VOA) | GE 重度突发 | dred_100 | [news.wav](inputs/news.wav) | [dred_100.wav](outputs/news/ge_heavy/dred_100.wav) | [reference.txt](transcripts/news/reference.txt) | [dred_100.txt](transcripts/news/ge_heavy/dred_100.txt) | [dred_100.json](stats/news/ge_heavy/dred_100.json) |
| 真实英文对话 (ELLLO) | GE 重度突发 | DRED-3 | [dialogue.wav](inputs/dialogue.wav) | [dred_3.wav](outputs/dialogue/ge_heavy/dred_3.wav) | [reference.txt](transcripts/dialogue/reference.txt) | [dred_3.txt](transcripts/dialogue/ge_heavy/dred_3.txt) | [dred_3.json](stats/dialogue/ge_heavy/dred_3.json) |
| 真实英文对话 (ELLLO) | GE 重度突发 | DRED-5 | [dialogue.wav](inputs/dialogue.wav) | [dred_5.wav](outputs/dialogue/ge_heavy/dred_5.wav) | [reference.txt](transcripts/dialogue/reference.txt) | [dred_5.txt](transcripts/dialogue/ge_heavy/dred_5.txt) | [dred_5.json](stats/dialogue/ge_heavy/dred_5.json) |
| 真实英文对话 (ELLLO) | GE 重度突发 | dred_10 | [dialogue.wav](inputs/dialogue.wav) | [dred_10.wav](outputs/dialogue/ge_heavy/dred_10.wav) | [reference.txt](transcripts/dialogue/reference.txt) | [dred_10.txt](transcripts/dialogue/ge_heavy/dred_10.txt) | [dred_10.json](stats/dialogue/ge_heavy/dred_10.json) |
| 真实英文对话 (ELLLO) | GE 重度突发 | dred_20 | [dialogue.wav](inputs/dialogue.wav) | [dred_20.wav](outputs/dialogue/ge_heavy/dred_20.wav) | [reference.txt](transcripts/dialogue/reference.txt) | [dred_20.txt](transcripts/dialogue/ge_heavy/dred_20.txt) | [dred_20.json](stats/dialogue/ge_heavy/dred_20.json) |
| 真实英文对话 (ELLLO) | GE 重度突发 | dred_50 | [dialogue.wav](inputs/dialogue.wav) | [dred_50.wav](outputs/dialogue/ge_heavy/dred_50.wav) | [reference.txt](transcripts/dialogue/reference.txt) | [dred_50.txt](transcripts/dialogue/ge_heavy/dred_50.txt) | [dred_50.json](stats/dialogue/ge_heavy/dred_50.json) |
| 真实英文对话 (ELLLO) | GE 重度突发 | dred_100 | [dialogue.wav](inputs/dialogue.wav) | [dred_100.wav](outputs/dialogue/ge_heavy/dred_100.wav) | [reference.txt](transcripts/dialogue/reference.txt) | [dred_100.txt](transcripts/dialogue/ge_heavy/dred_100.txt) | [dred_100.json](stats/dialogue/ge_heavy/dred_100.json) |
