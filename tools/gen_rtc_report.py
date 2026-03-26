#!/usr/bin/env python3
"""
gen_rtc_report.py - 从实验汇总 CSV 生成 Markdown 报告

输入: webrtc_demo/scripts/run_rtc_experiments.sh 或 offline_validation/run_experiments.sh 产生的汇总 CSV
输出: 对应 run 目录下的 Markdown 报告（RTC / 离线统一使用 WER）

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
import json
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
    "adaptive_auto": "自适应",
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


def _split_text_units(text: str) -> List[str]:
    if not text:
        return []
    if "\n" in text:
        units: List[str] = []
        for line in text.splitlines():
            units.extend(_split_sentences(line))
        if units:
            return units
    return _split_sentences(text)


def _count_words(text: str) -> int:
    norm = _normalize_transcript(text)
    if not norm:
        return 0
    return len(norm.split())


def _detect_repeated_ngram(text: str) -> Optional[str]:
    tokens = _normalize_transcript(text).split()
    if len(tokens) < 12:
        return None
    for n in (8, 6, 4):
        if len(tokens) < n * 3:
            continue
        for i in range(0, len(tokens) - n * 3 + 1):
            gram = tokens[i : i + n]
            repeats = 1
            j = i + n
            while j + n <= len(tokens) and tokens[j : j + n] == gram:
                repeats += 1
                j += n
            if repeats >= 3:
                return f"repeated {n}-gram x{repeats}"
    return None


def _detect_asr_instability(ref_text: str, hyp_text: str) -> Optional[str]:
    ref_words = _count_words(ref_text)
    hyp_words = _count_words(hyp_text)
    if ref_words > 0 and hyp_words > max(int(ref_words * 1.6), ref_words + 20):
        return f"word_count {hyp_words}>{ref_words}"
    return _detect_repeated_ngram(hyp_text)


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
    global mlx_whisper
    if mlx_whisper is not None:
        return True
    try:
        import mlx_whisper as imported_mlx_whisper
    except ImportError:
        return False
    mlx_whisper = imported_mlx_whisper
    return True


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
) -> Tuple[Optional[float], Optional[str], Optional[str], Optional[str], Optional[str], bool, str]:
    if jiwer_wer is None:
        raise RuntimeError(
            "jiwer is not installed; use the repo .venv_asr Python or install the ASR dependencies."
        )
    if not input_wav or not output_wav:
        return None, None, None, None, None, False, ""
    if not os.path.isfile(input_wav) or not os.path.isfile(output_wav):
        return None, None, None, None, None, False, ""

    ref_text = transcript_cache.get(input_wav)
    if ref_text is None:
        ref_text = _load_reference_text(input_wav)
        if ref_text is None:
            ref_text, _ = _transcribe_audio_segments(input_wav, model_name, language)
        transcript_cache[input_wav] = ref_text

    hyp_text, _ = _transcribe_audio_segments(output_wav, model_name, language)
    if not ref_text:
        return None, ref_text, hyp_text, None, None, False, ""

    ref_text_norm = _normalize_transcript(ref_text)
    hyp_text_norm = _normalize_transcript(hyp_text)
    wer_value = jiwer_wer(ref_text_norm, hyp_text_norm)
    instability_reason = _detect_asr_instability(ref_text, hyp_text) or ""

    ref_path = None
    hyp_path = None
    if transcript_dir:
        ref_path = os.path.join(transcript_dir, audio_type, "reference.txt")
        hyp_path = os.path.join(transcript_dir, audio_type, scenario, f"{case}.txt")
        _write_text(ref_path, ref_text)
        _write_text(hyp_path, hyp_text)
    return wer_value, ref_text, hyp_text, ref_path, hyp_path, bool(instability_reason), instability_reason


def _load_adaptation_summary(path: str) -> dict:
    summary = {
        "switch_count": 0,
        "first_dred_seconds": None,
        "dred_duration_seconds": 0.0,
        "bitrate_bps": 0,
        "bitrate_tier": "-",
        "decision_class": "-",
        "dred_allowed": False,
        "dred_level_cap": "-",
        "entered_dred": False,
    }
    if not path or not os.path.isfile(path):
        return summary
    try:
        with open(path, "r", encoding="utf-8") as f:
            trace = json.load(f)
    except Exception:
        return summary

    samples = trace.get("samples") or []
    if not samples:
        return summary

    first_sample = samples[0]
    summary["bitrate_bps"] = int(first_sample.get("bitrate_bps") or 0)
    summary["bitrate_tier"] = first_sample.get("bitrate_tier") or "-"
    summary["dred_level_cap"] = first_sample.get("dred_level_cap") or "-"
    summary["dred_allowed"] = any(bool(sample.get("dred_allowed")) for sample in samples)
    for sample in reversed(samples):
        decision_class = sample.get("decision_class") or ""
        if decision_class:
            summary["decision_class"] = decision_class
            break

    parsed = []
    for sample in samples:
        timestamp = sample.get("timestamp")
        mode = sample.get("mode", "")
        if not timestamp:
            continue
        try:
            ts = datetime.fromisoformat(timestamp.replace("Z", "+00:00"))
        except ValueError:
            continue
        parsed.append((ts, mode))

    if not parsed:
        return summary

    start_ts = parsed[0][0]
    prev_mode = parsed[0][1]
    if prev_mode.startswith("dred_"):
        summary["first_dred_seconds"] = 0.0
        summary["entered_dred"] = True

    for idx in range(1, len(parsed)):
        ts, mode = parsed[idx]
        prev_ts, _ = parsed[idx - 1]
        interval = max(0.0, (ts - prev_ts).total_seconds())
        if prev_mode.startswith("dred_"):
            summary["dred_duration_seconds"] += interval
        if mode != prev_mode:
            summary["switch_count"] += 1
        if summary["first_dred_seconds"] is None and mode.startswith("dred_"):
            summary["first_dred_seconds"] = max(0.0, (ts - start_ts).total_seconds())
            summary["entered_dred"] = True
        prev_mode = mode
    return summary


def _load_sender_stats(path: str) -> dict:
    summary = {
        "bytes_sent": 0,
        "packets_sent": 0,
        "avg_payload_bytes": 0.0,
        "effective_payload_kbps": 0.0,
    }
    if not path or not os.path.isfile(path):
        return summary
    try:
        with open(path, "r", encoding="utf-8") as f:
            stats = json.load(f)
    except Exception:
        return summary

    summary["bytes_sent"] = int(stats.get("bytes_sent") or 0)
    summary["packets_sent"] = int(stats.get("packets_sent") or 0)
    if summary["packets_sent"] > 0:
        summary["avg_payload_bytes"] = summary["bytes_sent"] / summary["packets_sent"]
    summary["effective_payload_kbps"] = float(stats.get("effective_payload_kbps") or 0.0)
    return summary


def _group_best_wer(rows: List[dict], keys: Tuple[str, ...]) -> dict:
    best: dict = {}
    for row in rows:
        wer_text = row.get("wer", "")
        if not wer_text or wer_text == "-":
            continue
        try:
            wer_value = float(wer_text.rstrip("%"))
        except ValueError:
            continue
        group = tuple(row.get(key, "") for key in keys)
        current = best.get(group)
        if current is None or wer_value < current:
            best[group] = wer_value
    return best


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
        wer_value, ref_text, hyp_text, ref_path, hyp_path, asr_unstable, instability_reason = _stt_metrics(
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
        row["reference_text"] = ref_text or ""
        row["hypothesis_text"] = hyp_text or ""
        row["reference_text_path"] = ref_path or ""
        row["hypothesis_text_path"] = hyp_path or ""
        row["asr_unstable"] = "yes" if asr_unstable else "no"
        row["asr_unstable_reason"] = instability_reason
        adaptation_summary = _load_adaptation_summary(row.get("adaptation_json", ""))
        row["adapt_switch_count"] = str(adaptation_summary["switch_count"])
        first_dred = adaptation_summary["first_dred_seconds"]
        row["adapt_first_dred_seconds"] = "-" if first_dred is None else f"{first_dred:.1f}s"
        row["adapt_dred_duration_seconds"] = f"{adaptation_summary['dred_duration_seconds']:.1f}s"
        row["adapt_bitrate_bps"] = str(adaptation_summary["bitrate_bps"] or "-")
        row["adapt_bitrate_tier"] = adaptation_summary["bitrate_tier"]
        row["adapt_decision_class"] = adaptation_summary["decision_class"]
        row["adapt_dred_allowed"] = "yes" if adaptation_summary["dred_allowed"] else "no"
        row["adapt_dred_level_cap"] = adaptation_summary["dred_level_cap"]
        row["adapt_entered_dred"] = "yes" if adaptation_summary["entered_dred"] else "no"
        recovered_dred = int(row.get("recovered_dred", "0") or 0)
        if adaptation_summary["entered_dred"] and recovered_dred == 0:
            row["adapt_dred_effectiveness"] = "entered_dred_but_zero_recovery"
        elif recovered_dred > 0:
            row["adapt_dred_effectiveness"] = "effective"
        elif adaptation_summary["entered_dred"]:
            row["adapt_dred_effectiveness"] = "entered_dred"
        else:
            row["adapt_dred_effectiveness"] = "not_entered"
        sender_stats = _load_sender_stats(row.get("sender_stats_json", ""))
        row["tx_bytes"] = str(sender_stats["bytes_sent"])
        row["tx_packets_sent"] = str(sender_stats["packets_sent"])
        row["tx_avg_payload_bytes"] = f"{sender_stats['avg_payload_bytes']:.1f}" if sender_stats["packets_sent"] > 0 else "-"
        row["tx_effective_payload_kbps"] = f"{sender_stats['effective_payload_kbps']:.2f}" if sender_stats["effective_payload_kbps"] > 0 else "-"

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
    best_rtc_wer = _group_best_wer(rows, ("audio_type", "scenario"))
    has_adaptive_rows = any(r.get("case", r.get("name", "")) == "adaptive_auto" for r in rows)

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
    lines.append("- `WER` 是主评估指标。")
    lines.append("- WER 使用同一个 ASR 后端分别转写干净输入音频与 RTC 输出音频。")
    lines.append("- 干净输入音频的转写结果作为参考文本，因此这里衡量的是相对可懂度退化，而不是人工标注基准上的绝对识别率。")
    lines.append("- `ASR unstable` 表示输出文本出现明显插入膨胀或重复 n-gram 循环，这类 case 的 WER 仍保留，但应结合原始 transcript 复核。")
    lines.append("- `发送载荷字节` 基于 sender `sender_stats.json`，表示实验期间 sender 实际写出的 Opus payload 总字节数。")
    lines.append("- `平均载荷/包` 与 `有效载荷码率(kbps)` 也都来自 sender 侧统计。")
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

    if has_adaptive_rows:
        lines.append("## 控制轨迹摘要\n")
        lines.append("| 音频类型 | 丢包场景 | 保护策略 | 码率 | 档位 | 首次进入DRED | DRED持续时长 | 模式切换次数 |")
        lines.append("|---------|---------|---------|-----|-----|-------------|-------------|-------------|")
        for r in rows:
            at = r.get("audio_type", "")
            scenario = r.get("scenario", r.get("experiment", ""))
            case = r.get("case", r.get("name", ""))
            at_label = AUDIO_LABELS.get(at, at)
            s_label = SCENARIO_LABELS.get(scenario, scenario)
            c_label = CASE_LABELS.get(case, case)
            lines.append(
                f"| {at_label} | {s_label} | {c_label} | {r.get('adapt_bitrate_bps', '-')} | {r.get('adapt_bitrate_tier', '-')} | {r.get('adapt_first_dred_seconds', '-')} | "
                f"{r.get('adapt_dred_duration_seconds', '0.0s')} | {r.get('adapt_switch_count', '0')} |"
            )
        lines.append("")

        lines.append("## 低码率 DRED 有效性摘要\n")
        lines.append("| 音频类型 | 丢包场景 | 保护策略 | 决策类别 | DRED允许 | DRED上限 | 首次进入DRED | DRED恢复 | 状态 |")
        lines.append("|---------|---------|---------|---------|---------|---------|-------------|---------|------|")
        for r in rows:
            at = r.get("audio_type", "")
            scenario = r.get("scenario", r.get("experiment", ""))
            case = r.get("case", r.get("name", ""))
            at_label = AUDIO_LABELS.get(at, at)
            s_label = SCENARIO_LABELS.get(scenario, scenario)
            c_label = CASE_LABELS.get(case, case)
            lines.append(
                f"| {at_label} | {s_label} | {c_label} | {r.get('adapt_decision_class', '-')} | "
                f"{r.get('adapt_dred_allowed', 'no')} | {r.get('adapt_dred_level_cap', '-')} | {r.get('adapt_first_dred_seconds', '-')} | "
                f"{r.get('recovered_dred', '0')} | {r.get('adapt_dred_effectiveness', 'not_entered')} |"
            )
        lines.append("")

    lines.append("## 带宽 / WER 综合对比\n")
    lines.append("| 音频类型 | 丢包场景 | 保护策略 | 发送载荷字节 | 平均载荷/包(bytes) | 有效载荷码率(kbps) | WER | 恢复率 |")
    lines.append("|---------|---------|---------|-------------:|-------------------:|-------------------:|:---:|-------|")
    for r in rows:
        at = r.get("audio_type", "")
        scenario = r.get("scenario", r.get("experiment", ""))
        case = r.get("case", r.get("name", ""))
        at_label = AUDIO_LABELS.get(at, at)
        s_label = SCENARIO_LABELS.get(scenario, scenario)
        c_label = CASE_LABELS.get(case, case)
        wer_val = r.get("wer", "-")
        rate = r.get("recovery_rate", "0")
        rate_val = _safe_float(rate)
        rate_str = f"{rate_val:.2%}" if rate_val <= 1.0 else f"{rate_val:.2f}%"
        lines.append(
            f"| {at_label} | {s_label} | {c_label} | {r.get('tx_bytes', '0')} | "
            f"{r.get('tx_avg_payload_bytes', '-')} | {r.get('tx_effective_payload_kbps', '-')} | {wer_val} | {rate_str} |"
        )
    lines.append("")

    # ---- WER 表 ----
    lines.append("## 文本指标 (WER)\n")
    lines.append("| 音频类型 | 丢包场景 | 保护策略 | WER | ASR稳定性 |")
    lines.append("|---------|---------|---------|:---:|:---:|")
    for r in rows:
        at = r.get("audio_type", "")
        scenario = r.get("scenario", r.get("experiment", ""))
        case = r.get("case", r.get("name", ""))
        at_label = AUDIO_LABELS.get(at, at)
        s_label = SCENARIO_LABELS.get(scenario, scenario)
        c_label = CASE_LABELS.get(case, case)
        wer_val = r.get("wer", "-")
        asr_state = "unstable" if r.get("asr_unstable") == "yes" else "ok"
        try:
            wer_num = float(str(wer_val).rstrip("%"))
        except ValueError:
            wer_num = None
        best_wer = best_rtc_wer.get((at, scenario))
        if wer_num is not None and best_wer is not None and abs(wer_num - best_wer) < 1e-9:
            wer_val = f"**{wer_val}**"
        lines.append(f"| {at_label} | {s_label} | {c_label} | {wer_val} | {asr_state} |")
    lines.append("")

    lines.append("## 音频工件对照\n")
    if has_adaptive_rows:
        lines.append("| 音频类型 | 丢包场景 | 保护策略 | 输入音频 | 输出音频 | 参考转写 | 输出转写 | 统计 JSON | 自适应轨迹 |")
        lines.append("|---------|---------|---------|---------|---------|---------|---------|---------|---------|")
    else:
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
        adaptation_ref = _artifact_link(output_path, r.get("adaptation_json", ""))
        if has_adaptive_rows:
            lines.append(
                f"| {at_label} | {s_label} | {c_label} | {input_ref} | {output_ref} | "
                f"{ref_text_ref} | {hyp_text_ref} | {stats_ref} | {adaptation_ref} |"
            )
        else:
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
        wer_value, ref_text, hyp_text, ref_path, hyp_path, asr_unstable, instability_reason = _stt_metrics(
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
        row["reference_text"] = ref_text or ""
        row["hypothesis_text"] = hyp_text or ""
        row["reference_text_path"] = ref_path or ""
        row["hypothesis_text_path"] = hyp_path or ""
        row["asr_unstable"] = "yes" if asr_unstable else "no"
        row["asr_unstable_reason"] = instability_reason
    best_opus_wer = _group_best_wer(rows, ("audio_type", "experiment"))

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
    lines.append("- `WER` 是主评估指标。")
    lines.append("- WER 使用同一个 ASR 后端分别转写干净输入音频与离线输出音频。")
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

    lines.append("## 文本指标 (WER)\n")
    lines.append("| 音频类型 | 实验 | 方案 | WER | ASR稳定性 |")
    lines.append("|---------|------|------|:---:|:---:|")
    for r in rows:
        at = r.get("audio_type", "")
        at_label = AUDIO_LABELS.get(at, at)
        exp = r.get("experiment", "")
        name = r.get("name", "")
        wer_val = r.get("wer", "-")
        asr_state = "unstable" if r.get("asr_unstable") == "yes" else "ok"
        try:
            wer_num = float(str(wer_val).rstrip("%"))
        except ValueError:
            wer_num = None
        best_wer = best_opus_wer.get((at, exp))
        if wer_num is not None and best_wer is not None and abs(wer_num - best_wer) < 1e-9:
            wer_val = f"**{wer_val}**"
        lines.append(f"| {at_label} | {exp} | {name} | {wer_val} | {asr_state} |")
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
