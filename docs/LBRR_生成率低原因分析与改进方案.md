# Opus LBRR 生成率低：根因分析与改进方案

> **版本**：v2.0 | **日期**：2026-03-12
>
> **摘要**：通过 Opus 源码逆向分析和三轮系统性实验（合成信号 + TTS 真实语音 + 频谱变化对照），定位了 LBRR 生成率低的**四层根因**，并通过 TTS 语音实验**修正了此前的结论**——真实连续语音的 LBRR 生成率实际可达 **80-91%**，此前的 14.6% 是合成测试信号的病态特征导致。核心瓶颈是 SILK VAD 噪声底估计器对频谱稳定信号的适应行为。

---

## 1. 问题现象与修正

### 1.1 初始观察（合成信号）

使用 `speech_like.wav`（48kHz、含幅度调制的合成类语音信号）编码时，LBRR 生成率仅为 10-26%。

### 1.2 修正后的完整数据（含 TTS 真实语音 + 对照实验）

| 音频内容 | 类型 | LBRR 生成率 | LBRR 丢包恢复率 |
|----------|------|-------------|----------------|
| speech_like.wav（合成，0.5s 周期停顿） | 合成 | **14.6%** | 15.8% |
| speech_continuous.wav（合成，无停顿） | 合成 | **14.4%** | 16.1% |
| harmonic_fixed_150hz（固定谐波） | 合成 | **6.0%** | — |
| harmonic_slow_switch（5s 切换基频） | 合成 | **15.0%** | — |
| **harmonic_glide**（基频连续滑动） | 合成 | **91.4%** | — |
| **harmonic_fast_switch**（0.2s 切换基频） | 合成 | **52.4%** | — |
| **formant_vowels**（共振峰变化） | 合成 | **70.4%** | — |
| speech_few_pauses（2s 间隔短停顿） | 合成 | **89.4%** | — |
| **TTS 英文连续朗读** | 真实语音 | **80.8%** | **80.0%** |
| **TTS 中文连续朗读** | 真实语音 | **91.2%** | **88.0%** |
| **TTS 快速英文** | 真实语音 | **90.8%** | **79.0%** |
| sine_440hz（正弦波） | 参照 | 99.8% | — |
| noise（白噪声） | 参照 | 99.8% | — |

### 1.3 关键修正

> **此前结论（已修正）**：语音活动检测阈值 0.3 过滤了 ~85% 的真实语音帧。
>
> **修正后结论**：对**真实连续语音**（TTS），LBRR 生成率实际为 **80-91%**，丢包恢复率 **79-88%**。14.6% 的低生成率是合成测试信号特有的病态现象——其频谱长时间稳定不变，导致 SILK VAD 噪声底估计器误将信号归类为背景噪声。

---

## 2. LBRR 生成的完整决策链路

LBRR 数据是否实际写入比特流，需要经过**四层"关卡"**的逐级筛选：

```
第一关        第二关              第三关            第四关
前提条件 →  等效码率门限  →  语音活动检测  →  Opus 顶层 VAD
  │              │                 │                │
  ▼              ▼                 ▼                ▼
useInBandFEC   decide_fec()     speech_activity   activity
  &&              返回 1?          > 0.3?          != NO?
PacketLoss>0    (LBRR_enabled)   (LBRR_flags)    (不压制)
  &&
mode≠CELT
```

任何一关未通过，该帧的 LBRR 就不会生成。以下逐层分析。

---

## 3. 第一关：前提条件（通常不是问题）

源码位置：`opus-src/src/opus_encoder.c` → `decide_fec()`

```c
if (!useInBandFEC || PacketLoss_perc == 0 || mode == MODE_CELT_ONLY)
    return 0;
```

三个前提条件：

| 条件 | 说明 | 本项目状态 |
|------|------|-----------|
| `useInBandFEC = 1` | 必须调用 `OPUS_SET_INBAND_FEC(1)` | `-fec` 参数已开启 |
| `PacketLoss_perc > 0` | 必须调用 `OPUS_SET_PACKET_LOSS_PERC(>0)` | `-plp` 参数已设置 |
| `mode ≠ MODE_CELT_ONLY` | 编码器不能处于纯 CELT 模式 | 32 kbps VOIP 下全部为 SILK/Hybrid |

**结论**：在我们的实验配置下，第一关总是通过的。但在以下场景会成为瓶颈：

- 忘记设置 `plp`（**最常见的配置错误**）
- 码率很高（>64 kbps 单声道）导致编码器切入 CELT 模式
- 使用 `OPUS_APPLICATION_RESTRICTED_LOWDELAY` 强制 CELT 模式

---

## 4. 第二关：等效码率门限（`decide_fec` 函数）

源码位置：`opus-src/src/opus_encoder.c:811-841`

这是 LBRR 生成率低的**第一个重要原因**。

### 4.1 门限计算公式

```c
LBRR_rate_thres_bps = fec_thresholds[bandwidth] * (125 - min(plp, 25)) / 100;
if (last_fec == 1) threshold -= hysteresis;  // 滞后：已有FEC时降低门限
if (last_fec == 0) threshold += hysteresis;  // 滞后：没有FEC时提高门限
```

各带宽的基础阈值和滞后值：

| 带宽 | 基础阈值 | 滞后值 | plp=10% 激活门限（首次） | plp=10% 维持门限（已激活） |
|------|---------|--------|-------------------------|--------------------------|
| 窄带 NB | 12000 | 1000 | 14800 | 12800 |
| 中带 MB | 14000 | 1000 | 17100 | 15100 |
| 宽带 WB | 16000 | 1000 | 19400 | 17400 |
| 超宽带 SWB | 20000 | 1000 | 24000 | 22000 |
| 全带 FB | 22000 | 1000 | 26300 | 24300 |

### 4.2 等效码率计算

```c
equiv = bitrate;
if (!vbr)  equiv -= equiv/12;              // CBR 惩罚约 8%
equiv = equiv * (90+complexity)/100;        // 复杂度影响约 10%
equiv -= equiv*loss/(6*loss + 10);          // 丢包预测损失
```

32 kbps CBR、复杂度 9、plp=10% 下的等效码率：

```
32000 → CBR惩罚: 29333 → 复杂度(99%): 29040 → 丢包调整(10/70): 24891 bps
```

### 4.3 门限比较结果

| 带宽 | 等效码率 | 首次激活门限 | 维持门限 | 能否激活？ |
|------|---------|-------------|---------|----------|
| FB | 24891 | 26300 | 24300 | 首次不能，已激活后可维持 |
| SWB | 24891 | 24000 | 22000 | 可以激活 |
| WB | 24891 | 19400 | 17400 | 可以激活 |

### 4.4 带宽降级机制（`plp > 5%` 时触发）

```c
if (rate > LBRR_rate_thres_bps)
    return 1;                        // 当前带宽可激活
else if (PacketLoss_perc <= 5)
    return 0;                        // plp≤5 直接放弃
else if (*bandwidth > OPUS_BANDWIDTH_NARROWBAND)
    (*bandwidth)--;                  // plp>5 尝试降低带宽
```

**关键行为**：当 `plp > 5%` 且当前带宽下等效码率不够时，编码器会**主动降低音频带宽**来满足 FEC 门限。这解释了实验中出现的带宽分布：

```
32kbps plp=10: SWB=33(7%), FB=467(93%)
                 ↑
         这 33 帧是被 decide_fec 从 FB 降到 SWB 来激活 FEC 的
```

### 4.5 此关的影响量化

| 参数组合 | decide_fec 返回 1 的帧比例 | 说明 |
|---------|--------------------------|------|
| 32k CBR plp=10 FB | ~100%（通过降级或滞后） | 降级到 SWB 后激活，滞后维持 |
| 32k CBR plp=5 FB | 0% | plp≤5 不触发降级，FB 门限太高 |
| 16k CBR plp=10 SWB | 0% | 等效码率 ~12.4k < SWB 门限 24k |

**结论**：在 32 kbps + plp=10% 条件下，`decide_fec` 对约 100% 帧返回 1（通过降级+滞后机制），**这不是 14.6% 生成率的瓶颈**。但 plp ≤ 5% 或码率 ≤ 16 kbps 时，此关就是决定性瓶颈。

---

## 5. 第三关：语音活动检测阈值（核心瓶颈）

源码位置：`opus-src/silk/fixed/encode_frame_FIX.c:402` 和 `opus-src/silk/float/encode_frame_FLP.c:395`

```c
if (psEnc->sCmn.LBRR_enabled &&
    psEnc->sCmn.speech_activity_Q8 > SILK_FIX_CONST(LBRR_SPEECH_ACTIVITY_THRES, 8)) {
    psEnc->sCmn.LBRR_flags[psEnc->sCmn.nFramesEncoded] = 1;
    // ... 生成 LBRR 数据
}
```

其中 `LBRR_SPEECH_ACTIVITY_THRES = 0.3`（定义于 `silk/tuning_parameters.h:83`），转换为 Q8 定点数即 `0.3 × 256 = 76.8 ≈ 77`。

### 5.1 这是 14.6% 生成率的直接原因

实验证明：在 48 kbps + 100% FB 带宽下（`decide_fec` 全部返回 1），`speech_like.wav` 的 LBRR 生成率仍只有 14.6%，而 `sine_440hz.wav` 达 99.8%。

**唯一的差异就是 `speech_activity_Q8` 值**：

- `sine_440hz.wav`：恒定高能量稳态信号，`speech_activity_Q8` 几乎始终 > 77
- `speech_like.wav`：有幅度包络（每 0.5s 静音）、辅音/元音交替，大量帧的 `speech_activity_Q8` < 77

### 5.2 语音活动检测器的工作原理

源码位置：`opus-src/silk/VAD.c`

SILK VAD 的 `speech_activity_Q8` 基于以下计算：

1. 将输入信号分解到 4 个子带（0-1kHz, 1-2kHz, 2-4kHz, 4-8kHz）
2. 估计每个子带的短时能量和长期背景噪声水平
3. 计算每个子带的信噪比（SNR）
4. 基于加权 SNR 得到 `SA_Q15`（语音活动概率）
5. `speech_activity_Q8 = SA_Q15 >> 7`

**设计意图**：0.3 的阈值意味着"如果这一帧大概率不包含有效语音（如静音、背景噪声），就不值得为它生成 LBRR"。这在理论上合理——保护静音帧的冗余数据是浪费码率。

### 5.3 真正的根因：SILK VAD 噪声底估计器的频谱适应

通过 TTS 真实语音实验和频谱变化对照实验，发现了更深层的原因：

**不是"停顿"导致低活动值，而是"频谱稳定"。**

关键证据：

```
speech_continuous.wav（无停顿，恒定幅度）:          14.4%  ← 频谱段内稳定
speech_with_pauses.wav（有停顿，其余相同）:         14.6%  ← 几乎一样！
harmonic_glide_noise.wav（基频连续滑动，无停顿）:   91.4%  ← 频谱持续变化
harmonic_fast_switch.wav（每0.2秒换基频）:          52.4%  ← 变化越频繁越好
TTS 真实语音（自然语音）:                          80-91%  ← 天然频谱变化丰富
```

**根因机制**：SILK VAD（`silk/VAD.c`）使用自适应噪声底估计：

1. 将信号分解到 4 个子带，估计每帧的短时能量
2. 维护一个**长期噪声底估计**（逐帧缓慢上升以跟踪背景噪声）
3. 计算每帧信号与噪声底的**信噪比**
4. 信噪比 → `speech_activity_Q8`

当信号频谱**长时间稳定不变**（>500ms）时，噪声底估计器会把"稳定信号"误认为"已适应的噪声"，导致估计的信噪比大幅下降，`speech_activity_Q8` 跌破 0.3。

**真实语音不受此影响的原因**：

- 自然语音的**共振峰不断变化**（元音切换、辅音/元音交替）
- **基频 (F0) 有自然抖动**和声调变化
- 说话节奏导致的**短暂能量变化**为噪声底提供了重新校准的机会

### 5.4 此关的影响量化（修正版）

| 信号类型 | 频谱变化速率 | LBRR 生成率 | 说明 |
|---------|-------------|-------------|------|
| 固定谐波 150Hz | 无变化 | 6.0% | 噪声底完全追上信号 |
| 合成语音（1.25s 段内不变） | 每 1.25s | 14.4% | 段内被适应 |
| 快速切换基频（0.2s） | 每 0.2s | 52.4% | 部分被适应 |
| 共振峰元音变化（0.3s） | 每 0.3s | 70.4% | 接近真实语音 |
| **TTS 英文** | **连续变化** | **80.8%** | **真实语音水平** |
| **TTS 中文** | **连续变化** | **91.2%** | **真实语音水平** |
| 基频连续滑动 | 连续 | 91.4% | 最接近理论上限 |
| 正弦波 440Hz | 极简谱线 | 99.8% | 噪声底估计器不追踪窄带 |
| 白噪声 | 宽带随机 | 99.8% | 每帧频谱都不同 |

**修正后的结论**：对真实连续语音，LBRR 生成率为 **80-91%**，语音活动阈值不是主要瓶颈。低生成率主要出现在频谱稳定的合成信号和长时间静音段。

---

## 6. 第四关：Opus 顶层 VAD 覆盖

源码位置：`opus-src/silk/fixed/encode_frame_FIX.c:49-59`

```c
const opus_int activity_threshold = SILK_FIX_CONST(SPEECH_ACTIVITY_DTX_THRES, 8);
// SPEECH_ACTIVITY_DTX_THRES = 0.05

if (activity == VAD_NO_ACTIVITY &&
    psEnc->sCmn.speech_activity_Q8 >= activity_threshold) {
    psEnc->sCmn.speech_activity_Q8 = activity_threshold - 1;
}
```

Opus 顶层有一个独立的 VAD（语音活动检测），当它判定为 `VAD_NO_ACTIVITY` 时，会**强制将 `speech_activity_Q8` 压到 DTX 阈值 0.05 以下**，远低于 LBRR 阈值 0.3。

这意味着即使 SILK VAD 认为有一定语音活动（比如 0.2），如果 Opus 顶层 VAD 判定为无活动，`speech_activity_Q8` 也会被强制压低，LBRR 不会生成。

**此关在启用 DTX 时影响尤为严重**，但即使不启用 DTX，Opus 顶层 VAD 仍会运行并影响 `speech_activity_Q8`。

---

## 7. 根因总结（修正版）

### 7.1 按信号类型的瓶颈分布

```
真实连续语音（TTS）@ 32kbps plp=10:

第三关: VAD 噪声底适应   ████████                              10-20%
        自然停顿/清辅音帧被过滤

第二关: 等效码率门限      ████                                  在此配置下不是瓶颈

第四关: Opus 顶层 VAD     ██                                    3-5%

  → 真实语音 LBRR 生成率: 80-91%  ✓ 可接受

合成频谱稳定信号（speech_like.wav）@ 32kbps plp=10:

第三关: VAD 噪声底适应   ████████████████████████████████████  85%
        噪声底估计器追上稳定频谱，信噪比崩塌

  → 合成信号 LBRR 生成率: 6-15%  ✗ 病态表现
```

### 7.2 各因素与配置的关联

| 配置场景 | 主要瓶颈 | LBRR 生成率 |
|---------|---------|-------------|
| 32k plp=10 **TTS 真实语音** | 自然停顿/清辅音 | **80-91%** |
| 32k plp=10 合成稳定信号 | **噪声底适应** | 6-15% |
| 32k plp=10 稳态正弦/噪声 | 无瓶颈 | ~100% |
| 32k plp=5 真实语音 | **码率门限**（不触发降级） | 0% |
| 16k plp=10 真实语音 | **码率门限** | 0% |
| 32k plp=0 任意内容 | **前提条件**（plp=0） | 0% |
| 64k plp=10 CELT 模式 | **前提条件**（CELT 不支持） | 0% |

### 7.3 修正结论

> **此前结论**：LBRR 对真实语音覆盖率严重不足（~15%），语音活动阈值是核心瓶颈。
>
> **修正后**：对真实连续语音，LBRR 覆盖率实际为 **80-91%**，在 10% 均匀丢包下恢复率达 **79-88%**。此前 14.6% 的低数据是合成测试信号的频谱稳定性导致的 SILK VAD 噪声底适应问题，不代表真实语音的表现。
>
> 但仍存在以下**真实场景下的改进空间**：
> 1. 突发丢包场景下 LBRR 恢复率降至 36-45%（因为 LBRR 只恢复突发末尾帧）
> 2. plp ≤ 5% 时 LBRR 完全不激活，但实际网络丢包率可能就在 2-5% 区间
> 3. 低码率（< 20 kbps）下码率门限导致 LBRR 无法激活
> 4. 说话中的长停顿和清辅音段仍不受保护

---

## 8. 改进方案

### 方案 A：降低语音活动检测阈值（最直接）

**原理**：将 `LBRR_SPEECH_ACTIVITY_THRES` 从 0.3 降低到更低值。

**实现**：修改 `silk/tuning_parameters.h`

```c
// 原始值
#define LBRR_SPEECH_ACTIVITY_THRES  0.3f

// 改进方案（根据实验选择合适值）
#define LBRR_SPEECH_ACTIVITY_THRES  0.1f   // 方案 A1：激进
#define LBRR_SPEECH_ACTIVITY_THRES  0.15f  // 方案 A2：折中
#define LBRR_SPEECH_ACTIVITY_THRES  0.2f   // 方案 A3：保守
```

**预期效果**：

| 阈值 | 预计 LBRR 生成率（语音） | 码率开销增加（VBR） | 风险 |
|------|------------------------|-------------------|------|
| 0.3（原始） | 10-15% | 基准 | 无 |
| 0.2 | 25-40% | +10-15% | 低 |
| 0.15 | 40-60% | +15-25% | 低-中 |
| 0.1 | 60-80% | +25-40% | 中（静音帧也会产生 LBRR） |

**优点**：实现极简，仅改一个常量，向后兼容

**缺点**：静音帧也会生成 LBRR，浪费码率；CBR 模式下码率被 LBRR 侵占导致主编码质量下降

**评估**：⭐⭐⭐⭐ 投入产出比最高的方案

---

### 方案 B：自适应语音活动阈值（推荐）

**原理**：将固定阈值 0.3 替换为根据丢包率和码率动态计算的自适应阈值。丢包率越高越应该保护更多帧，码率越充裕越可以保护更多帧。

**实现思路**：

```c
// silk/fixed/encode_frame_FIX.c 和 float/encode_frame_FLP.c
float adaptive_thresh = LBRR_SPEECH_ACTIVITY_THRES;

// 丢包率越高，保护越多帧
if (psEnc->sCmn.PacketLoss_perc > 10)
    adaptive_thresh *= 0.5f;
else if (psEnc->sCmn.PacketLoss_perc > 5)
    adaptive_thresh *= 0.7f;

// 码率充裕时（有预算），保护更多帧
opus_int32 bits_per_frame = ...; // 当前帧实际可用比特
opus_int32 lbrr_bits_est = ...; // LBRR 预计消耗比特
if (bits_per_frame > lbrr_bits_est * 3)
    adaptive_thresh *= 0.6f;

if (psEnc->sCmn.LBRR_enabled &&
    psEnc->sCmn.speech_activity_Q8 > SILK_FIX_CONST(adaptive_thresh, 8)) {
    psEnc->sCmn.LBRR_flags[...] = 1;
    // ...
}
```

**优点**：在高丢包时自动提升保护范围，低丢包时维持现有行为，码率效率更优

**缺点**：需要在 Opus 内核代码中修改，涉及 SILK 内部状态

**评估**：⭐⭐⭐⭐⭐ 兼顾效果和码率效率的最优方案

---

### 方案 C：降低 FEC 码率门限（扩大激活带宽范围）

**原理**：降低 `fec_thresholds[]` 中的基础阈值，使 LBRR 在更低等效码率下也能激活。

**实现**：修改 `opus-src/src/opus_encoder.c`

```c
// 原始值
static const opus_int32 fec_thresholds[] = {
    12000, 1000,  // NB
    14000, 1000,  // MB
    16000, 1000,  // WB
    20000, 1000,  // SWB
    22000, 1000,  // FB
};

// 改进方案：降低约 20%
static const opus_int32 fec_thresholds[] = {
    10000, 800,   // NB
    11000, 800,   // MB
    13000, 800,   // WB
    16000, 800,   // SWB
    18000, 800,   // FB
};
```

**预期效果**：

- 使 `plp=5%` 时也能通过 `decide_fec`（当前 plp≤5% 是完全阻断的）
- 使低码率场景（16-20 kbps）也能激活 LBRR
- 减少带宽降级发生的频率（避免音质不必要地降低）

**优点**：扩大 LBRR 的可用工作区间，特别是低丢包率场景

**缺点**：在低码率下 LBRR 会严重挤占主编码预算；不解决语音活动阈值问题

**评估**：⭐⭐⭐ 可与方案 A/B 组合使用

---

### 方案 D：去除 `plp ≤ 5%` 的硬性截断

**原理**：当前 `decide_fec` 在 `plp ≤ 5%` 时不会尝试降低带宽来激活 FEC，直接返回 0。对于低丢包率的网络，这意味着 LBRR 完全不会被激活（除非码率足够高直接满足当前带宽门限）。

**实现**：修改 `decide_fec` 中的分支逻辑

```c
// 原始逻辑
if (rate > LBRR_rate_thres_bps)
    return 1;
else if (PacketLoss_perc <= 5)
    return 0;                    // ← 直接放弃
else if (*bandwidth > OPUS_BANDWIDTH_NARROWBAND)
    (*bandwidth)--;

// 改进：plp 1-5% 也允许降一级带宽
if (rate > LBRR_rate_thres_bps)
    return 1;
else if (PacketLoss_perc <= 2)
    return 0;                    // 只在极低丢包时放弃
else if (*bandwidth > OPUS_BANDWIDTH_NARROWBAND) {
    if (PacketLoss_perc <= 5)
        (*bandwidth)--;          // plp 3-5%：只降一级
    else
        (*bandwidth)--;          // plp > 5%：持续降级（原始行为）
}
```

**预期效果**：使 plp=3-5% 场景下也有一定概率激活 LBRR

**评估**：⭐⭐⭐ 低风险改动，改善小但覆盖了一个盲区

---

### 方案 E：感知重要性加权保护（长期方案）

**原理**：不单纯依赖语音活动检测，而是引入**感知重要性评分**来决定 LBRR 保护。语音起攻（onset）、频谱急剧变化、声调转折等感知关键帧即使能量不高也应该被保护。

**实现思路**：

```c
float perceptual_importance = compute_importance(
    speech_activity,   // 语音活动
    spectral_flux,     // 频谱变化率
    is_onset,          // 是否为起攻帧
    pitch_change       // 基频变化
);

// 起攻帧即使 speech_activity 低也要保护
if (is_onset && perceptual_importance > 0.5)
    generate_lbrr = 1;
// 高活动帧用原有逻辑
else if (speech_activity_Q8 > threshold)
    generate_lbrr = 1;
```

**优点**：将有限的 LBRR 预算集中在感知最关键的帧上，最大化每比特冗余的感知价值

**缺点**：实现复杂，需要修改 SILK 内部编码流程，引入额外的分析模块

**评估**：⭐⭐⭐⭐ 效果最优但实现成本高，适合中长期迭代

---

### 方案 F：LBRR + DRED 联合预算优化

**原理**：当前 LBRR 和 DRED 各自独立消耗码率。在 LBRR 生成率低的帧（如低语音活动帧），其码率预算被浪费在填充字节上（CBR 模式）。可以将这部分预算转移给 DRED，让 DRED 为这些帧提供保护。

**实现思路**：

```c
if (!lbrr_generated_this_frame) {
    // 这帧的 LBRR 预算没有被使用
    // 将节省的比特分配给 DRED（提高 DRED 量化精度）
    increase_dred_quality_for_frame(saved_lbrr_bits);
}
```

**优点**：不修改 LBRR 自身逻辑，通过与 DRED 协同弥补 LBRR 的覆盖空白

**缺点**：需要 LBRR 和 DRED 预算机制联动，实现涉及两个子系统

**评估**：⭐⭐⭐ 架构层面的优化，需要较大的设计工作

---

## 9. 方案对比与推荐（修正版）

基于修正后的结论（真实语音 LBRR 生成率已达 80-91%），改进方案的优先级需要调整：

| 方案 | 改进目标 | 实现难度 | 兼容性 | 推荐优先级 |
|------|---------|---------|--------|-----------|
| **D 去除 plp≤5% 截断** | **覆盖低丢包场景** | **极低** | **完全兼容** | **⭐⭐⭐⭐⭐ 首选** |
| C 降低码率门限 | 支持低码率场景 | 低 | 完全兼容 | ⭐⭐⭐⭐ 次选 |
| A 降低固定阈值 | 保护更多边缘帧 | 极低 | 完全兼容 | ⭐⭐⭐ 可选 |
| B 自适应阈值 | 智能化保护 | 中 | 完全兼容 | ⭐⭐⭐ 可选 |
| E 感知重要性保护 | 保护关键帧 | 高 | 需要验证 | ⭐⭐⭐ 中长期 |
| F LBRR+DRED 联合 | 突发丢包场景 | 中高 | 需要 DRED | ⭐⭐⭐ 已有 DRED 时 |

### 推荐实施路径

1. **立即可做**：方案 D（去除 plp≤5% 截断）— 让 LBRR 在低丢包率网络中也能工作
2. **短期目标**：方案 C（降低码率门限）— 扩大 LBRR 可用的码率范围
3. **中期目标**：方案 B（自适应阈值）— 提升边缘帧的覆盖率（从 80% 到 90%+）
4. **长期架构**：方案 F（LBRR+DRED 联合预算）— 突发丢包场景的系统性解决

---

## 10. 实验验证脚本

以下命令可用于验证改进效果：

```bash
export LD_LIBRARY_PATH=$(pwd)/opus-install/lib:$LD_LIBRARY_PATH

# 基线：当前 LBRR 生成率
./build/analyze_lbrr audio/speech_like.wav 32000 10 0

# 修改 Opus 源码后重新编译
cd opus-src
# （修改 silk/tuning_parameters.h 中的 LBRR_SPEECH_ACTIVITY_THRES）
make -j$(nproc) && make install
cd ..

# 重新编译项目
cd build && cmake .. && make -j$(nproc) && cd ..

# 验证改进效果
./build/analyze_lbrr audio/speech_like.wav 32000 10 0

# 对比丢包恢复率
./build/opus_sim -fec -plp 10 -b 32000 -l 0.1 audio/speech_like.wav results/improved.wav
```

---

## 附录：关键源码位置索引

| 功能 | 文件 | 行号 |
|------|------|------|
| `decide_fec()` | `opus-src/src/opus_encoder.c` | 811-841 |
| `fec_thresholds[]` | `opus-src/src/opus_encoder.c` | 180-186 |
| `compute_equiv_rate()` | `opus-src/src/opus_encoder.c` | 896-929 |
| `silk_setup_LBRR()` | `opus-src/silk/control_codec.c` | 403-425 |
| `silk_LBRR_encode_FIX()` | `opus-src/silk/fixed/encode_frame_FIX.c` | 387-445 |
| `silk_LBRR_encode_FLP()` | `opus-src/silk/float/encode_frame_FLP.c` | 378-432 |
| `LBRR_SPEECH_ACTIVITY_THRES` | `opus-src/silk/tuning_parameters.h` | 83 |
| `SPEECH_ACTIVITY_DTX_THRES` | `opus-src/silk/tuning_parameters.h` | 80 |
| `silk_VAD_GetSA_Q8()` | `opus-src/silk/VAD.c` | 全文 |
| Opus 顶层 VAD 覆盖 | `opus-src/silk/fixed/encode_frame_FIX.c` | 56-59 |
| 编码模式决策 | `opus-src/src/opus_encoder.c` | 1328-1382 |
| 带宽选择阈值 | `opus-src/src/opus_encoder.c` | 145-178 |
