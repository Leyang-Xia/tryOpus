#!/usr/bin/env python3
"""
prepare_representative_audio.py - 下载三类代表性音频并转码为 48kHz 单声道 WAV

音频类型:
  music    - 音乐片段（SoundHelix）
  news     - 新闻播报片段（BBC podcast RSS 自动解析）
  dialogue - 多人对话场景片段（餐厅会话环境音）

用法:
  python3 tools/prepare_representative_audio.py \\
      --out-dir /tmp/audio --manifest /tmp/manifest.txt --clip-seconds 10
"""

import argparse
import os
import pathlib
import re
import shutil
import subprocess
import urllib.request
from dataclasses import dataclass


@dataclass
class AudioSpec:
    audio_type: str
    source_url: str
    clip_seconds: int


def _extract_clip_to_wav(src: str, dst: pathlib.Path, clip_seconds: int) -> None:
    dst.parent.mkdir(parents=True, exist_ok=True)
    cmd = [
        "ffmpeg", "-y", "-hide_banner", "-loglevel", "error",
        "-t", str(clip_seconds),
        "-i", src,
        "-ac", "1", "-ar", "48000",
        str(dst),
    ]
    subprocess.run(cmd, check=True, timeout=max(60, clip_seconds * 10))


def _resolve_news_url(default_url: str) -> str:
    if os.environ.get("REP_AUDIO_PREFER_LIVE_NEWS", "").lower() not in {"1", "true", "yes"}:
        return default_url
    rss_url = "https://podcasts.files.bbci.co.uk/p02nq0gn.rss"
    try:
        with urllib.request.urlopen(rss_url, timeout=15) as resp:
            xml = resp.read().decode("utf-8", errors="ignore")
        m = re.search(r'<enclosure url="([^"]+\.mp3)"', xml)
        if m:
            return m.group(1)
    except Exception:
        pass
    return default_url


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Download representative internet audio and normalize to 48k mono wav.")
    parser.add_argument("--out-dir", required=True,
                        help="Directory for output wav files")
    parser.add_argument("--manifest", required=True,
                        help="Manifest output path, each line: audio_type|wav_path")
    parser.add_argument("--cache-dir",
                        help="Optional persistent cache dir for normalized wav files; defaults to --out-dir")
    parser.add_argument("--clip-seconds", type=int, default=10,
                        help="Clip length in seconds for each sample")
    parser.add_argument("--force", action="store_true",
                        help="Redownload and reconvert even if cached files already exist")
    args = parser.parse_args()

    out_dir = pathlib.Path(args.out_dir).resolve()
    cache_dir = pathlib.Path(args.cache_dir).resolve() if args.cache_dir else out_dir
    wav_dir = cache_dir / "wav"
    manifest_path = pathlib.Path(args.manifest).resolve()

    news_fallback = "https://huggingface.co/datasets/Narsil/asr_dummy/resolve/main/mlk.flac"
    specs = [
        AudioSpec(
            audio_type="music",
            source_url="https://www.soundhelix.com/examples/mp3/SoundHelix-Song-1.mp3",
            clip_seconds=args.clip_seconds,
        ),
        AudioSpec(
            audio_type="news",
            source_url=_resolve_news_url(news_fallback),
            clip_seconds=args.clip_seconds,
        ),
        AudioSpec(
            audio_type="dialogue",
            source_url="https://bigsoundbank.com/UPLOAD/mp3/3542.mp3",
            clip_seconds=args.clip_seconds,
        ),
    ]

    manifest_path.parent.mkdir(parents=True, exist_ok=True)
    lines = []

    for spec in specs:
        wav_path = wav_dir / f"{spec.audio_type}_48k_mono.wav"
        out_wav_path = out_dir / "wav" / f"{spec.audio_type}_48k_mono.wav"

        if args.force or not wav_path.exists():
            print(f"[audio] extracting {spec.clip_seconds}s clip for {spec.audio_type} from {spec.source_url}")
            _extract_clip_to_wav(spec.source_url, wav_path, spec.clip_seconds)
        else:
            print(f"[audio] reuse cached wav {spec.audio_type}: {wav_path}")

        out_wav_path.parent.mkdir(parents=True, exist_ok=True)
        if out_wav_path != wav_path:
            if args.force or not out_wav_path.exists():
                print(f"[audio] copying {spec.audio_type} wav -> {out_wav_path}")
                shutil.copy2(wav_path, out_wav_path)
            else:
                src_mtime = wav_path.stat().st_mtime
                dst_mtime = out_wav_path.stat().st_mtime
                if src_mtime > dst_mtime or os.path.getsize(out_wav_path) != os.path.getsize(wav_path):
                    print(f"[audio] refreshing copied wav {spec.audio_type} -> {out_wav_path}")
                    shutil.copy2(wav_path, out_wav_path)
        else:
            out_wav_path = wav_path

        lines.append(f"{spec.audio_type}|{out_wav_path}")

    manifest_path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    print(f"[audio] manifest={manifest_path}")
    for line in lines:
        print(f"  {line}")


if __name__ == "__main__":
    main()
