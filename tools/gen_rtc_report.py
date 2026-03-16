#!/usr/bin/env python3
"""
gen_rtc_report.py - 从实验汇总 CSV 生成 Markdown 报告

输入: run_rtc_experiments.sh 或 run_experiments.sh 产生的汇总 CSV
输出: results/rtc_report.md（恢复策略表 + SNR/SegSNR 表）

CSV 列（RTC）:
  audio_type,scenario,case,sim_mode,sim_loss,
  recovered_lbrr,recovered_dred,plc,decode_errors,recovery_rate,
  input_wav,output_wav

CSV 列（离线仿真）:
  audio_type,experiment,name,loss_model,loss_param,
  recovered_lbrr,recovered_dred,plc,recovery_rate,
  input_wav,output_wav
"""

import argparse
import csv
import math
import os
import struct
import sys
from datetime import datetime
from typing import List, Tuple, Optional


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
    "music": "音乐 (SoundHelix)",
    "news": "新闻播报 (BBC)",
    "dialogue": "对话场景 (餐厅)",
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


def generate_rtc_report(csv_path: str, output_path: str) -> None:
    rows: list = []
    with open(csv_path, "r", encoding="utf-8") as f:
        reader = csv.DictReader(f)
        for row in reader:
            rows.append(row)

    if not rows:
        print("[report] no data rows in CSV, skipping report", file=sys.stderr)
        return

    for row in rows:
        input_wav = row.get("input_wav", "")
        output_wav = row.get("output_wav", "")
        snr, segsnr = _try_snr(input_wav, output_wav)
        row["snr"] = f"{snr:.2f}" if snr is not None else "-"
        row["segsnr"] = f"{segsnr:.2f}" if segsnr is not None else "-"

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

    # ---- SNR / SegSNR 表 ----
    lines.append("## 音质指标 (SNR / SegSNR)\n")
    lines.append("| 音频类型 | 丢包场景 | 保护策略 | 全局 SNR (dB) | 分段 SegSNR (dB) |")
    lines.append("|---------|---------|---------|:------------:|:---------------:|")
    for r in rows:
        at = r.get("audio_type", "")
        scenario = r.get("scenario", r.get("experiment", ""))
        case = r.get("case", r.get("name", ""))
        at_label = AUDIO_LABELS.get(at, at)
        s_label = SCENARIO_LABELS.get(scenario, scenario)
        c_label = CASE_LABELS.get(case, case)
        snr = r.get("snr", "-")
        segsnr = r.get("segsnr", "-")
        lines.append(f"| {at_label} | {s_label} | {c_label} | {snr} | {segsnr} |")
    lines.append("")

    os.makedirs(os.path.dirname(output_path) or ".", exist_ok=True)
    with open(output_path, "w", encoding="utf-8") as f:
        f.write("\n".join(lines))
    print(f"[report] generated: {output_path}")


def generate_opus_report(csv_path: str, output_path: str) -> None:
    """Generate report for offline opus_sim experiments."""
    rows: list = []
    with open(csv_path, "r", encoding="utf-8") as f:
        reader = csv.DictReader(f)
        for row in reader:
            rows.append(row)

    if not rows:
        print("[report] no data rows in CSV, skipping report", file=sys.stderr)
        return

    for row in rows:
        input_wav = row.get("input_wav", "")
        output_wav = row.get("output_wav", "")
        snr, segsnr = _try_snr(input_wav, output_wav)
        row["snr"] = f"{snr:.2f}" if snr is not None else "-"
        row["segsnr"] = f"{segsnr:.2f}" if segsnr is not None else "-"

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

    lines.append("## 音质指标 (SNR / SegSNR)\n")
    lines.append("| 音频类型 | 实验 | 方案 | 全局 SNR (dB) | 分段 SegSNR (dB) |")
    lines.append("|---------|------|------|:------------:|:---------------:|")
    for r in rows:
        at = r.get("audio_type", "")
        at_label = AUDIO_LABELS.get(at, at)
        exp = r.get("experiment", "")
        name = r.get("name", "")
        snr = r.get("snr", "-")
        segsnr = r.get("segsnr", "-")
        lines.append(f"| {at_label} | {exp} | {name} | {snr} | {segsnr} |")
    lines.append("")

    os.makedirs(os.path.dirname(output_path) or ".", exist_ok=True)
    with open(output_path, "w", encoding="utf-8") as f:
        f.write("\n".join(lines))
    print(f"[report] generated: {output_path}")


def main() -> None:
    parser = argparse.ArgumentParser(description="Generate Markdown experiment report from CSV")
    parser.add_argument("--csv", required=True, help="Summary CSV path")
    parser.add_argument("--output", required=True, help="Output Markdown path")
    parser.add_argument("--mode", default="rtc", choices=["rtc", "opus"],
                        help="Report mode: rtc (WebRTC) or opus (offline sim)")
    args = parser.parse_args()

    if args.mode == "rtc":
        generate_rtc_report(args.csv, args.output)
    else:
        generate_opus_report(args.csv, args.output)


if __name__ == "__main__":
    main()
