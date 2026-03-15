# Pion WebRTC 轻量 Demo

本目录现在包含两个层次的 WebRTC Demo：

1. **DataChannel 浏览器演示**（原有）
2. **方案 B：Pion 音频 P2P 轻量实现**（新增）

---

## 1) DataChannel 浏览器演示

```bash
cd webrtc_demo
go run .
```

浏览器打开 `http://127.0.0.1:8080`，点击“连接”后可发送消息并看到回显。

---

## 2) 方案 B：Pion 音频 P2P 轻量实现

对应 `docs/端到端落地方案.md` 的 **方案 B**，目录如下：

```
webrtc_demo/
├── signaling/            # HTTP 信令服务
├── sender/               # WAV -> Opus -> WebRTC 发送
├── receiver/             # WebRTC 接收 -> Opus 解码 -> WAV
├── scripts/run_test.sh   # 一键端到端测试
└── scripts/gen_tone.py   # 生成 48k 单声道测试音频
```

### 手动运行

终端 1：启动信令

```bash
cd webrtc_demo
go run ./signaling -addr :8090
```

终端 2：启动接收端

```bash
cd webrtc_demo
go run ./receiver \
  --signal http://127.0.0.1:8090 \
  --session demo-session \
  --output /tmp/received.wav \
  --duration 8s
```

终端 3：启动发送端

```bash
cd webrtc_demo
go run ./sender \
  --signal http://127.0.0.1:8090 \
  --session demo-session \
  --input /path/to/input_48k_mono.wav
```

### 一键端到端测试

```bash
cd webrtc_demo
bash scripts/run_test.sh
```

脚本会自动：

- 启动信令服务
- 生成测试音频
- 启动 receiver/sender 建链并传输
- 校验输出 WAV 是否生成

### 当前实现说明

- 发送端使用 `github.com/hraban/opus` 进行 Opus 编码，支持 FEC/PLP 配置。
- 接收端进行 Opus 解码并输出 WAV，同时打印接收统计。
- 网络损伤（`tc netem`）需要在宿主机上单独注入。
- **DRED API 仍需自定义 libopus 绑定支持**，本版先完成方案 B 的 WebRTC 音频链路最小落地。

---

## 自动化测试

```bash
cd webrtc_demo
go test -v .
```

该测试用于校验浏览器 DataChannel 示例的 Offer/Answer 与回显逻辑。
