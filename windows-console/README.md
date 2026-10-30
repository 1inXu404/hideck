# HiDeck 简易控制台

双击项目根目录的 **HiDeck 控制台.lnk**。这是本机窗口，运行环境已准备好，不需要每次输入命令。

1. 插上已经完成 USB 身份适配的大疆一代模块。
2. 点击“启动”：自动找到 USB 编号、启动 Ubuntu、连接模块、检查 QMI 接口、启动 HiDeck，并在默认浏览器打开当前 WSL IP 的管理页面。
3. 用完后点击“停止”。看到“已停止 · 可以拔下模块”后，手动拔下模块。

首次使用新的 USB 连接且尚未共享设备时，会出现 Windows 管理员授权提示；日常连接已共享的设备无需管理员终端。控制台不会自动使用 `--force`。

“停止”会停止 `hideck.service`；当前服务的 `KillMode=control-group` 会一起结束它启动的 `qmi-proxy`。随后释放选定 USB 设备并结束控制台创建的 WSL 保活进程。它不会关闭整个 Ubuntu 发行版、全局 usbipd 服务或用户的浏览器。网页标签保留，但停止后服务无法访问。

控制台负责连接与进程启停。SIM 的飞行模式、流量、短信和电话仍在 HiDeck 网页里控制。退出窗口时，若服务或 USB 仍在使用，会提示先停止。

## 当前安装

- WSL 发行版：`Ubuntu-26.04`
- 服务：`hideck.service`
- 网页端口：`7575`，直接使用 WSL 的 Windows 侧网卡 IP
- 支持的目标 USB 身份：`2c7c:0125`；原始 `2ca3:4006` 只识别并提示先适配
- Windows GUI：Python 3.12 + tkinter，只有标准库；独立环境在本目录 `.venv/`
- 错误记录：本目录 `console.log`（自动轮转）

USB 编号和 WSL IP 每次都会重新查询，换端口或重启 WSL 后不用改代码。检测到多个匹配设备时需要在下拉列表中选择。

## 重新创建环境

仅在移动目录或删除虚拟环境后需要，在项目根目录的 PowerShell 执行：

```powershell
uv venv windows-console/.venv --python 3.12
.\windows-console\.venv\Scripts\pythonw.exe .\windows-console\app.py
```

也可通过 `windows-console/Start-HiDeck-Control.ps1` 启动现有环境。没有更改 PowerShell 执行策略；若本机策略不允许执行脚本，直接使用快捷方式。

移动项目后，需重新创建指向新路径下 `.venv\Scripts\pythonw.exe` 和 `app.py` 的快捷方式。
