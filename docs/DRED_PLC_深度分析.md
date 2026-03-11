# Opus DRED / PLC 深度技术分析

> **摘要**：本文面向本仓库 `opus_sim` / `opus_sender` / `opus_receiver` 的实现，分析 DRED（神经网络冗余恢复）与 PLC（丢包隐藏）的链路位置、触发条件、恢复窗口、参数耦合与调优方法，并给出可复现实验方案。

---

## 1. 在本项目中的恢复链路位置

在 `src/opus_sim.c` 的丢包突发处理逻辑中，恢复顺序是：

1. LBRR（仅突发末尾帧）
2. DRED（按偏移逐帧恢复）
3. PLC（兜底隐藏）
4. 静音填零（当 `--no-plc`）

核心含义是：**DRED 不是替代 PLC，而是位于 PLC 之前的“可恢复通道”**。只有当 DRED 无可用信息或解码失败时，才退化为 PLC。

---

## 2. DRED 深度分析

### 2.1 编码侧激活条件（本仓库行为）

在 `opus_sim` 中，DRED 启用至少需要同时满足：

- `-dred <n>`：设置 `OPUS_SET_DRED_DURATION(n)`。
- 载入 `weights_blob.bin` 并通过 `OPUS_SET_DNN_BLOB` 注入编码器。
- `OPUS_SET_PACKET_LOSS_PERC(plp)` 非零（影响冗余预算）。

`opus_sim` 有一个很关键的“自动兜底”：

- 若用户设置了 `-dred` 但未显式给 `-plp`，程序会从 `-l/--loss` 推导 `plp`。
- 当推导值过低时，会提升到至少 10%，避免 DRED 比特预算为 0。

这意味着在离线仿真里，DRED 更容易被“正确激活”；但在 `opus_sender` 里没有同样的自动兜底，实时发送时建议显式传 `-plp`。

### 2.2 解码侧恢复窗口与偏移公式

`opus_sim` 在一个丢包突发 `[burst_start..burst_end]` 后，找到第一个到达包 `next_ok`，然后：

1. 对 `next_ok` 包执行 `opus_dred_parse(...)`。
2. 对突发内每个丢失帧 `j` 调用  
   `opus_decoder_dred_decode(dec, dred, dred_off, out, frame_samples)`。
3. 偏移公式为：`dred_off = (next_ok - j) * frame_samples`。

这个偏移公式的物理含义是“从未来包携带的历史冗余里，取回距当前多少帧之前的音频”。  
例如 20ms 帧长、48kHz 时 `frame_samples=960`：

- 若 `next_ok-j=1`，偏移 `960`（恢复最近一帧历史）
- 若 `next_ok-j=3`，偏移 `2880`（恢复更早的丢失帧）

### 2.3 DRED 覆盖上限与 burst 长度关系

`opus_sim` 里有两个直接影响覆盖能力的边界：

- **编码配置边界**：`-dred <n>` 决定可恢复历史的时间窗。
- **实现边界**：解析时 `max_need` 被限制到 48000 样本（1 秒@48k）。

因此可近似理解为：

`可恢复突发长度 <= min(配置窗口, 1秒解析上限)`

如果突发长度超过窗口，超出部分会回退到 PLC。

### 2.4 离线仿真与实时接收的实现差异

`opus_receiver` 中 DRED 依赖抖动缓冲窥视 `next_seq` 的“下一包”：

- 解析窗口使用 `consecutive_lost * frame_samples`
- 解码偏移使用 `(consecutive_lost - 1) * frame_samples`

这套逻辑在连续丢包场景下可工作，但受抖动缓冲深度和下一包可见性影响更大。  
所以同一参数下，实时链路效果会更依赖 `-jbuf` 和网络时序，而离线仿真结果更“理想可复现”。

---

## 3. PLC 深度分析

### 3.1 PLC 的触发点与语义

本项目 PLC 调用统一为：

`opus_decode(dec, NULL, 0, pcm_out, frame_samples, 0)`

触发条件是：该帧未被 LBRR 或 DRED 恢复，且 `--no-plc` 未开启。  
PLC 不是“恢复原始音频”，而是根据解码器内部历史状态做波形外推和能量平滑，因此：

- 单帧丢失时通常主观连续性较好；
- 连续突发丢失时失真会快速累积；
- 对强瞬态、音乐类内容通常弱于 DRED。

### 3.2 PLC 与 DTX / 静音填零的区别

在 `opus_sim` 中，`pkt_size<=1` 会被记为 DTX 帧并写静音，这不是 PLC。  
另外当用户指定 `--no-plc` 时，丢包帧也会走静音填零（`FRAME_LOST`），用于做“无隐藏”基线对比。

### 3.3 为什么 PLC 必须保留为兜底

即使启用 DRED，PLC 仍是必要的：

- 未来包本身也可能丢失，DRED 信息不可达；
- 丢包长度超出 DRED 窗口；
- 未加载 DNN blob 或 DRED 解码异常。

因此工程上应视作：**DRED 提升恢复率，PLC 保底连续性**。

---

## 4. DRED 与 PLC 的系统级权衡

| 维度 | DRED | PLC |
|---|---|---|
| 额外码率开销 | 有（随 `-dred` 与 `plp` 增加） | 无 |
| 计算开销 | 更高（含 DNN） | 低 |
| 连续突发恢复 | 强（窗口内可逐帧恢复） | 弱（靠外推） |
| 对未来包依赖 | 有（需后续包携带冗余） | 无 |
| 极端丢包可用性 | 受窗口限制 | 始终可用 |

一个常见误区是盲目增大 `-dred`。  
`-dred` 增大确实提升覆盖窗口，但会挤占主码流预算；在固定总码率下，正常未丢包帧质量可能下降。应结合 `tools/analyze.py` 的 SNR/SegSNR 做整体最优而非只看恢复率。

---

## 5. 参数联动建议（工程实践）

### 5.1 三个关键控制旋钮

- `-dred <n>`：恢复历史窗口（10ms 单位）
- `-plp <pct>`：编码器分配冗余预算的先验
- `-b <bps>`：总预算上限（主流质量与冗余强度的总和）

推荐调参顺序：

1. 先用网络统计确定目标 burst（例如 95 分位连续丢包长度）。
2. 将 `-dred` 设到可覆盖该 burst 的最小值。
3. 用 `-plp` 对齐真实丢包水平（不要长期严重高估）。
4. 最后调 `-b`，平衡“恢复率”和“无丢包音质”。

### 5.2 典型配置建议

- **轻度随机丢包（<5%）**：`-dred 2~3`，避免过大冗余。
- **中度随机丢包（5~10%）**：`-dred 3~5`，`-plp` 与真实值对齐。
- **突发丢包明显（GE）**：`-dred 5~10`，并提升码率防止主流质量塌陷。

---

## 6. 可复现实验矩阵（建议直接使用）

先准备：

1. `export LD_LIBRARY_PATH=$(pwd)/opus-install/lib:$LD_LIBRARY_PATH`
2. `python3 tools/gen_audio.py`

然后做三组对比：

### A. PLC 基线（禁用 DRED/LBRR）

`./build/opus_sim -l 0.1 --no-lbrr --no-dred audio/speech_like.wav results/plc_10.wav --csv results/plc_10.csv`

### B. DRED 增益（同网络条件）

`./build/opus_sim -l 0.1 -dred 3 -plp 10 audio/speech_like.wav results/dred3_10.wav --csv results/dred3_10.csv`

### C. 突发场景（检验窗口能力）

`./build/opus_sim -ge -ge-p2b 0.05 -ge-b2g 0.3 -ge-bloss 0.8 -dred 5 -plp 15 audio/speech_like.wav results/ge_dred5.wav --csv results/ge_dred5.csv`

用分析脚本查看音质变化：

- `python3 tools/analyze.py --ref audio/speech_like.wav --deg results/plc_10.wav`
- `python3 tools/analyze.py --ref audio/speech_like.wav --deg results/dred3_10.wav`
- `python3 tools/analyze.py --csv results/ge_dred5.csv`

---

## 7. 常见问题定位清单

### Q1: 配了 `-dred` 但统计里 `DRED恢复` 接近 0

按顺序排查：

1. 是否加载到 `weights_blob.bin`（日志会打印“已加载DNN权重”）。
2. 是否设置了合理 `-plp`（实时发送端建议显式设置）。
3. `-dred` 是否足以覆盖真实 burst。
4. 是否被 `--no-dred` 关闭（发送端或接收端）。

### Q2: DRED 开大后听感反而变差

典型原因是总码率固定时冗余占比过高，主流质量被压缩。  
建议减少 `-dred` 或提高 `-b`，并用 `tools/analyze.py` 同时看恢复率和 SNR。

### Q3: 是否可以禁用 PLC 只看 DRED？

可以（`--no-plc`）用于研究边界，但不建议用于实际链路。  
真实系统应保留 PLC 作为 DRED 失败时的最后兜底。

---

## 8. 结论

- DRED 在本项目中承担“可恢复通道”，对连续丢包显著优于纯 PLC。
- PLC 是必须保留的兜底机制，保障在 DRED 不可用时的连续性。
- 最优配置不是单纯追求更大 `-dred`，而是基于网络 burst 分布做窗口匹配，再用码率和 `plp` 做联合优化。
