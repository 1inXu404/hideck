# 在这台 Windows 电脑上按需使用 HiDeck 与大疆一代 4G 模块

本指南按“使用时插 USB，不使用时拔下”的场景整理。

源码已克隆到 `D:\mess\hideck`，分支 `main`，核对版本为 `dc4b173a5261a5e0832a26b04a1d6c42415e4e26`（2026-09-03）。本文是本地部署指南，不是上游的实机兼容认证。

## 这台电脑已经具备的条件

首次准备时检查到：

- Windows 中已有 `Ubuntu-26.04`，使用 WSL 2。
- Ubuntu 已启用 systemd，内核为 `6.18.33.2-microsoft-standard-WSL2`。
- `option`、`qmi_wwan`、`cdc_wdm`、`vhci_hcd` 的内核配置和驱动文件均存在。
- Ubuntu 中已有 curl、Git 和 kmod；尚未找到 usbutils、socat 或 `hideck.service`。
- Windows 尚未找到 usbipd 命令、服务或默认路径下的程序。

这些检查证明运行环境具备基础条件；尚未完成 USB 模块连接、型号确认和 HiDeck 实机测试。

使用方式：

```text
Windows 浏览器：http://<WSL-IP>:7575
                 ↓
WSL 2 / Ubuntu-26.04：HiDeck Linux 二进制
                 ↓ USB/IP
Windows USB 接口上的大疆一代 4G 模块
```

HiDeck 的上游部署对象是 Linux 主机。本流程把程序安装在 Ubuntu 内的 `/opt/hideck`，通过 USB 管理模块。下面注明 PowerShell 的命令在 Windows 中执行，注明 Ubuntu 的命令在 WSL 内执行。

## 一次性准备

### 1. Windows 安装 USB 连接工具

以管理员身份打开 PowerShell：

```powershell
winget install --interactive --exact dorssel.usbipd-win
```

安装结束后重新打开 PowerShell，确认：

```powershell
usbipd --version
```

如果安装器要求重启，先保存其他工作后重启。`--interactive` 用于让安装器展示交互提示。

这是微软文档支持的 WSL USB 连接方式；USB 交给 WSL 时，Windows 程序不能同时使用该设备。参见 [微软 WSL USB 指南](https://learn.microsoft.com/en-us/windows/wsl/connect-usb)。

### 2. 准备 Ubuntu

在普通 PowerShell 中打开 Ubuntu，并在连接 USB 期间保持这个窗口打开：

```powershell
wsl -d Ubuntu-26.04
```

接下来的命令在这个 Ubuntu 窗口中执行：

```bash
sudo apt-get update
sudo apt-get install -y usbutils socat ca-certificates libqmi-proxy
sudo modprobe option
sudo modprobe qmi_wwan
sudo modprobe cdc_wdm
```

`sudo` 要求输入的是 Ubuntu 用户密码，输入时没有字符回显。

`libqmi-proxy` 是 QMI 控制通道需要的组件。核对版本的二进制安装脚本只会补装录音库，没有安装它；缺失时页面可能一直显示“QMI 启动中”，后台报 `/usr/libexec/qmi-proxy: no such file or directory`。

### 3. 安装 HiDeck，设为手动启动

继续在 Ubuntu 中执行。以下下载的是上面已核对提交中的官方安装脚本；脚本会从 Releases 获取最新二进制并校验 SHA-256：

```bash
curl -fL --retry 3 \
  https://raw.githubusercontent.com/yibaiba/hideck/dc4b173a5261a5e0832a26b04a1d6c42415e4e26/deploy-binary.sh \
  -o /tmp/hideck-deploy-binary.sh
sudo env HIDECK_DIR=/opt/hideck sh /tmp/hideck-deploy-binary.sh
```

确认安装成功后，关闭开机自启并停止它，留待接好模块再启动：

```bash
sudo systemctl disable --now hideck.service
systemctl is-enabled hideck.service
```

预期最后显示 `disabled`；该查询在 disabled 状态下返回非零退出码是正常的。

安装脚本会创建配置和数据目录、安装录音相关依赖，并短暂启动服务。上面的 `disable --now` 会关闭其自启并停止服务。以后不必重复安装，只需手动启动。

主要路径：

| 内容 | Ubuntu 路径 |
| --- | --- |
| 程序 | `/opt/hideck/hideck` |
| 配置 | `/opt/hideck/config/config.yaml` |
| 数据库与其他数据 | `/opt/hideck/data/` |
| 日志目录 | `/opt/hideck/logs/` |

如果下载或校验失败，不要继续把它当作安装成功。可查看 [上游 Releases](https://github.com/yibaiba/hideck/releases)；该脚本也支持通过 `HIDECK_VERSION` 指定实际存在的版本。

## 首次连接和识别模块

### 4. 把 USB 设备交给 WSL

先在模块断电时插好 SIM，再用支持数据传输的 USB 线连接 Windows。保持 Ubuntu 窗口打开。

另开一个管理员 PowerShell：

```powershell
usbipd list
```

找到模块对应的行。项目文档涉及的原始标识是 `2ca3:4006`，改为移远身份后是 `2c7c:0125`；设备名也可能显示为 Baiwang 或 Quectel。

找到该行的实际 BUSID。下面以本机曾出现的 `2-2` 为例；如果当前列表里的编号不同，先修改第一行的值，再逐行执行：

```powershell
$ModemBusId = '2-2'
usbipd bind --busid $ModemBusId
usbipd attach --wsl --busid $ModemBusId
usbipd list
```

模块应变为 `Attached`。`bind` 需要管理员权限；后续 `attach` 可以在普通 PowerShell 中执行。

回到 Ubuntu：

```bash
lsusb
lsusb -t
ls -l /dev/ttyUSB* /dev/cdc-wdm*
```

判断结果：

- 已出现 `2c7c:0125`，并有 QMI 控制口和 AT 口：跳过下一节的 USB 身份修改。
- 仍为 `2ca3:4006`：按下一节先识别型号和 AT 口。
- `lsusb` 都看不到模块：先解决线缆、Windows 枚举或 USB/IP 连接问题。
- 只有 AT 口而没有 `/dev/cdc-wdm*`：还不能据此认定 QMI 数据功能已可用。

### 5. 仅在匹配型号且需要适配时修改 USB 身份

上游将这组命令限定为：当前 USB ID 是 `2ca3:4006`，且已确认底层为移远 EC25 的大疆定制模块。

**USB ID 本身不能证明型号兼容。`usbcfg` 会持久化修改 USB 身份与接口组合；错误参数可能使 AT 口或网络接口消失。型号未确认时，只做查询。**

首先确认 HiDeck 已停止。在 Ubuntu 中临时让串口驱动识别 DJI USB ID：

```bash
sudo modprobe option
printf '2ca3 4006\n' | sudo tee /sys/bus/usb-serial/drivers/option1/new_id
ls -l /dev/ttyUSB*
```

这条 `new_id` 写入的是本次 Linux 运行的驱动匹配表，不会修改模块闪存。

项目示例使用 `/dev/ttyUSB2`，但实际编号可能变化。先根据插拔前后的节点变化定位模块的端口，再用终端确认 AT 响应。以下仅以 `/dev/ttyUSB2` 为例：

```bash
sudo socat - /dev/ttyUSB2,raw,echo=0,crnl,b115200
```

进入 socat 后，以下是发给模块的 AT 指令，每行输入后回车，不是 Linux shell 命令：

```text
AT
ATI
AT+CGMM
AT+CGMR
AT+QCFG="usbcfg"?
AT+QCFG="usbnet"?
AT+QSIMDET?
AT+CPIN?
```

需要确认端口能返回 `OK`，核对型号与固件，并保存原始 USB 配置。某些定制固件会拒绝查询；如果返回 `ERROR`、型号不明或与上游条件不符，先保留输出排查，不能把通用参数当作已验证配置。

只有确认满足上述条件、确实需要改身份时，才在 AT 终端执行上游命令：

```text
AT+QCFG="usbcfg",0x2C7C,0x0125,1,1,1,1,1,0,0
```

收到 `OK` 后重启模块：

```text
AT+CFUN=1,1
```

USB 会断开并重新枚举。退出 socat（Ctrl+C），回到 PowerShell 重新运行 `usbipd list`，按新的设备行重新设置 `$ModemBusId`，再执行 `bind` 和 `attach`。不要假定重启后仍自动连接到 WSL。

再次在 Ubuntu 中核对 `lsusb`、`lsusb -t` 和 `/dev/cdc-wdm*`。预期 USB 身份为 `2c7c:0125`，QMI 接口使用 `qmi_wwan` 驱动。若接口缺失，记录当前 USB 模式与固件后排查，不继续尝试随机接口组合。上游实测的大疆定制 EC25 在 MBIM 模式下可能枚举成功却无法通信，不要仅为出现网卡而切换 MBIM。

如果已确认是上述定制模块，插好实体卡后仍不认卡，且 `QSIMDET` 未关闭，可按上游说明在 AT 终端设置：

```text
AT+QSIMDET=0,0
AT+CFUN=1,1
```

这会持久化关闭 SIM 检测脚功能。之后应断电换卡；重新连接后用 `AT+CPIN?` 验证，正常返回 `+CPIN: READY`。不要无依据地改成 `QSIMDET=1,0`。模块再次重启时同样需要检查 USB/IP 是否重新连接。

以上依据 [上游模组硬件说明](https://github.com/yibaiba/hideck/blob/dc4b173a5261a5e0832a26b04a1d6c42415e4e26/docs/modem-hardware.md)。这些硬件配置只在必要时做一次，不是每次插入都执行。

## 每次使用时

首次硬件适配完成后，通常只需下面几步。

1. 插上模块，在 PowerShell 执行 `wsl -d Ubuntu-26.04`，保持 Ubuntu 窗口打开。
2. 在另一个 PowerShell 中执行：

```powershell
usbipd list
$ModemBusId = '2-2' # 替换为本次列表中大疆模块的实际 BUSID
usbipd attach --wsl --busid $ModemBusId
```

如果显示未共享，先在管理员 PowerShell 中对当前 BUSID 执行 `usbipd bind --busid $ModemBusId`。换 USB 插口或修改 USB 身份后尤其要重新确认。

3. 在 Ubuntu 中确认设备已连接，再启动服务：

```bash
lsusb
sudo systemctl start hideck.service
systemctl status hideck.service --no-pager
```

4. 在 PowerShell 查询当前 WSL IP：

```powershell
wsl -d Ubuntu-26.04 -- hostname -I
```

在 Windows 浏览器打开 `http://查询到的IP:7575`。本机曾验证可访问的地址是 [HiDeck 管理页面](http://172.18.157.220:7575/)，但 WSL 重启后 IP 可能变化。初始账号、密码均为 `admin`，首次登录后立即修改。设备页应显示模块及 SIM 状态。

先验证设备识别和 SIM 状态，再测试需要的短信或网络功能。若只管理短信，可先保持电话功能关闭；不用为了登录管理页而开启模块的数据流量。

Windows 通常能通过 localhost 访问 WSL 中的服务，见 [微软 WSL 网络文档](https://learn.microsoft.com/en-us/windows/wsl/networking)。但本机实测 Windows 的 `127.0.0.1:7575` 转发失败，而直连 WSL IP 返回 HTTP 200，因此按用户选择使用 WSL IP，不修改 Windows 转发设置。

## 每次用完时

先结束正在进行的通话、短信发送或 eSIM 写入，在 Ubuntu 中停止服务：

```bash
sudo systemctl stop hideck.service
```

在保留 `$ModemBusId` 的 PowerShell 窗口中断开 USB/IP，然后拔下模块：

```powershell
usbipd detach --busid $ModemBusId
```

如果换了 PowerShell 窗口，先用 `usbipd list` 查询当前 BUSID，再重新设置变量。若模块已经物理拔下，无需为了 detach 再插回去。

确认 Ubuntu 没有其他任务运行时，也可以结束这个 WSL 发行版，释放资源：

```powershell
wsl --terminate Ubuntu-26.04
```

停止服务或关闭 WSL 不会删除配置及已保存的短信记录。模块拔掉后，HiDeck 无法继续接收新的短信或来电。

## 常见问题

| 现象 | 下一步 |
| --- | --- |
| PowerShell 找不到 `usbipd` | 安装后重新打开终端，确认安装器成功结束 |
| Windows 有设备，Ubuntu 的 `lsusb` 没有 | 保持 WSL 窗口打开，检查 `usbipd list` 是否显示 Attached |
| 修改身份或重启模块后消失 | 重新查询 USB 列表，对新的设备行重新 bind / attach |
| `/dev/ttyUSB2` 不存在 | 查看实际 `/dev/ttyUSB*` 节点，编号不固定 |
| AT 终端没有响应 | 先停止 HiDeck，确认端口归属和是否被其他程序占用 |
| USB 存在但 QMI 接口缺失 | 检查 `lsusb -t`、当前 USB 组合及 `qmi_wwan` 驱动 |
| 页面能打开但没有设备 | 先验证 USB/IP、Linux 节点和驱动；页面可访问不能证明硬件已接入 |
| 一直显示 QMI 启动中，日志提示 qmi-proxy 缺失 | Ubuntu 执行 `sudo apt-get install -y libqmi-proxy`，然后 `sudo systemctl restart hideck.service` |
| 页面无法打开 | 在 Ubuntu 看服务日志，并用 `curl -I http://127.0.0.1:7575` 区分服务问题与 Windows 到 WSL 的连接问题 |
| 短信/AT 端口忙 | 检查是否有 ModemManager 或串口终端同时占用；确认没有其他网络依赖后再处理占用程序 |

查看日志：

```bash
sudo journalctl -u hideck.service -n 100 --no-pager
```

最后一个功能限制：上游指出，大疆/Baiwang 一类定制 EC25 的 USB 声卡可能无法安全打开，原生 VoLTE 即便信令能接通，也可能没有模块音频。识别成功或短信正常不能保证能当完整的语音电话使用。参见 [上游原生 VoLTE 说明](https://github.com/yibaiba/hideck/blob/dc4b173a5261a5e0832a26b04a1d6c42415e4e26/docs/volte-native.md)。
