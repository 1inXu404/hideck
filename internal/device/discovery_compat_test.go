package device

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverCompatibleModemsFromQMI_MergeAndDedupByUSB(t *testing.T) {
	orig := discoverFallbackModemsFn
	t.Cleanup(func() { discoverFallbackModemsFn = orig })

	discoverFallbackModemsFn = func() ([]CompatibleModem, error) {
		return []CompatibleModem{
			{
				USBPath:      "/sys/bus/usb/devices/1-1",
				ATPort:       "/dev/ttyUSB9",
				Mode:         "ecm",
				DriverName:   "cdc_ether",
				NetInterface: "enx1",
			},
			{
				USBPath:      "/sys/bus/usb/devices/1-2",
				ATPort:       "/dev/ttyUSB4",
				Mode:         "ecm",
				DriverName:   "cdc_ether",
				NetInterface: "enx2",
			},
		}, nil
	}

	qmiList := []QMIDevice{
		{
			USBPath:      "/sys/bus/usb/devices/1-1",
			ATPort:       "/dev/ttyUSB2",
			ATPorts:      []string{"/dev/ttyUSB2", "/dev/ttyUSB3"},
			ControlPath:  "/dev/cdc-wdm0",
			DriverName:   "qmi_wwan",
			NetInterface: "wwan0",
		},
	}

	got, err := DiscoverCompatibleModemsFromQMI(qmiList)
	if err != nil {
		t.Fatalf("DiscoverCompatibleModemsFromQMI() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(got))
	}

	if got[0].USBPath != "/sys/bus/usb/devices/1-1" || got[0].Mode != "qmi" {
		t.Fatalf("expected first device to be qmi USB 1-1, got %+v", got[0])
	}

	foundFallback := false
	for _, d := range got {
		if d.USBPath == "/sys/bus/usb/devices/1-2" {
			foundFallback = true
			break
		}
	}
	if !foundFallback {
		t.Fatalf("expected fallback device USB 1-2 to be included, got %+v", got)
	}
}

func TestDiscoverCompatibleModemsFromQMI_FallbackErrorWithQMIStillSucceeds(t *testing.T) {
	orig := discoverFallbackModemsFn
	t.Cleanup(func() { discoverFallbackModemsFn = orig })

	discoverFallbackModemsFn = func() ([]CompatibleModem, error) {
		return nil, errors.New("fallback failed")
	}

	qmiList := []QMIDevice{
		{
			USBPath:      "/sys/bus/usb/devices/2-1",
			ATPort:       "/dev/ttyUSB2",
			ControlPath:  "/dev/cdc-wdm1",
			DriverName:   "qmi_wwan",
			NetInterface: "wwan1",
		},
	}

	got, err := DiscoverCompatibleModemsFromQMI(qmiList)
	if err != nil {
		t.Fatalf("DiscoverCompatibleModemsFromQMI() unexpected error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 qmi device, got %d", len(got))
	}
}

func TestDiscoverCompatibleModemsFromQMI_DoesNotInventATPortOrIMEI(t *testing.T) {
	orig := discoverFallbackModemsFn
	t.Cleanup(func() { discoverFallbackModemsFn = orig })

	discoverFallbackModemsFn = func() ([]CompatibleModem, error) {
		return nil, nil
	}

	qmiList := []QMIDevice{
		{
			USBPath:      "/sys/bus/usb/devices/3-1",
			ATPorts:      []string{"/dev/ttyUSB6", "/dev/ttyUSB7"},
			ATPort:       "",
			ControlPath:  "/dev/cdc-wdm3",
			DriverName:   "qmi_wwan",
			NetInterface: "wwan3",
		},
	}

	got, err := DiscoverCompatibleModemsFromQMI(qmiList)
	if err != nil {
		t.Fatalf("DiscoverCompatibleModemsFromQMI() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 qmi device, got %d", len(got))
	}
	if got[0].ATPort != "" {
		t.Fatalf("expected ATPort to stay empty, got %q", got[0].ATPort)
	}
	if got[0].IMEI != "" {
		t.Fatalf("expected IMEI to stay empty, got %q", got[0].IMEI)
	}
}

func TestDiscoverCompatibleModemsFromQMI_NoQMIAndFallbackError(t *testing.T) {
	orig := discoverFallbackModemsFn
	t.Cleanup(func() { discoverFallbackModemsFn = orig })

	discoverFallbackModemsFn = func() ([]CompatibleModem, error) {
		return nil, errors.New("fallback failed")
	}

	_, err := DiscoverCompatibleModemsFromQMI(nil)
	if err == nil {
		t.Fatal("expected error when qmi list empty and fallback failed")
	}
}

func TestCompatibleModemsFromQMIIncludesWWANFallbackWhenQMIListEmpty(t *testing.T) {
	orig := discoverFallbackModemsFn
	t.Cleanup(func() { discoverFallbackModemsFn = orig })

	discoverFallbackModemsFn = func() ([]CompatibleModem, error) {
		return []CompatibleModem{
			{
				ControlPath:    "/dev/wwan0qmi0",
				NetInterface:   "wwan0",
				USBPath:        "/sys/class/wwan/wwan0",
				DriverName:     "wwan_qmi",
				ATPorts:        []string{"/dev/wwan0at0", "/dev/wwan0at1"},
				ATPort:         "/dev/wwan0at0",
				Mode:           "qmi",
				NetworkCapable: true,
			},
		}, nil
	}

	got, err := DiscoverCompatibleModemsFromQMI(nil)
	if err != nil {
		t.Fatalf("DiscoverCompatibleModemsFromQMI(nil) error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len=%d want 1 (%+v)", len(got), got)
	}
	if got[0].ControlPath != "/dev/wwan0qmi0" || got[0].Mode != "qmi" {
		t.Fatalf("unexpected WWAN fallback result: %+v", got[0])
	}
}

func TestCompatibleModemDiscoveryKey(t *testing.T) {
	m := CompatibleModem{USBPath: "/sys/bus/usb/devices/1-1", ATPort: "/dev/ttyUSB2"}
	if got := m.DiscoveryKey(); got != "/sys/bus/usb/devices/1-1|/dev/ttyUSB2" {
		t.Fatalf("unexpected key: %q", got)
	}
}

func TestDiscoverFallbackOneAcceptsVendorAgnosticQMIWithoutAT(t *testing.T) {
	usbPath := t.TempDir()
	usbName := filepath.Base(usbPath)

	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(usbPath, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	write("idVendor", "1199\n")
	write("idProduct", "9077\n")

	ifacePath := filepath.Join(usbPath, usbName+":1.8")
	if err := os.MkdirAll(filepath.Join(ifacePath, "net", "wwan5"), 0o755); err != nil {
		t.Fatalf("mkdir net iface: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(ifacePath, "usbmisc", "cdc-wdm5"), 0o755); err != nil {
		t.Fatalf("mkdir cdc-wdm tree: %v", err)
	}
	if err := os.Symlink("/tmp/qmi_wwan", filepath.Join(ifacePath, "driver")); err != nil {
		t.Fatalf("symlink driver: %v", err)
	}

	got, ok := discoverFallbackOne(usbPath)
	if !ok {
		t.Fatal("discoverFallbackOne() rejected vendor-agnostic QMI device")
	}
	if got.ControlPath != "/dev/cdc-wdm5" {
		t.Fatalf("ControlPath=%q want /dev/cdc-wdm5", got.ControlPath)
	}
	if got.NetInterface != "wwan5" {
		t.Fatalf("NetInterface=%q want wwan5", got.NetInterface)
	}
	if got.Mode != "qmi" || !got.NetworkCapable {
		t.Fatalf("mode=%q networkCapable=%v, want qmi true", got.Mode, got.NetworkCapable)
	}
	if got.ATPort != "" || len(got.ATPorts) != 0 {
		t.Fatalf("expected pure QMI without AT, got ATPort=%q ports=%v", got.ATPort, got.ATPorts)
	}
}

func TestDiscoverFallbackOneRejectsUnknownVendorWithoutNetworkCapability(t *testing.T) {
	usbPath := t.TempDir()
	usbName := filepath.Base(usbPath)

	if err := os.WriteFile(filepath.Join(usbPath, "idVendor"), []byte("1199\n"), 0o644); err != nil {
		t.Fatalf("write idVendor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(usbPath, "idProduct"), []byte("9077\n"), 0o644); err != nil {
		t.Fatalf("write idProduct: %v", err)
	}
	ifacePath := filepath.Join(usbPath, usbName+":1.8")
	if err := os.MkdirAll(filepath.Join(ifacePath, "net", "wwan5"), 0o755); err != nil {
		t.Fatalf("mkdir net iface: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(ifacePath, "usbmisc", "cdc-wdm5"), 0o755); err != nil {
		t.Fatalf("mkdir cdc-wdm tree: %v", err)
	}
	if err := os.Symlink("/tmp/cdc_ncm", filepath.Join(ifacePath, "driver")); err != nil {
		t.Fatalf("symlink driver: %v", err)
	}

	got, ok := discoverFallbackOne(usbPath)
	if ok {
		t.Fatalf("expected rejection for unknown non-QMI/non-MBIM device, got %+v", got)
	}
}

func TestDiscoverFallbackOneRejectsQMIWithoutControlPath(t *testing.T) {
	usbPath := t.TempDir()
	usbName := filepath.Base(usbPath)

	if err := os.WriteFile(filepath.Join(usbPath, "idVendor"), []byte("1199\n"), 0o644); err != nil {
		t.Fatalf("write idVendor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(usbPath, "idProduct"), []byte("9077\n"), 0o644); err != nil {
		t.Fatalf("write idProduct: %v", err)
	}

	ifacePath := filepath.Join(usbPath, usbName+":1.8")
	if err := os.MkdirAll(filepath.Join(ifacePath, "net", "wwan5"), 0o755); err != nil {
		t.Fatalf("mkdir net iface: %v", err)
	}
	if err := os.Symlink("/tmp/qmi_wwan", filepath.Join(ifacePath, "driver")); err != nil {
		t.Fatalf("symlink driver: %v", err)
	}

	got, ok := discoverFallbackOne(usbPath)
	if ok {
		t.Fatalf("expected rejection without cdc-wdm, got %+v", got)
	}
}

func TestDiscoverFallbackOneSelectsDeviceLocalATPortForQuectel0125RNDIS(t *testing.T) {
	usbPath := t.TempDir()
	usbName := filepath.Base(usbPath)

	if err := os.WriteFile(filepath.Join(usbPath, "idVendor"), []byte("2c7c\n"), 0o644); err != nil {
		t.Fatalf("write idVendor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(usbPath, "idProduct"), []byte("0125\n"), 0o644); err != nil {
		t.Fatalf("write idProduct: %v", err)
	}

	netPath := filepath.Join(usbPath, usbName+":1.0")
	if err := os.MkdirAll(filepath.Join(netPath, "net", "usb0"), 0o755); err != nil {
		t.Fatalf("mkdir RNDIS network interface: %v", err)
	}
	if err := os.Symlink("/tmp/rndis_host", filepath.Join(netPath, "driver")); err != nil {
		t.Fatalf("symlink RNDIS driver: %v", err)
	}

	for _, port := range []struct {
		interfaceNumber int
		tty             string
	}{
		{interfaceNumber: 2, tty: "ttyUSB4"},
		{interfaceNumber: 3, tty: "ttyUSB5"},
		{interfaceNumber: 4, tty: "ttyUSB6"},
		{interfaceNumber: 5, tty: "ttyUSB7"},
	} {
		ifPath := filepath.Join(usbPath, fmt.Sprintf("%s:1.%d", usbName, port.interfaceNumber))
		if err := os.MkdirAll(filepath.Join(ifPath, port.tty), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", port.tty, err)
		}
	}

	got, ok := discoverFallbackOne(usbPath)
	if !ok {
		t.Fatal("discoverFallbackOne() rejected Quectel 0125 RNDIS device")
	}
	if got.ATPort != "/dev/ttyUSB6" {
		t.Fatalf("ATPort=%q want /dev/ttyUSB6", got.ATPort)
	}
	if got.Mode != "rndis" || got.NetInterface != "usb0" {
		t.Fatalf("mode=%q interface=%q want rndis usb0", got.Mode, got.NetInterface)
	}
}

func TestClassifyMode(t *testing.T) {
	cases := []struct {
		name       string
		control    string
		driver     string
		expectMode string
	}{
		{name: "qmi", control: "/dev/cdc-wdm0", driver: "qmi_wwan", expectMode: "qmi"},
		{name: "wwan qmi control path", control: "/dev/wwan0qmi0", driver: "wwan_qmi", expectMode: "qmi"},
		{name: "mbim", control: "/dev/cdc-wdm2", driver: "cdc_mbim", expectMode: "mbim"},
		{name: "ecm", driver: "cdc_ether", expectMode: "ecm"},
		{name: "rndis", driver: "rndis_host", expectMode: "rndis"},
		{name: "ncm", driver: "cdc_ncm", expectMode: "ncm"},
		{name: "unknown", driver: "usbserial", expectMode: "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyMode(tc.control, tc.driver); got != tc.expectMode {
				t.Fatalf("classifyMode()=%q, want %q", got, tc.expectMode)
			}
		})
	}
}

func TestDedupSortedNonEmpty(t *testing.T) {
	in := []string{" /dev/ttyUSB3 ", "/dev/ttyUSB2", "", "/dev/ttyUSB2"}
	got := dedupSortedNonEmpty(in)
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2", len(got))
	}
	if got[0] != "/dev/ttyUSB2" || got[1] != "/dev/ttyUSB3" {
		t.Fatalf("unexpected result: %#v", got)
	}
}

func TestFindCDCWDMInUSBPath_AllowsUSBMiscSymlink(t *testing.T) {
	usbPath := t.TempDir()
	ifacePath := filepath.Join(usbPath, "1-2:1.4")
	if err := os.MkdirAll(ifacePath, 0o755); err != nil {
		t.Fatalf("mkdir interface path: %v", err)
	}

	realUSBMisc := filepath.Join(ifacePath, "usbmisc-real")
	if err := os.MkdirAll(realUSBMisc, 0o755); err != nil {
		t.Fatalf("mkdir real usbmisc path: %v", err)
	}
	if err := os.WriteFile(filepath.Join(realUSBMisc, "cdc-wdm7"), []byte{}, 0o644); err != nil {
		t.Fatalf("create cdc-wdm file: %v", err)
	}
	if err := os.Symlink("usbmisc-real", filepath.Join(ifacePath, "usbmisc")); err != nil {
		t.Fatalf("create usbmisc symlink: %v", err)
	}

	got := findCDCWDMInUSBPath(usbPath)
	if got != "/dev/cdc-wdm7" {
		t.Fatalf("findCDCWDMInUSBPath()=%q, want %q", got, "/dev/cdc-wdm7")
	}
}

func TestFindCDCWDMInUSBPath_FollowsUSBPathSymlink(t *testing.T) {
	realUSBPath := t.TempDir()
	ifacePath := filepath.Join(realUSBPath, "1-4:1.4")
	if err := os.MkdirAll(filepath.Join(ifacePath, "usbmisc", "cdc-wdm9"), 0o755); err != nil {
		t.Fatalf("mkdir cdc-wdm tree: %v", err)
	}

	linkParent := t.TempDir()
	linkUSBPath := filepath.Join(linkParent, "1-4")
	if err := os.Symlink(realUSBPath, linkUSBPath); err != nil {
		t.Fatalf("create usb path symlink: %v", err)
	}

	got := findCDCWDMInUSBPath(linkUSBPath)
	if got != "/dev/cdc-wdm9" {
		t.Fatalf("findCDCWDMInUSBPath(symlink-root)=%q, want %q", got, "/dev/cdc-wdm9")
	}
}

func TestFindATPortsInUSBPathCollectsTTYUSBandTTYACM(t *testing.T) {
	usbPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(usbPath, "1-2:1.2", "ttyUSB6"), 0o755); err != nil {
		t.Fatalf("mkdir direct ttyUSB layout: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(usbPath, "1-2:1.3", "tty", "ttyUSB7"), 0o755); err != nil {
		t.Fatalf("mkdir nested ttyUSB layout: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(usbPath, "1-2:1.4", "ttyACM0"), 0o755); err != nil {
		t.Fatalf("mkdir direct ttyACM layout: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(usbPath, "1-2:1.5", "tty", "ttyACM1"), 0o755); err != nil {
		t.Fatalf("mkdir nested ttyACM layout: %v", err)
	}

	got := findATPortsInUSBPath(usbPath)
	want := []string{"/dev/ttyUSB6", "/dev/ttyUSB7", "/dev/ttyACM0", "/dev/ttyACM1"}
	if len(got) != len(want) {
		t.Fatalf("len=%d want=%d got=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d]=%q want=%q all=%v", i, got[i], want[i], got)
		}
	}
}

func TestSelectBestATPortForUSBDeviceEC25UsesDeviceLocalInterfaceOrder(t *testing.T) {
	usbPath := t.TempDir()

	for _, tc := range []struct {
		iface string
		tty   string
	}{
		{iface: "1-1.2:1.2", tty: "ttyUSB4"},
		{iface: "1-1.2:1.3", tty: "ttyUSB5"},
		{iface: "1-1.2:1.4", tty: "ttyUSB6"},
		{iface: "1-1.2:1.5", tty: "ttyUSB7"},
	} {
		if err := os.MkdirAll(filepath.Join(usbPath, tc.iface, tc.tty), 0o755); err != nil {
			t.Fatalf("mkdir %s/%s: %v", tc.iface, tc.tty, err)
		}
	}

	atPorts := findATPortsInUSBPath(usbPath)
	legacy, _ := selectBestATPort(atPorts)
	if legacy != "/dev/ttyUSB4" {
		t.Fatalf("test setup: legacy ATPort=%q want /dev/ttyUSB4", legacy)
	}

	portScan := scanATPortsForUSBDevice(usbPath)
	got, _ := selectBestATPortForUSBDevice(
		usbDeviceIdentity{vendorID: 0x2c7c, productID: 0x0125}, "rndis", portScan,
	)
	if got != "/dev/ttyUSB6" {
		t.Fatalf("EC25 ATPort=%q want /dev/ttyUSB6 (third device-local serial interface)", got)
	}

	fallback, _ := selectBestATPortForUSBDevice(
		usbDeviceIdentity{vendorID: 0x1199, productID: 0x9077}, "rndis", portScan,
	)
	if fallback != legacy {
		t.Fatalf("non-EC25 ATPort=%q want legacy choice %q", fallback, legacy)
	}
}

func TestSelectBestATPortForUSBDeviceEC25QMIUsesDeviceLocalInterfaceOrder(t *testing.T) {
	usbPath := t.TempDir()

	for _, tc := range []struct {
		iface string
		tty   string
	}{
		{iface: "2-1:1.0", tty: "ttyUSB8"},
		{iface: "2-1:1.1", tty: "ttyUSB9"},
		{iface: "2-1:1.2", tty: "ttyUSB10"},
		{iface: "2-1:1.3", tty: "ttyUSB11"},
	} {
		if err := os.MkdirAll(filepath.Join(usbPath, tc.iface, tc.tty), 0o755); err != nil {
			t.Fatalf("mkdir %s/%s: %v", tc.iface, tc.tty, err)
		}
	}

	atPorts := findATPortsInUSBPath(usbPath)
	legacy, _ := selectBestATPort(atPorts)
	if legacy != "/dev/ttyUSB8" {
		t.Fatalf("test setup: legacy ATPort=%q want /dev/ttyUSB8", legacy)
	}

	got, _ := selectBestATPortForUSBDevice(
		usbDeviceIdentity{vendorID: 0x2c7c, productID: 0x0125}, "qmi", scanATPortsForUSBDevice(usbPath),
	)
	if got != "/dev/ttyUSB10" {
		t.Fatalf("EC25 ATPort=%q want /dev/ttyUSB10 (third device-local serial interface)", got)
	}
}

func TestSelectBestATPortForUSBDeviceEC25IncompleteEnumerationFallsBack(t *testing.T) {
	usbPath := t.TempDir()

	for _, tc := range []struct {
		iface string
		tty   string
	}{
		{iface: "1-1.2:1.2", tty: "ttyUSB4"},
		{iface: "1-1.2:1.3", tty: "ttyUSB5"},
		{iface: "1-1.2:1.4", tty: "ttyUSB6"},
	} {
		if err := os.MkdirAll(filepath.Join(usbPath, tc.iface, tc.tty), 0o755); err != nil {
			t.Fatalf("mkdir %s/%s: %v", tc.iface, tc.tty, err)
		}
	}

	atPorts := findATPortsInUSBPath(usbPath)
	legacy, _ := selectBestATPort(atPorts)
	got, _ := selectBestATPortForUSBDevice(
		usbDeviceIdentity{vendorID: 0x2c7c, productID: 0x0125}, "rndis", scanATPortsForUSBDevice(usbPath),
	)
	if got != legacy {
		t.Fatalf("incomplete EC25 ATPort=%q want legacy choice %q", got, legacy)
	}
}

func TestSelectBestATPortForUSBDeviceEC25RejectsNewerSysfsSnapshot(t *testing.T) {
	usbPath := t.TempDir()

	for _, tc := range []struct {
		iface string
		tty   string
	}{
		{iface: "1-1.2:1.2", tty: "ttyUSB4"},
		{iface: "1-1.2:1.3", tty: "ttyUSB5"},
		{iface: "1-1.2:1.4", tty: "ttyUSB6"},
		{iface: "1-1.2:1.5", tty: "ttyUSB7"},
	} {
		if err := os.MkdirAll(filepath.Join(usbPath, tc.iface, tc.tty), 0o755); err != nil {
			t.Fatalf("mkdir %s/%s: %v", tc.iface, tc.tty, err)
		}
	}

	initialSnapshot := []string{"/dev/ttyUSB4", "/dev/ttyUSB5"}
	legacy, _ := selectBestATPort(initialSnapshot)
	portScan := scanATPortsForUSBDevice(usbPath)
	portScan.candidates = initialSnapshot
	got, _ := selectBestATPortForUSBDevice(
		usbDeviceIdentity{vendorID: quectelVendorID, productID: quectel0125ProductID},
		"rndis",
		portScan,
	)
	if got != legacy {
		t.Fatalf("ATPort=%q want initial snapshot fallback %q", got, legacy)
	}
	if containsPort(initialSnapshot, "/dev/ttyUSB6") {
		t.Fatal("test setup unexpectedly contains the later AT port")
	}
}
