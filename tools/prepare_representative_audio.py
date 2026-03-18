#!/usr/bin/env python3
"""
prepare_representative_audio.py - 更新仓库内置的 30s 代表性语音基线素材

输出目录:
  representative_audio/
    manifest.txt
    news/news_30s_48k_mono.wav
    dialogue/dialogue_30s_48k_mono.wav
    dialogue/dialogue_reference.txt

用法:
  python3 tools/prepare_representative_audio.py
  python3 tools/prepare_representative_audio.py --force
  python3 tools/prepare_representative_audio.py --out-dir /tmp/representative_audio --force
"""

import argparse
import pathlib
import shutil
import subprocess
import tempfile
from dataclasses import dataclass
from typing import Optional


ROOT_DIR = pathlib.Path(__file__).resolve().parent.parent
DEFAULT_OUT_DIR = ROOT_DIR / "representative_audio"
CLIP_SECONDS = 30


@dataclass(frozen=True)
class AudioSpec:
    audio_type: str
    source_url: str
    relative_wav: str
    start_offset_seconds: float = 0.0
    reference_text: Optional[str] = None


DIALOGUE_REFERENCE = (
    "Conversation one.\n"
    "Do you have a bike I can borrow?\n"
    "Yeah, I have one, but it's very old.\n"
    "Does it run well?\n"
    "Yeah, it works, but it might need a new tire.\n"
    "You know, I think I have one in the garage.\n"
    "Perfect. Then you are all set.\n"
    "Conversation two.\n"
    "Would you like a cookie?\n"
    "I would love one. Did you make them yourself?\n"
    "No."
)


SPECS = (
    AudioSpec(
        audio_type="news",
        source_url="https://voa-audio.voanews.eu/lere/2014/07/17/0e5a707a-9185-4ecc-ac0f-3b452905192a.mp3?download=1",
        relative_wav="news/news_30s_48k_mono.wav",
        start_offset_seconds=20.0,
    ),
    AudioSpec(
        audio_type="dialogue",
        source_url="https://elllo.org/Audio/SoundGrammar/A2-Audio/A2-25-One-It.mp3",
        relative_wav="dialogue/dialogue_30s_48k_mono.wav",
        reference_text=DIALOGUE_REFERENCE,
    ),
)


def _probe_duration_seconds(path: pathlib.Path) -> float:
    cmd = [
        "ffprobe",
        "-v", "error",
        "-show_entries", "format=duration",
        "-of", "default=nk=1:nw=1",
        str(path),
    ]
    out = subprocess.check_output(cmd, text=True, timeout=30).strip()
    return float(out)


def _download_to_temp(src: str, tmpdir: pathlib.Path) -> pathlib.Path:
    suffix = pathlib.Path(src).suffix or ".bin"
    tmp_src = tmpdir / f"source{suffix}"
    curl_cmd = [
        "curl",
        "-L",
        "--fail",
        "--silent",
        "--show-error",
        "--max-time",
        "600",
        "-o",
        str(tmp_src),
        src,
    ]
    subprocess.run(curl_cmd, check=True, timeout=660)
    return tmp_src


def _extract_clip_to_wav(src: str, dst: pathlib.Path, clip_seconds: int, start_offset_seconds: float) -> None:
    dst.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory() as tmp:
        tmpdir = pathlib.Path(tmp)
        input_src = _download_to_temp(src, tmpdir) if src.startswith(("http://", "https://")) else pathlib.Path(src)
        cmd = [
            "ffmpeg",
            "-y",
            "-hide_banner",
            "-loglevel",
            "error",
            "-ss",
            f"{start_offset_seconds:.3f}",
            "-t",
            str(clip_seconds),
            "-i",
            str(input_src),
            "-ac",
            "1",
            "-ar",
            "48000",
            str(dst),
        ]
        subprocess.run(cmd, check=True, timeout=240)

    duration = _probe_duration_seconds(dst)
    if duration < clip_seconds - 0.5:
        raise RuntimeError(
            f"prepared clip is too short for {dst.name}: got {duration:.2f}s, expected about {clip_seconds}s"
        )


def _write_manifest(out_dir: pathlib.Path) -> None:
    manifest_path = out_dir / "manifest.txt"
    lines = [f"{spec.audio_type}|{spec.relative_wav}" for spec in SPECS]
    manifest_path.write_text("\n".join(lines) + "\n", encoding="utf-8")


def _write_reference(out_dir: pathlib.Path, spec: AudioSpec) -> None:
    if not spec.reference_text:
        return
    ref_path = out_dir / pathlib.Path(spec.relative_wav).parent / f"{spec.audio_type}_reference.txt"
    ref_path.parent.mkdir(parents=True, exist_ok=True)
    ref_path.write_text(spec.reference_text + "\n", encoding="utf-8")


def _validate_output(out_dir: pathlib.Path) -> None:
    manifest_path = out_dir / "manifest.txt"
    if not manifest_path.is_file():
        raise RuntimeError(f"missing manifest: {manifest_path}")

    for spec in SPECS:
        wav_path = out_dir / spec.relative_wav
        if not wav_path.is_file():
            raise RuntimeError(f"missing wav: {wav_path}")
        duration = _probe_duration_seconds(wav_path)
        if duration < CLIP_SECONDS - 0.5:
            raise RuntimeError(f"unexpected wav duration for {wav_path}: {duration:.2f}s")
        if spec.reference_text:
            ref_path = wav_path.parent / f"{spec.audio_type}_reference.txt"
            if not ref_path.is_file():
                raise RuntimeError(f"missing reference transcript: {ref_path}")


def main() -> None:
    parser = argparse.ArgumentParser(description="Refresh repo-tracked representative audio assets.")
    parser.add_argument(
        "--out-dir",
        default=str(DEFAULT_OUT_DIR),
        help="Representative audio root directory; defaults to <repo>/representative_audio",
    )
    parser.add_argument(
        "--force",
        action="store_true",
        help="Redownload and reconvert the baseline wav files even if they already exist",
    )
    args = parser.parse_args()

    out_dir = pathlib.Path(args.out_dir).resolve()
    out_dir.mkdir(parents=True, exist_ok=True)

    for spec in SPECS:
        wav_path = out_dir / spec.relative_wav
        needs_build = args.force or not wav_path.exists()
        if needs_build:
            print(
                f"[audio] extracting {spec.audio_type} -> {wav_path} "
                f"(start={spec.start_offset_seconds:.1f}s, duration={CLIP_SECONDS}s)"
            )
            _extract_clip_to_wav(spec.source_url, wav_path, CLIP_SECONDS, spec.start_offset_seconds)
        else:
            print(f"[audio] reuse existing wav {spec.audio_type}: {wav_path}")
        _write_reference(out_dir, spec)

    _write_manifest(out_dir)
    _validate_output(out_dir)

    print(f"[audio] baseline assets refreshed under {out_dir}")
    print(f"[audio] manifest={out_dir / 'manifest.txt'}")
    for spec in SPECS:
        print(f"  {spec.audio_type}|{spec.relative_wav}")


if __name__ == "__main__":
    main()
