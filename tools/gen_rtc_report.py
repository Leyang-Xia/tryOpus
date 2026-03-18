#!/usr/bin/env python3
"""
gen_rtc_report.py - 从实验汇总 CSV 生成 Markdown 报告

输入: webrtc_demo/scripts/run_rtc_experiments.sh 或 offline_validation/run_experiments.sh 产生的汇总 CSV
输出: 对应 run 目录下的 Markdown 报告（RTC / 离线统一使用 WER/SER）

CSV 列（RTC）:
  audio_type,scenario,case,sim_mode,sim_loss,
  recovered_lbrr,recovered_dred,plc,decode_errors,recovery_rate,
  input_wav,output_wav,stats_json

CSV 列（离线仿真）:
  audio_type,experiment,name,loss_model,loss_param,
  recovered_lbrr,recovered_dred,plc,recovery_rate,
  input_wav,output_wav,stats_csv
"""

import argparse
import csv
import math
import os
import re
import struct
import sys
from datetime import datetime
from typing import List, Tuple, Optional

try:
    from faster_whisper import WhisperModel
    from jiwer import wer as jiwer_wer
except ImportError:
    WhisperModel = None
    jiwer_wer = None

try:
    import mlx_whisper
except ImportError:
    mlx_whisper = None


# --------------- WAV 读取 ---------------

def read_wav(path: str) -> Tuple[list, int]:
    with open(path, "rb") as f:
        riff = f.read(4)
        if riff != b"RIFF":
            raise ValueError(f"Not a RIFF file: {path}")
        f.read(4)
        f.read(4)
        sr = 0
        ch = 1
        bits = 16
        samples: list = []
        while True:
            chunk_id = f.read(4)
            if len(chunk_id) < 4:
                break
            chunk_size = struct.unpack("<I", f.read(4))[0]
            if chunk_id == b"fmt ":
                fmt_data = f.read(chunk_size)
                fmt = struct.unpack("<HHIIHH", fmt_data[:16])
                ch = fmt[1]
                sr = fmt[2]
                bits = fmt[5]
            elif chunk_id == b"data":
                raw = f.read(chunk_size)
                if bits == 16:
                    n = chunk_size // 2
                    samples = list(struct.unpack(f"<{n}h", raw))
                else:
                    raise ValueError(f"Unsupported bit depth: {bits}")
                break
            else:
                f.read(chunk_size)
        if ch > 1:
            samples = samples[::ch]
        return samples, sr


# --------------- 音质指标 ---------------

def compute_snr(ref: list, deg: list) -> float:
    n = min(len(ref), len(deg))
    if n == 0:
        return 0.0
    signal_power = sum(r * r for r in ref[:n]) / n
    noise_power = sum((r - d) ** 2 for r, d in zip(ref[:n], deg[:n])) / n
    if noise_power < 1e-10:
        return 99.0
    return 10 * math.log10(signal_power / noise_power)


def compute_segsnr(ref: list, deg: list, sr: int, frame_ms: int = 20) -> float:
    frame_len = sr * frame_ms // 1000
    n = min(len(ref), len(deg))
    seg_snrs: list = []
    for i in range(0, n - frame_len, frame_len):
        r_seg = ref[i : i + frame_len]
        d_seg = deg[i : i + frame_len]
        sp = sum(x * x for x in r_seg) / frame_len
        np_ = sum((a - b) ** 2 for a, b in zip(r_seg, d_seg)) / frame_len
        if sp > 100 and np_ > 0:
            seg_snrs.append(10 * math.log10(sp / np_))
    if not seg_snrs:
        return 0.0
    seg_snrs = [max(-10.0, min(35.0, s)) for s in seg_snrs]
    return sum(seg_snrs) / len(seg_snrs)


# --------------- 报告生成 ---------------

SCENARIO_LABELS = {
    "uniform_5": "均匀 5%",
    "uniform_10": "均匀 10%",
    "uniform_20": "均匀 20%",
    "ge_moderate": "GE 中等突发",
    "ge_heavy": "GE 重度突发",
    "delay_jitter_10": "均匀 10% + 延迟/抖动",
}

CASE_LABELS = {
    "baseline": "无保护 (PLC)",
    "baseline_no_protection": "无保护 (PLC)",
    "lbrr_only": "LBRR",
    "dred_3": "DRED-3",
    "dred_5": "DRED-5",
    "lbrr_dred_3": "LBRR + DRED-3",
    "lbrr_dred": "LBRR + DRED",
}

AUDIO_LABELS = {
    "news": "新闻播报 (VOA)",
    "dialogue": "真实英文对话 (ELLLO)",
}


def _safe_float(val: str, default: float = 0.0) -> float:
    try:
        return float(val)
    except (ValueError, TypeError):
        return default


def _try_snr(input_wav: str, output_wav: str) -> Tuple[Optional[float], Optional[float]]:
    if not input_wav or not output_wav:
        return None, None
    if not os.path.isfile(input_wav) or not os.path.isfile(output_wav):
        return None, None
    try:
        ref, sr = read_wav(input_wav)
        deg, _ = read_wav(output_wav)
        snr = compute_snr(ref, deg)
        segsnr = compute_segsnr(ref, deg, sr)
        return snr, segsnr
    except Exception:
        return None, None


def _artifact_link(report_path: str, artifact_path: str) -> str:
    if not artifact_path:
        return "-"
    if not os.path.isfile(artifact_path):
        return artifact_path
    report_dir = os.path.dirname(os.path.abspath(report_path)) or "."
    rel_path = os.path.relpath(os.path.abspath(artifact_path), report_dir)
    return f"[{os.path.basename(artifact_path)}]({rel_path})"


def _normalize_transcript(text: str) -> str:
    text = text.lower()
    text = re.sub(r"[^\w\s']", " ", text)
    return re.sub(r"\s+", " ", text).strip()


def _split_sentences(text: str) -> List[str]:
    if not text:
        return []
    parts = re.split(r"[.!?]+", text)
    sentences = [_normalize_transcript(part) for part in parts]
    return [sentence for sentence in sentences if sentence]


def _split_reference_units(text: str) -> List[str]:
    if not text:
        return []
    if "\n" in text:
        lines = [_normalize_transcript(line) for line in text.splitlines()]
        lines = [line for line in lines if line]
        if lines:
            return lines
    return _split_sentences(text)


def _edit_distance(ref_units: List[str], hyp_units: List[str]) -> int:
    rows = len(ref_units) + 1
    cols = len(hyp_units) + 1
    dp = [[0] * cols for _ in range(rows)]

    for i in range(rows):
        dp[i][0] = i
    for j in range(cols):
        dp[0][j] = j

    for i in range(1, rows):
        for j in range(1, cols):
            cost = 0 if ref_units[i - 1] == hyp_units[j - 1] else 1
            dp[i][j] = min(
                dp[i - 1][j] + 1,
                dp[i][j - 1] + 1,
                dp[i - 1][j - 1] + cost,
            )
    return dp[-1][-1]


_MODEL_CACHE = {}
_RESOLVED_MODEL_NAME = None
_RESOLVED_BACKEND = None


def _default_stt_backend() -> str:
    backend = os.environ.get("RTC_STT_BACKEND")
    if backend:
        return backend
    if sys.platform == "darwin" and os.uname().machine == "arm64":
        return "mlx"
    return "faster"


def _mlx_repo_for_model(model_name: str) -> str:
    aliases = {
        "tiny": "mlx-community/whisper-tiny-mlx",
        "tiny.en": "mlx-community/whisper-tiny.en-mlx",
        "base": "mlx-community/whisper-base-mlx",
        "base.en": "mlx-community/whisper-base.en-mlx",
        "small": "mlx-community/whisper-small-mlx",
        "small.en": "mlx-community/whisper-small.en-mlx",
        "medium": "mlx-community/whisper-medium-mlx",
        "medium.en": "mlx-community/whisper-medium.en-mlx",
        "large-v3": "mlx-community/whisper-large-v3-mlx",
    }
    return aliases.get(model_name, model_name)


def _load_whisper_model(model_name: str):
    if WhisperModel is None:
        raise RuntimeError(
            "faster-whisper is not installed; use the repo .venv_asr Python or install the ASR dependencies."
        )
    global _RESOLVED_MODEL_NAME, _RESOLVED_BACKEND
    device = os.environ.get("RTC_STT_DEVICE", "cpu")
    compute_type = os.environ.get("RTC_STT_COMPUTE_TYPE", "int8")
    download_root = os.environ.get("RTC_STT_DOWNLOAD_ROOT")
    local_only = os.environ.get("RTC_STT_LOCAL_ONLY", "1").lower() not in {"0", "false", "no"}

    fallback_models = [
        candidate.strip()
        for candidate in os.environ.get("RTC_STT_FALLBACK_MODELS", "base.en,tiny").split(",")
        if candidate.strip()
    ]
    candidates = []
    for candidate in [model_name, *fallback_models]:
        if candidate not in candidates:
            candidates.append(candidate)

    last_exc = None
    for candidate in candidates:
        if candidate in _MODEL_CACHE:
            _RESOLVED_MODEL_NAME = candidate
            _RESOLVED_BACKEND = "faster"
            return _MODEL_CACHE[candidate]
        try:
            _MODEL_CACHE[candidate] = WhisperModel(
                candidate,
                device=device,
                compute_type=compute_type,
                download_root=download_root,
                local_files_only=local_only,
            )
            if candidate != model_name:
                print(
                    f"[report] requested whisper model '{model_name}' is unavailable; "
                    f"falling back to local model '{candidate}'",
                    file=sys.stderr,
                )
            _RESOLVED_MODEL_NAME = candidate
            _RESOLVED_BACKEND = "faster"
            return _MODEL_CACHE[candidate]
        except Exception as exc:
            last_exc = exc

    if local_only:
        raise RuntimeError(
            f"failed to load local whisper model '{model_name}' or any fallback from {fallback_models}. "
            f"Pre-download one of them once or rerun with RTC_STT_LOCAL_ONLY=0. original error: {last_exc}"
        ) from last_exc
    raise last_exc


def _mlx_available() -> bool:
    return mlx_whisper is not None


def _transcribe_audio_segments_mlx(
    path: str, model_name: str, language: Optional[str]
) -> Tuple[str, List[str]]:
    if not _mlx_available():
        raise RuntimeError("mlx-whisper is not installed in the ASR environment.")
    global _RESOLVED_MODEL_NAME, _RESOLVED_BACKEND
    mlx_repo = _mlx_repo_for_model(model_name)
    decode_options = {
        "language": language,
        "condition_on_previous_text": False,
        "word_timestamps": False,
    }
    decode_options = {k: v for k, v in decode_options.items() if v is not None}
    result = mlx_whisper.transcribe(path, path_or_hf_repo=mlx_repo, **decode_options)
    text = (result.get("text") or "").strip()
    raw_segments = [seg.get("text", "").strip() for seg in result.get("segments", []) if seg.get("text", "").strip()]
    normalized_segments = [_normalize_transcript(seg) for seg in raw_segments]
    normalized_segments = [seg for seg in normalized_segments if seg]
    _RESOLVED_MODEL_NAME = mlx_repo
    _RESOLVED_BACKEND = "mlx"
    return text, normalized_segments


def _transcribe_audio_segments(
    path: str, model_name: str, language: Optional[str]
) -> Tuple[str, List[str]]:
    backend = _default_stt_backend()
    errors = []
    if backend in {"mlx", "auto"}:
        try:
            return _transcribe_audio_segments_mlx(path, model_name, language)
        except Exception as exc:
            errors.append(f"mlx failed: {exc}")
            if backend == "mlx":
                print(f"[report] mlx backend failed, falling back to faster-whisper: {exc}", file=sys.stderr)
    try:
        model = _load_whisper_model(model_name)
        beam_size = int(os.environ.get("RTC_STT_BEAM_SIZE", "1"))
        segments, _ = model.transcribe(
            path,
            beam_size=beam_size,
            condition_on_previous_text=False,
            vad_filter=False,
            language=language,
            word_timestamps=False,
        )
        raw_segments = [seg.text.strip() for seg in segments if seg.text.strip()]
        normalized_segments = [_normalize_transcript(seg) for seg in raw_segments]
        normalized_segments = [seg for seg in normalized_segments if seg]
        return " ".join(raw_segments).strip(), normalized_segments
    except Exception as exc:
        errors.append(f"faster-whisper failed: {exc}")
        raise RuntimeError("; ".join(errors))


def _resolved_asr_summary(requested_model: str) -> str:
    backend = _RESOLVED_BACKEND or _default_stt_backend()
    resolved_model = _RESOLVED_MODEL_NAME or requested_model
    return f"{backend} / {resolved_model}"


def _write_text(path: str, text: str) -> None:
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w", encoding="utf-8") as f:
        f.write(text + "\n")


def _load_reference_text(input_wav: str) -> Optional[str]:
    stem, _ = os.path.splitext(input_wav)
    sidecar_candidates = [f"{stem}_reference.txt"]

    audio_path = os.path.abspath(input_wav)
    audio_dir = os.path.dirname(audio_path)
    parent_name = os.path.basename(audio_dir)
    if parent_name:
        sidecar_candidates.append(os.path.join(audio_dir, f"{parent_name}_reference.txt"))

    for sidecar in sidecar_candidates:
        if os.path.isfile(sidecar):
            with open(sidecar, "r", encoding="utf-8") as f:
                return f.read().strip()
    return None


def _stt_metrics(
    input_wav: str,
    output_wav: str,
    audio_type: str,
    scenario: str,
    case: str,
    transcript_dir: Optional[str],
    model_name: str,
    language: Optional[str],
    transcript_cache: dict,
) -> Tuple[Optional[float], Optional[float], Optional[str], Optional[str], Optional[str], Optional[str]]:
    if jiwer_wer is None:
        raise RuntimeError(
            "jiwer is not installed; use the repo .venv_asr Python or install the ASR dependencies."
        )
    if not input_wav or not output_wav:
        return None, None, None, None, None, None
    if not os.path.isfile(input_wav) or not os.path.isfile(output_wav):
        return None, None, None, None, None, None

    ref_text = transcript_cache.get(input_wav)
    if ref_text is None:
        ref_text = _load_reference_text(input_wav)
        if ref_text is None:
            ref_text, _ = _transcribe_audio_segments(input_wav, model_name, language)
        transcript_cache[input_wav] = ref_text

    hyp_text, hyp_segments = _transcribe_audio_segments(output_wav, model_name, language)
    if not ref_text:
        return None, None, ref_text, hyp_text, None, None

    ref_text_norm = _normalize_transcript(ref_text)
    hyp_text_norm = _normalize_transcript(hyp_text)
    wer_value = jiwer_wer(ref_text_norm, hyp_text_norm)

    ref_sentences = _split_reference_units(ref_text)
    if not ref_sentences and ref_text_norm:
        ref_sentences = [ref_text_norm]
    hyp_sentences = hyp_segments or _split_sentences(hyp_text)
    if not hyp_sentences and hyp_text_norm:
        hyp_sentences = [hyp_text_norm]
    if ref_sentences:
        sentence_errors = min(_edit_distance(ref_sentences, hyp_sentences), len(ref_sentences))
        ser_value = sentence_errors / len(ref_sentences)
    else:
        ser_value = 0.0 if ref_text_norm == hyp_text_norm else 1.0

    ref_path = None
    hyp_path = None
    if transcript_dir:
        ref_path = os.path.join(transcript_dir, audio_type, "reference.txt")
        hyp_path = os.path.join(transcript_dir, audio_type, scenario, f"{case}.txt")
        _write_text(ref_path, ref_text)
        _write_text(hyp_path, hyp_text)
    return wer_value, ser_value, ref_text, hyp_text, ref_path, hyp_path


def generate_rtc_report(
    csv_path: str,
    output_path: str,
    transcript_dir: Optional[str],
    stt_model: str,
    stt_language: Optional[str],
) -> None:
    rows: list = []
    with open(csv_path, "r", encoding="utf-8") as f:
        reader = csv.DictReader(f)
        for row in reader:
            if row.get("audio_type") in {"news", "dialogue"}:
                rows.append(row)

    if not rows:
        print("[report] no data rows in CSV, skipping report", file=sys.stderr)
        return

    transcript_cache = {}
    for row in rows:
        input_wav = row.get("input_wav", "")
        output_wav = row.get("output_wav", "")
        scenario = row.get("scenario", "")
        case = row.get("case", "")
        wer_value, ser_value, ref_text, hyp_text, ref_path, hyp_path = _stt_metrics(
            input_wav,
            output_wav,
            row.get("audio_type", ""),
            scenario,
            case,
            transcript_dir,
            stt_model,
            stt_language,
            transcript_cache,
        )
        row["wer"] = f"{wer_value:.2%}" if wer_value is not None else "-"
        row["ser"] = f"{ser_value:.2%}" if ser_value is not None else "-"
        row["reference_text"] = ref_text or ""
        row["hypothesis_text"] = hyp_text or ""
        row["reference_text_path"] = ref_path or ""
        row["hypothesis_text_path"] = hyp_path or ""

    audio_types = []
    seen = set()
    for r in rows:
        at = r.get("audio_type", "unknown")
        if at not in seen:
            audio_types.append(at)
            seen.add(at)

    scenarios = []
    seen_s = set()
    for r in rows:
        s = r.get("scenario", r.get("experiment", "unknown"))
        if s not in seen_s:
            scenarios.append(s)
            seen_s.add(s)

    lines: List[str] = []
    lines.append("# RTC 传输实验报告\n")
    lines.append(f"> 自动生成于 {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}\n")

    lines.append("## 实验配置\n")
    lines.append("### 音频类型\n")
    lines.append("| 标识 | 说明 |")
    lines.append("|------|------|")
    for at in audio_types:
        label = AUDIO_LABELS.get(at, at)
        lines.append(f"| `{at}` | {label} |")
    lines.append("")

    lines.append("### 丢包场景\n")
    lines.append("| 标识 | 说明 |")
    lines.append("|------|------|")
    for s in scenarios:
        label = SCENARIO_LABELS.get(s, s)
        lines.append(f"| `{s}` | {label} |")
    lines.append("")
    lines.append("### 文本指标说明\n")
    lines.append(f"- 当前报告 ASR 后端/模型：`{_resolved_asr_summary(stt_model)}`。")
    lines.append("- WER / SER 使用同一个 ASR 后端分别转写干净输入音频与 RTC 输出音频。")
    lines.append("- 干净输入音频的转写结果作为参考文本，因此这里衡量的是相对可懂度退化，而不是人工标注基准上的绝对识别率。")
    lines.append("- SER 只有在音频中包含足够多、边界清晰的句子时才更有解释力，因此默认代表性语音片段已提升到 30 秒。")
    lines.append("")

    # ---- 恢复策略表 ----
    lines.append("## 恢复策略对比\n")
    for at in audio_types:
        at_label = AUDIO_LABELS.get(at, at)
        lines.append(f"### {at_label}\n")
        lines.append("| 丢包场景 | 保护策略 | LBRR 恢复 | DRED 恢复 | PLC 帧 | 解码错误 | 恢复率 |")
        lines.append("|---------|---------|----------|----------|-------|---------|-------|")
        for r in rows:
            if r.get("audio_type") != at:
                continue
            scenario = r.get("scenario", r.get("experiment", ""))
            case = r.get("case", r.get("name", ""))
            s_label = SCENARIO_LABELS.get(scenario, scenario)
            c_label = CASE_LABELS.get(case, case)
            lbrr = r.get("recovered_lbrr", "0")
            dred = r.get("recovered_dred", "0")
            plc = r.get("plc", "0")
            decode_err = r.get("decode_errors", "0")
            rate = r.get("recovery_rate", "0")
            rate_val = _safe_float(rate)
            rate_str = f"{rate_val:.2%}" if rate_val <= 1.0 else f"{rate_val:.2f}%"
            lines.append(
                f"| {s_label} | {c_label} | {lbrr} | {dred} | {plc} | {decode_err} | {rate_str} |"
            )
        lines.append("")

    # ---- WER / SER 表 ----
    lines.append("## 文本指标 (WER / SER)\n")
    lines.append("| 音频类型 | 丢包场景 | 保护策略 | WER | SER |")
    lines.append("|---------|---------|---------|:---:|:---:|")
    for r in rows:
        at = r.get("audio_type", "")
        scenario = r.get("scenario", r.get("experiment", ""))
        case = r.get("case", r.get("name", ""))
        at_label = AUDIO_LABELS.get(at, at)
        s_label = SCENARIO_LABELS.get(scenario, scenario)
        c_label = CASE_LABELS.get(case, case)
        wer_val = r.get("wer", "-")
        ser_val = r.get("ser", "-")
        lines.append(f"| {at_label} | {s_label} | {c_label} | {wer_val} | {ser_val} |")
    lines.append("")

    lines.append("## 音频工件对照\n")
    lines.append("| 音频类型 | 丢包场景 | 保护策略 | 输入音频 | 输出音频 | 参考转写 | 输出转写 | 统计 JSON |")
    lines.append("|---------|---------|---------|---------|---------|---------|---------|---------|")
    for r in rows:
        at = r.get("audio_type", "")
        scenario = r.get("scenario", r.get("experiment", ""))
        case = r.get("case", r.get("name", ""))
        at_label = AUDIO_LABELS.get(at, at)
        s_label = SCENARIO_LABELS.get(scenario, scenario)
        c_label = CASE_LABELS.get(case, case)
        input_ref = _artifact_link(output_path, r.get("input_wav", ""))
        output_ref = _artifact_link(output_path, r.get("output_wav", ""))
        ref_text_ref = _artifact_link(output_path, r.get("reference_text_path", ""))
        hyp_text_ref = _artifact_link(output_path, r.get("hypothesis_text_path", ""))
        stats_ref = _artifact_link(output_path, r.get("stats_json", ""))
        lines.append(
            f"| {at_label} | {s_label} | {c_label} | {input_ref} | {output_ref} | "
            f"{ref_text_ref} | {hyp_text_ref} | {stats_ref} |"
        )
    lines.append("")

    os.makedirs(os.path.dirname(output_path) or ".", exist_ok=True)
    with open(output_path, "w", encoding="utf-8") as f:
        f.write("\n".join(lines))
    print(f"[report] generated: {output_path}")


def generate_opus_report(
    csv_path: str,
    output_path: str,
    transcript_dir: Optional[str],
    stt_model: str,
    stt_language: Optional[str],
) -> None:
    """Generate transcript-based report for offline opus_sim experiments."""
    rows: list = []
    with open(csv_path, "r", encoding="utf-8") as f:
        reader = csv.DictReader(f)
        for row in reader:
            if row.get("audio_type") in {"news", "dialogue"}:
                rows.append(row)

    if not rows:
        print("[report] no data rows in CSV, skipping report", file=sys.stderr)
        return

    transcript_cache = {}
    for row in rows:
        input_wav = row.get("input_wav", "")
        output_wav = row.get("output_wav", "")
        experiment = row.get("experiment", "")
        case_name = row.get("name", "")
        wer_value, ser_value, ref_text, hyp_text, ref_path, hyp_path = _stt_metrics(
            input_wav,
            output_wav,
            row.get("audio_type", ""),
            experiment,
            case_name,
            transcript_dir,
            stt_model,
            stt_language,
            transcript_cache,
        )
        row["wer"] = f"{wer_value:.2%}" if wer_value is not None else "-"
        row["ser"] = f"{ser_value:.2%}" if ser_value is not None else "-"
        row["reference_text"] = ref_text or ""
        row["hypothesis_text"] = hyp_text or ""
        row["reference_text_path"] = ref_path or ""
        row["hypothesis_text_path"] = hyp_path or ""

    audio_types = list(dict.fromkeys(r.get("audio_type", "unknown") for r in rows))

    lines: List[str] = []
    lines.append("# Opus 离线仿真实验报告\n")
    lines.append(f"> 自动生成于 {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}\n")

    lines.append("## 音频类型\n")
    lines.append("| 标识 | 说明 |")
    lines.append("|------|------|")
    for at in audio_types:
        label = AUDIO_LABELS.get(at, at)
        lines.append(f"| `{at}` | {label} |")
    lines.append("")

    lines.append("## 文本指标说明\n")
    lines.append(f"- 当前报告 ASR 后端/模型：`{_resolved_asr_summary(stt_model)}`。")
    lines.append("- WER / SER 使用同一个 ASR 后端分别转写干净输入音频与离线输出音频。")
    lines.append("- `dialogue` 优先使用仓库内置参考文本，`news` 使用干净输入音频的转写结果作为参考文本。")
    lines.append("- 这里衡量的是不同恢复策略对可懂度的相对退化，作为离线回归的主评估指标。")
    lines.append("")

    lines.append("## 恢复策略对比\n")
    for at in audio_types:
        at_label = AUDIO_LABELS.get(at, at)
        lines.append(f"### {at_label}\n")
        lines.append("| 实验 | 方案 | 丢包模型 | LBRR 恢复 | DRED 恢复 | PLC 帧 | 恢复率 |")
        lines.append("|------|------|---------|----------|----------|-------|-------|")
        for r in rows:
            if r.get("audio_type") != at:
                continue
            exp = r.get("experiment", "")
            name = r.get("name", "")
            model = r.get("loss_model", "")
            lbrr = r.get("recovered_lbrr", "0")
            dred = r.get("recovered_dred", "0")
            plc = r.get("plc", "0")
            rate = r.get("recovery_rate", "0")
            rate_val = _safe_float(rate)
            rate_str = f"{rate_val}%"
            lines.append(f"| {exp} | {name} | {model} | {lbrr} | {dred} | {plc} | {rate_str} |")
        lines.append("")

    lines.append("## 文本指标 (WER / SER)\n")
    lines.append("| 音频类型 | 实验 | 方案 | WER | SER |")
    lines.append("|---------|------|------|:---:|:---:|")
    for r in rows:
        at = r.get("audio_type", "")
        at_label = AUDIO_LABELS.get(at, at)
        exp = r.get("experiment", "")
        name = r.get("name", "")
        wer_val = r.get("wer", "-")
        ser_val = r.get("ser", "-")
        lines.append(f"| {at_label} | {exp} | {name} | {wer_val} | {ser_val} |")
    lines.append("")

    lines.append("## 音频工件对照\n")
    lines.append("| 音频类型 | 实验 | 方案 | 输入音频 | 输出音频 | 参考转写 | 输出转写 | 统计 CSV |")
    lines.append("|---------|------|------|---------|---------|---------|---------|---------|")
    for r in rows:
        at = r.get("audio_type", "")
        at_label = AUDIO_LABELS.get(at, at)
        exp = r.get("experiment", "")
        name = r.get("name", "")
        input_ref = _artifact_link(output_path, r.get("input_wav", ""))
        output_ref = _artifact_link(output_path, r.get("output_wav", ""))
        ref_text_ref = _artifact_link(output_path, r.get("reference_text_path", ""))
        hyp_text_ref = _artifact_link(output_path, r.get("hypothesis_text_path", ""))
        stats_ref = _artifact_link(output_path, r.get("stats_csv", ""))
        lines.append(
            f"| {at_label} | {exp} | {name} | {input_ref} | {output_ref} | "
            f"{ref_text_ref} | {hyp_text_ref} | {stats_ref} |"
        )
    lines.append("")

    os.makedirs(os.path.dirname(output_path) or ".", exist_ok=True)
    with open(output_path, "w", encoding="utf-8") as f:
        f.write("\n".join(lines))
    print(f"[report] generated: {output_path}")


def main() -> None:
    parser = argparse.ArgumentParser(description="Generate Markdown experiment report from CSV")
    parser.add_argument("--csv", required=True, help="Summary CSV path")
    parser.add_argument("--output", required=True, help="Output Markdown path")
    parser.add_argument("--transcript-dir", help="Optional directory to store ASR transcripts")
    parser.add_argument("--stt-model", default=os.environ.get("RTC_STT_MODEL", "small.en"),
                        help="Whisper model name for RTC transcript-based metrics")
    parser.add_argument("--stt-language", default=os.environ.get("RTC_STT_LANGUAGE"),
                        help="Optional forced language for Whisper transcription")
    parser.add_argument("--mode", default="rtc", choices=["rtc", "opus"],
                        help="Report mode: rtc (WebRTC) or opus (offline sim)")
    args = parser.parse_args()

    if args.mode == "rtc":
        generate_rtc_report(args.csv, args.output, args.transcript_dir, args.stt_model, args.stt_language)
    else:
        generate_opus_report(args.csv, args.output, args.transcript_dir, args.stt_model, args.stt_language)


if __name__ == "__main__":
    main()
