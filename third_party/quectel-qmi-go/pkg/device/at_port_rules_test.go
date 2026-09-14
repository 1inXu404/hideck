package device

import "testing"

func TestStaticATInterfaceForUSBMode(t *testing.T) {
	tests := []struct {
		name       string
		vendorID   uint16
		productID  uint16
		driverName string
		want       int
	}{
		{name: "EC25 QMI", vendorID: 0x2c7c, productID: 0x0125, driverName: "qmi_wwan", want: 2},
		{name: "EC25 RNDIS", vendorID: 0x2c7c, productID: 0x0125, driverName: "rndis_host", want: 4},
		{name: "EC200U ECM", vendorID: 0x2c7c, productID: 0x0901, driverName: "cdc_ether", want: 2},
		{name: "EC200D ECM", vendorID: 0x2c7c, productID: 0x0902, driverName: "cdc_ether", want: 2},
		{name: "RG801H NCM", vendorID: 0x2c7c, productID: 0x8101, driverName: "cdc_ncm", want: 2},
		{name: "RG500U NCM", vendorID: 0x2c7c, productID: 0x0900, driverName: "cdc_ncm", want: 4},
		{name: "EC200T ECM", vendorID: 0x2c7c, productID: 0x6026, driverName: "cdc_ether", want: 3},
		{name: "EC200A ECM", vendorID: 0x2c7c, productID: 0x6005, driverName: "cdc_ether", want: 3},
		{name: "EC200S ECM", vendorID: 0x2c7c, productID: 0x6002, driverName: "cdc_ether", want: 3},
		{name: "EC100Y ECM", vendorID: 0x2c7c, productID: 0x6001, driverName: "cdc_ether", want: 3},
		{name: "EG915Q ECM", vendorID: 0x2c7c, productID: 0x6007, driverName: "cdc_ether", want: 3},
		{name: "EG915Q RNDIS", vendorID: 0x2c7c, productID: 0x6007, driverName: "rndis_host", want: 5},
		{name: "EC801E ECM", vendorID: 0x2c7c, productID: 0x0903, driverName: "cdc_ether", want: 3},
		{name: "EC801E RNDIS", vendorID: 0x2c7c, productID: 0x0903, driverName: "rndis_host", want: 3},
		{name: "EC801E NCM keeps prior default", vendorID: 0x2c7c, productID: 0x0903, driverName: "cdc_ncm", want: 2},
		{name: "EC801E QMI", vendorID: 0x2c7c, productID: 0x0903, driverName: "qmi_wwan", want: 2},
		{name: "Qualcomm default", vendorID: 0x05c6, productID: 0x9215, driverName: "qmi_wwan", want: 2},
		{name: "unsupported vendor", vendorID: 0x1199, productID: 0x9077, driverName: "qmi_wwan", want: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := staticATInterfaceForUSBMode(tt.vendorID, tt.productID, tt.driverName)
			if got != tt.want {
				t.Fatalf("interface=%d want=%d", got, tt.want)
			}
		})
	}
}
