# BTaskAssistant v0.1.0 MVP

这是第一版可直接运行的桌面客户端，包含任务录入、需求整理、人工确认、外部开发委托记录、审核清单和最终归档的完整主链路。

## 下载

- macOS：`BTaskAssistant-macOS-universal.zip`，兼容 Apple Silicon 与 Intel Mac。解压后双击 `BTaskAssistant.app`。
- Windows：`BTaskAssistant-Windows-x64.zip`，解压后双击 `BTaskAssistant.exe`。
- `SHA256SUMS.txt`：两个压缩包的完整性校验值。

## 首次启动提示

当前测试版尚未使用 Apple Developer 或 Windows 代码签名证书。

- macOS 如果阻止启动，请右键应用并选择“打开”；仍被阻止时，前往“系统设置 → 隐私与安全性”并选择“仍要打开”。
- Windows 如果出现 SmartScreen，请选择“更多信息 → 仍要运行”。

## 数据位置

桌面数据只保存在本机用户配置目录中的 `BTaskAssistant/database/btask.db`，不会自动上传。

## 当前边界

PI / oh-my-pi 与 Codex 的本机进程调用尚未启用。本版本只生成并复制经过人工确认的提示词，外部开发完成后再把真实结果记录回来。
