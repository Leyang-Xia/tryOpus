#!/usr/bin/env python3
"""
gen_audio.py - 生成测试音频WAV文件

支持以下类型:
  sine      - 纯正弦波（默认440Hz）
  speech    - 模拟语音信号（多正弦叠加 + 幅度包络）
  chirp     - 线性调频信号
  noise     - 高斯白噪声
  mixed     - 混合信号（语音+噪声）

用法:
  python3 gen_audio.py                    # 生成所有测试文件
  python3 gen_audio.py --type speech --out my_speech.wav
  python3 gen_audio.py --type sine --freq 1000 --duration 5
"""

import struct
import math
import random
import argparse
import os

def write_wav(filename: str, samples: list, sample_rate: int = 48000,
              channels: int = 1, bits: int = 16):
    """写入WAV文件（16-bit PCM）"""
    num_samples = len(samples) // channels
    data_size   = num_samples * channels * (bits // 8)
    with open(filename, 'wb') as f:
        # RIFF header
        f.write(b'RIFF')
        f.write(struct.pack('<I', 36 + data_size))
        f.write(b'WAVE')
        # fmt chunk
        f.write(b'fmt ')
        f.write(struct.pack('<IHHIIHH',
                            16,          # chunk size
                            1,           # PCM
                            channels,
                            sample_rate,
                            sample_rate * channels * (bits // 8),  # byte_rate
                            channels * (bits // 8),                # block_align
                            bits))
        # data chunk
        f.write(b'data')
        f.write(struct.pack('<I', data_size))
        for s in samples:
            # Clamp and convert to int16
            s_clamp = max(-32768, min(32767, int(s)))
            f.write(struct.pack('<h', s_clamp))
    print(f"已生成: {filename}  ({sample_rate}Hz, {channels}ch, "
          f"{num_samples/sample_rate:.1f}s, {data_size} bytes)")


def gen_sine(sample_rate: int, duration: float, freq: float = 440.0,
             amplitude: float = 0.8) -> list:
    """生成纯正弦波"""
    n = int(sample_rate * duration)
    max_amp = int(32767 * amplitude)
    return [int(max_amp * math.sin(2 * math.pi * freq * i / sample_rate))
            for i in range(n)]


def gen_speech_like(sample_rate: int, duration: float,
                    amplitude: float = 0.7) -> list:
    """
    模拟语音信号：
    - 多个谐波叠加（模拟基频+泛音）
    - 加上幅度包络（模拟韵律/停顿）
    - 加少量噪声
    """
    n = int(sample_rate * duration)
    max_amp = 32767 * amplitude
    samples = []

    # 基频序列（模拟音调变化）
    fundamental_freqs = [120, 150, 180, 200, 160, 140, 130, 120]
    segment_len = n // len(fundamental_freqs)

    rng = random.Random(42)

    for i in range(n):
        seg_idx = min(i // segment_len, len(fundamental_freqs) - 1)
        f0 = fundamental_freqs[seg_idx]

        # 谐波叠加 (基频 + 2~8次谐波，幅度衰减)
        s = 0.0
        harmonics = [1.0, 0.7, 0.5, 0.35, 0.25, 0.18, 0.12, 0.08]
        for k, amp in enumerate(harmonics):
            s += amp * math.sin(2 * math.pi * f0 * (k + 1) * i / sample_rate)
        s /= sum(harmonics)  # 归一化

        # 幅度包络：模拟语音的韵律（每0.5秒有短暂停顿）
        pos_in_seg = i % (sample_rate // 2)
        if pos_in_seg < 50:
            env = pos_in_seg / 50.0  # 攻击
        elif pos_in_seg > sample_rate // 2 - 50:
            env = (sample_rate // 2 - pos_in_seg) / 50.0  # 释放
        else:
            env = 1.0

        # 添加少量白噪声
        noise = rng.gauss(0, 0.02)
        s = (s * env + noise) * max_amp
        samples.append(s)

    return samples


def gen_chirp(sample_rate: int, duration: float,
              f_start: float = 200.0, f_end: float = 4000.0,
              amplitude: float = 0.8) -> list:
    """线性调频（chirp）信号，用于频率响应测试"""
    n = int(sample_rate * duration)
    max_amp = int(32767 * amplitude)
    samples = []
    for i in range(n):
        t = i / sample_rate
        # 瞬时频率线性增加
        f_inst = f_start + (f_end - f_start) * t / duration
        phase = 2 * math.pi * (f_start * t +
                                (f_end - f_start) * t * t / (2 * duration))
        samples.append(int(max_amp * math.sin(phase)))
    return samples


def gen_noise(sample_rate: int, duration: float,
              amplitude: float = 0.5) -> list:
    """高斯白噪声"""
    n = int(sample_rate * duration)
    max_amp = 32767 * amplitude
    rng = random.Random(123)
    return [int(rng.gauss(0, max_amp)) for _ in range(n)]


def gen_mixed(sample_rate: int, duration: float) -> list:
    """语音 + 背景噪声混合"""
    speech = gen_speech_like(sample_rate, duration, amplitude=0.65)
    noise  = gen_noise(sample_rate, duration, amplitude=0.15)
    return [s + n for s, n in zip(speech, noise)]


def main():
    parser = argparse.ArgumentParser(description='生成Opus测试音频')
    parser.add_argument('--type', default='all',
                        choices=['sine', 'speech', 'chirp', 'noise',
                                 'mixed', 'all'],
                        help='音频类型 (默认: all)')
    parser.add_argument('--out', default='',     help='输出文件名')
    parser.add_argument('--freq', type=float, default=440.0,
                        help='正弦波频率 (默认: 440 Hz)')
    parser.add_argument('--duration', type=float, default=10.0,
                        help='时长（秒，默认: 10）')
    parser.add_argument('--rate', type=int, default=48000,
                        help='采样率 (默认: 48000)')
    parser.add_argument('--outdir', default='audio',
                        help='输出目录 (默认: audio)')
    args = parser.parse_args()

    os.makedirs(args.outdir, exist_ok=True)
    sr  = args.rate
    dur = args.duration

    if args.type == 'all' or not args.out:
        # 生成所有测试文件
        write_wav(f'{args.outdir}/sine_440hz.wav',
                  gen_sine(sr, dur, 440.0), sr)
        write_wav(f'{args.outdir}/sine_1khz.wav',
                  gen_sine(sr, dur, 1000.0), sr)
        write_wav(f'{args.outdir}/speech_like.wav',
                  gen_speech_like(sr, dur), sr)
        write_wav(f'{args.outdir}/chirp_200_4000.wav',
                  gen_chirp(sr, dur, 200, 4000), sr)
        write_wav(f'{args.outdir}/noise.wav',
                  gen_noise(sr, dur), sr)
        write_wav(f'{args.outdir}/mixed.wav',
                  gen_mixed(sr, dur), sr)
        # 低采样率版本（16kHz，模拟窄带语音）
        write_wav(f'{args.outdir}/speech_16k.wav',
                  gen_speech_like(16000, dur), 16000)
        print(f"\n所有测试文件已生成到 {args.outdir}/ 目录")
    else:
        # 生成指定类型
        out = args.out if args.out else f'{args.outdir}/{args.type}.wav'
        gen_map = {
            'sine':   lambda: gen_sine(sr, dur, args.freq),
            'speech': lambda: gen_speech_like(sr, dur),
            'chirp':  lambda: gen_chirp(sr, dur),
            'noise':  lambda: gen_noise(sr, dur),
            'mixed':  lambda: gen_mixed(sr, dur),
        }
        samples = gen_map[args.type]()
        write_wav(out, samples, sr)


if __name__ == '__main__':
    main()
