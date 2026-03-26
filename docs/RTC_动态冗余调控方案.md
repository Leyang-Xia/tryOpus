# RTC 动态冗余调控方案

> 日期：2026-03-20
>
> 适用范围：`webrtc_demo/`

本文总结当前 `webrtc_demo` 中已经实现的 sender 侧动态冗余调控方案。目标不是动态调码率，而是在标准 RTC 反馈闭环下，根据链路丢包总量与突发特征，在 `LBRR` 与 `DRED` 之间切换，并动态更新 Opus 编码器参数。

当前策略已经从“保守优先 LBRR”调整为“更偏向 DRED 以优化 ASR 指标”，但现在已经进一步做成**码率感知**：

- `WER` 是主优化目标，`SER` 只作为辅助指标
- `24 kbps+` 下在中高丢包时更早尝试 `DRED`
- 默认优先 `DRED-3`
- 只有在重突发场景下才更早升到 `DRED-5`
- `< 24 kbps` 下默认更偏 `LBRR`

---

## 1. 目标

当前方案解决的问题是：

- 在 RTC 链路中实时读取标准反馈
- 用平滑后的链路状态决定当前更适合 `LBRR` 还是 `DRED`
- 将决策映射为 Opus encoder CTL
- 在不动态调码率的前提下，尽量优化 `WER`

当前版本只动态调这些参数：

- `OPUS_SET_INBAND_FEC`
- `OPUS_SET_PACKET_LOSS_PERC`
- `OPUS_SET_DRED_DURATION`

当前版本**不**动态调这些参数：

- bitrate
- VBR / CBR
- complexity
- signal hint

---

## 2. 反馈来源

实现位于 [webrtc_demo/internal/adaptation/adaptation.go](/Users/leyang/Projects/tryOpus/webrtc_demo/internal/adaptation/adaptation.go)。

动态调控只使用标准 RTC 路径：

1. `RTPSender.ReadRTCP()`
2. `PeerConnection.GetStats()`

当前采集的核心信号包括：

- `fraction_lost`
- `packets_lost_delta`
- `burst_loss_rate`
- `jitter`
- `round_trip_time`
- `REMB`（如果存在）
- `transport-cc` 包状态序列

其中 `burst_loss_rate` 的来源分两层：

1. 优先使用 `GetStats().RemoteInboundRTPStreamStats.BurstLossRate`
2. 如果该字段缺失、恒为 0 或当前运行环境没有稳定填充，则退回到 sender 本地估算：
   - 从 `RTPSender.ReadRTCP()` 收到的 `TransportLayerCC`
   - 解码 `PacketChunks`
   - 把 `TypeTCCPacketNotReceived` 展平成收包/缺包序列
   - 统计“连续缺包长度 >= 2 的 lost packet 数 / 当前 TWCC 覆盖的总 packet 数”

这个本地 estimator 仍然基于标准 RTC 反馈路径，因为它消费的是标准 `TWCC` 报文，而不是自定义回传。

其中主决策信号是：

- `fraction_lost`
- `burst_loss_rate`

辅助信号是：

- `jitter`
- `RTT`

保护信号是：

- `REMB`

---

## 3. 平滑与滞回

默认参数位于 [webrtc_demo/internal/adaptation/adaptation.go](/Users/leyang/Projects/tryOpus/webrtc_demo/internal/adaptation/adaptation.go)，sender 接线在 [webrtc_demo/sender/main.go](/Users/leyang/Projects/tryOpus/webrtc_demo/sender/main.go)。

默认参数：

- 采样周期：`1s`
- EMA 系数：`0.35`
- 平滑窗口目标：约 `5s`
- 普通升档：连续 `2` 次命中更高档位
- `DRED` 升档：连续 `2` 次命中更高档位
- 降档：连续 `5` 次命中更低档位
- 冷却时间：`3s`

当前还额外约束了：

- sender 启动阶段不再强制先走 `LBRR`
- 但首次进入 `DRED` 时，最高只允许到 `DRED-3`
- 这样可以避免一开局就直接冲到 `DRED-5`

这样做的目的是避免：

- 单个采样点抖动导致频繁切换
- 刚升档后又立即降档
- burst 短时出现时过度反应
- 在高总丢包但证据还不充分时过快进入过重 DRED

---

## 4. 模式与档位

实现中的模式枚举定义在 [webrtc_demo/internal/adaptation/adaptation.go](/Users/leyang/Projects/tryOpus/webrtc_demo/internal/adaptation/adaptation.go)。

### 4.1 模式判定

当前控制器先看码率档位，再决定是否走偏 DRED 还是偏 LBRR。

### 4.1 码率档位

- `24k+`：`bitrate >= 24000`
- `<24k`：`bitrate < 24000`

### 4.2 模式判定

`24k+` 下不是“只有 burst-heavy 才能进 DRED”，而是分两层：

1. `prefer-dred`
2. `burst-heavy`

其中：

- `prefer-dred` 条件：
  - `fractionLostEMA >= 0.15`
  - 或 `fractionLostEMA >= 0.10` 且 `burstLossRateEMA >= 0.10`

- `burst-heavy` 条件：
  - `burstLossRateEMA >= 0.18`
  - 且 `burstLossRateEMA / fractionLostEMA >= 0.70`

这意味着：

- 均匀高丢包时，也允许提前进入 `DRED-3`
- 真正重突发时，才进一步升到 `DRED-5`，不再拖到尾段

`<24k` 下则改成 `LBRR-first`：

- 不允许仅因高总丢包就进入 `DRED`
- 只有满足更强 burst 条件才允许 `DRED-3`
  - `loss >= 0.20`
  - `burst >= 0.25`
  - `burst/loss >= 0.80`
  - 连续命中 `3` 个窗口
- `<24k` 下默认禁用 `DRED-5`

### 4.3 决策规则

如果**不是** `prefer-dred`，优先 `LBRR`：

- `< 5%`：`off`
- `5% ~ 12%`：`LBRR-Medium`
- `12% ~ 18%`：`LBRR-High`
- `>= 18%`：`LBRR-Ultra`

如果是 `prefer-dred` 但**不是** `burst-heavy`，优先 `DRED-3`：

- `>= 10%`：`DRED-Medium`

如果是 `burst-heavy`，再升级到更高 `DRED` 档：

- `burst-heavy` 且 `loss < 18%` 且 `burst < 25%`：`DRED-Medium`
- `loss >= 18%` 或 `burst >= 25%`：`DRED-High`

### 4.4 档位映射

`LBRR` 档位：

- `LBRR-Medium` → `FEC=true`, `plp=10`, `dred=0`
- `LBRR-High` → `FEC=true`, `plp=15`, `dred=0`
- `LBRR-Ultra` → `FEC=true`, `plp=20`, `dred=0`

`DRED-only` 档位：

- `DRED-Medium` → `FEC=false`, `plp=10`, `dred=3`
- `DRED-High` → `FEC=false`, `plp=15`, `dred=5`
- `DRED-Ultra` → `FEC=false`, `plp=20`, `dred=10`

注意：

- 当前实现里，一旦进入 `DRED` 档位，会关闭 LBRR，不混用
- `plp` 在这里仍然保留非零，因为当前 Opus 的 DRED 比特预算也依赖 `packetLossPercentage`
- 如果底层库不支持 `OPUS_SET_DRED_DURATION`，控制器会自动回退到 `LBRR-only`
- 当前实际主用的是 `DRED-Medium(dred=3)` 和 `DRED-High(dred=5)`；`DRED-Ultra` 保留为扩展档，但不是当前推荐主路径
- `<24k` 下即使控制器允许进入 `DRED`，当前也只允许到 `DRED-Medium(dred=3)`

---

## 5. REMB 保护规则

如果能解析到 `REMB`，控制器会对高冗余做上限保护：

- `< 24 kbps`：禁止进入 `Ultra`
- `< 16 kbps`：最大只允许 `Medium`

此外在 `<24k` 档位下，如果 `REMB < 16000`：

- 直接钳制回 `LBRR-Medium/High`
- 不允许进入 `DRED`

当前版本里，REMB 只负责“防止过度冗余”，不直接驱动码率变化。

---

## 6. 当前已实现的工程入口

### 6.1 sender CLI

当前 sender 支持：

- `--bitrate`
- `--adaptive-redundancy`
- `--feedback-interval`
- `--adapt-window`
- `--adapt-log`

### 6.2 实验脚本

[webrtc_demo/scripts/run_rtc_experiments.sh](/Users/leyang/Projects/tryOpus/webrtc_demo/scripts/run_rtc_experiments.sh) 当前支持：

- `SENDER_BITRATE`
- `SENDER_ADAPTIVE_REDUNDANCY`
- `SENDER_FEEDBACK_INTERVAL`
- `SENDER_ADAPT_WINDOW`
- `STRATEGY_FILTER`
- `SCENARIO_FILTER`

这允许我们单独跑 `adaptive_auto` 并测试不同码率：

```bash
cd webrtc_demo
EXPERIMENT_SUITE=standard \
SENDER_BITRATE=32000 \
SENDER_ADAPTIVE_REDUNDANCY=true \
STRATEGY_FILTER=adaptive_auto \
SCENARIO_FILTER=uniform_10,ge_moderate,ge_heavy \
bash scripts/run_rtc_experiments.sh
```

当前 sender 的 `adaptation.json` 还会额外输出：

- `bitrate_bps`
- `bitrate_tier`
- `decision_class`
- `dred_allowed`
- `dred_level_cap`

---

## 7. 已知限制

### 7.1 当前 DRED 动态档位仍有离散性

在当前 Opus 1.6.1 代码里，`DRED_DURATION` 会被离散映射成内部 `dred_chunks`。因此：

- `dred=3` 和 `dred=5` 在部分实现路径上可能落到接近的内部 chunk 数
- 导致两者在部分场景下差异很小
- 同时在固定平均码率下，`dred=5` 还可能比 `dred=3` 更挤压主音频质量，因此对 ASR 未必更优

### 7.2 结果强依赖 frame size 和码率

当前 sender 默认：

- `20ms` 分包
- `32 kbps`
- `VBR=true`

对 `DRED` 的利用率会和：

- packetization
- burst 长度分布
- 码率预算
- receiver 等到“下一包”才做恢复

共同决定。

实际已验证出的结论是：

- `24 kbps` 下 `DRED` 仍然经常能形成有效恢复
- `16 kbps` 下会出现“逻辑进入 DRED，但 `RecoveredDRED=0`”的情况
- 因此 `16 kbps` 当前不应沿用和 `24/32 kbps` 一样积极的 DRED 策略

### 7.3 当前还没有把控制轨迹完整汇总进实验报告

现在 sender 会输出 `adaptation.json`，但报告里还没有完整单独汇总：

- 每次档位切换时间
- 切换原因
- loss / burst / RTT / jitter 的轨迹

因此当前动态实验仍更偏功能验证，不是最终版性能分析报告。

---

## 8. 当前推荐测试方法

为了验证当前“码率感知、24k+ 偏 DRED、16k 偏 LBRR”的控制器，建议用：

- 起始策略：`adaptive_auto`
- adaptive：`true`
- 场景：`uniform_10`、`ge_moderate`、`ge_heavy`
- 音频：`news`、`dialogue`

原因是：

- `uniform_10` 能看是否仍然避免过度 DRED
- `ge_moderate` 能看是否更早切入 `DRED-3`
- `ge_heavy` 能看是否能快速升到 `DRED-5`

推荐命令：

```bash
cd webrtc_demo
EXPERIMENT_SUITE=standard \
RUN_ID=rtc_adaptive_probe_$(date +%Y%m%d_%H%M%S) \
SENDER_BITRATE=32000 \
SENDER_ADAPTIVE_REDUNDANCY=true \
STRATEGY_FILTER=adaptive_auto \
SCENARIO_FILTER=uniform_10,ge_moderate,ge_heavy \
bash scripts/run_rtc_experiments.sh
```

---

## 9. 下一步建议

如果继续推进，建议优先做这三件事：

1. 跑 `32 / 24 / 16 kbps × adaptive_auto / lbrr_only / dred_3 / dred_5` 定向矩阵，确认码率分层阈值是否合理
2. 在报告中继续强化“entered DRED but zero recovery”这类低码率失效标记
3. 用 `ge_heavy` 和 `10ms frame` 进一步验证 `DRED-5` 的边界收益

---

## 10. 当前验证结论（2026-03-20）

已经完成多轮动态测试，当前策略已经从“保守优先 LBRR”调整为“更偏向 DRED 来优化 ASR”。

```bash
cd webrtc_demo
EXPERIMENT_SUITE=standard \
SENDER_ADAPTIVE_REDUNDANCY=true \
STRATEGY_FILTER=adaptive_auto \
SCENARIO_FILTER=uniform_10,ge_moderate,ge_heavy \
RUN_ID=rtc_adaptive_dred_bias_20260320 \
bash scripts/run_rtc_experiments.sh
```

结果目录：

- [rtc_report.md](/Users/leyang/Projects/tryOpus/results/rtc_runs/rtc_adaptive_dred_bias_20260320/rtc_report.md)

当前状态比上一轮更进一步：

- 丢包注入已经移到 receiver-side RTP interceptor
- sender 侧 `RTCP RR` / `GetStats()` 已经能看到 `fraction_lost`
- 已经额外启用 `TWCC header extension`，sender 能持续收到 `TransportLayerCC`
- sender 侧本地 burst estimator 已接入控制器，并能在 `ge_moderate / ge_heavy` 下驱动 `DRED`
- 控制器现在会更早偏向 `DRED-3`
- 在重突发场景下会进一步升到 `DRED-5`

从 sender 轨迹和 sender 日志可以确认：

- `news / ge_heavy` 可以从开局就进入 `dred_medium`，随后升到 `dred_high`
- `news / ge_moderate` 会在 `lbrr` 与 `dred_medium` 之间动态切换
- `uniform_10` 仍以 `LBRR` 为主，但可能短暂探入 `DRED-3`

这意味着：

- **动态闭环已经真正接通**
- **控制器已经不再只偏向 `LBRR`**
- **`DRED-3` 已经成为更积极的默认 DRED 档**

当前仍然存在的限制是：

- `uniform_10` 这类场景下仍可能短暂误切 `DRED`
- `DRED-5` 并不单调优于 `DRED-3`，尤其在当前 `32 kbps + VBR` 下会和主音频质量竞争预算
- `16 kbps` 下当前最重要的问题不是“切不切 DRED”，而是“切了 DRED 也不一定产生有效恢复”

下一步若要继续收敛策略，优先建议：

1. 对比 `32 / 24 / 16 kbps` 下 `adaptive_auto` 与静态 `dred_3 / dred_5` 的 `WER`
2. 检查低码率下是否需要进一步抬高 `DRED-5` 的触发门槛
3. 进一步验证 `ge_heavy` 与 `10ms frame`，看 burst estimator 在更强突发场景下是否更稳定

补充约定：

- 定向 RTC 对照矩阵默认包含 `adaptive_auto / lbrr_only / dred_3 / dred_5`
- `ge_heavy` 场景不能只拿 `dred_3` 当静态上限；需要同时看 `dred_5`
