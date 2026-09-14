package device

import "strings"

const (
	quectelVendorID        = 0x2c7c
	qualcommVendorID       = 0x05c6
	quectel0125ProductID   = 0x0125
	quectelEC200UProductID = 0x0901
	quectelEC200DProductID = 0x0902
	quectelRG801HProductID = 0x8101
	quectelRG500UProductID = 0x0900
	quectelEC200TProductID = 0x6026
	quectelEC200AProductID = 0x6005
	quectelEC200SProductID = 0x6002
	quectelEC100YProductID = 0x6001
	quectelEC801EProductID = 0x0903
	quectelEG915QProductID = 0x6007
)

// staticATInterfaceForUSBMode uses device-local interface numbers so global
// ttyUSB numbering cannot associate an AT port with the wrong modem. Known
// Ethernet mappings follow Quectel QConnectManager; QMI defaults are retained.
func staticATInterfaceForUSBMode(vendorID, productID uint16, driverName string) int {
	if vendorID == qualcommVendorID {
		return 2
	}
	if vendorID != quectelVendorID {
		return -1
	}
	driverName = strings.ToLower(strings.TrimSpace(driverName))
	rndis := strings.Contains(driverName, "rndis")
	switch productID {
	case quectel0125ProductID:
		if rndis {
			return 4
		}
		return 2
	case quectelEC200UProductID, quectelEC200DProductID, quectelRG801HProductID:
		return 2
	case quectelRG500UProductID:
		return 4
	case quectelEC200TProductID, quectelEC200AProductID, quectelEC200SProductID,
		quectelEC100YProductID:
		return 3
	case quectelEC801EProductID:
		if rndis || strings.Contains(driverName, "ether") {
			return 3
		}
		return 2
	case quectelEG915QProductID:
		if rndis {
			return 5
		}
		return 3
	default:
		return 2
	}
}
