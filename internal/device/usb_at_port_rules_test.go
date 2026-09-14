package device

import "testing"

func TestUSBATPortRuleForKnownModemCompositions(t *testing.T) {
	tests := usbATPortRuleTestCases()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, ok := usbATPortRuleFor(tt.identity, tt.mode)
			if ok != tt.wantRule {
				t.Fatalf("rule found=%v want=%v rule=%+v", ok, tt.wantRule, rule)
			}
			if !ok {
				return
			}
			if rule.interfaceNumber != tt.wantInterface || rule.serialPortCount != tt.wantCount || rule.serialPortIndex != tt.wantIndex {
				t.Fatalf("rule=%+v want interface=%d count=%d index=%d", rule, tt.wantInterface, tt.wantCount, tt.wantIndex)
			}
		})
	}
}

type usbATPortRuleTestCase struct {
	name          string
	identity      usbDeviceIdentity
	mode          string
	wantInterface int
	wantCount     int
	wantIndex     int
	wantRule      bool
}

func usbATPortRuleTestCases() []usbATPortRuleTestCase {
	quectel := func(productID uint16) usbDeviceIdentity {
		return usbDeviceIdentity{vendorID: quectelVendorID, productID: productID}
	}
	return []usbATPortRuleTestCase{
		{name: "0125 qmi", identity: quectel(0x0125), mode: "qmi", wantInterface: 2, wantCount: 4, wantIndex: 2, wantRule: true},
		{name: "0125 rndis", identity: quectel(0x0125), mode: "rndis", wantInterface: -1, wantCount: 4, wantIndex: 2, wantRule: true},
		{name: "ec200u ecm", identity: quectel(0x0901), mode: "ecm", wantInterface: 2, wantIndex: -1, wantRule: true},
		{name: "ec200d ecm", identity: quectel(0x0902), mode: "ecm", wantInterface: 2, wantIndex: -1, wantRule: true},
		{name: "rg801h ncm", identity: quectel(0x8101), mode: "ncm", wantInterface: 2, wantIndex: -1, wantRule: true},
		{name: "rg500u ncm", identity: quectel(0x0900), mode: "ncm", wantInterface: 4, wantIndex: -1, wantRule: true},
		{name: "ec200t ecm", identity: quectel(0x6026), mode: "ecm", wantInterface: 3, wantIndex: -1, wantRule: true},
		{name: "ec200a ecm", identity: quectel(0x6005), mode: "ecm", wantInterface: 3, wantIndex: -1, wantRule: true},
		{name: "ec200s ecm", identity: quectel(0x6002), mode: "ecm", wantInterface: 3, wantIndex: -1, wantRule: true},
		{name: "ec100y ecm", identity: quectel(0x6001), mode: "ecm", wantInterface: 3, wantIndex: -1, wantRule: true},
		{name: "eg915q ecm", identity: quectel(0x6007), mode: "ecm", wantInterface: 3, wantIndex: -1, wantRule: true},
		{name: "eg915q rndis", identity: quectel(0x6007), mode: "rndis", wantInterface: 5, wantIndex: -1, wantRule: true},
		{name: "ec801e ecm", identity: quectel(0x0903), mode: "ecm", wantInterface: 3, wantIndex: -1, wantRule: true},
		{name: "ec801e rndis", identity: quectel(0x0903), mode: "rndis", wantInterface: 3, wantIndex: -1, wantRule: true},
		{name: "ec801e ncm has no documented rule", identity: quectel(0x0903), mode: "ncm"},
		{name: "ec801e qmi keeps prior default", identity: quectel(0x0903), mode: "qmi", wantInterface: 2, wantIndex: -1, wantRule: true},
		{name: "known quectel mbim has no ethernet rule", identity: quectel(0x0901), mode: "mbim"},
		{name: "unknown quectel qmi", identity: quectel(0xffff), mode: "qmi", wantInterface: 2, wantIndex: -1, wantRule: true},
		{name: "unknown quectel rndis", identity: quectel(0xffff), mode: "rndis"},
		{name: "qualcomm qmi", identity: usbDeviceIdentity{vendorID: qualcommVendorID, productID: 0x9215}, mode: "qmi", wantInterface: 2, wantIndex: -1, wantRule: true},
		{name: "qualcomm rndis", identity: usbDeviceIdentity{vendorID: qualcommVendorID, productID: 0x9215}, mode: "rndis"},
		{name: "other vendor", identity: usbDeviceIdentity{vendorID: 0x1199, productID: 0x9077}, mode: "qmi"},
	}
}

func TestSelectBestATPortForKnownQuectelCompositions(t *testing.T) {
	scan := usbATPortScan{
		candidates: []string{"/dev/ttyUSB8", "/dev/ttyUSB9", "/dev/ttyUSB10", "/dev/ttyUSB11"},
		interfaceOrdered: []usbSerialPort{
			{path: "/dev/ttyUSB10", interfaceNumber: 2},
			{path: "/dev/ttyUSB8", interfaceNumber: 3},
			{path: "/dev/ttyUSB11", interfaceNumber: 4},
			{path: "/dev/ttyUSB9", interfaceNumber: 5},
		},
	}
	tests := []struct {
		name      string
		productID uint16
		mode      string
		want      string
	}{
		{name: "EC200U ECM", productID: 0x0901, mode: "ecm", want: "/dev/ttyUSB10"},
		{name: "RG500U NCM", productID: 0x0900, mode: "ncm", want: "/dev/ttyUSB11"},
		{name: "EG915Q RNDIS", productID: 0x6007, mode: "rndis", want: "/dev/ttyUSB9"},
		{name: "unknown RNDIS", productID: 0xffff, mode: "rndis", want: "/dev/ttyUSB8"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := selectBestATPortForUSBDevice(
				usbDeviceIdentity{vendorID: quectelVendorID, productID: tt.productID}, tt.mode, scan,
			)
			if got != tt.want {
				t.Fatalf("ATPort=%q want=%q", got, tt.want)
			}
		})
	}
}
