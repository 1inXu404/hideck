package device

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type usbSerialPort struct {
	path            string
	interfaceNumber int
}

type usbATPortScan struct {
	candidates       []string
	interfaceOrdered []usbSerialPort
}

func scanATPortsForUSBDevice(usbPath string) usbATPortScan {
	interfaceOrdered := scanUSBSerialPorts(usbPath)
	if len(interfaceOrdered) == 0 {
		return usbATPortScan{candidates: findATPortsInUSBPath(usbPath)}
	}

	interfacePaths := make([]string, 0, len(interfaceOrdered))
	for _, port := range interfaceOrdered {
		interfacePaths = append(interfacePaths, port.path)
	}
	return usbATPortScan{
		candidates:       sortATPortCandidates(interfacePaths),
		interfaceOrdered: interfaceOrdered,
	}
}

// scanUSBSerialPorts reads one device-local topology snapshot without opening
// any serial device. Global tty numbers are retained only as node paths.
func scanUSBSerialPorts(usbPath string) []usbSerialPort {
	ifaces, _ := filepath.Glob(filepath.Join(usbPath, "*:1.*"))
	sortUSBInterfacePaths(ifaces)

	seen := make(map[string]struct{})
	ports := make([]usbSerialPort, 0)
	for _, ifPath := range ifaces {
		interfaceNumber, ok := usbInterfaceNumber(ifPath)
		if !ok {
			continue
		}
		for _, port := range findTTYDevicePathsInInterface(ifPath) {
			if _, duplicate := seen[port]; duplicate {
				continue
			}
			seen[port] = struct{}{}
			ports = append(ports, usbSerialPort{path: port, interfaceNumber: interfaceNumber})
		}
	}
	return ports
}

func findTTYDevicePathsInInterface(ifPath string) []string {
	ports := make([]string, 0)
	for _, ttyPattern := range []string{"ttyUSB*", "ttyACM*"} {
		for _, pattern := range []string{
			filepath.Join(ifPath, ttyPattern),
			filepath.Join(ifPath, "tty", ttyPattern),
		} {
			matches, _ := filepath.Glob(pattern)
			sort.Strings(matches)
			for _, match := range matches {
				ports = append(ports, filepath.Join("/dev", filepath.Base(match)))
			}
		}
	}
	return ports
}

func findATPortsInUSBPath(usbPath string) []string {
	ports := make([]string, 0)
	for _, ttyPattern := range []string{"ttyUSB*", "ttyACM*"} {
		for _, pattern := range []string{
			filepath.Join(usbPath, "*", ttyPattern),
			filepath.Join(usbPath, "*", "tty", ttyPattern),
		} {
			matches, _ := filepath.Glob(pattern)
			for _, match := range matches {
				ports = append(ports, filepath.Join("/dev", filepath.Base(match)))
			}
		}
	}
	return sortATPortCandidates(ports)
}

func usbATPortScanIsConsistent(scan usbATPortScan) bool {
	interfacePaths := make([]string, 0, len(scan.interfaceOrdered))
	for _, port := range scan.interfaceOrdered {
		interfacePaths = append(interfacePaths, port.path)
	}
	left := dedupSortedNonEmpty(interfacePaths)
	right := dedupSortedNonEmpty(scan.candidates)
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func selectBestATPort(atPorts []string) (bestPort, imei string) {
	ports := sortATPortCandidates(atPorts)
	if len(ports) == 0 {
		return "", ""
	}
	return ports[0], ""
}

func sortATPortCandidates(atPorts []string) []string {
	out := dedupSortedNonEmpty(atPorts)
	sort.SliceStable(out, func(i, j int) bool {
		leftPriority := atPortPriority(out[i])
		rightPriority := atPortPriority(out[j])
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		return out[i] < out[j]
	})
	return out
}

func atPortPriority(port string) int {
	base := filepath.Base(strings.TrimSpace(port))
	if strings.HasPrefix(base, "ttyACM") {
		number, err := strconv.Atoi(strings.TrimPrefix(base, "ttyACM"))
		if err != nil {
			return 1900
		}
		return 1000 + number
	}
	if !strings.HasPrefix(base, "ttyUSB") {
		return 2000
	}
	number, err := strconv.Atoi(strings.TrimPrefix(base, "ttyUSB"))
	if err != nil {
		return 900
	}
	switch {
	case number >= 2 && number <= 5:
		return number - 2
	case number > 5:
		return 20 + number
	default:
		return 200 + number
	}
}

func dedupSortedNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
