#!/usr/bin/env python3
"""
analyze.py - Opus仿真结果辅助分析

功能:
  1. 解析仿真统计CSV文件，绘制丢包分布、恢复情况
  2. 计算两段音频之间的 WER / SER
  3. 辅助对比实验目录中的旧式结果

用法:
  python3 analyze.py --csv results/offline_runs/manual/dred3_10.csv
  python3 analyze.py --ref representative_audio/dialogue/dialogue_30s_48k_mono.wav --deg results/offline_runs/manual/dred3_10.wav
"""

import sys
import os
import argparse
import struct
import math
import csv

try:
    from gen_rtc_report import _stt_metrics
except ImportError:
    _stt_metrics = None

# ---- WAV读取 ----
def read_wav(path: str):
    """读取WAV文件，返回 (samples_list, sample_rate, channels)"""
    with open(path, 'rb') as f:
        # 读取RIFF头
        riff   = f.read(4)
        if riff != b'RIFF':
            raise ValueError(f"非RIFF文件: {path}")
        f.read(4)  # file size
        wave = f.read(4)
        if wave != b'WAVE':
            raise ValueError(f"非WAVE格式: {path}")

        # 查找fmt和data块
        sr = ch = 0
        bits = 16
        while True:
            chunk_id   = f.read(4)
            if len(chunk_id) < 4:
                break
            chunk_size = struct.unpack('<I', f.read(4))[0]
            if chunk_id == b'fmt ':
                fmt_data = f.read(chunk_size)
                fmt = struct.unpack('<HHIIHH', fmt_data[:16])
                ch   = fmt[1]
                sr   = fmt[2]
                bits = fmt[5]
                if chunk_size > 16:
                    pass  # 跳过额外字节（已包含在读取中）
            elif chunk_id == b'data':
                raw = f.read(chunk_size)
                if bits == 16:
                    n = chunk_size // 2
                    samples = list(struct.unpack(f'<{n}h', raw))
                else:
                    raise ValueError(f"不支持的位深: {bits}")
                break
            else:
                f.read(chunk_size)

    if ch > 1:
        # 取第一声道
        samples = samples[::ch]
    return samples, sr, ch


# ---- 信号质量指标 ----
def compute_snr(ref: list, deg: list) -> float:
    """计算信噪比(dB)"""
    n = min(len(ref), len(deg))
    if n == 0:
        return 0.0
    signal_power = sum(r * r for r in ref[:n]) / n
    noise_power  = sum((r - d) ** 2 for r, d in zip(ref[:n], deg[:n])) / n
    if noise_power < 1e-10:
        return 99.0
    return 10 * math.log10(signal_power / noise_power)


def compute_segsnr(ref: list, deg: list, sr: int,
                   frame_ms: int = 20) -> float:
    """分段SNR（更接近主观质量评估）"""
    frame_len = sr * frame_ms // 1000
    n = min(len(ref), len(deg))
    seg_snrs = []
    for i in range(0, n - frame_len, frame_len):
        r_seg = ref[i:i + frame_len]
        d_seg = deg[i:i + frame_len]
        sp = sum(x * x for x in r_seg) / frame_len
        np_ = sum((r - d) ** 2 for r, d in zip(r_seg, d_seg)) / frame_len
        if sp > 100 and np_ > 0:  # 只计算有信号的段
            seg_snrs.append(10 * math.log10(sp / np_))
    if not seg_snrs:
        return 0.0
    # 截断到 [-10, 35] dB 范围
    seg_snrs = [max(-10.0, min(35.0, s)) for s in seg_snrs]
    return sum(seg_snrs) / len(seg_snrs)


def compute_lsd(ref: list, deg: list, sr: int) -> float:
    """
    Log-Spectral Distance (LSD, dB)
    使用简单的DFT帧级比较
    """
    frame_len = 320  # ~6.7ms @48kHz
    n = min(len(ref), len(deg))
    lsds = []

    def simple_dft_mag(seg):
        N = len(seg)
        mags = []
        for k in range(N // 2):
            re = sum(seg[i] * math.cos(2 * math.pi * k * i / N)
                     for i in range(N)) / N
            im = sum(seg[i] * math.sin(2 * math.pi * k * i / N)
                     for i in range(N)) / N
            mags.append(math.sqrt(re * re + im * im) + 1e-6)
        return mags

    for i in range(0, min(n, 10 * frame_len) - frame_len, frame_len):
        r_seg = ref[i:i + frame_len]
        d_seg = deg[i:i + frame_len]
        r_mag = simple_dft_mag(r_seg)
        d_mag = simple_dft_mag(d_seg)
        lsd = math.sqrt(sum((20 * math.log10(r / d)) ** 2
                            for r, d in zip(r_mag, d_mag)) / len(r_mag))
        lsds.append(lsd)

    return sum(lsds) / len(lsds) if lsds else 0.0


# ---- CSV统计分析 ----
def analyze_csv(csv_path: str):
    """解析仿真统计CSV，输出统计信息并生成ASCII图表"""
    frames = []
    with open(csv_path, 'r') as f:
        reader = csv.DictReader(f)
        for row in reader:
            frames.append({
                'frame':      int(row['frame']),
                'size_bytes': int(row['size_bytes']),
                'lost':       int(row['lost']),
                'status':     row['status'].strip(),
            })

    total = len(frames)
    lost  = sum(1 for f in frames if f['lost'])
    recv  = total - lost
    status_counts = {}
    for f in frames:
        s = f['status']
        status_counts[s] = status_counts.get(s, 0) + 1

    avg_size = (sum(f['size_bytes'] for f in frames if not f['lost']) /
                max(recv, 1))

    print(f"\n===== 仿真统计报告 =====")
    print(f"总帧数: {total}  丢包: {lost} ({100.0*lost/total:.1f}%)  收包: {recv}")
    print(f"平均包大小 (收包): {avg_size:.1f} bytes")
    print(f"\n恢复情况:")
    for st, cnt in sorted(status_counts.items()):
        bar = '#' * (cnt * 40 // max(total, 1))
        print(f"  {st:<8}: {cnt:5d} ({100.0*cnt/total:5.1f}%) |{bar}|")

    # 丢包分布（每50帧统计一次）
    print(f"\n丢包分布（每50帧）:")
    block_size = 50
    for start in range(0, total, block_size):
        block = frames[start:start + block_size]
        blost = sum(1 for f in block if f['lost'])
        rate  = blost / len(block)
        bar   = '#' * int(rate * 20)
        print(f"  帧{start:4d}-{min(start+block_size-1,total-1):4d}: "
              f"{100*rate:5.1f}% |{bar:<20}|")

    # 包大小分布
    sizes = [f['size_bytes'] for f in frames if not f['lost'] and f['size_bytes'] > 0]
    if sizes:
        buckets = {}
        for s in sizes:
            b = (s // 50) * 50
            buckets[b] = buckets.get(b, 0) + 1
        print(f"\n包大小分布 (收包):")
        for b in sorted(buckets.keys()):
            bar = '#' * (buckets[b] * 30 // len(sizes))
            print(f"  {b:4d}-{b+49:4d}B: {buckets[b]:4d} |{bar}|")


# ---- 对比分析 ----
def compare_experiment(results_dir: str):
    """
    对比目录下所有CSV+WAV文件，输出综合对比表格
    期望文件命名规范: {scheme}_{loss}.csv / {scheme}_{loss}.wav
    例如: plc_10.csv, lbrr_10.csv, dred_10.csv
    """
    print("\n===== 方案对比 =====")
    print(f"{'方案':<20} {'丢包率':<8} {'SNR(dB)':<10} {'SegSNR(dB)':<12} {'LBRR恢':<8} {'DRED恢':<8}")
    print("-" * 70)

    ref_path = os.path.join(results_dir, 'reference.wav')
    if not os.path.exists(ref_path):
        print(f"警告: 找不到参考文件 {ref_path}")
        return

    ref_samples, ref_sr, _ = read_wav(ref_path)

    for fname in sorted(os.listdir(results_dir)):
        if not fname.endswith('.wav') or fname == 'reference.wav':
            continue
        wav_path = os.path.join(results_dir, fname)
        csv_path = wav_path.replace('.wav', '.csv')

        try:
            deg_samples, deg_sr, _ = read_wav(wav_path)
        except Exception as e:
            print(f"  {fname}: 读取失败 ({e})")
            continue

        snr     = compute_snr(ref_samples, deg_samples)
        seg_snr = compute_segsnr(ref_samples, deg_samples, ref_sr)

        lbrr_cnt = dred_cnt = 0
        loss_rate = 0.0
        if os.path.exists(csv_path):
            with open(csv_path) as f:
                reader = csv.DictReader(f)
                rows = list(reader)
            total = len(rows)
            lost  = sum(1 for r in rows if int(r['lost']))
            lbrr_cnt = sum(1 for r in rows if r['status'].strip() == 'LBRR')
            dred_cnt = sum(1 for r in rows if r['status'].strip() == 'DRED')
            loss_rate = 100.0 * lost / max(total, 1)

        scheme = fname.replace('.wav', '')
        print(f"  {scheme:<20} {loss_rate:5.1f}%   {snr:8.1f}   {seg_snr:10.1f}   "
              f"{lbrr_cnt:6d}   {dred_cnt:6d}")


def main():
    parser = argparse.ArgumentParser(description='Opus仿真结果分析')
    parser.add_argument('--csv',     help='统计CSV文件')
    parser.add_argument('--ref',     help='参考WAV文件（原始音频）')
    parser.add_argument('--deg',     help='降质WAV文件（解码输出）')
    parser.add_argument('--compare', help='对比分析目录')
    parser.add_argument('--stt-model', default=os.environ.get('RTC_STT_MODEL', 'small.en'),
                        help='Whisper model name for transcript-based metrics')
    parser.add_argument('--stt-language', default=os.environ.get('RTC_STT_LANGUAGE'),
                        help='Optional forced language for Whisper transcription')
    args = parser.parse_args()

    if args.csv:
        analyze_csv(args.csv)

    if args.ref and args.deg:
        print(f"\n===== 文本指标对比 (WER / SER) =====")
        if _stt_metrics is None:
            print("错误: 无法导入 transcript 评估逻辑，请使用仓库内置环境运行。")
            return
        try:
            wer, ser, ref_text, hyp_text, _, _ = _stt_metrics(
                args.ref,
                args.deg,
                "analysis",
                "manual",
                os.path.splitext(os.path.basename(args.deg))[0],
                None,
                args.stt_model,
                args.stt_language,
                {},
            )
        except Exception as e:
            print(f"错误: {e}")
            return

        print(f"参考文件 : {args.ref}")
        print(f"测试文件 : {args.deg}")
        print(f"WER      : {wer:.2%}" if wer is not None else "WER      : -")
        print(f"SER      : {ser:.2%}" if ser is not None else "SER      : -")
        print(f"参考转写 : {ref_text or '-'}")
        print(f"输出转写 : {hyp_text or '-'}")

    if args.compare:
        compare_experiment(args.compare)

    if not any([args.csv, args.ref, args.compare]):
        parser.print_help()


if __name__ == '__main__':
    main()
