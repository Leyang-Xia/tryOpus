#!/usr/bin/env python3
import argparse
import pathlib
import re
import subprocess
import urllib.request
import urllib.parse
from dataclasses import dataclass


@dataclass
class AudioSpec:
    audio_type: str
    source_url: str
    clip_seconds: int


def _download(url: str, target: pathlib.Path) -> None:
    target.parent.mkdir(parents=True, exist_ok=True)
    urllib.request.urlretrieve(url, str(target))


def _convert_to_wav(src: pathlib.Path, dst: pathlib.Path, clip_seconds: int) -> None:
    dst.parent.mkdir(parents=True, exist_ok=True)
    cmd = [
        "ffmpeg",
        "-y",
        "-hide_banner",
        "-loglevel",
        "error",
        "-i",
        str(src),
        "-ac",
        "1",
        "-ar",
        "48000",
        "-t",
        str(clip_seconds),
        str(dst),
    ]
    subprocess.run(cmd, check=True)


def _resolve_news_url(default_url: str) -> str:
    rss_url = "https://podcasts.files.bbci.co.uk/p02nq0gn.rss"
    try:
        with urllib.request.urlopen(rss_url, timeout=15) as resp:
            xml = resp.read().decode("utf-8", errors="ignore")
        m = re.search(r'<enclosure url="([^"]+\\.mp3)"', xml)
        if m:
            return m.group(1)
    except Exception:
        pass
    return default_url


def main() -> None:
    parser = argparse.ArgumentParser(description="Download representative internet audio and normalize to 48k mono wav.")
    parser.add_argument("--out-dir", required=True, help="Directory for output wav files")
    parser.add_argument("--manifest", required=True, help="Manifest output path, each line: audio_type|wav_path")
    parser.add_argument("--clip-seconds", type=int, default=4, help="Clip length in seconds for each sample")
    args = parser.parse_args()

    out_dir = pathlib.Path(args.out_dir).resolve()
    raw_dir = out_dir / "raw"
    wav_dir = out_dir / "wav"
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
        suffix = pathlib.Path(urllib.parse.urlparse(spec.source_url).path).suffix or ".bin"
        raw_path = raw_dir / f"{spec.audio_type}{suffix}"
        wav_path = wav_dir / f"{spec.audio_type}_48k_mono.wav"
        _download(spec.source_url, raw_path)
        _convert_to_wav(raw_path, wav_path, spec.clip_seconds)
        lines.append(f"{spec.audio_type}|{wav_path}")

    manifest_path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    print(f"manifest={manifest_path}")
    for line in lines:
        print(line)


if __name__ == "__main__":
    main()
