#!/usr/bin/env python3
import math
import struct
import wave


def main() -> None:
    sample_rate = 48000
    duration_s = 3
    freq_hz = 440.0
    amplitude = 0.3
    total = sample_rate * duration_s

    with wave.open("tone_48k_mono.wav", "wb") as wf:
        wf.setnchannels(1)
        wf.setsampwidth(2)
        wf.setframerate(sample_rate)
        for i in range(total):
            t = i / sample_rate
            value = int(32767.0 * amplitude * math.sin(2.0 * math.pi * freq_hz * t))
            wf.writeframes(struct.pack("<h", value))


if __name__ == "__main__":
    main()
