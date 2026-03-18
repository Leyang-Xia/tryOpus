# 离线验证

`offline_validation/` 承接非 RTC 的标准验证入口，和顶层 `webrtc_demo/` 保持同级，同时收纳离线侧 C 源码。

包含内容：

- `src/`：`opus_sim`、`opus_sender`、`opus_receiver` 及其头文件
- `run_experiments.sh`：批量离线仿真实验矩阵，固定读取 `representative_audio/manifest.txt`
- `run_udp_test.sh`：本机 UDP 回环测试脚本

## 输出约定

离线批量实验与 UDP 回环测试默认都写入 `results/offline_runs/<RUN_ID>/`。批量离线实验会基于 ASR 生成 `WER/SER` 报告，并维护两个便捷入口：

- `results/offline_report.md`：最近一次离线批量实验报告
- `results/offline_latest`：指向最近一次离线运行目录的符号链接

## 常用命令

```bash
export LD_LIBRARY_PATH=$(pwd)/opus-install/lib:$LD_LIBRARY_PATH

# 批量离线仿真
bash offline_validation/run_experiments.sh

# UDP 回环测试
bash offline_validation/run_udp_test.sh --loss 0.1 --dred 5
```
