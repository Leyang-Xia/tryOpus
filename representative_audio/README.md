# Representative Audio

仓库内置的标准测试输入只保留两类 30 秒语音素材：

- `news/news_30s_48k_mono.wav`
  来源：VOA Learning English
  处理：固定从源音频 20 秒处开始裁切，避开片头音乐
- `dialogue/dialogue_30s_48k_mono.wav`
  来源：ELLLO 英文真实对话
  参考文本：`dialogue/dialogue_reference.txt`

这套文件同时用于：

- `scripts/run_experiments.sh`
- `webrtc_demo/scripts/run_rtc_experiments.sh`

如需刷新这套基线素材：

```bash
python3 tools/prepare_representative_audio.py --force
```

刷新后应把变更后的 WAV、reference 文本和 `manifest.txt` 一并提交。
