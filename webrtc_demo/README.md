# Pion WebRTC 轻量 Demo

这个目录提供一个最小可运行的 Pion WebRTC 示例：

- Go 服务端使用 `pion/webrtc` 建立 `PeerConnection`
- 浏览器侧通过 `/offer` 接口完成 SDP 交换
- 双方通过 DataChannel 通信，服务端自动回显消息

## 运行

```bash
cd webrtc_demo
go mod tidy
go run .
```

启动后访问：`http://127.0.0.1:8080`

## 使用步骤

1. 点击“连接”
2. 等待日志出现 `DataChannel 已打开`
3. 输入任意文本并点击“发送”
4. 页面日志会显示服务端回显

## 测试

```bash
cd webrtc_demo
go test -v .
```

该测试会在本地启动 HTTP 服务，自动走完 Offer/Answer 握手并校验 DataChannel 回显。
