package exporter

import (
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/net"
	"github.com/shirou/gopsutil/process"
)

var (
	swiftObjectServerConnection = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "swift_object_server_connection",
		Help: "Number of object server connections at this moment. This is calculated base on # of drives * object server per port setting",
	})
	swiftDriveReallocatedSectorCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_drive_reallocated_sector_count",
		Help: "swift_drive_reallocated_sector_count comes from the reallocated sector counts from smartctl command. This is an indicator to see if a drive starts to fail",
	}, []string{"drive_label", "drive_type", "FQDN", "UUID"})
	swiftDriveOfflineUncorrectableCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_drive_offline_uncorrectable_count",
		Help: "swift_drive_offline_uncorrectable_count is one of the outputs from smartctl that '[i]ndicates how many defective sectors were found during the off-line scan'",
	}, []string{"drive_label", "drive_type", "FQDN", "UUID"})
	swiftDriveMediaWearoutIndicatorCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_drive_media_wearout_indicator_count",
		Help: "This is Intel SSD specific metric used to indicate the health status of an Intel SSD. The value of 100 indicates a brand new drive, and the value will decrease from that point on",
	}, []string{"drive_label", "drive_type", "FQDN", "UUID"})
	swiftDriveWearLevelingCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_drive_wear_leveling_count",
		Help: "This is Samsung SSD specific metrics used to indicate the health status of an Samsung SSD. The value of 100 indicates a brand new drive, and the value will decrease from that point on",
	}, []string{"drive_label", "drive_type", "FQDN", "UUID"})
)

func init() {
	prometheus.MustRegister(swiftDriveReallocatedSectorCount)
	prometheus.MustRegister(swiftDriveOfflineUncorrectableCount)
	prometheus.MustRegister(swiftDriveMediaWearoutIndicatorCount)
	prometheus.MustRegister(swiftDriveWearLevelingCount)
	prometheus.MustRegister(swiftObjectServerConnection)
}

// CheckObjectServerConnection makes use of the gopsutil library to get the number of object-server
// instance that is being run.
func CheckObjectServerConnection(CheckObjectServerConnectionEnable bool) {

	writeLogFile := log.New(swiftExporterLog, "CheckObjectServerConnection: ", log.Ldate|log.Ltime|log.Lshortfile)

	if CheckObjectServerConnectionEnable {
		// Get all running processes in the node
		runningProcess, _ := process.Pids()
		counter := 0
		numberOfRunningProcess := len(runningProcess)
		writeLogFile.Println(numberOfRunningProcess)

		// For each running process, check to see if there is an opened network connection
		for i := 0; i < len(runningProcess); i++ {
			currentEntryPid := runningProcess[i]
			processConnected, _ := net.ConnectionsPid("tcp", currentEntryPid)
			if len(processConnected) == 0 {
				continue
			} else {
				if processConnected[0].Laddr.Port == uint32(6000) {
					counter++
					writeLogFile.Println(processConnected)
				}
			}
		}
		swiftObjectServerConnection.Set(float64(counter) - 1)
	} else {
		writeLogFile.Println("CheckObjectServerConnection Module DISABLED")
	}

}

// RunSMARTCTL module run "smartctl -A <device_label" to get the "reallocated sectors" and
// offline uncorrectable count that serve as an indicator to see if a drive is failing.
// Unlike other modules that can be turned on/off, this module runs all the time as drives
// health is important in the Swift cluster."
func RunSMARTCTL() {
	var reallocationSectorsCount float64
	var offlineUncorrectableCount float64
	var wearLevelingCount float64
	var mediaWearoutIndicator float64

	fmt.Println("Staring RunSMARTCTL Module...")
	// get the FQDN and UUID of the node as part tag used when exposing the data out to prometheus.
	nodeFQDN, nodeUUID, _ := GetUUIDAndFQDN(ssnodeConfFile)
	// grabbing the device list from the node using the disk library in gopsutil library.
	grabNodeDeviceList, _ := disk.Partitions(false)

	// for each of the drive in the node...
	for i := 0; i < len(grabNodeDeviceList); i++ {
		driveList := grabNodeDeviceList[i].Device // get device list
		swiftDriveType := HddOrSSD(driveList)     // find out whether the drive is a HDD or SSD
		fmt.Println(driveList)
		smartctlExist, smartctlDoesNotExist := exec.Command("which", "smartctl").Output()
		smartctlLocation := string(smartctlExist)

		fmt.Println("Checking to see if RunSMARTCTL exist...")
		if smartctlDoesNotExist != nil {
			// if exec.Command returns error, that is either binary is not available / there is something wrong with the binary,
			// print the error message out.
			fmt.Println("smartctl may not exist in the node, or you may have other problems with it")
			break
		}

		fmt.Println("smartctl exists...Running smartctl command to grab SMART data...")
		runCommand, _ := exec.Command(smartctlLocation, "-A", driveList).Output() // run "smartctl -A <device_label>" command

		// if exec.Command returns good result, reformat the output to expose them out in prometheus.
		result := string(runCommand)          // convert the []byte slice to text string - which is one big text delimited by /n
		output := strings.Split(result, "\n") // break the text string and convert them into string array
		// for each element in the string array, scan for word "Reallocated_Sector_Ct" and "Offline_Uncorrectable"
		if strings.Compare(swiftDriveType, "HDD") == 0 {
			for j := 0; j < len(output); j++ {
				if strings.Contains(output[j], "Reallocated_Sector_Ct") {
					parseOutput := strings.Split(output[j], " ")
					reallocationSectorsCount, _ = strconv.ParseFloat(string(parseOutput[len(parseOutput)-1]), 64)
					swiftDriveReallocatedSectorCount.WithLabelValues(swiftDriveType, nodeFQDN, nodeUUID).Set(reallocationSectorsCount)
				} else if strings.Contains(output[j], "Offline_Uncorrectable") {
					parseOutput := strings.Split(output[j], " ")
					offlineUncorrectableCount, _ = strconv.ParseFloat(string(parseOutput[len(parseOutput)-1]), 64)
					swiftDriveOfflineUncorrectableCount.WithLabelValues(swiftDriveType, nodeFQDN, nodeUUID).Set(offlineUncorrectableCount)
				} else {
					fmt.Println(output)
					fmt.Println("False")
				}
			}
		} else if strings.Compare(swiftDriveType, "SSD") == 0 {
			getManufacture, _ := exec.Command("smartctl", "-i", driveList).Output()
			manufactureConvertToString := string(getManufacture)
			manufactureOutput := strings.Split(manufactureConvertToString, "\n")
			for k := 0; k < len(manufactureOutput); k++ {
				if strings.Contains(manufactureOutput[k], "Samsung") {
					for j := 0; j < len(output); j++ {
						if strings.Contains(output[j], "Wear Leveling Count") {
							parseOutput := strings.Split(output[j], " ")
							wearLevelingCount, _ = strconv.ParseFloat(string(parseOutput[len(parseOutput)-1]), 64)
							swiftDriveWearLevelingCount.WithLabelValues(driveList, swiftDriveType, nodeFQDN, nodeUUID).Set(wearLevelingCount)
						}
					}
				} else if strings.Contains(manufactureOutput[k], "Intel") {
					for j := 0; j < len(output); j++ {
						if strings.Contains(output[j], "Media Wearout Indicator") {
							parseOutput := strings.Split(output[j], " ")
							mediaWearoutIndicator, _ = strconv.ParseFloat(string(parseOutput[3]), 64)
							swiftDriveMediaWearoutIndicatorCount.WithLabelValues(driveList, swiftDriveType, nodeFQDN, nodeUUID).Set(mediaWearoutIndicator)
						}
					}
				}
			}
		}
	}
}
