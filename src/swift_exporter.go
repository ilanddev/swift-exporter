package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"exporter"

	"github.com/docopt/docopt-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/yaml.v2"
)

// Location of the .recon files in Swift nodes
const accountReconFile = "/var/cache/swift/account.recon"
const containerReconFile = "/var/cache/swift/container.recon"
const objectReconFile = "/var/cache/swift/object.recon"
const replicationProgressFile = "/opt/ss/var/lib/replication_progress.json"
const swiftConfig = "/etc/swift/swift.conf"
const swiftLog = "/var/log/swift/all.log"

type ModulesOnOff struct {
	CheckObjectServerConnectionEnable    bool
	GrabSwiftPartitionEnable             bool
	GatherReplicationEstimateEnable      bool
	GatherStoragePolicyUtilizationEnable bool
	ExposePerCPUUsageEnable              bool
	ExposePerNICMetricEnable             bool
	ReadReconFileEnable                  bool
	SwiftDiskUsageEnable                 bool
	SwiftDriveIOEnable                   bool
	SwiftLogSizeEnable                   bool
}

/*
This var() section sets the port which promohttp (Promethes HTTP server) uses.
In addition, accountServer, containerServer, and objectServer initializes gauge-type prometheus
metrics data.
*/
var (
	scriptVersion                           = "0.8.4"
	timeLastRun                             = "00:00:00"
	swiftExporterLog, swiftExporterLogError = os.OpenFile("/var/log/swift_exporter.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	addr                                    = flag.String("listen-address", ":53167", "The addres to listen on for HTTP requests.")
	abScriptVersionPara                     = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ac_script_version",
		Help: "swift_exporter version 0.8.4",
	}, []string{"script_version"})

	defaultConfig = map[string]bool{
		"ReadReconFile":                  true,
		"GrabSwiftPartition":             true,
		"SwiftDiskUsage":                 true,
		"SwiftDriveIO":                   true,
		"GatherReplicationEstimate":      true,
		"GatherStoragePolicyUtilization": true,
		"CheckObjectServerConnection":    true,
		"ExposePerCPUUsage":              true,
		"ExposePerNICMetric":             true,
	}

	SelectedModule ModulesOnOff
	argv           []string
	Usage          = `Usage:
	   /opt/ss/bin/swift_exporter 
	   /opt/ss/bin/swift_exporter [<swift_export_config_file>]
	   /opt/ss/bin/swift_exporter --help | --version `
)

// Metrics have to be registeered to be expose, so this is done below.
func init() {
	prometheus.MustRegister(abScriptVersionPara)

	if swiftExporterLogError != nil {
		fmt.Println("Error Opening File: %", swiftExporterLog)
	}
}

// SanityCheckOnFiles checks is a function being called in
func SanityCheckOnFiles(SelectedModule ModulesOnOff) {

	writeLogFile := log.New(swiftExporterLog, "SanityCheckOnFiles: ", log.Ldate|log.Ltime|log.Lshortfile)

	if _, swiftConfigErr := os.Stat(swiftConfig); os.IsNotExist(swiftConfigErr) {
		writeLogFile.Println("swift.conf does not exist! Exiting this script!")
		os.Exit(1)
	} else {
		writeLogFile.Println("Swift config file (swift.conf) exist. Continue checking other files")
		writeLogFile.Println("Checking if *.recon (/var/cache/swift/*recon) file exist...")
		if SelectedModule.ReadReconFileEnable {
			writeLogFile.Println("Script is set to expose data collected from /var/cache/swift/*.recon files (ReadReconFile module enable). Check to see if those file exist")
			if _, err := os.Stat(accountReconFile); err == nil {
				writeLogFile.Println(" ===> account.recon file exists. Moving on to check if container.recon file exists...")
			} else {
				writeLogFile.Println(" ===> account.recon file does not exist. We will need all 3 (account, container, object) recon files for this module to work, but you have enable the ReadReconFile module. Turning it off...")
				SelectedModule.ReadReconFileEnable = false
			}
			if _, err := os.Stat(containerReconFile); err == nil {
				writeLogFile.Println(" ===> container.recon file exists. Moving on to check if container.recon file exists...")
			} else {
				writeLogFile.Println(" ===> container.recon file does not exist. We will need all 3 (account, container, object) recon files for this module to work, but you have enable the ReadReconFile module. Turning it off...")
				SelectedModule.ReadReconFileEnable = false
			}
			if _, err := os.Stat(objectReconFile); err == nil {
				writeLogFile.Println(" ===> object.recon file exists. Moving on to check if object.recon file exists")
			} else {
				writeLogFile.Println(" ===> object.recon file does not exist. We will need all 3 (account, container, object) recon files for this module to work, but you have enable the ReadReconFile module. Turning it off...")
				SelectedModule.ReadReconFileEnable = false
			}
			writeLogFile.Println("===> account.recon, container.recon, and object.recon file exist. Check for this module has completed. Enable this module...")
			SelectedModule.ReadReconFileEnable = true
			writeLogFile.Println()
		} else {
			writeLogFile.Println("ReadReconFile module is disabled. Skip this check.")
			writeLogFile.Println()
		}
		if SelectedModule.GrabSwiftPartitionEnable {
			writeLogFile.Println("Script is set to expose data collected from /opt/ss/var/lib/replication_progress.json (GrabSwiftPartition module enable). Check to see if that file exist...")
			if _, err := os.Stat(replicationProgressFile); err == nil {
				log.Println("===> /opt/ss/var/lib/replication_progress.json exists. Check for this module has completed. Enable the module...")
				SelectedModule.GrabSwiftPartitionEnable = true
				writeLogFile.Println()
			} else {
				writeLogFile.Println("===> /opt/ss/var/lib/replication_progress.json does not exists, but you have enabled it. Disable the module...")
				SelectedModule.GrabSwiftPartitionEnable = false
				writeLogFile.Println()
			}
		} else {
			writeLogFile.Println("GrabSwiftPartition module is disabled. Skip this check.")
		}
		if SelectedModule.GatherReplicationEstimateEnable {
			writeLogFile.Println("Script is set to expose data collected from /var/log/swift/all.log (GatherReplicationEstimate module enable). Check to see if that file exist...")
			if _, err := os.Stat(swiftLog); err == nil {
				writeLogFile.Println("===> /var/log/swift/all.log exists. Check for this module has completed. Enable the module...")
				SelectedModule.GatherReplicationEstimateEnable = true
				writeLogFile.Println()
			} else {
				writeLogFile.Println("===> /var/log/swift/all.log does not exists, but you have enabled it. Disable the module...")
				SelectedModule.GatherReplicationEstimateEnable = false
				writeLogFile.Println()
			}
		} else {
			writeLogFile.Println("GatherReplicationEstimate module is disabled. Skip this check.")
			writeLogFile.Println()
		}
		if SelectedModule.GatherStoragePolicyUtilizationEnable {
			writeLogFile.Println("GatherStoragePolicyUtilization module is enabled. Since there is no config, there is nothing to check.")
			writeLogFile.Println()
		} else {
			writeLogFile.Println("GatherStoragePolicyUtilization module is disabled. Skip this check.")
			writeLogFile.Println()
		}
		if SelectedModule.ExposePerCPUUsageEnable {
			writeLogFile.Println("ExposePerCPUUsage module is enabled. Since there is no config, there is nothing to check.")
			writeLogFile.Println()
		} else {
			writeLogFile.Println("ExposePerCPUUsage module is disabled. Skip this check.")
			writeLogFile.Println()
		}
		if SelectedModule.ExposePerNICMetricEnable {
			writeLogFile.Println("ExposePerNICMetric module is enabled. Since there is no config, there is nothing to check.")
			writeLogFile.Println()
		} else {
			writeLogFile.Println("ExposePerNICMetric module is disabled. Skip this check.")
			writeLogFile.Println()
		}
		writeLogFile.Println("All checks complete. Proceed on turning modules on / off.")
		writeLogFile.Println()
	}
}

//TurnOnModules reads through the yaml file and turns on the modules available in this script.
//Input Argument: Location of the yaml file.
//Output Argument: 3 boolean values that will enable func GrabSwiftPartition,
//func SwiftDiskUsage, and func GatherReplicationEstimate
func TurnOnModules(configFileLocation string) (SelectedModule ModulesOnOff) {

	writeLogFile := log.New(swiftExporterLog, "TurnOnModules: ", log.Ldate|log.Ltime|log.Lshortfile)

	// To parse the data correctly, we need the following.
	// Reference: http://squarism.com/2014/10/13/yaml-go/
	var config = make(map[string][]bool)

	filename, _ := os.Open(configFileLocation)
	yamlFile, _ := ioutil.ReadAll(filename)

	err := yaml.Unmarshal(yamlFile, &config)

	// If yaml.Unmarshal cannot extra data and put into the map data structure, do the following:
	if err != nil {
		writeLogFile.Fatalf("cannot unmarshal %v", err)
		writeLogFile.Println(err)
	}

	// If yaml.Unmarshal can output the data into map data structure, do the following:
	SelectedModule.ReadReconFileEnable = config["ReadReconFile"][0]
	SelectedModule.GrabSwiftPartitionEnable = config["GrabSwiftPartition"][0]
	SelectedModule.SwiftDiskUsageEnable = config["SwiftDiskUsage"][0]
	SelectedModule.SwiftDriveIOEnable = config["SwiftDriveIO"][0]
	SelectedModule.GatherStoragePolicyUtilizationEnable = config["GatherStoragePolicyUtilization"][0]
	SelectedModule.CheckObjectServerConnectionEnable = config["CheckObjectServerConnection"][0]
	SelectedModule.ExposePerCPUUsageEnable = config["ExposePerCPUUsage"][0]
	SelectedModule.ExposePerNICMetricEnable = config["ExposePerNICMetric"][0]

	return SelectedModule
	//return ReadReconFileEnable, GrabSwiftPartitionEnable, SwiftDiskUsageEnable, SwiftDriveIOEnable, GatherReplicationEstimateEnable, GatherStoragePolicyUtilizationEnable, CheckObjectServerConnectionEnable, ExposePerCPUUsageEnable, ExposePerNICMetricEnable
}

func main() {

	writeLogFile := log.New(swiftExporterLog, "main: ", log.Ldate|log.Ltime|log.Lshortfile)

	// If user pass an empty argument to the script, use the default value. Assign dummy variable "all"
	// that turns on ALL modules in this script.
	if len(os.Args) < 2 {
		argv = []string{"all"}
	} else if _, err := os.Stat(os.Args[1]); err == nil { // To check if a file exists, equivalent to Python's if os.path.exists(filename):
		argv = []string{os.Args[1]}
	}

	// user docopt to show menu, forward argument, and display version number.
	opts, _ := docopt.ParseArgs(Usage, argv, scriptVersion)

	// extra config file entry.
	ConfigFileExist, _ := opts.String("<swift_export_config_file>")

	abScriptVersionPara.WithLabelValues(scriptVersion).Set(0.00)

	// If no agrument is presented when the code is run.
	if ConfigFileExist == "all" {
		writeLogFile.Println("swift_export_config.yaml is NOT detected")
		SelectedModule.ReadReconFileEnable = defaultConfig["ReadReconFile"]
		SelectedModule.GrabSwiftPartitionEnable = defaultConfig["GrabSwiftPartition"]
		SelectedModule.SwiftDiskUsageEnable = defaultConfig["SwiftDiskUsage"]
		SelectedModule.SwiftDriveIOEnable = defaultConfig["SwiftDriveIO"]
		SelectedModule.GatherReplicationEstimateEnable = defaultConfig["GatherReplicationEstimate"]
		SelectedModule.GatherStoragePolicyUtilizationEnable = defaultConfig["GatherStoragePolicyUtilization"]
		SelectedModule.CheckObjectServerConnectionEnable = defaultConfig["CheckObjectServerConnection"]
		SelectedModule.ExposePerCPUUsageEnable = defaultConfig["ExposePerCPUUsage"]
		SelectedModule.ExposePerNICMetricEnable = defaultConfig["ExposePerNICMetric"]
		SanityCheckOnFiles(SelectedModule)
	} else if _, err := os.Stat(ConfigFileExist); err == nil { // To check if a file exists, equivalent to Python's if os.path.exists(filename):
		writeLogFile.Println("swift_export_config.yaml is detected")
		SelectedModule = TurnOnModules(ConfigFileExist)
		SanityCheckOnFiles(SelectedModule)
	}

	// Declare Go routines below so that we can grab the metrics and expose them to the
	// prometheus HTTP server periodically.
	// Fixed issue #6 in gitlab
	// Reference: https://gobyexample.com/goroutines
	// Reference2: https://github.com/prometheus/client_golang/blob/master/examples/random/main.go
	go func() {
		for {
			exporter.ReadReconFile(accountReconFile, "account", SelectedModule.ReadReconFileEnable)
			exporter.ReadReconFile(containerReconFile, "container", SelectedModule.ReadReconFileEnable)
			exporter.ReadReconFile(objectReconFile, "object", SelectedModule.ReadReconFileEnable)
			exporter.GrabSwiftPartition(replicationProgressFile, SelectedModule.GrabSwiftPartitionEnable)
			exporter.SwiftDiskUsage(SelectedModule.SwiftDiskUsageEnable)
			exporter.SwiftDriveIO(SelectedModule.SwiftDriveIOEnable)
			exporter.CheckObjectServerConnection(SelectedModule.CheckObjectServerConnectionEnable)
			exporter.ExposePerCPUUsage(SelectedModule.ExposePerCPUUsageEnable)
			exporter.ExposePerNICMetric(SelectedModule.ExposePerNICMetricEnable)
			exporter.GrabNICMTU()

			// Setting time to sleep for 1 Minute. If you need to set it to milliseconds, change the
			// "time.Minute" to "time.Millisecond"
			// Reference: https://golang.org/pkg/time/#Sleep
			time.Sleep(1 * time.Minute)
		}
	}()

	// the following go routine will be run every 5 minutes
	go func() {
		for {
			//GatherReplicationEstimate(swiftLog, timeLastRun, SelectedModule.GatherReplicationEstimateEnable)
			exporter.CheckSwiftService()
			time.Sleep(5 * time.Minute)
		}
	}()

	// the following go routine will be run every hour.
	go func() {
		for {
			exporter.RunSMARTCTL()
			time.Sleep(1 * time.Hour)
		}
	}()

	// the following go routine will be run every 3 hours.
	go func() {
		for {
			exporter.CheckSwiftLogSize(swiftLog)
			exporter.CountFilesPerSwiftDrive()
			time.Sleep(3 * time.Hour)
		}
	}()

	// the following go routine will be run every 6 hours.
	go func() {
		for {
			exporter.GatherStoragePolicyUtilization(SelectedModule.GatherStoragePolicyUtilizationEnable)
			time.Sleep(6 * time.Hour)
		}
	}()
	// Call the promhttp method in Prometheus to expose the data for Prometheus to grab.
	flag.Parse()
	http.Handle("/metrics", promhttp.Handler())
	writeLogFile.Fatal(http.ListenAndServe(*addr, nil))
}
