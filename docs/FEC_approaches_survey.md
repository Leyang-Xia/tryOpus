# Forward Error Correction (FEC) in Audio Codecs and Real-Time Audio Systems

A comprehensive technical survey covering codec-level, transport-level, and emerging FEC approaches, with comparison to Opus's mechanisms.

---

## Table of Contents

1. [Codec-Level FEC Approaches](#1-codec-level-fec-approaches)
2. [Transport-Level FEC for Audio](#2-transport-level-fec-for-audio)
3. [Emerging / Research FEC Approaches](#3-emerging--research-fec-approaches)
4. [Comparison with Opus's Approach](#4-comparison-with-opuss-approach)
5. [Summary Table](#5-summary-table)

---

## 1. Codec-Level FEC Approaches

### 1.1 AMR / AMR-WB

**Overview.** The Adaptive Multi-Rate (AMR, 3GPP TS 26.071) and AMR Wideband (AMR-WB, 3GPP TS 26.190) codecs do not define a codec-internal FEC mechanism comparable to Opus LBRR. Instead, they rely on three complementary strategies at the codec/payload level:

**Unequal Error Protection (UEP).** AMR classifies each frame's bits into sensitivity classes (Class A / B / C). Class A bits are the most sensitive—a single error renders the frame unusable—so the radio-layer or transport-layer applies stronger protection (e.g., convolutional codes, CRC) to Class A while applying lighter or no protection to Class C. The RTP payload format (RFC 4867) supports both *bandwidth-efficient* and *octet-aligned* modes; the latter includes per-frame CRC for UEP-style detection over IP.

**Codec Mode Request (CMR) and Rate Adaptation.** AMR/AMR-WB can switch between 8–9 modes (AMR: 4.75–12.2 kbps; AMR-WB: 6.6–23.85 kbps) on a frame-by-frame basis. In poor channel conditions, the system drops to a lower mode, freeing bits for stronger channel coding or reducing the impact of residual errors. CMR is carried in the RTP payload header (4-bit field).

**Application-Layer Redundancy via RFC 2198.** For VoIP over IP, AMR/AMR-WB commonly uses RFC 2198 RED to carry a redundant copy of the previous frame (at the same or lower codec mode) in the current packet. This doubles the gross bitrate but provides single-packet-loss recovery. This is the primary FEC mechanism used in practice for AMR over VoLTE/VoWiFi.

**Frame-Independent Design.** AMR's CELP design encodes LSP coefficients without inter-frame prediction and uses product-gain encoding (global gain × sub-frame corrections) to minimize inter-frame dependency, making each frame more independently decodable and reducing error propagation.

| Parameter | Value |
|---|---|
| Bitrate range | AMR: 4.75–12.2 kbps; AMR-WB: 6.6–23.85 kbps |
| Frame size | 20 ms |
| Redundancy overhead (RFC 2198) | ~100% (full duplicate) or ~60% (lower-mode copy) |
| Recovery capability | 1 packet (with RFC 2198 RED) |
| Latency impact | +20 ms (one frame buffering for RED) |
| Computational complexity | Very low (integer CELP) |

---

### 1.2 EVS (Enhanced Voice Services) Channel-Aware Mode

**Overview.** The EVS codec (3GPP TS 26.445) introduced a purpose-built *channel-aware mode (CAM)* that embeds partial redundancy directly in the bitstream—a true codec-level FEC, architecturally analogous to Opus LBRR but more sophisticated.

**Mechanism.** In CAM, the encoder partitions the total bitrate budget between the *primary frame encoding* and a *partial redundancy (pRED)* representation of a previous frame. The pRED data is a re-encoded version of the prior frame at reduced quality, not a simple bit copy. The encoder uses a modified quantization scheme for the core encoding to manage quality reduction when accommodating redundancy.

**Bitrate Modes.**
- The standard defines a 13.2 kbps channel-aware mode as the baseline.
- Research extensions propose modes at 9.6 kbps and other rates for wideband (WB) and super-wideband (SWB).
- The total bitrate envelope remains the same (e.g., 13.2 kbps total); the split between primary and redundancy is dynamically adjusted.

**Adaptation.** EVS CAM uses real-time channel feedback (estimated packet loss rate and jitter) to decide:
1. Whether to activate CAM at all (it is optional; clean channels use full-rate primary encoding).
2. How to split bits between primary and redundancy.
3. Which prior frame(s) to protect.

**Performance.** PESQ evaluations show CAM at 13.2 kbps achieves robustness comparable to running the codec at a lower rate with full application-layer redundancy (RFC 2198), but at lower total bandwidth. For degraded channels, EVS at 9.6 kbps with 100% app-layer redundancy (~19.2 kbps total) demonstrates significantly higher resilience at ≥3% PLR compared to non-redundant modes.

| Parameter | Value |
|---|---|
| Standard CAM bitrate | 13.2 kbps (total, primary + pRED combined) |
| Redundancy depth | 1 frame (partial re-encoding of previous frame) |
| Overhead model | Zero additional overhead (within the bitrate envelope) |
| Recovery quality | Lower than primary quality but significantly better than PLC |
| Latency impact | Minimal (no extra buffering beyond normal 20 ms framing) |
| Adaptation | Dynamic, based on channel state feedback |

---

### 1.3 Lyra / SoundStream and Neural Audio Codecs

**Overview.** Google's Lyra (v1: 3 kbps, v2/SoundStream: 3–18 kbps) and similar neural audio codecs (Encodec by Meta, DAC, APCodec) use an autoencoder architecture with Residual Vector Quantization (RVQ). These codecs do not define explicit FEC mechanisms in their published specifications. However, their architecture has implications for error resilience:

**Inherent Resilience Properties.**
- Neural codecs operate on short frames (typically 20–40 ms) with quantized latent representations. Each frame's latent code is relatively self-contained, reducing inter-frame error propagation compared to traditional CELP with adaptive codebook feedback.
- SoundStream/Lyra v2 uses structured dropout during training to support variable bitrate from a single model (dropping higher RVQ layers). This same mechanism could be repurposed: transmitting only base-layer quantizer indices as "FEC" redundancy for prior frames, at very low overhead.
- The learned latent space is compact (e.g., SoundStream: ~20–80 scalars per 20 ms frame at 3–18 kbps), making it inherently more efficient to transmit redundancy compared to waveform-domain codecs.

**Emerging FEC for Neural Codecs.** A 2024 Interspeech paper ("On Improving Error Resilience of Neural End-to-End Speech Coders," Gupta et al.) proposed two complementary methods:
1. **Latent-space PLC:** A lightweight network predicts the codebook indices of a missing frame from past received latent vectors. No extra bitrate needed.
2. **In-band FEC:** The encoder transmits a compressed secondary representation of the previous frame's latent code at only **0.8 kbps additional overhead**, enabling high-quality recovery when combined with the PLC network.

This approach achieves significant robustness gains with negligible bitrate cost because the redundancy operates in the already-compressed latent domain rather than the waveform domain.

| Parameter | Value |
|---|---|
| Base codec bitrate | 3–18 kbps (SoundStream); 3 kbps (Lyra v1) |
| In-band FEC overhead | ~0.8 kbps (Gupta et al., 2024) |
| Recovery depth | 1 frame (latent prediction + FEC) |
| Computational complexity | High (GPU or NPU for real-time; ~30 MFLOPS–1 GFLOPS) |
| Latency | 20–40 ms algorithmic + inference time |

---

### 1.4 WebRTC's Approach to Audio FEC

WebRTC employs a layered FEC strategy combining codec-level and transport-level mechanisms:

#### 1.4.1 Opus In-Band FEC (LBRR)

The default audio FEC in WebRTC. When packet loss is signaled to the Opus encoder via `OPUS_SET_PACKET_LOSS_PERC`, the SILK layer generates **Low Bit-Rate Redundancy (LBRR)** frames—a requantized copy of the previous frame at reduced quality embedded within the current packet.

**Key characteristics:**
- Redundancy depth: **1 frame only** (the immediately preceding frame).
- Overhead: Uses approximately **2/3 of the regular packet's bitrate** for the FEC copy. The encoder reduces primary-frame quality to stay within the target bitrate.
- Activation threshold: Depends on bandwidth mode; at 40 kbps mono, ~12 kbps minimum for single-channel FEC activation.
- Quantization gain: Receives a boost that decreases from ~3× at 0% declared loss to ~1.37× at 12.5% declared loss.
- Recovery quality: Moderate—the LBRR frame is encoded at lower quality than the primary frame.
- Effective range: Meaningful improvement at **~4–10% packet loss**; diminishing returns above that.
- Decoder delay: Requires **one additional frame of buffering** (20 ms typical) because FEC data for frame N is carried in frame N+1.
- Applicability: **SILK mode only** (typically ≤ 32 kbps, VOIP application type). Does not protect CELT-mode frames.

#### 1.4.2 RED (RFC 2198)

WebRTC can wrap Opus packets in RED (Redundant Audio Data) payloads, carrying one or more previous packets in the current RTP packet. This is a transport-level mechanism transparent to the codec.

- Typically carries **1 redundant copy** (N-1 in packet N).
- Overhead: ~100% for full redundancy, or less if the redundant copy uses a lower bitrate.
- Works for any codec (not just Opus); the SFU can add/strip RED as needed.
- Recovery depth: 1 packet per redundancy level (can nest for 2+ but rarely done for audio).
- WebRTC SFUs (Selective Forwarding Units) can selectively apply RED to active speaker streams to save bandwidth.

#### 1.4.3 FlexFEC (for Video; Limited Audio Use)

FlexFEC (RFC 8627) is primarily used for video (H.264/VP8/VP9) in WebRTC. It generates XOR-based parity repair packets sent in a separate RTP stream. While technically applicable to audio, it is not commonly used for audio because:
- Audio packets are small and frequent; XOR parity overhead is proportionally higher.
- Opus in-band FEC + RED already provides adequate single-loss protection at lower complexity.
- FlexFEC's strength is protecting against bursty video frame losses with 2D (row + column) parity, which is less relevant for audio's uniform packet stream.

#### 1.4.4 DRED (Deep Redundancy) — Opus 1.5+

Since Opus 1.5 (March 2024), WebRTC can leverage DRED for dramatically improved packet loss resilience. See Section 4 for detailed comparison.

---

### 1.5 Speex FEC

**Overview.** Speex (now deprecated in favor of Opus) did not implement a traditional FEC mechanism. Instead, Speex achieved packet-loss resilience through **frame-independent design:**

- **No inter-frame LSP prediction:** LSP coefficients are quantized independently each frame, so losing one frame does not corrupt subsequent frame's spectral envelope decoding.
- **Product gain encoding:** Global excitation gain × sub-frame gain corrections—eliminates gain prediction chains that could propagate errors.
- **Wideband layering:** QMF split into low-band (narrowband CELP) and high-band; the low-band bitstream is independently decodable, providing graceful degradation if high-band data is lost.

Speex's PLC on the decoder side uses pitch-period repetition with gradual fade-out, similar in spirit to G.711 Appendix I but operating on the decoded CELP signal.

For actual redundancy, Speex relied on external transport-level mechanisms (RFC 2198 RED) rather than in-band FEC.

| Parameter | Value |
|---|---|
| In-band FEC | None (design-level robustness only) |
| External FEC | RFC 2198 RED (application-dependent) |
| PLC method | Pitch-period waveform repetition with fade |
| Frame independence | High (no inter-frame prediction on LSP/gain) |

---

### 1.6 AAC-LD / AAC-ELD Error Resilience

**Overview.** AAC-LD (Low Delay) and AAC-ELD (Enhanced Low Delay) are MPEG-4 codecs designed for full-duplex communication. They do not include codec-level FEC (no redundant frame embedding), but define several **error resilience (ER) tools** specified in MPEG-4 Audio (ISO 14496-3):

**Error Resilience Tools:**

1. **HCR (Huffman Codeword Reordering):** Reorders the spectral Huffman codewords so that the most perceptually important (lowest-frequency) coefficients are placed at the beginning of the bitstream segment. If bit errors corrupt the tail, only less important high-frequency data is lost.

2. **RVLC (Reversible Variable Length Coding):** Encodes scale factors using a variable-length code that can be decoded in both forward and reverse directions. If a bit error corrupts the middle of the scale factor data, the decoder can decode from both ends and recover a larger fraction of scale factors than with unidirectional VLC.

3. **VCB11 (Virtual Codebooks):** Maps spectral Huffman codewords to error-detection virtual codebooks, enabling the decoder to detect (though not correct) bit errors in spectral data.

4. **Error Protection (EP) Tool:** Allows the encoder to specify unequal error protection classes and interleave bits across classes, similar to AMR's UEP.

**Limitations:**
- These tools are designed for **bit-error channels** (e.g., wireless links), not packet-erasure channels. They do not help when an entire packet is lost.
- For packet-loss scenarios, AAC-LD/ELD relies on decoder-side PLC (waveform extrapolation, spectral repetition) or external transport-level FEC.
- Algorithmic delay: AAC-LD ≈ 20 ms; AAC-ELD ≈ 15 ms (with SBR: 32 ms).

| Parameter | Value |
|---|---|
| In-band FEC | None |
| Error resilience | HCR, RVLC, VCB11, EP tool (bit-error protection) |
| Packet-loss protection | External (transport-level FEC or app-layer RED) |
| PLC | Decoder-side spectral extrapolation |
| Typical bitrate | 24–64 kbps (AAC-ELD); 32–128 kbps (AAC-LD) |

---

### 1.7 G.711 Appendix I PLC

**Overview.** G.711 (ITU-T, 1988) is a waveform codec (A-law / μ-law PCM at 64 kbps) with no source coding redundancy. Appendix I (ITU-T, 1999) defines a receiver-side **Packet Loss Concealment** algorithm. This is not FEC—no redundant data is transmitted—but is the standard reference PLC for PCM codecs.

**Algorithm (based on the ITU-T specification and reference implementation):**

1. **Normal operation:** The decoder maintains a circular history buffer of the most recent 48.75 ms (390 samples at 8 kHz) of decoded audio.

2. **On packet loss:**
   - **Pitch estimation:** An autocorrelation-based pitch detector estimates the pitch period from the history buffer (search range: 40–120 samples, i.e., 5–15 ms / 67–200 Hz).
   - **Waveform replication:** The last pitch period of the history buffer is repeatedly copied (with overlap-add smoothing) to fill the missing frame duration.
   - **Fade-out:** If consecutive packets are lost, the repeated waveform is gradually attenuated (linear ramp to zero over ~10 ms) to avoid sustained artificial periodicity.

3. **On packet recovery:**
   - **Overlap-add fade-in:** The first few milliseconds of the newly received packet are cross-faded with the tail of the concealment signal to avoid discontinuities.

**Characteristics:**
- Quality: Acceptable for single isolated losses; degrades noticeably for bursts > 2–3 packets (> 40–60 ms).
- Complexity: Very low (~100 multiplies per sample).
- No bitrate overhead (receiver-only algorithm).
- The algorithm is defined for 8 kHz sampling rate with 10 ms packets but can be adapted.
- Widely referenced as the baseline PLC algorithm against which more advanced methods are benchmarked.

---

## 2. Transport-Level FEC for Audio

### 2.1 RFC 2198 — RTP Payload for Redundant Audio Data (RED)

**Standard:** RFC 2198 (September 1997), updated by RFC 7656.

**Mechanism:** The sender encapsulates one or more *redundant* audio payloads alongside the *primary* payload in a single RTP packet. Each redundant payload has its own payload type (potentially a different codec or mode) and a timestamp offset indicating which original frame it represents.

**Typical configuration for audio:**
- Primary: current frame (e.g., Opus at 32 kbps, 20 ms)
- Redundant: previous frame (same codec, same or lower rate)
- Net overhead: ~100% if redundant frame is same size; lower if redundant uses a lower rate.

**Key properties:**
- **Recovery depth:** Typically 1 packet (N-1 in packet N). Can carry 2+ levels of redundancy but each adds one full frame of overhead.
- **Latency:** +20 ms per redundancy level (receiver must wait for the next packet to check for FEC recovery).
- **Codec-agnostic:** Works with any RTP audio codec.
- **SFU-friendly:** SFUs can add, strip, or forward RED transparently.
- **Limitations:** High overhead for multi-level redundancy; no burst-loss protection beyond the redundancy depth; no error correction coding (just duplication).

---

### 2.2 RFC 5109 — RTP Payload Format for Generic FEC (ULP-FEC)

**Standard:** RFC 5109 (December 2007), obsoletes RFC 2733.

**Mechanism:** Uses XOR-based parity codes to generate FEC repair packets from groups of source packets. Supports **Uneven Level Protection (ULP)**—different regions of the payload can receive different levels of protection.

**How it works:**
1. The sender groups N source packets into a protection group.
2. For each group, one or more FEC packets are generated by XOR-ing the source packet payloads (and headers).
3. If any single packet in the group is lost, it can be reconstructed by XOR-ing the FEC packet with all other received source packets.

**Key properties:**
- **Overhead:** 1/N per protection level (e.g., 1 FEC packet per 4 source packets = 25% overhead for single-loss recovery within the group).
- **Recovery capability:** Can recover 1 loss per protection group (per parity row). More losses require multiple FEC packets or 2D arrangements.
- **Latency:** Must wait for the entire protection group to arrive before recovery can occur. For audio at 20 ms/packet with group size 5, this means up to 100 ms of additional latency.
- **ULP advantage:** Can apply stronger protection to the first K bytes of each packet (e.g., spectral envelope) and lighter protection to the rest.
- **Backward compatible:** Non-FEC receivers simply ignore FEC packets.

| Group size | Overhead | Max recoverable losses | Additional latency |
|---|---|---|---|
| 2 | 50% | 1 | 40 ms |
| 4 | 25% | 1 | 80 ms |
| 8 | 12.5% | 1 | 160 ms |

---

### 2.3 FlexFEC (RFC 8627)

**Standard:** RFC 8627 (July 2019), IETF Standards Track.

**Mechanism:** An evolution of ULP-FEC with flexible protection topologies. FEC repair packets are generated using systematic parity codes (XOR) and sent as a separate redundancy RTP stream.

**Protection topologies:**

1. **1-D Non-interleaved (Row):** Protects a consecutive row of L packets with one parity packet. Recovers 1 random loss per row. Good for random loss.
2. **1-D Interleaved (Column):** Protects every D-th packet (column) with one parity packet. Recovers burst losses of up to D−1 consecutive packets. Good for burst loss.
3. **2-D (Row + Column):** Combines both. Rows handle random losses; columns handle bursts. Can recover multiple losses per block if they are spread across different rows/columns.
4. **Flexible Mask:** Arbitrary bitmask-defined protection patterns for custom protection schemes.

**Key properties:**
- **Overhead:** Configurable; typically 10–50% depending on protection strength.
- **Burst protection:** 2D mode with D=5 and L=5 can protect against bursts up to 4 consecutive packets (~80 ms at 20 ms/packet).
- **Latency:** Depends on block dimensions. Row-only: L × 20 ms. Column: D × 20 ms.
- **Backward compatible:** Source packets are unmodified; non-FlexFEC receivers simply ignore repair packets.
- **Multi-stream:** Can protect packets across multiple source RTP streams simultaneously.

**Audio relevance:** While primarily used for video in WebRTC, FlexFEC's 2D topology is theoretically valuable for audio in high-loss or bursty environments. However, the per-packet overhead ratio is worse for small audio packets than for large video frames.

---

### 2.4 Reed-Solomon Codes for Audio Streaming

**Overview.** Reed-Solomon (RS) codes are algebraic block codes operating over finite fields (typically GF(2^8) or GF(2^16)). An RS(n, k) code encodes k source symbols into n symbols (n − k parity symbols), and can correct up to (n − k)/2 symbol errors or recover up to (n − k) erasures (known-position losses).

**Application to audio streaming:**

In packet-erasure channels, RS erasure codes treat each packet as a "symbol." RS(n, k) generates n packets from k source packets; any k of the n received packets suffice to recover all k source packets.

**Practical parameters for real-time audio:**
- RS(5, 4): 25% overhead, recovers 1 loss per block of 5. Block latency: 5 × 20 ms = 100 ms.
- RS(8, 6): 33% overhead, recovers 2 losses per block of 8. Block latency: 160 ms.
- RS(16, 12): 33% overhead, recovers 4 losses per block of 16. Block latency: 320 ms (too high for interactive audio; suitable for streaming/broadcast).

**Advantages over XOR:**
- Can recover **multiple** losses per block (XOR recovers only 1).
- Well-understood mathematical guarantees; MDS (Maximum Distance Separable) property means optimal recovery for given overhead.

**Disadvantages:**
- **Block latency** is the primary constraint for interactive audio. Must buffer the entire block before decoding.
- **Computational complexity:** O(n × k) finite-field multiplications. Efficient implementations exist (SIMD-optimized GF(2^8) with lookup tables), but more expensive than XOR.
- **Block alignment:** All source packets in a block should be similar size for efficient encoding.

**Use cases:**
- Music streaming (non-interactive): RS(255, 223) with large blocks, high burst tolerance.
- Audio broadcast (DAB/DAB+): RS codes applied at the transport layer.
- Pro-audio networking (AES67/Dante): Some implementations use RS for studio-grade reliability.

---

### 2.5 Fountain Codes / Raptor Codes for Audio

**Standards:** Raptor codes: RFC 5053. RaptorQ: RFC 6330. RTP payload for Raptor: RFC 6682.

**Mechanism.** Fountain codes are rateless erasure codes—the encoder can generate an essentially unlimited number of encoding symbols from k source symbols, and the decoder can recover from any k (or slightly more) received symbols regardless of which specific ones were received.

**Raptor / RaptorQ specifics:**
- **Systematic:** The first k encoding symbols are the original source symbols (sent unmodified), so if no loss occurs there is zero decoding overhead.
- **Near-ideal recovery:** RaptorQ typically recovers from k received symbols with >99% probability; from k + 1 with >99.99%.
- **Linear-time encoding/decoding:** O(k) operations per symbol. Implementations achieve >1 Gbps on commodity hardware.
- **Source block sizes:** Up to ~56,000 symbols (RFC 6330); practical for large files or long audio streams.

**Application to real-time audio:**
- RFC 6682 defines RTP encapsulation of Raptor repair symbols for streaming.
- Best suited for **one-way streaming** or **broadcast** (e.g., IPTV audio, podcast streaming) where latency constraints are relaxed (100+ ms acceptable).
- For interactive audio (<150 ms end-to-end), fountain codes require very small source blocks (5–10 packets), reducing their advantage over simpler RS or XOR codes.
- **Bandwidth probing:** Repair symbols can serve double duty as bandwidth probes (send more repair when bandwidth permits, reducing the need for retransmissions).

| Parameter | XOR (FlexFEC) | Reed-Solomon | RaptorQ |
|---|---|---|---|
| Recovery per block | 1 (per parity row) | (n−k) erasures | (n−k) erasures |
| Overhead for 1 loss/5 packets | 20% | 20% | ~20% |
| Overhead for 2 losses/10 packets | 40% (2D) or N/A | 20% | 20% |
| Encoding complexity | O(k) XOR | O(n·k) GF-mul | O(k) |
| Decoding complexity | O(k) XOR | O(k²) GF-mul | O(k) |
| Latency model | Block-based | Block-based | Block-based |
| Burst protection | 2D topology | Configurable | Inherent |

---

### 2.6 Interleaving-Based FEC

**Mechanism.** Instead of (or in addition to) adding redundant data, the sender interleaves audio frames across packets so that a burst loss of consecutive packets destroys at most one frame per interleave group, converting burst loss into distributed isolated losses that PLC can handle better.

**Example (interleave depth D = 4):**
```
Packet 0: frames [0, 4, 8, 12]
Packet 1: frames [1, 5, 9, 13]
Packet 2: frames [2, 6, 10, 14]
Packet 3: frames [3, 7, 11, 15]

If packets 1 and 2 are lost:
  Missing frames: [1, 2, 5, 6, 9, 10, 13, 14]
  But each is isolated by received neighbors, enabling better PLC.
```

**Key properties:**
- **Zero bitrate overhead** (no redundant data transmitted).
- **Latency cost:** D × frame_duration. For D=4 at 20 ms frames: +80 ms additional delay.
- **Effectiveness:** Converts burst loss into isolated losses. Works well when combined with good PLC but does not recover lost data—it only makes PLC's job easier.
- **Used in:** IETF RFC 4867 (AMR payload format supports optional interleaving); some proprietary VoIP systems.

**Trade-off vs. FEC:**
- Interleaving is complementary to FEC. It is most useful when bandwidth is constrained (no room for redundancy) but some additional latency is acceptable.
- For interactive audio, the latency cost often makes interleaving impractical (D ≥ 3 adds ≥ 60 ms).

---

## 3. Emerging / Research FEC Approaches

### 3.1 Neural Network–Based Packet Loss Concealment

**State of the art.** The ICASSP 2024 Audio Deep Packet Loss Concealment Grand Challenge and the 2025 IEEE-IS² Music PLC Challenge have driven rapid progress in neural PLC.

**Key systems:**

**BS-PLCNet 2 (ICASSP 2024 winner):**
- Two-stage band-split architecture with intra-model knowledge distillation.
- Dual-path encoder (non-causal + causal) compensates for missing future context.
- Lightweight post-processing module for speech distortion removal.
- 40% fewer parameters than BS-PLCNet 1; 8.95 GFLOPS; +0.18 PLCMOS over prior art.
- Operates at 48 kHz full-band with 20 ms look-ahead (real-time compatible).

**Opus DeepPLC (libopus 1.5):**
- Integrated into the Opus decoder; uses a small RNN (LACE/NoLACE architecture).
- Operates on CELT spectral features; provides significantly better PLC than the traditional waveform repetition.
- The `weights_blob.bin` file (included in this repo) contains the trained DNN weights.
- Computational cost: ~5–10 MFLOPS; runs in real-time on all modern CPUs.

**Microsoft PLC Challenge baselines:**
- Recurrent and convolutional architectures operating on 20 ms frames at 48 kHz.
- Evaluation via crowdsourced ITU P.804 MOS and Word Error Rate (WER).
- Best systems achieve MOS > 4.0 for single-frame losses; degrades for bursts > 60 ms.

**2025 Music PLC Challenge:**
- Extends neural PLC to music signals (44.1 kHz, 11.6 ms packets).
- Addresses the harder problem of concealing musical content which has less predictable structure than speech.

**Key insight:** Neural PLC is **receiver-side only** (no bitrate overhead) and improves with better models. It is orthogonal to FEC—the two can be combined for layered protection: FEC recovers what it can; neural PLC conceals what FEC cannot recover.

---

### 3.2 Generative Model–Based Audio Recovery

**GAN-Based Approaches:**

- **bin2bin GAN-PLC:** Time-frequency domain GAN for packet loss concealment. Operates on STFT magnitude and phase spectrograms. Can conceal gaps up to ~80 ms with good perceptual quality.
- **Frequency-Consistent GAN (IEEE ICASSP 2022):** Enforces frequency-domain consistency constraints in the GAN generator, improving spectral continuity across the concealment boundary.
- **Mel-spectrogram GAN:** Generates missing audio segments from Mel-spectrogram context. Handles gaps up to 320 ms; achieves MOS ~3.74 on 240 ms gaps. Runs near real-time on GPU.

**Diffusion-Based Approaches:**

- **Phoneme-Guided Diffusion Inpainting (PGDI):** Uses a diffusion model conditioned on phoneme-level guidance to reconstruct gaps up to **1 second** while preserving speaker identity and prosody. High quality but computationally expensive (~100 ms per denoising step × 50 steps for high quality).
- **Constant-Q Transform Diffusion:** Exploits pitch-equivariant symmetries in the CQT domain. Handles gaps up to 300 ms; outperforms baselines for wider gaps. Zero-shot conditioning (no fine-tuning needed for new speakers).

**Trade-offs:**
| Method | Max gap | MOS quality | Latency | Compute |
|---|---|---|---|---|
| Traditional PLC (G.711 App I) | ~40 ms | 2.5–3.0 | <1 ms | CPU, trivial |
| Neural PLC (BS-PLCNet 2) | ~100 ms | 3.5–4.0 | 20 ms look-ahead | CPU, moderate |
| GAN-PLC | ~320 ms | 3.5–3.8 | ~50 ms | GPU |
| Diffusion inpainting | ~1000 ms | 4.0+ | 500+ ms | GPU, heavy |

**Practical applicability:** GAN-based PLC is approaching real-time feasibility for VoIP. Diffusion models currently have too much latency for interactive use but are suitable for post-processing (e.g., restoring archived recordings or non-real-time streaming).

---

### 3.3 Bandwidth-Adaptive FEC Strategies

Modern research treats FEC rate allocation as a dynamic optimization problem:

**Reinforcement Learning–Based FEC (RL-AFEC):**
- Uses RL agents to dynamically select FEC redundancy rates based on observed network conditions (loss rate, RTT, bandwidth).
- Achieves good quality in >95% of cases with only 40% additional bandwidth (compared to 100% for static full-redundancy).
- Trained in simulation with packet-level quality assessment; transferable to real networks with domain adaptation.

**FRACTaL (FEC-based Rate Control):**
- Uses FEC packets as bandwidth probes: varies FEC rate to fill the congestion-controlled sending rate without changing the media bitrate.
- When network capacity exceeds media rate, surplus is used for FEC. When capacity drops, FEC is reduced first.
- Achieves better TCP-friendliness and more consistent media quality than traditional rate-control algorithms (SCReAM).

**Bandwidth Estimation Integration (ACM MMSys 2024):**
- Winning approaches combine offline RL with real-world traces.
- Audio/video quality scores used as rewards aligned with user-perceived QoE.
- The sim-to-real gap remains a key challenge for RL-based approaches.

**Key principle:** The optimal FEC rate is a function of current network conditions and codec behavior. Static FEC rates are either wasteful (too much redundancy on good channels) or insufficient (too little on bad channels). Adaptive strategies can save 30–60% of FEC bandwidth compared to static allocation at equivalent protection levels.

---

### 3.4 Cross-Packet Redundancy Using Learned Representations

**DRED (Deep Redundancy for Opus):**

The most mature example. DRED uses a Rate-Distortion-Optimized Variational Autoencoder (RDO-VAE) to:
1. Continuously encode the audio into 20 acoustic features per frame (18 BFCCs + pitch + voicing).
2. Quantize these features into a compact latent code (~1/50 of regular Opus bitrate).
3. Embed up to ~50 frames (~1 second) of redundancy in each packet using a backward-running recurrent decoder.
4. On loss, the receiver decodes the latent codes from the next received packet and synthesizes the missing audio.

- **Bitrate:** ~32 kbps total for 1 second of redundancy (vs. ~21 kbps for the primary Opus stream at 32 kbps).
- **Quality:** Synthesized audio preserves spectral envelope, pitch, and voicing but lacks waveform-level accuracy (no phase information). Subjective quality is significantly better than PLC but lower than the original.

**Neural Codec FEC (Gupta et al., 2024):**

Extends the concept to neural codecs: a secondary network predicts the latent codebook indices of adjacent frames, providing cross-packet redundancy at 0.8 kbps overhead. The key advantage is that the "redundancy" is not a copy of the original data but a *prediction* in the learned latent space, which is inherently more compact.

**Conceptual framework:**
```
Traditional FEC:       source frame → re-encode at lower rate → transmit copy
DRED:                  source frame → RDO-VAE latent → quantize → transmit
Neural codec FEC:      source frame → predict neighbor's latent → transmit prediction
```

Each successive approach achieves better compression of the redundancy signal by operating in progressively more abstract representation spaces.

---

### 3.5 Joint Source-Channel Coding (JSCC) for Audio

**Overview.** Traditional systems separate source coding (compression) and channel coding (error protection)—the "separation theorem" (Shannon, 1948) shows this is optimal for infinite block lengths. However, for finite block lengths (real-time audio frames are 10–20 ms / 160–960 samples), joint optimization can significantly outperform separation.

**Deep JSCC (DeepJSCC):**
- Replaces the separate source encoder → channel encoder → channel decoder → source decoder pipeline with an end-to-end trained neural network.
- The network learns to simultaneously compress the audio and add channel-appropriate redundancy.
- Avoids the **cliff effect** (abrupt quality collapse when channel capacity drops below source rate) and the **leveling-off effect** (no quality gain when channel improves beyond source rate) that plague separated systems.

**D²-JSCC (Digital Deep JSCC, 2024):**
- Makes DeepJSCC compatible with digital communication infrastructure by using discrete constellation mappings.
- Enables deployment on existing IP/RTP networks without analog signal modifications.

**Audio-specific JSCC:**
- Research is less mature for audio than for images/video, but the principles apply directly.
- A JSCC audio encoder would output channel symbols (rather than bits) that are directly transmitted, with the neural network learning the optimal trade-off between compression and error protection for the given channel statistics.
- Potential for 2–5 dB SNR gain over separated coding at low SNR / high loss rates.

**Practical barriers:**
- Requires channel model knowledge at training time (or adaptive training).
- Not compatible with existing codec standards or RTP payload formats without a wrapper.
- Computational complexity of end-to-end neural inference.
- Standardization path unclear.

---

### 3.6 Autoencoder-Based Audio FEC

**Concept.** Rather than using a hand-designed FEC code (XOR, RS, etc.), train an autoencoder where:
- The encoder takes K source audio frames and produces K + R latent codes.
- The K latent codes are "source symbols" and the R codes are "parity symbols."
- The decoder can reconstruct all K frames from any K of the K + R received codes.

This is essentially a learned erasure code operating in the audio feature domain.

**Advantages:**
- The code is optimized for the specific statistics of audio (or speech), potentially achieving better rate-distortion-recovery trade-offs than generic algebraic codes.
- Can jointly optimize compression and redundancy (a form of JSCC).
- Graceful degradation: even with more than R losses, the autoencoder may produce a reasonable (if degraded) reconstruction, unlike algebraic codes which either succeed perfectly or fail completely.

**State of research:**
- Early-stage; most work is in the image/video domain (e.g., learned image compression with error resilience).
- The DRED RDO-VAE can be viewed as a specialized instance: it is an autoencoder-based redundancy encoder operating on audio features.
- Full autoencoder FEC for audio (where both source coding and channel coding are jointly learned) remains an open research direction.

---

## 4. Comparison with Opus's Approach

### 4.1 Opus FEC Mechanisms Summary

Opus (as of v1.5.2) has three layers of packet-loss protection:

| Layer | Mechanism | Bitrate overhead | Recovery depth | Quality | Latency | Mode |
|---|---|---|---|---|---|---|
| LBRR | SILK in-band FEC | ~67% of primary rate | 1 frame (20 ms) | Moderate (lower-rate requantization) | +20 ms | SILK only |
| DRED | Neural redundancy (RDO-VAE) | ~2% of primary rate per 20 ms of coverage | Up to ~50 frames (1 s) | Good (spectral envelope preserved; no phase) | +0 ms (data already in packet) | All modes |
| DeepPLC | Neural PLC at decoder | 0% (receiver-only) | Unlimited (quality degrades over time) | Good for <60 ms; degrades for longer | 0 ms | All modes |

### 4.2 Strengths of Opus's Approach

1. **Layered protection.** LBRR handles single isolated losses at high quality (waveform-level recovery); DRED handles burst losses at lower but acceptable quality; DeepPLC provides a safety net when both fail. This layering is more bandwidth-efficient than any single mechanism.

2. **DRED's extraordinary compression.** At ~1/50 of the primary bitrate, DRED provides 1 second of redundancy—a feat impossible with traditional FEC approaches. A traditional RED approach covering 1 second would need 50× the bitrate.

3. **Codec-integrated.** In-band FEC avoids the overhead of additional RTP headers, FEC streams, and transport-layer complexity. The redundancy is part of the Opus bitstream, simplifying deployment.

4. **Backward compatible.** DRED data is placed in the Opus padding area; decoders that don't support DRED simply ignore it. LBRR is part of the standard Opus bitstream since RFC 6716.

5. **No separate FEC stream.** Unlike FlexFEC or RS-based approaches, there is no separate repair stream to negotiate, route through SFUs, or manage.

6. **Adaptive.** The encoder adjusts LBRR and DRED bitrate allocation based on the declared packet loss percentage, automatically trading primary quality for redundancy as conditions worsen.

### 4.3 Weaknesses of Opus's Approach

1. **LBRR is SILK-only.** The traditional FEC does not protect CELT-mode audio (music at higher bitrates). DRED fills this gap but with synthesized (not waveform-accurate) audio.

2. **LBRR covers only 1 frame.** Two consecutive losses defeat LBRR entirely. EVS CAM has the same limitation, but DRED addresses this for Opus.

3. **DRED quality ceiling.** DRED reconstructs from 20 acoustic features—adequate for speech but loses fine detail for music. The synthesized audio sounds "vocoder-like" for complex signals. Neural codec FEC (operating on richer latent representations) could potentially achieve higher reconstruction quality.

4. **Fixed DRED architecture.** The RDO-VAE model is trained offline and shipped as weights. It cannot adapt to new audio types or speakers without retraining. Future systems with on-the-fly adaptation could be more flexible.

5. **No multi-loss algebraic guarantees.** Unlike RS codes where recovery from K-of-N packets is mathematically guaranteed, DRED's recovery quality degrades gracefully but unpredictably with the distance to the recovery point. For safety-critical applications (emergency calls), deterministic recovery guarantees may be preferred.

6. **Compute requirements.** DRED encoding and decoding require neural network inference. While the models are small (~5 MFLOPS), this is non-trivial for the lowest-power embedded devices where G.711 + simple PLC is still common.

7. **No transport-layer coordination.** Opus FEC operates independently of transport-layer FEC (FlexFEC, NACK). Better coordination between in-band FEC and transport-level mechanisms could reduce total redundancy overhead.

### 4.4 Innovations from Other Systems That Could Improve Opus

| Innovation | Source | Potential benefit for Opus |
|---|---|---|
| **Channel-aware dynamic FEC allocation** | EVS CAM | More granular real-time adjustment of LBRR/DRED split based on channel feedback. EVS's CAM dynamically activates/deactivates based on measured conditions, while Opus requires explicit `OPUS_SET_PACKET_LOSS_PERC` from the application. |
| **Latent-space FEC for neural codecs** | Gupta et al. (2024) | If Opus evolves toward neural encoding modes, FEC in the latent domain (0.8 kbps overhead) could replace or supplement DRED. |
| **RL-based adaptive FEC rate** | RL-AFEC | Train a reinforcement learning agent to set `OPUS_SET_PACKET_LOSS_PERC` and DRED frame count based on observed network metrics, removing the need for the application to estimate loss rates. |
| **Interleaving for burst resilience** | AMR RFC 4867 | Optionally interleave Opus frames across packets to convert bursts into isolated losses that LBRR can handle. Adds latency but could reduce DRED dependency for moderate bursts. |
| **Generative PLC for music** | GAN-PLC, Diffusion models | Improve CELT-mode PLC for music signals where the current DeepPLC (speech-optimized) underperforms. |
| **Joint source-channel coding** | DeepJSCC | Long-term: replace separate compression + FEC with a jointly optimized neural encoder that outputs channel-coded symbols, potentially achieving 2–5 dB gains at high loss rates. |
| **2D FEC topology** | FlexFEC | Combine Opus in-band FEC with transport-layer 2D FlexFEC for defense-in-depth, especially for audio in highly lossy environments (satellite, cellular edge). |
| **Fountain codes for streaming** | RaptorQ (RFC 6330) | For non-interactive Opus streaming (podcasts, music), RaptorQ provides near-optimal erasure recovery with linear-time decoding. |

---

## 5. Summary Table

| Approach | Type | Overhead | Recovery depth | Burst protection | Latency impact | Compute | Best for |
|---|---|---|---|---|---|---|---|
| **Opus LBRR** | Codec in-band | ~67% of primary | 1 frame | None | +20 ms | Low | Single isolated losses, SILK speech |
| **Opus DRED** | Codec in-band (neural) | ~2%/frame of coverage | Up to 1 s | Excellent | ~0 ms | Moderate (DNN) | Burst losses, all Opus modes |
| **Opus DeepPLC** | Decoder-side (neural) | 0% | Unlimited (degrading) | Moderate | 0 ms | Moderate (DNN) | Last resort, short gaps |
| **EVS CAM** | Codec in-band | Within bitrate envelope | 1 frame | None | ~0 ms | Low | VoLTE speech |
| **AMR + RED** | Transport (RFC 2198) | ~100% | 1 frame | None | +20 ms | Very low | Legacy VoIP |
| **RFC 5109 ULP-FEC** | Transport (XOR) | 1/N per group | 1 per group | Limited | N × 20 ms | Low | Moderate-loss channels |
| **FlexFEC 2D** | Transport (XOR) | 10–50% | Multiple (2D) | Good (column FEC) | Block-based | Low | Bursty loss, video+audio |
| **Reed-Solomon** | Transport (algebraic) | (n−k)/k | (n−k) erasures | Configurable | Block-based | Moderate | Streaming, broadcast |
| **RaptorQ** | Transport (fountain) | ~(n−k)/k | (n−k) erasures | Inherent | Block-based | Low (linear-time) | Non-interactive streaming |
| **Interleaving** | Transport (reorder) | 0% | N/A (improves PLC) | Converts burst→isolated | D × frame | Very low | Bandwidth-constrained, moderate latency |
| **Neural PLC** | Decoder-side (DNN) | 0% | ~100 ms gap | Moderate | 20 ms look-ahead | Moderate–High | VoIP, missing-frame concealment |
| **GAN/Diffusion PLC** | Decoder-side (generative) | 0% | Up to 1 s | Good | 50–500+ ms | High (GPU) | Post-processing, non-RT |
| **Neural codec FEC** | Codec in-band (latent) | ~0.8 kbps | 1 frame | None | ~0 ms | Moderate (DNN) | Next-gen neural codecs |
| **Joint source-channel** | End-to-end (DNN) | Integrated | Integrated | Adaptive | Architecture-dependent | High | Future 6G/beyond systems |

---

*Document compiled for the Opus 编解码实验框架 project. Sources include IETF RFCs, 3GPP specifications, ITU-T recommendations, IEEE/ICASSP publications, and IETF Internet-Drafts.*
