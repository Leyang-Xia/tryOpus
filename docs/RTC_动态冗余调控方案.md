# RTC 动态冗余调控方案

> 日期：2026-03-19
>
> 适用范围：`webrtc_demo/`

本文总结当前 `webrtc_demo` 中已经实现的 sender 侧动态冗余调控方案。目标不是动态调码率，而是在标准 RTC 反馈闭环下，根据链路丢包总量与突发特征，在 `LBRR` 与 `DRED` 之间切换，并动态更新 Opus 编码器参数。

---

## 1. 目标

当前方案解决的问题是：

- 在 RTC 链路中实时读取标准反馈
- 用平滑后的链路状态决定当前更适合 `LBRR` 还是 `DRED`
- 将决策映射为 Opus encoder CTL

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
- `transport-cc` 到达标记（当前只作观测）

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

当前 sender 侧控制器位于 [webrtc_demo/sender/main.go](/Users/leyang/Projects/tryOpus/webrtc_demo/sender/main.go)。

默认参数：

- 采样周期：`1s`
- EMA 系数：`0.35`
- 平滑窗口目标：约 `5s`
- 升档：连续 `2` 次命中更高档位
- 降档：连续 `5` 次命中更低档位
- 冷却时间：`3s`

这样做的目的是避免：

- 单个采样点抖动导致频繁切换
- 刚升档后又立即降档
- burst 短时出现时过度反应

---

## 4. 模式与档位

实现中的模式枚举定义在 [webrtc_demo/internal/adaptation/adaptation.go](/Users/leyang/Projects/tryOpus/webrtc_demo/internal/adaptation/adaptation.go)。

### 4.1 模式判定

先判断是否进入 `burst-heavy`：

- 进入条件：
  - `fractionLostEMA >= 0.08`
  - `burstLossRateEMA >= 0.15`

- 退出条件：
  - 当前实现采用同一套目标模式计算 + 滞回机制，没有单独再暴露显式退出状态机

### 4.2 决策规则

如果**不是** `burst-heavy`，只允许 `LBRR`：

- `< 5%`：`off`
- `5% ~ 12%`：`LBRR-Medium`
- `12% ~ 18%`：`LBRR-High`
- `>= 18%`：`LBRR-Ultra`

如果是 `burst-heavy`，只允许 `DRED-only`：

- `8% ~ 12%`：`DRED-Medium`
- `12% ~ 18%`：`DRED-High`
- `>= 18%`：`DRED-Ultra`

### 4.3 档位映射

`LBRR` 档位：

- `LBRR-Medium` → `FEC=true`, `plp=10`, `dred=0`
- `LBRR-High` → `FEC=true`, `plp=15`, `dred=0`
- `LBRR-Ultra` → `FEC=true`, `plp=20`, `dred=0`

`DRED-only` 档位：

- `DRED-Medium` → `FEC=false`, `plp=0`, `dred=3`
- `DRED-High` → `FEC=false`, `plp=0`, `dred=5`
- `DRED-Ultra` → `FEC=false`, `plp=0`, `dred=10`

注意：

- 当前实现里，一旦判断为 burst-heavy，会关闭 LBRR，不混用
- 如果底层库不支持 `OPUS_SET_DRED_DURATION`，控制器会自动回退到 `LBRR-only`

---

## 5. REMB 保护规则

如果能解析到 `REMB`，控制器会对高冗余做上限保护：

- `< 24 kbps`：禁止进入 `Ultra`
- `< 16 kbps`：最大只允许 `Medium`

当前版本里，REMB 只负责“防止过度冗余”，不直接驱动码率变化。

---

## 6. 当前已实现的工程入口

### 6.1 sender CLI

当前 sender 支持：

- `--adaptive-redundancy`
- `--feedback-interval`
- `--adapt-window`
- `--adapt-log`

### 6.2 实验脚本

[webrtc_demo/scripts/run_rtc_experiments.sh](/Users/leyang/Projects/tryOpus/webrtc_demo/scripts/run_rtc_experiments.sh) 当前支持：

- `SENDER_ADAPTIVE_REDUNDANCY`
- `SENDER_FEEDBACK_INTERVAL`
- `SENDER_ADAPT_WINDOW`
- `STRATEGY_FILTER`
- `SCENARIO_FILTER`

这允许我们单独跑“adaptive baseline”：

```bash
cd webrtc_demo
EXPERIMENT_SUITE=quick \
SENDER_ADAPTIVE_REDUNDANCY=true \
STRATEGY_FILTER=baseline \
SCENARIO_FILTER=uniform_10,ge_moderate \
bash scripts/run_rtc_experiments.sh
```

---

## 7. 已知限制

### 7.1 当前 DRED 动态档位的离散性很强

在当前 Opus 1.6.1 代码里，`DRED_DURATION` 会被离散映射成内部 `dred_chunks`。因此：

- `dred=3` 和 `dred=5` 很可能落到同一档内部 chunk 数
- 导致两者在部分场景下差异很小

### 7.2 结果强依赖 frame size

当前 sender 默认 `20ms` 分包。对 `DRED_DURATION` 的利用率会和：

- `20ms` packetization
- burst 长度分布
- receiver 等到“下一包”才做恢复

共同决定。

### 7.3 当前还没有把控制轨迹写入实验报告

现在 sender 会打印动态日志，但报告里还没有单独汇总：

- 每次档位切换时间
- 切换原因
- loss / burst / RTT / jitter 的轨迹

因此当前动态实验更偏功能验证，不是最终版性能分析报告。

---

## 8. 当前推荐测试方法

为了验证动态控制器本身，建议用：

- 起始策略：`baseline`
- adaptive：`true`
- 场景：`uniform_10`、`ge_moderate`
- 音频：`news`、`dialogue`

原因是：

- 起始 `baseline` 最干净，所有冗余都由控制器自己打开
- `uniform_10` 能看是否稳定进入 `LBRR`
- `ge_moderate` 能看是否切入 `DRED-only`

推荐命令：

```bash
cd webrtc_demo
EXPERIMENT_SUITE=quick \
RUN_ID=rtc_adaptive_probe_$(date +%Y%m%d_%H%M%S) \
SENDER_ADAPTIVE_REDUNDANCY=true \
STRATEGY_FILTER=baseline \
SCENARIO_FILTER=uniform_10,ge_moderate \
bash scripts/run_rtc_experiments.sh
```

---

## 9. 下一步建议

如果继续推进，建议优先做这三件事：

1. 把控制器切换日志写入 `stats.json` 或单独的 `adaptation.json`
2. 在报告中增加“动态轨迹对比”表，而不只看最终 `WER/SER`
3. 用 `ge_heavy` 和 `10ms frame` 进一步验证 `DRED-High / Ultra` 的边界收益

---

## 10. 当前验证结论（2026-03-19）

已经完成两轮动态测试。

```bash
cd webrtc_demo
EXPERIMENT_SUITE=quick \
SENDER_ADAPTIVE_REDUNDANCY=true \
STRATEGY_FILTER=adaptive_auto \
SCENARIO_FILTER=uniform_10,ge_moderate \
RUN_ID=rtc_adaptive_auto_20260319 \
bash scripts/run_rtc_experiments.sh
```

结果目录：

- [rtc_report.md](/Users/leyang/Projects/tryOpus/results/rtc_runs/rtc_adaptive_auto_20260319/rtc_report.md)

当前状态比第一轮验证更进一步：

- 现在丢包注入已经移到 receiver-side RTP interceptor
- sender 侧 `RTCP RR` / `GetStats()` 已经能看到 `fraction_lost`
- 在 `uniform_10` 场景下，控制器会从 `off` 升到 `LBRR-Medium/High`
- `adaptive_auto` 的恢复率也已经优于静态 `baseline`

这意味着：

- **动态闭环已经真正接通**
- 但**当前还没有稳定拿到 burst 指标**

当前仍然存在的限制是：

- `ge_moderate` 场景下 sender 看到的 `burstLossRate` 仍接近 0
- 因此控制器尚未切入 `DRED-only`
- 现在的动态策略实际表现为“基于总丢包率的 LBRR 自适应”

下一步若要验证 `DRED-only` 动态切换，优先建议：

1. 检查当前 Pion `RemoteInboundRTPStreamStats.BurstLossRate` 在本地 P2P 场景下为何始终为 0
2. 如该指标在当前栈中不可用，则补一个基于 RTCP RR 序列模式或接收端低层 RTP gap 的 burst 估计，并通过标准可见路径驱动
