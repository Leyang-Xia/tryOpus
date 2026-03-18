# AGENTS.md

## Cursor Cloud specific instructions

### Project overview

Opus 编解码实验框架 (`opus_lab`) — C 语言项目，构建 3 个二进制文件：`opus_sim`（离线仿真）、`opus_sender`（UDP 发送端）、`opus_receiver`（UDP 接收端）。Python 脚本用于音频生成和结果分析。详见 `README.md`。

### Build and run

1. **前提**：`opus-install/` 目录必须存在且包含 Opus 1.6.1（含 DRED/OSCE/BWE）。若缺失，需从源码编译：
 ```
 git clone --depth 1 --branch v1.6.1 https://github.com/xiph/opus.git opus-src
 cd opus-src && ./autogen.sh && ./configure --prefix=$(pwd)/../opus-install --enable-dred --enable-deep-plc --enable-osce && make -j$(nproc) && make install && cd ..
 ```
 编译完成后需更新 DNN 权重：`cd opus-src && ./dump_weights_blob weights_blob.bin && cp weights_blob.bin .. && cd ..`
 系统自带的 `libopus0` 是 v1.4，缺少 DRED API，不可用。

2. **构建**：`mkdir -p build && cd build && cmake .. && make -j$(nproc)`

3. **运行时必须设置 `LD_LIBRARY_PATH`**：
   ```
   export LD_LIBRARY_PATH=$(pwd)/opus-install/lib:$LD_LIBRARY_PATH
   ```
   否则二进制文件会链接到系统的旧版 libopus 而崩溃。

4. **刷新标准测试音频**：`python3 tools/prepare_representative_audio.py --force`（输出到 `representative_audio/` 目录）

5. **运行仿真示例**：参见 `README.md` 的"快速开始"部分。

### Key gotchas

- `opus-src/` 和 `opus-install/` 不在 git 仓库中（被 gitignore 或未提交）。构建 Opus 需要 `autoconf`、`automake`、`libtool`。
- `weights_blob.bin`（DNN 权重）已在仓库中，DRED/DeepPLC 解码器会自动加载。
- UDP 测试 (`offline_validation/run_udp_test.sh`) 中的 `--netem` 模式需要 root 权限和 `tc` 工具。软件仿真模式无需特殊权限。
- 此项目无 lint 工具和自动化测试框架。验证通过构建成功 + 运行仿真 + Python 分析工具输出来确认。

### Cursor Cloud 环境预热（Pion WebRTC Demo）

- 启动阶段执行：`bash scripts/cursor_cloud_startup.sh`
- 该脚本会确保 Go 版本满足 `>= 1.22`：
  - 若当前环境已有合适版本，则直接复用
  - 若版本过低/缺失，则下载并安装 Go 到用户目录（`$HOME/.local/go-<version>/go`）
- 脚本随后执行：`cd webrtc_demo && go mod download`，用于预热 Pion 依赖，减少后续 agent 首次构建等待。
