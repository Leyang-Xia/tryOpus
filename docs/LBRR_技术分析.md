# Opus LBRR 带内前向纠错：技术分析

> **版本**：v3.0 | **日期**：2026-03-12 | **Opus 版本**：1.6.1
>
> **摘要**：本文档基于 Opus 1.6.1 源码分析、三轮系统性实验（合成信号 + TTS 真实语音 + 频谱变化对照）和官方文献对照，完整分析 LBRR 的工作机制、激活条件、实测覆盖率、生成率影响因素和改进方向。LBRR 机制在 1.5→1.6 版本间无变化（属 SILK 层），实验数据已在 1.6.1 上复现验证。

---

## 1. 概述

LBRR（Low Bit-Rate Redundancy，低码率冗余）是 Opus SILK 层的带内前向纠错机制，定义于 RFC 6716 §5.3.3。

编码器在包 N 中附带包 N-1 的低质量再编码副本。接收端若丢失包 N-1，可从包 N 中的 LBRR 数据恢复。

```
  包 N-1                包 N                包 N+1
┌────────────┐       ┌─────────────────┐    ┌────────────────────┐
│ 主帧: N-1  │   →   │ 主帧: N         │ →  │ 主帧: N+1          │
│ (正常质量) │       │ LBRR帧: N-1副本 │    │ LBRR帧: N副本      │
└────────────┘       │ (降级质量)      │    │ (降级质量)         │
                     └─────────────────┘    └────────────────────┘

若包 N-1 丢失 → 用包 N 中的 LBRR 数据恢复帧 N-1
```

### 与 DRED 的对比

| 特性 | LBRR | DRED |
|------|------|------|
| 冗余机制 | SILK LPC 参数降级再编码 | 神经网络（RDO-VAE）特征压缩 |
| 适用模式 | 仅 SILK / Hybrid | SILK + CELT 均支持 |
| 覆盖范围 | 仅恢复突发末尾 1 帧 | 可恢复连续多帧（最多 1 秒） |
| 码率开销 | CBR 零开销（替换填充）；VBR 约增 20-40% | 全 1 秒冗余 < 32 kbps |
| 激活条件 | 严格的码率门限 + 语音活动检测 | 仅需 `PACKET_LOSS_PERC > 0` |
| 计算开销 | 无（复用 SILK 编码器） | 编解码端均需 DNN 推理 |

---

## 2. 激活决策链路

LBRR 是否生成取决于**四层关卡**，任何一层未通过则该帧不生成 LBRR：

```
第一关          第二关              第三关              第四关
前提条件  →   等效码率门限  →   语音活动检测  →   Opus 顶层 VAD
useInBandFEC    decide_fec()      speech_activity     activity
 && plp>0        返回 1?            > 0.3?            != NO?
 && mode≠CELT
```

### 2.1 第一关：前提条件

```c
// opus-src/src/opus_encoder.c → decide_fec()
if (!useInBandFEC || PacketLoss_perc == 0 || mode == MODE_CELT_ONLY)
    return 0;
```

| 条件 | 说明 |
|------|------|
| `OPUS_SET_INBAND_FEC(1)` | 必须显式启用 |
| `OPUS_SET_PACKET_LOSS_PERC(>0)` | **必须非零**（最常见的配置遗漏） |
| 非 CELT 模式 | 高码率(>64kbps) 或 RESTRICTED_LOWDELAY 会进入纯 CELT |

### 2.2 第二关：等效码率门限（`decide_fec` 函数）

各带宽的 FEC 基础阈值（`fec_thresholds[]`）：

| 带宽 | 基础阈值 | 滞后值 | plp=10% 首次激活门限 | plp=10% 维持门限 |
|------|---------|--------|---------------------|-----------------|
| NB 8kHz | 12000 | 1000 | 14800 | 12800 |
| MB 12kHz | 14000 | 1000 | 17100 | 15100 |
| WB 16kHz | 16000 | 1000 | 19400 | 17400 |
| SWB 24kHz | 20000 | 1000 | 24000 | 22000 |
| FB 48kHz | 22000 | 1000 | 26300 | 24300 |

等效码率经过多重折算：CBR 惩罚(−8%) → 复杂度折算 → 丢包预测损失。32kbps CBR/cx=9/plp=10% 的等效码率约 24891 bps。

**带宽降级机制**：当 `plp > 5%` 且当前带宽不满足门限时，编码器会自动降低带宽来激活 FEC（以音质换保护）。`plp ≤ 5%` 时不触发降级，直接放弃。

### 2.3 第三关：语音活动检测阈值

```c
// opus-src/silk/fixed/encode_frame_FIX.c
// LBRR_SPEECH_ACTIVITY_THRES = 0.3 (silk/tuning_parameters.h)
if (psEnc->sCmn.LBRR_enabled &&
    psEnc->sCmn.speech_activity_Q8 > 77) {  // 0.3 × 256
    psEnc->sCmn.LBRR_flags[...] = 1;
}
```

SILK VAD 基于 4 子带信噪比计算 `speech_activity_Q8`。低于 0.3 的帧不生成 LBRR。

### 2.4 第四关：Opus 顶层 VAD 覆盖

```c
// Opus 顶层 VAD 可强制压制 SILK VAD 结果
if (activity == VAD_NO_ACTIVITY && speech_activity_Q8 >= threshold)
    speech_activity_Q8 = threshold - 1;  // 强制压到 0.05 以下
```

---

## 3. 实测数据

### 3.1 真实语音 vs 合成信号（32kbps CBR, plp=10%）

| 音频 | 类型 | LBRR 生成率 | 10% 均匀丢包恢复率 |
|------|------|-------------|-------------------|
| **TTS 中文连续朗读** | 真实语音 | **91.2%** | **85.2%** |
| **TTS 快速英文** | 真实语音 | **90.8%** | **84.0%** |
| **TTS 英文连续朗读** | 真实语音 | **80.8%** | **67.6%** |
| 早期合成样本（0.5s 停顿） | 合成 | 14.6% | 14.6% |
| speech_continuous.wav（合成，无停顿） | 合成 | 14.4% | — |
| sine_440hz.wav | 参照 | 99.8% | — |
| noise.wav | 参照 | 99.8% | — |

**核心结论**：对真实连续语音，LBRR 生成率为 **80-91%**，10% 均匀丢包下恢复率 **68-85%**。（LBRR 仅恢复突发末尾帧，且受随机丢包分布影响。）

### 3.2 频谱变化对 LBRR 生成率的影响

| 信号 | 频谱变化特征 | LBRR 生成率 |
|------|-------------|-------------|
| 固定谐波 150Hz | 完全稳定 | 6.0% |
| 合成语音（1.25s 段内不变） | 慢速跳变 | 14.4% |
| 快速切换基频（0.2s） | 中速跳变 | 52.4% |
| 共振峰元音变化（0.3s） | 较快变化 | 70.4% |
| TTS 真实语音 | 自然连续变化 | 80-91% |
| 基频连续滑动 | 持续变化 | 91.4% |

**根因**：SILK VAD 的噪声底估计器会缓慢追踪信号电平。对频谱长时间稳定的合成信号，噪声底逐步逼近信号水平，导致估计信噪比崩塌、`speech_activity_Q8` 跌破 0.3。真实语音因共振峰/基频/能量持续变化不受此影响。

### 3.3 码率与 plp 的影响

| 码率 | plp | 带宽分布 | LBRR 生成率 |
|------|-----|---------|-------------|
| 16 kbps | 10% | SWB 100% | 0%（等效码率不足） |
| 20 kbps | 10% | NB 7% + MB 93% | 26.4% |
| 32 kbps | 5% | FB 100% | 0%（plp≤5% 不降级） |
| 32 kbps | 10% | SWB 7% + FB 93% | 14.6% |
| 48 kbps | 10% | FB 100% | 14.6% |

（以上为早期合成信号数据；真实语音在相同码率/plp 下生成率更高）

### 3.4 LBRR 恢复质量

| 帧 | LBRR SNR | PLC SNR | 提升 |
|-----|----------|---------|------|
| 帧 5 | 20.5 dB | 13.0 dB | +7.5 dB |
| 帧 13 | 9.8 dB | −3.7 dB | +13.5 dB |
| 帧 15 | 21.2 dB | −2.8 dB | +24.0 dB |

LBRR 的价值在于当它有效时相比 PLC 的**质量飞跃**（+5 到 +24 dB）。

---

## 4. 局限与真实场景的改进空间

### 4.1 结构性限制

- **仅恢复突发末尾 1 帧**：连续丢 3 包只能恢复最后 1 包，其余退 PLC
- **仅 SILK 模式**：高码率(>64kbps) 切入 CELT 后 LBRR 不可用
- **突发丢包场景下恢复率降至 42-45%**

### 4.2 实际可改进的瓶颈

| 瓶颈 | 影响 | 改进方向 |
|------|------|---------|
| `plp ≤ 5%` 时完全不激活 | 2-5% 丢包率的网络无保护 | 降低截断阈值或允许有限降级 |
| 低码率(< 20kbps) 码率门限 | 窄带场景无法激活 | 降低 `fec_thresholds[]` |
| 停顿/清辅音帧不受保护 | 10-20% 的真实语音帧无 LBRR | 自适应阈值或感知重要性加权 |
| LBRR 与 DRED 预算割裂 | 两者各自消耗码率不协同 | 联合预算分配 |

### 4.3 改进方案概要

| 方案 | 改进目标 | 实现难度 | 推荐优先级 |
|------|---------|---------|-----------|
| 去除 plp≤5% 截断 | 覆盖低丢包场景 | 极低 | ★★★★★ |
| 降低码率门限 | 支持低码率场景 | 低 | ★★★★ |
| 自适应语音活动阈值 | 保护更多边缘帧 | 中 | ★★★ |
| 感知重要性加权保护 | 保护关键帧 | 高 | ★★★（中长期） |
| LBRR+DRED 联合预算 | 突发丢包场景 | 中高 | ★★★ |

详细方案设计见 [FEC 新方案探索报告](Opus_FEC_新方案探索报告.md)。

---

## 5. 使用指南

### 5.1 API 调用

```c
// 编码端：必须同时设置两个参数
opus_encoder_ctl(enc, OPUS_SET_INBAND_FEC(1));
opus_encoder_ctl(enc, OPUS_SET_PACKET_LOSS_PERC(plp));

// 解码端：丢包时从下一包提取 LBRR
if (opus_packet_has_lbrr(next_pkt, next_len)) {
    opus_decode(dec, next_pkt, next_len, out, frame_size, /*decode_fec=*/1);
} else {
    opus_decode(dec, NULL, 0, out, frame_size, 0);  // 回退 PLC
}
```

### 5.2 推荐配置

| 场景 | 码率 | plp | 预期 LBRR 覆盖率（真实语音） |
|------|------|-----|----------------------------|
| 窄带 VoIP (16kHz) | ≥ 20 kbps | 10% | 60-80% |
| 宽带 WebRTC (48kHz) | ≥ 32 kbps | 10% | 80-91% |
| 高丢包环境 | ≥ 40 kbps | 20% | 80-90%（建议配合 DRED） |

### 5.3 何时不该使用 LBRR

| 情况 | 原因 | 替代方案 |
|------|------|---------|
| 码率 < 20kbps（48kHz） | 等效码率低于门限 | 仅用 PLC |
| 纯 CELT 模式 | 无 SILK 层 | DRED |
| 突发丢包（≥ 2 包） | 只能恢复最后 1 包 | DRED |
| 需要高保证恢复率 | LBRR 不保证 100% | DRED（VBR 模式 100%） |

### 5.4 常见问题

**Q: `opus_packet_has_lbrr()` 一直返回 0？**
检查：① `OPUS_SET_PACKET_LOSS_PERC` 是否 > 0 ② 码率是否足够 ③ 是否在 CELT 模式

**Q: LBRR 激活后包大小会增加吗？**
CBR 模式不增加（替换填充字节），VBR 模式增加约 20-40%。

**Q: WebRTC 中如何使用？**
通过 SDP 的 `useinbandfec=1` 启用，`plp` 通过 RTCP 反馈自动调整。

---

## 6. 在本项目中的实现

本项目 `offline_validation/src/opus_sim.c` 的 LBRR 恢复逻辑：

```c
// 仅恢复突发中最后一帧，从 next_ok 包中提取 LBRR
if (!recovered && j == burst_end &&
    cfg.dec_cfg.use_lbrr && cfg.enc_cfg.use_fec &&
    next_ok < num_frames && enc_sizes[next_ok] > 1 &&
    opus_packet_has_lbrr(enc_pkts[next_ok], enc_sizes[next_ok])) {
    decode_ret = opus_decode(dec,
        enc_pkts[next_ok], enc_sizes[next_ok],
        pcm_out, frame_samples, /*decode_fec=*/1);
}
```

LBRR 生成率分析工具：`tools/analyze_lbrr_rate.c`

```bash
# 编译
gcc -O2 -o build/analyze_lbrr tools/analyze_lbrr_rate.c \
    -I opus-install/include -L opus-install/lib -lopus -lm

# 使用
./build/analyze_lbrr representative_audio/dialogue/dialogue_30s_48k_mono.wav 32000 10 0
```

---

## 附录：关键源码位置

| 功能 | 文件 | 位置 |
|------|------|------|
| `decide_fec()` | `opus-src/src/opus_encoder.c` | 811-841 |
| `fec_thresholds[]` | `opus-src/src/opus_encoder.c` | 180-186 |
| `compute_equiv_rate()` | `opus-src/src/opus_encoder.c` | 896-929 |
| `silk_setup_LBRR()` | `opus-src/silk/control_codec.c` | 403-425 |
| `silk_LBRR_encode_FIX()` | `opus-src/silk/fixed/encode_frame_FIX.c` | 387-445 |
| `LBRR_SPEECH_ACTIVITY_THRES` | `opus-src/silk/tuning_parameters.h` | 83 |
| `silk_VAD_GetSA_Q8()` | `opus-src/silk/VAD.c` | — |
| Opus 顶层 VAD 覆盖 | `opus-src/silk/fixed/encode_frame_FIX.c` | 56-59 |
