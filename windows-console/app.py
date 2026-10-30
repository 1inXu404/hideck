"""Native Windows launcher for the existing HiDeck installation (stdlib only)."""
from __future__ import annotations

import ctypes
from ctypes import wintypes
import logging
from logging.handlers import RotatingFileHandler
import os
from pathlib import Path
import queue
import threading
import time
import tkinter as tk
from tkinter import messagebox, ttk

from controller import Controller, ControlError

TITLE = "HiDeck 简易控制台"
ROOT = Path(__file__).resolve().parent


class Console(tk.Tk):
    def __init__(self):
        super().__init__()
        self.title(TITLE)
        self.geometry("720x610")
        self.minsize(640, 550)
        self.configure(bg="#f3f6fa")
        self.events = queue.Queue()
        self.lock = threading.Lock()
        self.busy = False
        self.refreshing = False
        self.running = False
        self.attached = False
        self.devices = []
        self.url = None
        self.exit_after_stop = False
        self.error_latched = False
        self.controller = None
        self.configure_styles()
        self.build_ui()
        self.protocol("WM_DELETE_WINDOW", self.close_requested)
        try:
            self.controller = Controller(emit=lambda text: self.events.put(("log", text)))
        except Exception as exc:
            self.set_status("缺少运行组件", str(exc), "#b93838")
            self.append_log(str(exc))
        self.after(100, self.drain_events)
        self.after(150, self.refresh)
        self.after(5000, self.poll)

    def configure_styles(self):
        style = ttk.Style(self)
        style.theme_use("clam")
        style.configure("TCombobox", font=("Microsoft YaHei UI", 10), padding=9,
                        fieldbackground="white", bordercolor="#d7e0eb")
        style.configure("Blue.TButton", font=("Microsoft YaHei UI", 12, "bold"),
                        padding=(20, 14), foreground="white", background="#1769e0",
                        borderwidth=0)
        style.map("Blue.TButton", background=[("disabled", "#b9c8dc"), ("active", "#1254b5")])
        style.configure("Stop.TButton", font=("Microsoft YaHei UI", 12, "bold"),
                        padding=(20, 14), foreground="#a83232", background="#fdecec",
                        borderwidth=0)
        style.map("Stop.TButton", background=[("disabled", "#ebedf0"), ("active", "#f9dada")],
                  foreground=[("disabled", "#9aa4af")])
        style.configure("Quiet.TButton", font=("Microsoft YaHei UI", 10),
                        padding=(12, 9), foreground="#325477", background="#e9eff7", borderwidth=0)
        style.configure("TProgressbar", background="#1769e0", troughcolor="#e8edf5", borderwidth=0)
        self.option_add("*TCombobox*Listbox.font", ("Microsoft YaHei UI", 10))

    def label(self, parent, text="", size=10, color="#24374d", bold=False, bg=None, **kwargs):
        return tk.Label(parent, text=text, font=("Microsoft YaHei UI", size, "bold" if bold else "normal"),
                        fg=color, bg=bg or parent.cget("bg"), **kwargs)

    def build_ui(self):
        outer = tk.Frame(self, bg="#f3f6fa", padx=26, pady=22)
        outer.pack(fill="both", expand=True)
        header = tk.Frame(outer, bg="#f3f6fa")
        header.pack(fill="x", pady=(0, 20))
        self.label(header, "H", size=22, bold=True, color="white", bg="#1769e0", width=3).pack(side="left", ipady=7)
        titles = tk.Frame(header, bg="#f3f6fa")
        titles.pack(side="left", padx=14)
        self.label(titles, "HiDeck 控制台", size=20, bold=True).pack(anchor="w")
        self.label(titles, "大疆 4G 模块 · 随用随开", color="#65768b").pack(anchor="w", pady=(3, 0))

        card = tk.Frame(outer, bg="white", padx=22, pady=18,
                        highlightbackground="#dce5ef", highlightthickness=1)
        card.pack(fill="x")
        self.status_title = self.label(card, "正在检查…", size=17, bold=True, color="#1769e0")
        self.status_title.pack(anchor="w")
        self.status_detail = self.label(card, "请连接模块，然后点击启动。", color="#65768b",
                                        anchor="w", justify="left", wraplength=570)
        self.status_detail.pack(fill="x", pady=(6, 18))
        self.label(card, "USB 模块", size=10, bold=True).pack(anchor="w", pady=(0, 7))
        picker = tk.Frame(card, bg="white")
        picker.pack(fill="x")
        self.device_box = ttk.Combobox(picker, state="readonly")
        self.device_box.pack(side="left", fill="x", expand=True, padx=(0, 8))
        self.device_box.bind("<<ComboboxSelected>>", lambda event: self.update_buttons())
        self.refresh_button = ttk.Button(picker, text="刷新", style="Quiet.TButton", command=lambda: self.refresh(manual=True))
        self.refresh_button.pack(side="right")
        actions = tk.Frame(card, bg="white")
        actions.pack(fill="x", pady=(20, 12))
        actions.columnconfigure(0, weight=1)
        actions.columnconfigure(1, weight=1)
        self.start_button = ttk.Button(actions, text="启动", style="Blue.TButton", command=lambda: self.operate("start"))
        self.start_button.grid(row=0, column=0, sticky="ew", padx=(0, 7))
        self.stop_button = ttk.Button(actions, text="停止", style="Stop.TButton", command=lambda: self.operate("stop"))
        self.stop_button.grid(row=0, column=1, sticky="ew", padx=(7, 0))
        self.progress = ttk.Progressbar(card, mode="indeterminate")
        self.progress.pack(fill="x", pady=(0, 12))
        web = tk.Frame(card, bg="white")
        web.pack(fill="x")
        self.address_text = tk.StringVar(value="启动后自动打开网页控制台")
        self.address_label = self.label(web, textvariable=self.address_text, color="#65768b", anchor="w")
        self.address_label.pack(side="left", fill="x", expand=True)
        self.web_button = ttk.Button(web, text="打开网页", style="Quiet.TButton", command=self.open_web)
        self.web_button.pack(side="right")

        self.label(outer, "操作记录", size=10, bold=True).pack(anchor="w", pady=(17, 7))
        log_frame = tk.Frame(outer, bg="white")
        log_frame.pack(fill="both", expand=True)
        scrollbar = ttk.Scrollbar(log_frame)
        scrollbar.pack(side="right", fill="y")
        self.log_box = tk.Text(log_frame, height=7, bg="white", fg="#4d6076", relief="flat",
                               padx=12, pady=10, font=("Microsoft YaHei UI", 9), state="disabled",
                               wrap="word", yscrollcommand=scrollbar.set)
        self.log_box.pack(fill="both", expand=True)
        scrollbar.config(command=self.log_box.yview)
        self.label(outer, "停止完成后，再拔下模块。", size=9, color="#77869a").pack(anchor="w", pady=(10, 0))
        self.update_buttons()

    def set_status(self, title, detail, color="#24374d"):
        self.status_title.config(text=title, fg=color)
        self.status_detail.config(text=detail)

    def append_log(self, message):
        logging.info(message)
        self.log_box.configure(state="normal")
        self.log_box.insert("end", f"{time.strftime('%H:%M:%S')}  {message}\n")
        if int(self.log_box.index("end-1c").split(".")[0]) > 300:
            self.log_box.delete("1.0", "50.0")
        self.log_box.see("end")
        self.log_box.configure(state="disabled")

    def selected_id(self):
        index = self.device_box.current()
        return self.devices[index].instance_id if 0 <= index < len(self.devices) else None

    def update_buttons(self):
        self.start_button.config(state="normal" if self.controller and not self.busy else "disabled")
        self.stop_button.config(state="normal" if self.controller and not self.busy else "disabled")
        self.refresh_button.config(state="normal" if self.controller and not self.busy else "disabled")
        self.web_button.config(state="normal" if self.url and not self.busy else "disabled")
        self.device_box.config(state="disabled" if self.busy or self.running else "readonly")

    def refresh(self, manual=False):
        if not self.controller or self.busy or self.refreshing:
            return
        if manual:
            self.error_latched = False
        self.refreshing = True
        def worker():
            try:
                with self.lock:
                    state = self.controller.snapshot()
                    if state[1]:
                        self.controller.keep_alive()
                self.events.put(("snapshot", state))
            except Exception as exc:
                self.events.put(("refresh_error", str(exc)))
        threading.Thread(target=worker, daemon=True).start()

    def poll(self):
        self.refresh()
        self.after(5000, self.poll)

    def operate(self, operation):
        if self.busy or not self.controller:
            return
        selected = self.controller.target_id if operation == "stop" and self.controller.target_id else self.selected_id()
        self.busy = True
        self.error_latched = False
        self.progress.start(12)
        self.set_status("正在启动…" if operation == "start" else "正在停止…", "请稍候，完成前请保持模块连接。", "#1769e0")
        self.update_buttons()
        def worker():
            try:
                with self.lock:
                    result = self.controller.start(selected) if operation == "start" else self.controller.stop(selected)
                self.events.put(("done", (operation, result)))
            except Exception as exc:
                logging.exception("Operation failed")
                self.events.put(("error", str(exc)))
        threading.Thread(target=worker, daemon=True).start()

    def apply_snapshot(self, state):
        old = self.controller.target_id or self.selected_id()
        self.devices, self.running, self.url = state
        self.attached = any(device.client_ip for device in self.devices)
        labels = [device.label for device in self.devices]
        self.device_box["values"] = labels
        index = next((i for i, device in enumerate(self.devices) if device.instance_id == old), None)
        if index is None and len(self.devices) == 1:
            index = 0
        if index is not None:
            self.device_box.current(index)
        else:
            self.device_box.set("请选择模块" if self.devices else "未检测到模块，请插入 USB 数据线")
        self.address_text.set(self.url or "启动后自动打开网页控制台")
        if not self.error_latched:
            if self.running and self.attached:
                self.set_status("运行中", "模块已连接。可以打开网页；用完后点击停止。", "#138052")
            elif self.running:
                self.set_status("服务运行中 · 模块未连接", "连接模块后点击启动，自动接入 USB。", "#ad6a10")
            elif self.attached:
                self.set_status("模块已连接 · HiDeck 未启动", "点击启动打开网页，或点击停止释放模块。", "#ad6a10")
            elif self.devices:
                self.set_status("已就绪", "检测到模块，点击启动即可使用。", "#1769e0")
            else:
                self.set_status("等待连接模块", "插入模块后，控制台会自动识别。", "#65768b")
        self.update_buttons()

    def drain_events(self):
        try:
            while True:
                kind, payload = self.events.get_nowait()
                if kind == "log":
                    self.append_log(payload)
                    if self.busy:
                        self.status_detail.config(text=payload)
                elif kind == "snapshot":
                    self.refreshing = False
                    if not self.busy:
                        self.apply_snapshot(payload)
                elif kind == "refresh_error":
                    self.refreshing = False
                    if not self.busy and not self.error_latched:
                        self.error_latched = True
                        self.set_status("状态检查失败", payload, "#b93838")
                        self.append_log(payload)
                elif kind == "done":
                    operation, result = payload
                    self.busy = False
                    self.progress.stop()
                    self.running = operation == "start"
                    self.url = result if self.running else None
                    self.attached = self.running
                    self.address_text.set(self.url or "已停止，可以拔下模块")
                    self.set_status("运行中" if self.running else "已停止 · 可以拔下模块",
                                    "网页控制台已就绪。" if self.running else "HiDeck 已关闭，USB 连接已释放。",
                                    "#138052" if self.running else "#65768b")
                    self.update_buttons()
                    if self.exit_after_stop and operation == "stop":
                        self.destroy()
                        return
                    self.refresh()
                elif kind == "error":
                    self.busy = False
                    self.exit_after_stop = False
                    self.error_latched = True
                    self.progress.stop()
                    self.set_status("操作未完成", payload.splitlines()[-1][:180], "#b93838")
                    self.append_log(payload)
                    self.update_buttons()
                    self.refresh()
        except queue.Empty:
            pass
        self.after(100, self.drain_events)

    def open_web(self):
        if self.url:
            try:
                os.startfile(self.url)
            except OSError as exc:
                self.append_log(f"无法打开默认浏览器：{exc}")

    def close_requested(self):
        if self.busy:
            messagebox.showinfo("正在处理", "请等当前操作完成后再关闭控制台。", parent=self)
            return
        if self.running or self.attached or (self.controller and self.controller.keeper):
            if messagebox.askokcancel("退出控制台", "退出前会停止 HiDeck 并断开模块。\n是否继续？", parent=self):
                self.exit_after_stop = True
                self.operate("stop")
        else:
            self.destroy()


def main():
    handler = RotatingFileHandler(ROOT / "console.log", maxBytes=300_000, backupCount=2, encoding="utf-8")
    logging.basicConfig(level=logging.INFO, handlers=[handler], format="%(asctime)s %(levelname)s %(message)s")
    mutex = None
    if os.name == "nt":
        kernel = ctypes.WinDLL("kernel32", use_last_error=True)
        kernel.CreateMutexW.argtypes = [ctypes.c_void_p, wintypes.BOOL, wintypes.LPCWSTR]
        kernel.CreateMutexW.restype = wintypes.HANDLE
        mutex = kernel.CreateMutexW(None, False, "Local\\HiDeckSimpleConsole")
        if ctypes.get_last_error() == 183:
            ctypes.windll.user32.MessageBoxW(None, "控制台已经打开，请从任务栏切换到它。", TITLE, 0x40)
            return
        try:
            ctypes.windll.shcore.SetProcessDpiAwareness(1)
        except (OSError, AttributeError):
            pass
    try:
        app = Console()
        app.mainloop()
    except Exception as exc:
        logging.exception("Console crashed")
        if os.name == "nt":
            ctypes.windll.user32.MessageBoxW(None, f"控制台启动失败：{exc}\n详情见 console.log。", TITLE, 0x10)
    finally:
        if mutex:
            kernel.CloseHandle.argtypes = [wintypes.HANDLE]
            kernel.CloseHandle(mutex)


if __name__ == "__main__":
    main()
