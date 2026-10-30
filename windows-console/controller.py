"""Local, on-demand HiDeck control. No modem AT writes or network policy changes."""
from __future__ import annotations

import ctypes
from ctypes import wintypes
from dataclasses import dataclass
import ipaddress
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import time
import urllib.request


DISTRO = "Ubuntu-26.04"
SERVICE = "hideck.service"
PORT = 7575
HARDWARE_IDS = {"2c7c:0125", "2ca3:4006"}
NO_WINDOW = getattr(subprocess, "CREATE_NO_WINDOW", 0)


class ControlError(RuntimeError):
    pass


def decode_output(data: bytes) -> str:
    if data.startswith((b"\xff\xfe", b"\xfe\xff")):
        return data.decode("utf-16", errors="replace").strip()
    if data and data.count(b"\0") > len(data) / 8:
        return data.decode("utf-16-le", errors="replace").strip()
    return data.decode("utf-8-sig", errors="replace").strip()


@dataclass(frozen=True)
class Device:
    instance_id: str
    bus_id: str
    name: str
    hardware_id: str
    shared: bool
    client_ip: str | None

    @property
    def label(self):
        state = "已连接" if self.client_ip else "待连接"
        return f"{self.name}   ·   USB {self.bus_id}   ·   {state}"


def parse_devices(text: str) -> list[Device]:
    result = []
    for item in json.loads(text).get("Devices", []):
        instance = item.get("InstanceId") or ""
        match = re.search(r"VID_([0-9A-F]{4})&PID_([0-9A-F]{4})", instance, re.I)
        bus = item.get("BusId")
        if not match or not bus or not re.fullmatch(r"\d+-\d+", bus):
            continue
        hardware = f"{match[1]}:{match[2]}".lower()
        if hardware in HARDWARE_IDS:
            result.append(Device(instance, bus, item.get("Description") or "4G 模块",
                                 hardware, bool(item.get("PersistedGuid")),
                                 item.get("ClientIPAddress")))
    return result


def find_usbipd() -> str:
    candidates = [shutil.which("usbipd.exe"),
                  str(Path(os.environ.get("ProgramFiles", r"C:\Program Files")) /
                      "usbipd-win" / "usbipd.exe")]
    if os.name == "nt":
        import winreg
        try:
            with winreg.OpenKey(winreg.HKEY_LOCAL_MACHINE,
                                r"SYSTEM\CurrentControlSet\Services\usbipd") as key:
                path = winreg.QueryValueEx(key, "ImagePath")[0]
                match = re.match(r'"([^"]+\.exe)"', path, re.I)
                if match:
                    candidates.append(match[1])
        except OSError:
            pass
    for path in candidates:
        if path and Path(path).is_file():
            return path
    raise ControlError("未找到 usbipd-win，请先安装 USB 连接工具。")


class Runner:
    def run(self, args: list[str], timeout=25, check=True) -> tuple[int, str]:
        try:
            proc = subprocess.run(args, capture_output=True, timeout=timeout,
                                  creationflags=NO_WINDOW)
        except subprocess.TimeoutExpired as exc:
            raise ControlError(f"操作超时（{timeout} 秒）：{Path(args[0]).name}") from exc
        except OSError as exc:
            raise ControlError(f"无法执行 {Path(args[0]).name}：{exc}") from exc
        output = "\n".join(filter(None, [decode_output(proc.stdout), decode_output(proc.stderr)]))
        if check and proc.returncode:
            raise ControlError(output[-2000:] or f"命令退出码：{proc.returncode}")
        return proc.returncode, output


def elevated_bind(executable: str, bus_id: str):
    """Ask Windows UAC for this specific bind only; never silently force-bind."""
    if not re.fullmatch(r"\d+-\d+", bus_id):
        raise ControlError("USB 编号无效。")

    class ShellExecuteInfo(ctypes.Structure):
        _fields_ = [("cbSize", wintypes.DWORD), ("fMask", wintypes.ULONG),
                    ("hwnd", wintypes.HWND), ("lpVerb", wintypes.LPCWSTR),
                    ("lpFile", wintypes.LPCWSTR), ("lpParameters", wintypes.LPCWSTR),
                    ("lpDirectory", wintypes.LPCWSTR), ("nShow", ctypes.c_int),
                    ("hInstApp", wintypes.HINSTANCE), ("lpIDList", ctypes.c_void_p),
                    ("lpClass", wintypes.LPCWSTR), ("hkeyClass", wintypes.HKEY),
                    ("dwHotKey", wintypes.DWORD), ("hIcon", wintypes.HANDLE),
                    ("hProcess", wintypes.HANDLE)]

    shell = ctypes.WinDLL("shell32", use_last_error=True)
    kernel = ctypes.WinDLL("kernel32", use_last_error=True)
    shell.ShellExecuteExW.argtypes = [ctypes.POINTER(ShellExecuteInfo)]
    shell.ShellExecuteExW.restype = wintypes.BOOL
    kernel.WaitForSingleObject.argtypes = [wintypes.HANDLE, wintypes.DWORD]
    kernel.GetExitCodeProcess.argtypes = [wintypes.HANDLE, ctypes.POINTER(wintypes.DWORD)]
    kernel.CloseHandle.argtypes = [wintypes.HANDLE]
    info = ShellExecuteInfo()
    info.cbSize = ctypes.sizeof(info)
    info.fMask = 0x40  # SEE_MASK_NOCLOSEPROCESS
    info.lpVerb = "runas"
    info.lpFile = executable
    info.lpParameters = subprocess.list2cmdline(["bind", "--busid", bus_id])
    info.nShow = 0
    if not shell.ShellExecuteExW(ctypes.byref(info)):
        code = ctypes.get_last_error()
        if code == 1223:
            raise ControlError("已取消管理员授权，设备尚未共享。可以再次点击启动。")
        raise ControlError(f"无法请求管理员授权（Windows 错误 {code}）。")
    try:
        if kernel.WaitForSingleObject(info.hProcess, 120_000) != 0:
            raise ControlError("等待共享授权超时，请检查 Windows 授权窗口。")
        exit_code = wintypes.DWORD()
        if not kernel.GetExitCodeProcess(info.hProcess, ctypes.byref(exit_code)) or exit_code.value:
            raise ControlError("USB 共享失败，请检查管理员授权或 usbipd 服务。")
    finally:
        kernel.CloseHandle(info.hProcess)


class Controller:
    def __init__(self, emit=lambda text: None, runner=None, usbipd=None,
                 binder=elevated_bind, open_url=None):
        self.emit = emit
        self.runner = runner or Runner()
        self.usbipd = usbipd or find_usbipd()
        self.wsl = str(Path(os.environ.get("WINDIR", r"C:\Windows")) / "System32" / "wsl.exe")
        self.binder = binder
        self.open_url = open_url or os.startfile
        self.keeper = None
        self.target_id = None
        self.url = None
        self.http = urllib.request.build_opener(urllib.request.ProxyHandler({}))

    def linux(self, *args, check=True, timeout=30):
        return self.runner.run([self.wsl, "--distribution", DISTRO, "--user", "root",
                                "--exec", *args], timeout=timeout, check=check)

    def devices(self):
        return parse_devices(self.runner.run([self.usbipd, "state"])[1])

    def distro_running(self):
        output = self.runner.run([self.wsl, "--list", "--running", "--quiet"])[1]
        return DISTRO in [line.strip().lstrip("*").strip() for line in output.splitlines()]

    def service_active(self):
        code, text = self.linux("systemctl", "is-active", SERVICE, check=False)
        return code == 0 and text.strip() == "active"

    def address(self):
        # Select the Windows-facing adapter, never the SIM's cellular address.
        data = json.loads(self.linux("ip", "-j", "-4", "address", "show", "dev", "eth0")[1])
        for interface in data:
            for addr in interface.get("addr_info", []):
                candidate = addr.get("local", "")
                try:
                    ip = ipaddress.IPv4Address(candidate)
                except ipaddress.AddressValueError:
                    continue
                if addr.get("scope") == "global" and not ip.is_loopback:
                    return str(ip)
        raise ControlError("没有找到 Ubuntu 面向 Windows 的 IPv4 地址。")

    def snapshot(self):
        devices = self.devices()
        running = self.distro_running()
        active = running and self.service_active()
        url = None
        if active:
            url = f"http://{self.address()}:{PORT}/"
        return devices, active, url

    def keep_alive(self):
        if self.keeper and self.keeper.poll() is None:
            return
        # cat exits when this console closes its stdin pipe, with no orphan sleeper.
        self.keeper = subprocess.Popen(
            [self.wsl, "--distribution", DISTRO, "--exec", "/bin/cat"],
            stdin=subprocess.PIPE, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
            creationflags=NO_WINDOW)

    def release_keeper(self):
        proc, self.keeper = self.keeper, None
        if not proc:
            return
        try:
            if proc.stdin:
                proc.stdin.close()
            proc.wait(timeout=3)
        except (OSError, subprocess.TimeoutExpired):
            if proc.poll() is None:
                proc.terminate()
                try:
                    proc.wait(timeout=3)
                except subprocess.TimeoutExpired:
                    pass

    def choose(self, instance_id=None):
        devices = self.devices()
        if instance_id:
            for device in devices:
                if device.instance_id == instance_id:
                    return device
            raise ControlError("选定的模块已断开，请重新插入后点击刷新。")
        if not devices:
            raise ControlError("没有检测到大疆模块。请插好 USB 数据线后再启动。")
        if len(devices) != 1:
            raise ControlError("检测到多个模块，请先在设备列表中选择要使用的模块。")
        return devices[0]

    def verify_nodes(self):
        script = r'''for node in /sys/class/usbmisc/cdc-wdm*; do
          test -e "$node" || continue
          interface=$(readlink -f "$node/device")
          parent=$(dirname "$interface")
          test "$(cat "$parent/idVendor" 2>/dev/null)" = 2c7c || continue
          test "$(cat "$parent/idProduct" 2>/dev/null)" = 0125 || continue
          test -e "/dev/$(basename "$node")" && exit 0
        done
        exit 1'''
        for _ in range(15):
            if self.linux("sh", "-c", script, check=False)[0] == 0:
                return
            time.sleep(1)
        raise ControlError("模块已接入，但没有出现 QMI 控制口。请查看 USB 连接和驱动。")

    def wait_http(self, url):
        deadline = time.monotonic() + 30
        while time.monotonic() < deadline:
            try:
                with self.http.open(url, timeout=3) as response:
                    if response.status == 200:
                        return
            except (OSError, ValueError):
                pass
            time.sleep(1)
        raise ControlError(f"HiDeck 服务未能在 30 秒内提供网页：{url}")

    def start(self, instance_id=None):
        device = self.choose(instance_id)
        if device.hardware_id != "2c7c:0125":
            raise ControlError("模块仍是 DJI 原始 USB 身份，请先按部署教程完成首次适配。")
        self.target_id = device.instance_id
        self.emit("正在启动 Ubuntu…")
        self.linux("/bin/true", timeout=60)
        was_active = self.service_active()
        self.keep_alive()
        attached_here = False
        service_started_here = False
        try:
            if self.linux("test", "-x", "/usr/libexec/qmi-proxy", check=False)[0]:
                raise ControlError("Ubuntu 缺少 qmi-proxy。请先安装 libqmi-proxy 后重试。")
            host_ip = self.address()
            if device.client_ip and device.client_ip != host_ip:
                raise ControlError("模块正被其他 USB/IP 客户端使用，请先在原客户端断开。")
            if not device.shared:
                self.emit("首次共享这个 USB 连接，请确认 Windows 管理员授权。")
                self.binder(self.usbipd, device.bus_id)
                device = self.choose(device.instance_id)
                if not device.shared:
                    raise ControlError("共享尚未生效，请重试。")
            if not device.client_ip:
                self.emit(f"正在连接模块（USB {device.bus_id}）…")
                self.runner.run([self.usbipd, "attach", "--wsl", DISTRO,
                                 "--busid", device.bus_id], timeout=45)
                attached_here = True
                device = self.choose(device.instance_id)
                if not device.client_ip:
                    raise ControlError("USB 连接没有生效，请重试。")
            else:
                self.emit("模块已连接到 Ubuntu。")
            self.emit("正在检查模块接口…")
            self.linux("/usr/sbin/modprobe", "option")
            self.linux("/usr/sbin/modprobe", "qmi_wwan")
            self.verify_nodes()
            self.emit("正在启动 HiDeck…")
            service_started_here = not was_active
            self.linux("systemctl", "start", SERVICE, timeout=45)
            if not self.service_active():
                raise ControlError("HiDeck 服务未启动，请查看服务日志。")
            url = f"http://{self.address()}:{PORT}/"
            self.emit("正在等待网页就绪…")
            self.wait_http(url)
            self.url = url
        except Exception:
            rollback_ok = True
            if service_started_here:
                try:
                    self.linux("systemctl", "stop", SERVICE, timeout=45)
                except Exception:
                    rollback_ok = False
                    self.emit("清理时未能停止服务，请点击停止重试。")
            if attached_here and rollback_ok:
                try:
                    current = self.choose(device.instance_id)
                    self.runner.run([self.usbipd, "detach", "--busid", current.bus_id])
                except Exception:
                    self.emit("未能释放 USB，请点击停止重试。")
            if not was_active and rollback_ok:
                self.release_keeper()
            raise
        self.emit("启动完成，正在打开网页控制台。")
        try:
            self.open_url(self.url)
        except OSError:
            self.emit("默认浏览器未能打开，请点击“打开网页”或复制页面地址。")
        return self.url

    def stop(self, instance_id=None):
        selected = instance_id or self.target_id
        self.emit("正在停止 HiDeck 及其关联进程…")
        running = self.distro_running()
        host_ip = self.address() if running else None
        if running:
            self.linux("systemctl", "stop", SERVICE, timeout=45)
            if self.service_active():
                raise ControlError("服务仍在运行，暂未断开 USB。请再次点击停止。")
        devices = self.devices()
        if selected:
            targets = [device for device in devices if device.instance_id == selected]
        elif len(devices) <= 1:
            targets = devices
        else:
            raise ControlError("服务已停止；请选定模块后再次点击停止，以释放对应 USB。")
        for device in targets:
            if device.client_ip:
                if not host_ip or device.client_ip != host_ip:
                    raise ControlError("服务已停止，但模块属于其他 USB/IP 客户端，未断开它。")
                self.emit(f"正在释放模块（USB {device.bus_id}）…")
                self.runner.run([self.usbipd, "detach", "--busid", device.bus_id], timeout=30)
                remaining = self.devices()
                if any(d.instance_id == device.instance_id and d.client_ip for d in remaining):
                    raise ControlError("USB 尚未释放，请再次点击停止。")
        self.release_keeper()
        self.url = None
        self.target_id = None
        self.emit("已停止，可以拔下模块。")
