# 标准测试音频

`representative_audio/` 保存仓库统一使用的标准输入资产。当前所有离线批量实验和 RTC 实验矩阵都默认从这里取样。

## 当前内容

```text
representative_audio/
├── manifest.txt
├── news/
│   └── news_30s_48k_mono.wav
└── dialogue/
    ├── dialogue_30s_48k_mono.wav
    └── dialogue_reference.txt
```

## 约定

### `manifest.txt`

格式：

```text
audio_type|relative_path
```

当前内容：

```text
news|news/news_30s_48k_mono.wav
dialogue|dialogue/dialogue_30s_48k_mono.wav
```

### 音频集

- `news/news_30s_48k_mono.wav`
  - 来源：VOA Learning English
  - 用途：新闻播报类基线样本
- `dialogue/dialogue_30s_48k_mono.wav`
  - 来源：ELLLO 英文真实对话
  - 用途：对话类基线样本
- `dialogue/dialogue_reference.txt`
  - `dialogue` 的人工参考文本

## 谁会使用这些文件

- `offline_validation/run_experiments.sh`
- `webrtc_demo/scripts/run_rtc_experiments.sh`
- 顶层 `tools/gen_rtc_report.py`

在报告链路里：

- `dialogue` 优先使用 `dialogue_reference.txt`
- `news` 默认使用干净输入音频的 ASR 转写作为参考文本

## 刷新基线素材

```bash
python3 tools/prepare_representative_audio.py --force
```

刷新后通常需要一起检查并提交：

- 新的 WAV 文件
- `dialogue_reference.txt`（如果内容变了）
- `manifest.txt`

## 选择这套基线的原因

- 统一为 30 秒长度，便于离线和 RTC 报告横向对比
- 一条偏新闻播报，一条偏自然对话，覆盖两种主要语音风格
- 长度足够支撑 `WER / SER` 报告，而不会让回归实验过慢
