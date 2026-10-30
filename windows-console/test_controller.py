import json
import unittest

from controller import Controller, ControlError, DISTRO, decode_output, parse_devices


INSTANCE = r"USB\VID_2C7C&PID_0125\module-one"


def usb_item(bus="2-2", attached=None, instance=INSTANCE, shared=True):
    return {"BusId": bus, "ClientIPAddress": attached, "Description": "EG25G-QDC507",
            "InstanceId": instance, "PersistedGuid": "guid" if shared else None}


class FakeRunner:
    def __init__(self):
        self.calls = []
        self.ip = "172.30.8.22"
        self.bus = "7-3"
        self.attached = False
        self.shared = True
        self.present = True
        self.running = False
        self.active = False
        self.fail_start = False
        self.fail_stop = False
        self.foreign_ip = None
        self.missing_proxy = False

    def run(self, args, timeout=25, check=True):
        self.calls.append(args)
        if args[0] == "usbipd.exe":
            if args[1] == "state":
                devices = [usb_item(self.bus, self.foreign_ip or (self.ip if self.attached else None),
                                    shared=self.shared)] if self.present else []
                return 0, json.dumps({"Devices": devices})
            if args[1] == "attach":
                self.attached = True
                return 0, ""
            if args[1] == "detach":
                self.attached = False
                return 0, ""
        if "--list" in args:
            return 0, DISTRO if self.running else ""
        command = args[args.index("--exec") + 1:]
        if command == ["/bin/true"]:
            self.running = True
            return 0, ""
        if command[:2] == ["systemctl", "is-active"]:
            return (0, "active") if self.active else (3, "inactive")
        if command[:2] == ["systemctl", "start"]:
            self.active = True
            if self.fail_start:
                raise ControlError("simulated partial start failure")
            return 0, ""
        if command[:2] == ["systemctl", "stop"]:
            if self.fail_stop:
                raise ControlError("simulated stop failure")
            self.active = False
            return 0, ""
        if command[0] == "ip":
            return 0, json.dumps([{"addr_info": [{"scope": "global", "local": self.ip}]}])
        if command[:2] == ["test", "-x"]:
            return (1, "") if self.missing_proxy else (0, "")
        if command[0] == "/usr/sbin/modprobe":
            return 0, ""
        raise AssertionError(f"Unexpected command: {args}")


class FakeController(Controller):
    def __init__(self, runner):
        self.opened = []
        self.binding_requests = []
        self.events = []
        self.alive = False
        self.http_fail = False
        super().__init__(emit=self.events.append, runner=runner, usbipd="usbipd.exe",
                         binder=self.bind, open_url=self.opened.append)

    def bind(self, path, bus):
        self.binding_requests.append(bus)
        self.runner.shared = True

    def keep_alive(self):
        self.alive = True

    def release_keeper(self):
        self.alive = False

    def verify_nodes(self):
        pass

    def wait_http(self, url):
        if self.http_fail:
            raise ControlError("simulated HTTP failure")


class ControllerTests(unittest.TestCase):
    def test_devices_exclude_unplugged_history_and_unrelated_hardware(self):
        items = [usb_item(), usb_item(None), usb_item("4-2", instance=r"USB\VID_373B&PID_119A\keyboard")]
        found = parse_devices(json.dumps({"Devices": items}))
        self.assertEqual([device.bus_id for device in found], ["2-2"])

    def test_wsl_utf16_and_usbipd_utf8(self):
        self.assertEqual(decode_output((DISTRO + "\r\n").encode("utf-16-le")), DISTRO)
        self.assertEqual(decode_output("模块".encode("utf-8")), "模块")

    def test_start_uses_current_usb_bus_and_ip(self):
        runner = FakeRunner()
        app = FakeController(runner)
        self.assertEqual(app.start(), "http://172.30.8.22:7575/")
        self.assertIn(["usbipd.exe", "attach", "--wsl", DISTRO, "--busid", "7-3"], runner.calls)
        self.assertEqual(app.opened, ["http://172.30.8.22:7575/"])
        self.assertTrue(runner.active and runner.attached and app.alive)

    def test_repeat_start_does_not_attach_or_restart_again(self):
        runner = FakeRunner()
        app = FakeController(runner)
        app.start()
        app.start()
        self.assertEqual(sum(call[1] == "attach" for call in runner.calls), 1)
        self.assertFalse(any("restart" in call for call in runner.calls))

    def test_unshared_device_requests_scoped_binding_once(self):
        runner = FakeRunner()
        runner.shared = False
        app = FakeController(runner)
        app.start()
        app.start()
        self.assertEqual(app.binding_requests, ["7-3"])
        self.assertFalse(any("--force" in call for call in runner.calls))

    def test_stop_releases_usb_after_service_stop(self):
        runner = FakeRunner()
        app = FakeController(runner)
        app.start()
        runner.calls.clear()
        app.stop()
        stop_index = next(i for i, call in enumerate(runner.calls) if "stop" in call)
        detach_index = next(i for i, call in enumerate(runner.calls) if call[1] == "detach")
        self.assertLess(stop_index, detach_index)
        self.assertFalse(runner.active or runner.attached or app.alive)

    def test_failed_stop_keeps_usb_connected(self):
        runner = FakeRunner()
        app = FakeController(runner)
        app.start()
        runner.fail_stop = True
        with self.assertRaises(ControlError):
            app.stop()
        self.assertTrue(runner.attached and app.alive)

    def test_unplugged_module_can_still_stop_service(self):
        runner = FakeRunner()
        app = FakeController(runner)
        app.start()
        runner.present = False
        app.stop()
        self.assertFalse(runner.active or app.alive)

    def test_repeated_stop_does_not_wake_stopped_wsl(self):
        runner = FakeRunner()
        runner.present = False
        app = FakeController(runner)
        app.stop()
        app.stop()
        self.assertFalse(any("--exec" in call for call in runner.calls))

    def test_start_failure_rolls_back_owned_service_and_usb(self):
        runner = FakeRunner()
        runner.fail_start = True
        app = FakeController(runner)
        with self.assertRaises(ControlError):
            app.start()
        self.assertFalse(runner.active or runner.attached or app.alive)
        self.assertFalse(app.opened)

    def test_http_failure_does_not_stop_preexisting_service_or_detach_preexisting_usb(self):
        runner = FakeRunner()
        runner.active = runner.attached = runner.running = True
        app = FakeController(runner)
        app.http_fail = True
        with self.assertRaises(ControlError):
            app.start()
        self.assertTrue(runner.active and runner.attached)
        self.assertFalse(any("stop" in call or "detach" in call for call in runner.calls))

    def test_missing_proxy_does_not_attach_usb(self):
        runner = FakeRunner()
        runner.missing_proxy = True
        app = FakeController(runner)
        with self.assertRaisesRegex(ControlError, "qmi-proxy"):
            app.start()
        self.assertFalse(runner.active or runner.attached or app.alive)

    def test_foreign_client_is_not_detached(self):
        runner = FakeRunner()
        runner.running = True
        runner.foreign_ip = "192.0.2.99"
        app = FakeController(runner)
        with self.assertRaises(ControlError):
            app.start()
        with self.assertRaises(ControlError):
            app.stop(INSTANCE)
        self.assertFalse(any("detach" in call for call in runner.calls))


if __name__ == "__main__":
    unittest.main()
