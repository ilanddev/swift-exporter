package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/ilanddev/swift-exporter/exporter"

	"github.com/docopt/docopt-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/yaml.v2"
)

// Config holds the configuration settings from the swift_exporter.yml file.
type Config struct {
	CheckObjectServerConnectionEnable    bool   `yaml:"CheckObjectServerConnection"`
	GrabSwiftPartitionEnable             bool   `yaml:"GrabSwiftPartition"`
	GatherReplicationEstimateEnable      bool   `yaml:"GatherReplicationEstimate"`
	GatherStoragePolicyUtilizationEnable bool   `yaml:"GatherStoragePolicyUtilization"`
	ExposePerCPUUsageEnable              bool   `yaml:"ExposePerCPUUsage"`
	ExposePerNICMetricEnable             bool   `yaml:"ExposePerNICMetric"`
	ReadReconFileEnable                  bool   `yaml:"ReadReconFile"`
	SwiftDiskUsageEnable                 bool   `yaml:"SwiftDiskUsage"`
	SwiftDriveIOEnable                   bool   `yaml:"SwiftDriveIO"`
	SwiftLogFile                         string `yaml:"SwiftLogFile"`
	SwiftConfigFile                      string `yaml:"SwiftConfigFile"`
	ReplicationProgressFile              string `yaml:"ReplicationProgressFile"`
	ObjectReconFile                      string `yaml:"ObjectReconFile"`
	ContainerReconFile                   string `yaml:"ContainerReconFile"`
	AccountReconFile                     string `yaml:"AccountReconFile"`
}

/*
This var() section sets the port which promohttp (Promethes HTTP server) uses.
In addition, accountServer, containerServer, and objectServer initializes gauge-type prometheus
metrics data.
*/
var (
	scriptVersion                           = "0.8.5"
	timeLastRun                             = "00:00:00"
	swiftExporterLogFile					= "/var/log/swift_exporter.log"
	swiftExporterLog, swiftExporterLogError = os.OpenFile(swiftExporterLogFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	addr                                    = flag.String("listen-address", ":53167", "The addres to listen on for HTTP requests.")
	abScriptVersionPara                     = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ac_script_version",
		Help: "swift_exporter version 0.8.5",
	}, []string{"script_version"})

	config = Config{
		ReadReconFileEnable:                  true,
		GrabSwiftPartitionEnable:             true,
		SwiftDiskUsageEnable:                 true,
		SwiftDriveIOEnable:                   true,
		GatherReplicationEstimateEnable:      true,
		GatherStoragePolicyUtilizationEnable: true,
		CheckObjectServerConnectionEnable:    true,
		ExposePerCPUUsageEnable:              true,
		ExposePerNICMetricEnable:             true,
		SwiftLogFile:                         "/var/log/swift/all.log",
		SwiftConfigFile:                      "/etc/swift/swift.conf",
		ReplicationProgressFile:              "/opt/ss/var/lib/replication_progress.json",
		ObjectReconFile:                      "/var/cache/swift/object.recon",
		ContainerReconFile:                   "/var/cache/swift/container.recon",
		AccountReconFile:                     "/var/cache/swift/account.recon",
	}
	argv  []string
	Usage = `Usage:
	   /opt/ss/bin/swift_exporter 
	   /opt/ss/bin/swift_exporter [<swift_export_config_file>]
	   /opt/ss/bin/swift_exporter --help | --version `
)

// Metrics have to be registeered to be expose, so this is done below.
func init() {
	prometheus.MustRegister(abScriptVersionPara)
	if swiftExporterLogError != nil {
		fmt.Printf("Error Opening File '%s': %v\n", swiftExporterLogFile, swiftExporterLogError)
	}
}

// SanityCheckOnFiles checks is a function being called in
func SanityCheckOnFiles() {

	writeLogFile := log.New(swiftExporterLog, "SanityCheckOnFiles: ", log.Ldate|log.Ltime|log.Lshortfile)

	if _, swiftConfigErr := os.Stat(config.SwiftConfigFile); os.IsNotExist(swiftConfigErr) {
		writeLogFile.Printf("%s does not exist! Exiting this script!\n", config.SwiftConfigFile)
		os.Exit(1)
	} else {
		writeLogFile.Println("Swift config file (swift.conf) exist. Continue checking other files")
		writeLogFile.Println("Checking if *.recon (default /var/cache/swift/*recon) file exist...")
		if config.ReadReconFileEnable {
			writeLogFile.Println("Script is set to expose data collected from /var/cache/swift/*.recon files (ReadReconFile module enable). Check to see if those file exist")
			if _, err := os.Stat(config.AccountReconFile); err == nil {
				writeLogFile.Println(" ===> account.recon file exists. Moving on to check if container.recon file exists...")
			} else {
				writeLogFile.Printf(" ===> %s file does not exist. We will need all 3 (account, container, object) recon files for this module to work, but you have enable the ReadReconFile module. Turning it off...\n", config.AccountReconFile)
				config.ReadReconFileEnable = false
			}
			if _, err := os.Stat(config.ContainerReconFile); err == nil {
				writeLogFile.Println(" ===> container.recon file exists. Moving on to check if container.recon file exists...")
			} else {
				writeLogFile.Printf(" ===> %s file does not exist. We will need all 3 (account, container, object) recon files for this module to work, but you have enable the ReadReconFile module. Turning it off...\n", config.ContainerReconFile)
				config.ReadReconFileEnable = false
			}
			if _, err := os.Stat(config.ObjectReconFile); err == nil {
				writeLogFile.Println(" ===> object.recon file exists. Moving on to check if object.recon file exists")
			} else {
				writeLogFile.Printf(" ===> %s file does not exist. We will need all 3 (account, container, object) recon files for this module to work, but you have enable the ReadReconFile module. Turning it off...\n", config.ObjectReconFile)
				config.ReadReconFileEnable = false
			}
			writeLogFile.Println("===> account.recon, container.recon, and object.recon file exist. Check for this module has completed. Enable this module...")
			config.ReadReconFileEnable = true
			writeLogFile.Println()
		} else {
			writeLogFile.Println("ReadReconFile module is disabled. Skip this check.")
			writeLogFile.Println()
		}
		if config.GrabSwiftPartitionEnable {
			writeLogFile.Printf("Script is set to expose data collected from %s (GrabSwiftPartition module enable). Check to see if that file exist...\n", config.ReplicationProgressFile)
			if _, err := os.Stat(config.ReplicationProgressFile); err == nil {
				log.Printf("===> %s exists. Check for this module has completed. Enable the module...\n", config.ReplicationProgressFile)
				config.GrabSwiftPartitionEnable = true
				writeLogFile.Println()
			} else {
				writeLogFile.Printf("===> %s does not exists, but you have enabled it. Disable the module...\n", config.ReplicationProgressFile)
				config.GrabSwiftPartitionEnable = false
				writeLogFile.Println()
			}
		} else {
			writeLogFile.Println("GrabSwiftPartition module is disabled. Skip this check.")
		}
		if config.GatherReplicationEstimateEnable {
			writeLogFile.Printf("Script is set to expose data collected from %s (GatherReplicationEstimate module enable). Check to see if that file exist...\n", config.SwiftLogFile)
			if _, err := os.Stat(config.SwiftLogFile); err == nil {
				writeLogFile.Printf("===> %s exists. Check for this module has completed. Enable the module...\n", config.SwiftLogFile)
				config.GatherReplicationEstimateEnable = true
				writeLogFile.Println()
			} else {
				writeLogFile.Printf("===> %s does not exists, but you have enabled it. Disable the module...\n", config.SwiftLogFile)
				config.GatherReplicationEstimateEnable = false
				writeLogFile.Println()
			}
		} else {
			writeLogFile.Println("GatherReplicationEstimate module is disabled. Skip this check.")
			writeLogFile.Println()
		}
		if config.GatherStoragePolicyUtilizationEnable {
			writeLogFile.Println("GatherStoragePolicyUtilization module is enabled. Since there is no config, there is nothing to check.")
			writeLogFile.Println()
		} else {
			writeLogFile.Println("GatherStoragePolicyUtilization module is disabled. Skip this check.")
			writeLogFile.Println()
		}
		if config.ExposePerCPUUsageEnable {
			writeLogFile.Println("ExposePerCPUUsage module is enabled. Since there is no config, there is nothing to check.")
			writeLogFile.Println()
		} else {
			writeLogFile.Println("ExposePerCPUUsage module is disabled. Skip this check.")
			writeLogFile.Println()
		}
		if config.ExposePerNICMetricEnable {
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

//ParseConfigFile reads through the yaml file, turns on the modules available in this script, and parses other config options.
func ParseConfigFile(configFileLocation string) () {
	writeLogFile := log.New(swiftExporterLog, "TurnOnModules: ", log.Ldate|log.Ltime|log.Lshortfile)
	filename, _ := os.Open(configFileLocation)
	yamlFile, _ := ioutil.ReadAll(filename)
	err := yaml.Unmarshal(yamlFile, &config)
	// If yaml.Unmarshal cannot extra data and put into the map data structure, do the following:
	if err != nil {
		writeLogFile.Fatalf("cannot unmarshal %v", err)
		writeLogFile.Println(err)
	}
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

	// If no argument is presented when the code is run.
	if ConfigFileExist == "all" {
		writeLogFile.Println("swift_export_config.yaml is NOT detected")
		SanityCheckOnFiles()
	} else if _, err := os.Stat(ConfigFileExist); err == nil { // To check if a file exists, equivalent to Python's if os.path.exists(filename):
		writeLogFile.Println("swift_export_config.yaml is detected")
		ParseConfigFile(ConfigFileExist)
		SanityCheckOnFiles()
	}

	// Declare Go routines below so that we can grab the metrics and expose them to the
	// prometheus HTTP server periodically.
	// Fixed issue #6 in gitlab
	// Reference: https://gobyexample.com/goroutines
	// Reference2: https://github.com/prometheus/client_golang/blob/master/examples/random/main.go
	go func() {
		for {
			exporter.ReadReconFile(config.AccountReconFile, "account", config.ReadReconFileEnable)
			exporter.ReadReconFile(config.ContainerReconFile, "container", config.ReadReconFileEnable)
			exporter.ReadReconFile(config.ObjectReconFile, "object", config.ReadReconFileEnable)
			exporter.GrabSwiftPartition(config.ReplicationProgressFile, config.GrabSwiftPartitionEnable)
			exporter.SwiftDiskUsage(config.SwiftDiskUsageEnable)
			exporter.SwiftDriveIO(config.SwiftDriveIOEnable)
			exporter.CheckObjectServerConnection(config.CheckObjectServerConnectionEnable)
			exporter.ExposePerCPUUsage(config.ExposePerCPUUsageEnable)
			exporter.ExposePerNICMetric(config.ExposePerNICMetricEnable)
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
			exporter.CheckSwiftLogSize(config.SwiftLogFile)
			exporter.CountFilesPerSwiftDrive()
			time.Sleep(3 * time.Hour)
		}
	}()

	// the following go routine will be run every 6 hours.
	go func() {
		for {
			exporter.GatherStoragePolicyUtilization(config.GatherStoragePolicyUtilizationEnable)
			time.Sleep(6 * time.Hour)
		}
	}()
	// Call the promhttp method in Prometheus to expose the data for Prometheus to grab.
	flag.Parse()
	http.Handle("/metrics", promhttp.Handler())
	writeLogFile.Fatal(http.ListenAndServe(*addr, nil))
}
