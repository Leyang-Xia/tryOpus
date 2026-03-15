package main

const indexHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Pion WebRTC 轻量 Demo</title>
  <style>
    body { font-family: system-ui, sans-serif; margin: 24px; line-height: 1.5; }
    h1 { margin: 0 0 12px; font-size: 22px; }
    .row { display: flex; gap: 8px; margin-bottom: 10px; }
    input { flex: 1; padding: 8px; }
    button { padding: 8px 12px; cursor: pointer; }
    #log {
      border: 1px solid #ddd;
      border-radius: 8px;
      padding: 10px;
      background: #f8f8f8;
      height: 260px;
      overflow: auto;
      white-space: pre-wrap;
      font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
      font-size: 13px;
    }
  </style>
</head>
<body>
  <h1>Pion WebRTC DataChannel Demo</h1>
  <p>点击“连接”后，浏览器会向 Go/Pion 服务端发起 WebRTC 握手，建立 DataChannel 并支持消息回显。</p>

  <div class="row">
    <button id="connectBtn">连接</button>
    <button id="sendBtn" disabled>发送</button>
  </div>
  <div class="row">
    <input id="msg" placeholder="输入消息，比如 hello pion" />
  </div>
  <div id="log"></div>

  <script>
    let pc = null;
    let dc = null;

    const logEl = document.getElementById("log");
    const connectBtn = document.getElementById("connectBtn");
    const sendBtn = document.getElementById("sendBtn");
    const msgInput = document.getElementById("msg");

    function log(message) {
      const now = new Date().toLocaleTimeString();
      logEl.textContent += "[" + now + "] " + message + "\n";
      logEl.scrollTop = logEl.scrollHeight;
    }

    async function connect() {
      if (pc) {
        log("连接已存在，无需重复创建。");
        return;
      }

      pc = new RTCPeerConnection();
      dc = pc.createDataChannel("demo");

      dc.onopen = () => {
        log("DataChannel 已打开。");
        sendBtn.disabled = false;
      };

      dc.onmessage = (event) => {
        log("收到服务端消息: " + event.data);
      };

      dc.onclose = () => {
        log("DataChannel 已关闭。");
        sendBtn.disabled = true;
      };

      pc.oniceconnectionstatechange = () => {
        log("ICE 状态: " + pc.iceConnectionState);
      };

      const offer = await pc.createOffer();
      await pc.setLocalDescription(offer);
      log("已生成本地 Offer，开始请求服务端 Answer...");

      const resp = await fetch("/offer", {
        method: "POST",
        headers: {"Content-Type": "application/json"},
        body: JSON.stringify(pc.localDescription),
      });
      if (!resp.ok) {
        const txt = await resp.text();
        throw new Error("信令失败: " + resp.status + " " + txt);
      }

      const answer = await resp.json();
      await pc.setRemoteDescription(answer);
      log("已设置远端 Answer，等待连接建立...");
    }

    connectBtn.addEventListener("click", async () => {
      connectBtn.disabled = true;
      try {
        await connect();
      } catch (err) {
        log("连接失败: " + err.message);
        connectBtn.disabled = false;
      }
    });

    sendBtn.addEventListener("click", () => {
      if (!dc || dc.readyState !== "open") {
        log("DataChannel 未就绪。");
        return;
      }
      const text = msgInput.value.trim() || "ping";
      dc.send(text);
      log("发送: " + text);
      msgInput.value = "";
      msgInput.focus();
    });
  </script>
</body>
</html>
`
