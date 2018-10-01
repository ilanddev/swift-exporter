package exporter

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/net"
)

var (
	swiftExporterLog, swiftExporterLogError = os.OpenFile("/var/log/swift_exporter.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	individualCPUStatValue                  = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cpu_stat",
		Help: "CPU Stat - the data exposed here is in percentage (with 1 = 100%).",
	}, []string{"cpu_name", "metrics_name", "FQDN", "UUID"})
	nicMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nic_stat",
		Help: "NIC Stat - 'byte_*' metrics is measured in bytes. While 'pckt_*' and 'err_*' are measured in packet counts.",
	}, []string{"nic_name", "mac_address", "metrics_name", "FQDN", "UUID"})
	nicMTU = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nic_mtu",
		Help: "NIC MTU Reading",
	}, []string{"nic_name", "FQDN", "UUID"})
	swiftDriveUsage = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_drive_usage",
		Help: "Swift Drive Usage in bytes (B)",
	}, []string{"swift_drive_label", "drive_type", "state"})
	swiftDrivePercentageUsed = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_drive_percentage_used",
		Help: "Swift Drive Used in Percentage",
	}, []string{"swift_drive_label"})
	swiftInodesUsage = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_inodes_total",
		Help: "Swift Drive Total Inodes - the number of inodes, no specific unit",
	}, []string{"swift_drive_label", "state", "drive_type"})
	swiftDriveIOStat = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_drive_io_stat",
		Help: "swift_drive_io_stat expose drive io-related data to prometheus measures in Bytes (B).",
	}, []string{"swift_drive", "metric_name", "drive_type", "FQDN", "UUID"})
	swiftStoragePolicyUsage = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_storage_policy_usage",
		Help: "Utilization Per Storage Policy. This metrics will be fetched every 6 hours instead of minutes",
	}, []string{"swift_drive_mountpoint", "swift_drive_label", "storage_policy_name", "FQDN", "UUID"})
)

func init() {
	prometheus.MustRegister(individualCPUStatValue)
	prometheus.MustRegister(nicMetric)
	prometheus.MustRegister(swiftDriveUsage)
	prometheus.MustRegister(swiftInodesUsage)
	prometheus.MustRegister(swiftDriveIOStat)
	prometheus.MustRegister(nicMTU)
	prometheus.MustRegister(swiftDrivePercentageUsed)
}

// ExposePerCPUUsage Description: this function makes use of shirou/gopsutil to expose cputime metrics
// and convert them into percentage (out of 1), and expose them out to prometheus.
func ExposePerCPUUsage(ExposePerCPUUsageEnable bool) {

	writeLogFile := log.New(swiftExporterLog, "ExposePerCPUUsage: ", log.Ldate|log.Ltime|log.Lshortfile)

	if ExposePerCPUUsageEnable {
		writeLogFile.Println("ExposeCPUUsage ENABLED")
		hostFQDN, hostUUID := GetUUIDAndFQDN()

		PerCPUUsageTime, _ := cpu.Times(true)
		for i := 0; i < len(PerCPUUsageTime); i++ {

			//read the splice []TimeStat
			cpuName := PerCPUUsageTime[i].CPU
			cpuUserTime := PerCPUUsageTime[i].User
			cpuSystemTime := PerCPUUsageTime[i].System
			cpuIdleTime := PerCPUUsageTime[i].Idle
			cpuNiceTime := PerCPUUsageTime[i].Nice
			cpuIOWaitTime := PerCPUUsageTime[i].Iowait
			cpuIRQTime := PerCPUUsageTime[i].Irq
			cpuSoftIRQTime := PerCPUUsageTime[i].Softirq
			cpuStealTime := PerCPUUsageTime[i].Steal
			cpuGuestTime := PerCPUUsageTime[i].Guest
			cpuGuestNiceTime := PerCPUUsageTime[i].GuestNice
			cpuStolenTime := PerCPUUsageTime[i].Stolen

			//calculate the total time used based on all the parameters it acquired
			totalTimeUsed := cpuUserTime + cpuSystemTime + cpuIdleTime + cpuNiceTime + cpuIOWaitTime + cpuIRQTime + cpuSoftIRQTime + cpuStealTime + cpuGuestTime + cpuGuestNiceTime + cpuStolenTime

			//for each metrcis, do the percentage calculate.
			usrValue := cpuUserTime / totalTimeUsed
			sysValue := cpuSystemTime / totalTimeUsed
			idleValue := cpuIdleTime / totalTimeUsed
			niceValue := cpuNiceTime / totalTimeUsed
			iowaitValue := cpuIOWaitTime / totalTimeUsed
			irqValue := cpuIRQTime / totalTimeUsed
			softirqValue := cpuSoftIRQTime / totalTimeUsed
			stealValue := cpuStealTime / totalTimeUsed
			guestValue := cpuGuestTime / totalTimeUsed
			guestniceValue := cpuGuestNiceTime / totalTimeUsed
			stolenValue := cpuStolenTime / totalTimeUsed

			//expose the metrics out to prometheus
			individualCPUStatValue.WithLabelValues(cpuName, "usr", hostFQDN, hostUUID).Set(usrValue)
			individualCPUStatValue.WithLabelValues(cpuName, "sys", hostFQDN, hostUUID).Set(sysValue)
			individualCPUStatValue.WithLabelValues(cpuName, "idle", hostFQDN, hostUUID).Set(idleValue)
			individualCPUStatValue.WithLabelValues(cpuName, "nice", hostFQDN, hostUUID).Set(niceValue)
			individualCPUStatValue.WithLabelValues(cpuName, "iowait", hostFQDN, hostUUID).Set(iowaitValue)
			individualCPUStatValue.WithLabelValues(cpuName, "irq", hostFQDN, hostUUID).Set(irqValue)
			individualCPUStatValue.WithLabelValues(cpuName, "softirq", hostFQDN, hostUUID).Set(softirqValue)
			individualCPUStatValue.WithLabelValues(cpuName, "steal", hostFQDN, hostUUID).Set(stealValue)
			individualCPUStatValue.WithLabelValues(cpuName, "guest", hostFQDN, hostUUID).Set(guestValue)
			individualCPUStatValue.WithLabelValues(cpuName, "guestnice", hostFQDN, hostUUID).Set(guestniceValue)
			individualCPUStatValue.WithLabelValues(cpuName, "stolen", hostFQDN, hostUUID).Set(stolenValue)

			//for each metrcis, do the percentage calculate.
		}
	} else {
		writeLogFile.Println("ExposeCPUUsage Disabled")
		writeLogFile.Println()
	}
}

// ExposePerNICMetric description: This function makes use of the net library in github.com/shirou/net
// library to gather network interface card related data such as byte sent, byte receive, packet sent,
// packet receive, error in, and error out. After these data is exposed, these data will be exposed to
// prometheus.
func ExposePerNICMetric(ExposePerNICMetricEnable bool) {

	writeLogFile := log.New(swiftExporterLog, "ExposePerNICMetric: ", log.Ldate|log.Ltime|log.Lshortfile)

	if ExposePerNICMetricEnable {
		// perNicMetric get the IO counts of each interface available in the node.
		// nicInfo gets the MAC and IP address of each interface available in the node.
		perNicMetric, _ := net.IOCounters(true)
		nicInfo, _ := net.Interfaces()
		hostFQDN, hostUUID := GetUUIDAndFQDN()

		writeLogFile.Println("ExposePerNICMetric Module ENABLED")

		for i := 0; i < len(perNicMetric); i++ {
			var nicName string
			var nicMACAddr string

			nicName = perNicMetric[i].Name
			for j := 0; j < len(nicInfo); j++ {
				if strings.Compare(nicName, nicInfo[j].Name) == 0 {
					nicMACAddr = nicInfo[j].HardwareAddr
				} else {
					continue
				}
			}
			nicByteSent := perNicMetric[i].BytesSent
			nicByteRecv := perNicMetric[i].BytesRecv
			nicPcktSent := perNicMetric[i].PacketsSent
			nicPcktRecv := perNicMetric[i].PacketsRecv
			nicErrIn := perNicMetric[i].Errin
			nicErrOut := perNicMetric[i].Errout

			nicMetric.WithLabelValues(nicName, nicMACAddr, "byte_sent", hostFQDN, hostUUID).Set(float64(nicByteSent))
			nicMetric.WithLabelValues(nicName, nicMACAddr, "byte_recv", hostFQDN, hostUUID).Set(float64(nicByteRecv))
			nicMetric.WithLabelValues(nicName, nicMACAddr, "pckt_sent", hostFQDN, hostUUID).Set(float64(nicPcktSent))
			nicMetric.WithLabelValues(nicName, nicMACAddr, "pckt_recv", hostFQDN, hostUUID).Set(float64(nicPcktRecv))
			nicMetric.WithLabelValues(nicName, nicMACAddr, "err_in", hostFQDN, hostUUID).Set(float64(nicErrIn))
			nicMetric.WithLabelValues(nicName, nicMACAddr, "err_out", hostFQDN, hostUUID).Set(float64(nicErrOut))
		}

	} else {
		writeLogFile.Println("ExposePerNICMetric Disabled")
		writeLogFile.Println()
	}

}

func GrabNICMTU() {

	hostFQDN, hostUUID := GetUUIDAndFQDN()
	cmd, _ := exec.Command("ls", "/sys/class/net").Output()
	outputString := strings.Split(string(cmd), "\n")
	outputString = outputString[:len(outputString)-1]
	for i := 0; i < len(outputString); i++ {
		if strings.Contains(outputString[i], "docker") {
			continue
		} else if strings.Contains(outputString[i], "lo") {
			continue
		}

		// grab the
		targetLocationArray := []string{"/sys/class/net/", outputString[i]}
		targetLocation := strings.Join(targetLocationArray, "")
		targetLocationArray[0] = targetLocation
		targetLocationArray[1] = "/mtu"
		targetLocation = strings.Join(targetLocationArray, "")

		getMTU, _ := exec.Command("cat", targetLocation).Output()
		mtu, _ := strconv.ParseFloat(string(getMTU[:len(getMTU)-1]), 64)
		nicMTU.WithLabelValues(outputString[i], hostFQDN, hostUUID).Set(mtu)
	}

}

// SwiftDiskUsage makes use of the "github.com/shirou/gopsutil/disk" golang library to grab total disk
// space, used space, inode total, inode free, and inode used. Once it grab the metrics, it will expose
// them via Prometheus.
func SwiftDiskUsage(SwiftDiskUsageEnable bool) {

	writeLogFile := log.New(swiftExporterLog, "SwiftDiskUsage: ", log.Ldate|log.Ltime|log.Lshortfile)

	if SwiftDiskUsageEnable {
		writeLogFile.Println("SwiftDiskUsage Module ENABLED")
		swiftDrive, _ := disk.Partitions(false)
		for i := 0; i < len(swiftDrive); i++ {
			swiftDriveLabel := swiftDrive[i].Mountpoint
			driveType := HddOrSSD(swiftDrive[i].Device)
			diskUsage, _ := disk.Usage(swiftDriveLabel)
			if strings.Contains(swiftDriveLabel, "/srv/node") {
				swiftMountPoint := strings.Split(swiftDriveLabel, "/")[3]
				swiftDriveUsage.WithLabelValues(swiftMountPoint, driveType, "total").Set(float64(diskUsage.Total))
				totalAvailableDiskSpace := float64(diskUsage.Total)
				swiftDriveUsage.WithLabelValues(swiftMountPoint, driveType, "used").Set(float64(diskUsage.Used))
				usedDiskSpace := float64(diskUsage.Used)
				swiftDriveUsage.WithLabelValues(swiftMountPoint, driveType, "free").Set(float64(diskUsage.Free))
				swiftInodesUsage.WithLabelValues(swiftMountPoint, driveType, "total").Set(float64(diskUsage.InodesTotal))
				swiftInodesUsage.WithLabelValues(swiftMountPoint, driveType, "used").Set(float64(diskUsage.InodesUsed))
				swiftInodesUsage.WithLabelValues(swiftMountPoint, driveType, "free").Set(float64(diskUsage.InodesFree))
				diskUsedPercentage := usedDiskSpace / totalAvailableDiskSpace
				swiftDrivePercentageUsed.WithLabelValues(swiftDriveLabel).Set(diskUsedPercentage)
			}
		}

	} else {
		writeLogFile.Println("SwiftDiskUsage Module DISABLED")
	}
}

// SwiftDriveIO uses gopsutil library from "github.com/shirou/gopsutil/disk" to grab various disk-io
// related metrics and expose them via Prometheus.
func SwiftDriveIO(SwiftDriveIOEnable bool) {

	writeLogFile := log.New(swiftExporterLog, "SwiftDriveIO: ", log.Ldate|log.Ltime|log.Lshortfile)

	if SwiftDriveIOEnable {
		writeLogFile.Println("SwiftDriveIO Module ENABLED")

		swiftDrive, _ := disk.Partitions(false)
		swiftDiskIO, _ := disk.IOCounters()
		nodeHostname, nodeUUID := GetUUIDAndFQDN()

		for i := 0; i < len(swiftDrive); i++ {
			if strings.Contains(swiftDrive[i].Mountpoint, "/srv/node") {
				deviceName := swiftDrive[i].Device
				deviceName = strings.Split(deviceName, "/")[2]
				deviceType := HddOrSSD(swiftDrive[i].Device)
				swiftDriveIOStat.WithLabelValues(deviceName, "readCount", deviceType, nodeHostname, nodeUUID).Set(float64(swiftDiskIO[deviceName].ReadCount))
				swiftDriveIOStat.WithLabelValues(deviceName, "mergedReadCount", deviceType, nodeHostname, nodeUUID).Set(float64(swiftDiskIO[deviceName].MergedReadCount))
				swiftDriveIOStat.WithLabelValues(deviceName, "writeCount", deviceType, nodeHostname, nodeUUID).Set(float64(swiftDiskIO[deviceName].WriteCount))
				swiftDriveIOStat.WithLabelValues(deviceName, "mergedWriteCount", deviceType, nodeHostname, nodeUUID).Set(float64(swiftDiskIO[deviceName].MergedWriteCount))
				swiftDriveIOStat.WithLabelValues(deviceName, "readBytes", deviceType, nodeHostname, nodeUUID).Set(float64(swiftDiskIO[deviceName].ReadBytes))
				swiftDriveIOStat.WithLabelValues(deviceName, "writeBytes", deviceType, nodeHostname, nodeUUID).Set(float64(swiftDiskIO[deviceName].WriteBytes))
				swiftDriveIOStat.WithLabelValues(deviceName, "readTime", deviceType, nodeHostname, nodeUUID).Set(float64(swiftDiskIO[deviceName].ReadTime))
				swiftDriveIOStat.WithLabelValues(deviceName, "writeTime", deviceType, nodeHostname, nodeUUID).Set(float64(swiftDiskIO[deviceName].WriteTime))
				swiftDriveIOStat.WithLabelValues(deviceName, "iopsInProgress", deviceType, nodeHostname, nodeUUID).Set(float64(swiftDiskIO[deviceName].IopsInProgress))
				swiftDriveIOStat.WithLabelValues(deviceName, "ioTime", deviceType, nodeHostname, nodeUUID).Set(float64(swiftDiskIO[deviceName].IoTime))
				swiftDriveIOStat.WithLabelValues(deviceName, "weightedIO", deviceType, nodeHostname, nodeUUID).Set(float64(swiftDiskIO[deviceName].WeightedIO))

			}
		}
	} else {
		writeLogFile.Println("SwiftDriveIO Module DISABLED")
	}
}

// HddOrSSD - this function determines if a particular disk is a hard drive or a solid state drive based on the
// valule stores in /sys/block/<drive>/queue/rotational
func HddOrSSD(deviceName string) (driveType string) {

	// the baseLocation and tailLocation are needed to form the entire path to the rotational file that contains
	// the bit that tells the OS whether it is a hard drive or a solid state drive.
	baseLocation := "/sys/block/"
	tailLocation := "/queue/rotational"
	driveLabel := strings.Split(deviceName, "/")
	fullPath := []string{baseLocation, driveLabel[2], tailLocation}
	var typeOfDrive string

	rotationalFilePath := strings.Join(fullPath, "")

	data, _ := ioutil.ReadFile(rotationalFilePath)
	if strings.Compare(strings.TrimSuffix(string(data), "\n"), "1") == 0 {
		typeOfDrive = "HDD"
	} else if strings.Compare(strings.TrimSuffix(string(data), "\n"), "0") == 0 {
		typeOfDrive = "SSD"
	} else {
		fmt.Println(data)
	}

	return typeOfDrive
}
