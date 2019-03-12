package exporter

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/shirou/gopsutil/disk"
)

// location of .recon files use to track replicator/reconstructor/auditor process
const accountReconFile = "/var/cache/swift/account.recon"
const containerReconFile = "/var/cache/swift/container.recon"
const objectReconFile = "/var/cache/swift/object.recon"
const replicationProgressFile = "/opt/ss/var/lib/replication_progress.json"
const swiftConfig = "/etc/swift/swift.conf"

// AccountContainerSwiftRole defines the data structure for account role in Swift cluster
// This is also needed as unmarshal requires the exact name in the JSON object for it
// to work"
type AccountSwiftRole struct {
	AccountReplicator   ReplicationStats `json:"replication_stats"`
	AccountAuditsPassed float64          `json:"account_audits_passed"`
	AccountAuditsFailed float64          `json:"account_audits_failed"`
	PassCompleted       float64          `json:"account_auditor_pass_completed"`
	ReplicationTime     float64          `json:"replication_time"`
}

// ContainerSwfiftRole defines the data structure for container role in a Swift cluster.
type ContainerSwiftRole struct {
	ContainerAuditsPassed         float64                `json:"container_audits_passed"`
	ContainerAuditsFailed         float64                `json:"container_audits_failed"`
	ContainerAuditorPassCompleted float64                `json:"container_auditor_pass_completed"`
	ContainerReplicator           ReplicationStats       `json:"replication_stats"`
	ReplicationTime               float64                `json:"replication_time"`
	ShardingLast                  float64                `json:"sharding_last"`
	ShardingStats                 ContainerShardingStats `json:"sharding_stats"`
}

// ContainerShardingStats struct holds the data read the container sharding from ReadConfFile.
type ContainerShardingStats struct {
	Attempted   float64         `json:"attempted"`
	Deferred    float64         `json:"deffered"`
	Diff        float64         `json:"diff"`
	DiffCapped  float64         `json:"diff_capped"`
	Empty       float64         `json:"empty"`
	Failure     float64         `json:"failure"`
	Hashmatch   float64         `json:"hashmatch"`
	NoChange    float64         `json:"no_change"`
	RemoteMerge float64         `json:"remote_merge"`
	Remove      float64         `json:"remove"`
	Rsync       float64         `json:"rsync"`
	Sharding    ShardingElement `json:"sharding"`
}

// ContainerShardingStats struct holds the sub set of data read the container sharding from ReadConfFile.
type ShardingElement struct {
	AuditRoot          ShardingStats `json:"audit_root"`
	AuditShard         ShardingStats `json:"audit_shard"`
	Cleaved            ShardingStats `json:"cleaved"`
	Created            ShardingStats `json:"created"`
	Misplaced          ShardingStats `json:"misplaced"`
	Scanned            ShardingStats `json:"scanned"`
	ShardingCandidates ShardingStats `json:"sharding_candidates"`
	Visited            ShardingStats `json:"visitred"`
}

// ShardingStats holds subset of data from ShardingElement.
type ShardingStats struct {
	Attempted float64 `json:"attempted"`
	Success   float64 `json:"success"`
	Failure   float64 `json:"failure"`
	Found     float64 `json:"found"`
	Placed    float64 `json:"placed"`
	Unplaced  float64 `json:"unplaced"`
	MaxTime   float64 `json:"max_time"`
	MinTime   float64 `json:"min_time"`
	Top       float64 `json:"top"`
	Skipped   float64 `json:"skipped"`
	Completed float64 `json:"completed"`
}

// ObjectSwiftRole is a struct created to hold the values that you can find in object.recon files.
type ObjectSwiftRole struct {
	AsyncPending             float64                       `json:"async_pending"`
	ExpiredLastPass          float64                       `json:"expired_last_pass"`
	ObjectReplicatorStats    ReplicationStats              `json:"replication_stats"`
	ObjectAuditorStatsALL    ObjectAuditorStats            `json:"object_auditor_stats_ALL"`
	ObjectAuditorStatsZBF    ObjectAuditorStats            `json:"object_auditor_stats_ZBF"`
	ObjectExpirationPass     float64                       `json:"object_expiration_pass"`
	ObjectReconstructionLast float64                       `json:"object_reconstruction_last"`
	ObjectReconstructionTime float64                       `json:"object_reconstruction_time"`
	ObjectReplicationPerDisk map[string]ReplicationPerDisk `json:"object_replication_per_disk"`
	ObjectUpdaterSweep       float64                       `json:"object_updater_sweep"`
	ObjectReplicationLast    float64                       `json:"replication_last"`
	ObjectReplicationTime    float64                       `json:"replication_time"`
}

// ObjectAuditorStats contains parts of the sub-list of account/container metrics that you can find in a *.recon file.
type ObjectAuditorStats struct {
	AuditTime     float64 `json:"audit_time"`
	ByteProcessed float64 `json:"bytes_processed"`
	Errors        float64 `json:"errors"`
	Passes        float64 `json:"passes"`
	Quarantined   float64 `json:"quarantined"`
}

// ReplicationStats is a struct that holds a list of replication metrics that you can read from *.recon files.
type ReplicationStats struct {
	Attempted       float64 `json:"attempted"`
	Diff            float64 `json:"diff"`
	DiffCapped      float64 `json:"diff_capped"`
	Failure         float64 `json:"failure"`
	Hashmatch       float64 `json:"hashmatch"`
	NoChange        float64 `json:"no_change"`
	RemoteMerge     float64 `json:"remote_merge"`
	ReplicationTime float64 `json:"replication_time"`
	Rsync           float64 `json:"rsync"`
	Success         float64 `json:"success"`
	Time            float64 `json:"time"`
	TsRepl          float64 `json:"ts_repl"`
	StartTime       float64 `json:"start"`
}

// ReplicationPerDisk struct holds the data obtained from object replication worker in object.json file.
type ReplicationPerDisk struct {
	ObjectReplicationLast float64             `json:"replication_last"`
	ObjectReplicatorStats Replication216Stats `json:"replication_stats"`
	ReplicationTime       float64             `json:"replication_time"`
}

// Replication216Stats struct holds the Swift replication status data prior to v2.16
type Replication216Stats struct {
	Attempted   float64 `json:"attempted"`
	Failure     float64 `json:"failure"`
	Hashmatch   float64 `json:"hashmatch"`
	Remove      float64 `json:"remove"`
	Rsync       float64 `json:"rsync"`
	Success     float64 `json:"success"`
	SuffixCount float64 `json:"suffix_count"`
	SuffixHash  float64 `json:"suffix_hash"`
	SuffixSync  float64 `json:"suffix_sync"`
}

// SwiftPartition is a struct that holds the partition count (both primary and handoff) for account/container and object servers.
type SwiftPartition struct {
	AccountPartCount   PartCounts `json:"accounts"`
	ContainerPartCount PartCounts `json:"containers"`
	ObjectPartCount    PartCounts `json:"objects"`
}

// PartCounts is a struct that is a sub structure to hold that actual part counts used by SwiftPartition
type PartCounts struct {
	Primary float64 `json:"primary"`
	Handoff float64 `json:"handoff"`
}

var (
	//swiftExporterLog, swiftExporterLogError = os.OpenFile("/var/log/swift_exporter.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	swiftAccountServer = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_account_server",
		Help: "Account Server Metrics",
	}, []string{"service_name", "metrics_name"})
	swiftAccountDBCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "swift_account_db",
		Help: "Number of Account DBs",
	})
	swiftAccountDBPendingCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "swift_account_db_pending",
		Help: "Number of Pending Account DBs",
	})
	swiftContainerServer = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_container_server",
		Help: "Container Server Metrics",
	}, []string{"service_name", "metrics_name"})
	swiftContainerDBCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "swift_container_db",
		Help: "Number of Container DBs",
	})
	swiftContainerDBPendingCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "swift_container_db_pending",
		Help: "Number of Pending Container DBs",
	})
	swiftObjectServer = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_object_server",
		Help: "Object Server Metrics",
	}, []string{"service_name", "metrics_name"})
	swiftObjectFileCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "swift_object_file_count",
		Help: "Number of Object Files",
	})
	swiftObjectReplicationPerDisk = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_object_replication_per_disk",
		Help: "Swift Object Replication Per Disk Metrics",
	}, []string{"service_name", "metrics_type", "swift_disk"})
	swiftObjectReplicationEstimate = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_object_replication_estimate",
		Help: "Swift Object Server - Replication Estimate in seconds (s) and parts/second (/sec)",
	}, []string{"metrics_type"})
	swiftObjectReplicationPerDiskEstimate = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_object_replication_per_disk_estimate",
		Help: "Swift Object Server - Replication Per Disk Estimate in seconds(s) and parts/second (/sec)",
	}, []string{"metrics_type", "swift_disk"})
	swiftContainerReplicationEstimate = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_container_replication_estimate",
		Help: "Swift Container Server - Replicator Estimate in parts/second (/sec)",
	}, []string{"metrics_type"})
	swiftContainerSharding = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_container_sharding",
		Help: "Swift Container Sharding",
	}, []string{"metric_name", "parameter"})
	swiftAccountReplicationEstimate = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_account_replication_estimage",
		Help: "Swift Account Server - Replicatin Estimage in parts/second (/sec)",
	}, []string{"metric_type"})
	swiftDrivePrimaryParitions = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_drive_primary_partitions",
		Help: "Swift Drive Primary Partitions - the number of primary partition, no specific unit.",
	}, []string{"swift_drive_label", "storage_policy", "swift_role", "drive_type"})
	swiftDriveHandoffPartitions = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_drive_handoff_partitions",
		Help: "Swift Drive Handoff Partitions - the number of handoff partition, no specific unit.",
	}, []string{"swift_drive_label", "storage_policy", "swift_role", "drive_type"})
	swiftLogFileSize = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "swift_log_file_size",
		Help: "Size of swift all.log",
	})
)

func init() {
	prometheus.MustRegister(swiftAccountServer)
	prometheus.MustRegister(swiftContainerServer)
	prometheus.MustRegister(swiftObjectServer)
	prometheus.MustRegister(swiftObjectReplicationPerDisk)
	prometheus.MustRegister(swiftObjectReplicationEstimate)
	prometheus.MustRegister(swiftObjectReplicationPerDiskEstimate)
	prometheus.MustRegister(swiftContainerSharding)
	prometheus.MustRegister(swiftDrivePrimaryParitions)
	prometheus.MustRegister(swiftDriveHandoffPartitions)
	prometheus.MustRegister(swiftLogFileSize)
	prometheus.MustRegister(swiftStoragePolicyUsage)
	prometheus.MustRegister(swiftContainerReplicationEstimate)
	prometheus.MustRegister(swiftAccountReplicationEstimate)
	prometheus.MustRegister(swiftAccountDBCount)
	prometheus.MustRegister(swiftAccountDBPendingCount)
	prometheus.MustRegister(swiftContainerDBCount)
	prometheus.MustRegister(swiftContainerDBPendingCount)
	prometheus.MustRegister(swiftObjectFileCount)
}

// GatherStoragePolicyCommonName reads throught /etc/swift/swift.conf file to get the storage policy name.
// Once it gets the policy name, it will put them into a slice and then share with other modules that needs to
// relate the storage policy name with the policy number.
func GatherStoragePolicyCommonName() map[string]string {

// TODO: fix logging
//	writeLogFile := log.New(swiftExporterLog, "GatherStoragePolicyCommonName: ", log.Ldate|log.Ltime|log.Lshortfile)

	openFile, err := os.Open(swiftConfig)
	StoragePolicyName := make(map[string]string)

	if err != nil {
//		writeLogFile.Fatal(err)
	}
	defer openFile.Close()

	readFile := bufio.NewScanner(openFile)
	for readFile.Scan() {
		if strings.Contains(readFile.Text(), "storage-policy:") {
			storagePolicyIndex := strings.TrimRight(strings.Split(readFile.Text(), ":")[1], "]")
			readFile.Scan() // read the next line
			readStoragePolicyNameBuffer := readFile.Text()
			readStoragePolicyName := strings.Split(readStoragePolicyNameBuffer, "=")
			StoragePolicyName[storagePolicyIndex] = strings.TrimLeft(readStoragePolicyName[1], " ")
		}
	}
	return StoragePolicyName
}

// ReadReconFile parses the .recon files, put them into the struct defined above and expose them out
//in prometheus. This function takes in 2 argument - ReconFile is the const configured above that reflects
//the exact location of the .recon file in Swift nodes.
func ReadReconFile(ReconFile string, SwiftRole string, ReadReconFileEnable bool, SwiftVersion string) {

// TODO: fix logging, this should be enabled via flag
//	writeLogFile := log.New(swiftExporterLog, "ReadReconFile: ", log.Ldate|log.Ltime|log.Lshortfile)
	swiftVersion := strings.Split(SwiftVersion, ".")
	swiftMajorVersion, _ := strconv.ParseInt(swiftVersion[0], 10, 64)
	swiftMinorVersion, _ := strconv.ParseInt(swiftVersion[1], 10, 64)

	if ReadReconFileEnable {
//		writeLogFile.Println("ReadReconFile Module ENABLED")
		jsonFile, err := os.Open(ReconFile)
		if err != nil {
			log.Println(err)
		}

		defer jsonFile.Close()

		byteValue, _ := ioutil.ReadAll(jsonFile)

		if SwiftRole == "account" {
			var account AccountSwiftRole
			json.Unmarshal(byteValue, &account)
//			writeLogFile.Println(string(byteValue))
			//fmt.Println(&account)

			swiftAccountServer.WithLabelValues("auditor", "passed").Set(account.AccountAuditsPassed)
			swiftAccountServer.WithLabelValues("auditor", "failed").Set(account.AccountAuditsFailed)
			swiftAccountServer.WithLabelValues("auditor", "passed_completed").Set(account.PassCompleted)
			swiftAccountServer.WithLabelValues("replication", "remote_merge").Set(account.AccountReplicator.RemoteMerge)
			swiftAccountServer.WithLabelValues("replication", "diff").Set(account.AccountReplicator.Diff)
			swiftAccountServer.WithLabelValues("replication", "diff_capped").Set(account.AccountReplicator.DiffCapped)
			swiftAccountServer.WithLabelValues("replication", "no_change").Set(account.AccountReplicator.NoChange)
			swiftAccountServer.WithLabelValues("replication", "ts_repl").Set(account.AccountReplicator.TsRepl)
			swiftAccountServer.WithLabelValues("replication", "replication_time").Set(account.ReplicationTime)

			swiftAccountServer.WithLabelValues("replication", "rsync").Set(account.AccountReplicator.Rsync)
			swiftAccountServer.WithLabelValues("replication", "success").Set(account.AccountReplicator.Success)
			swiftAccountServer.WithLabelValues("replication", "failure").Set(account.AccountReplicator.Failure)
			swiftAccountServer.WithLabelValues("replication", "attempted").Set(account.AccountReplicator.Attempted)
			swiftAccountServer.WithLabelValues("replication", "hashmatch").Set(account.AccountReplicator.Hashmatch)

		}
		if SwiftRole == "container" {
			var container ContainerSwiftRole
			json.Unmarshal(byteValue, &container)
//			writeLogFile.Println(string(byteValue))

			swiftContainerServer.WithLabelValues("auditor", "passed").Set(container.ContainerAuditsPassed)
			swiftContainerServer.WithLabelValues("auditor", "failed").Set(container.ContainerAuditsFailed)
			swiftContainerServer.WithLabelValues("auditor", "passed_completed").Set(container.ContainerAuditorPassCompleted)
			swiftContainerServer.WithLabelValues("replication", "remote_merge").Set(container.ContainerReplicator.RemoteMerge)
			swiftContainerServer.WithLabelValues("replication", "diff").Set(container.ContainerReplicator.Diff)
			swiftContainerServer.WithLabelValues("replication", "diff_capped").Set(container.ContainerReplicator.DiffCapped)
			swiftContainerServer.WithLabelValues("replication", "no_change").Set(container.ContainerReplicator.NoChange)
			swiftContainerServer.WithLabelValues("replication", "ts_repl").Set(container.ContainerReplicator.TsRepl)
			swiftContainerServer.WithLabelValues("replication", "replication_time").Set(container.ReplicationTime)

			swiftContainerServer.WithLabelValues("replication", "rsync").Set(container.ContainerReplicator.Rsync)
			swiftContainerServer.WithLabelValues("replication", "success").Set(container.ContainerReplicator.Success)
			swiftContainerServer.WithLabelValues("replication", "failure").Set(container.ContainerReplicator.Failure)
			swiftContainerServer.WithLabelValues("replication", "attempted").Set(container.ContainerReplicator.Attempted)
			swiftContainerServer.WithLabelValues("replication", "hashmatch").Set(container.ContainerReplicator.Hashmatch)

			if swiftMajorVersion >= 2 {
				if swiftMinorVersion >= 18 {
					swiftContainerSharding.WithLabelValues("sharding_stats", "attempted").Set(container.ShardingStats.Attempted)
					swiftContainerSharding.WithLabelValues("sharding_stats", "deffered").Set(container.ShardingStats.Deferred)
					swiftContainerSharding.WithLabelValues("sharding_stats", "diff").Set(container.ShardingStats.Diff)
					swiftContainerSharding.WithLabelValues("sharding_stats", "diff_capped").Set(container.ShardingStats.DiffCapped)
					swiftContainerSharding.WithLabelValues("sharding_stats", "empty").Set(container.ShardingStats.Empty)
					swiftContainerSharding.WithLabelValues("sharding_stats", "failure").Set(container.ShardingStats.Failure)
					swiftContainerSharding.WithLabelValues("sharding_stats", "hashmatch").Set(container.ShardingStats.Hashmatch)
					swiftContainerSharding.WithLabelValues("sharding_stats", "no_change").Set(container.ShardingStats.NoChange)
					swiftContainerSharding.WithLabelValues("sharding_stats", "remote_merge").Set(container.ShardingStats.RemoteMerge)
					swiftContainerSharding.WithLabelValues("sharding_stats", "remove").Set(container.ShardingStats.Remove)
					swiftContainerSharding.WithLabelValues("sharding_stats", "rsync").Set(container.ShardingStats.Rsync)

					swiftContainerSharding.WithLabelValues("audit_root", "attempted").Set(container.ShardingStats.Sharding.AuditRoot.Attempted)
					swiftContainerSharding.WithLabelValues("audit_root", "failure").Set(container.ShardingStats.Sharding.AuditRoot.Failure)
					swiftContainerSharding.WithLabelValues("audit_root", "success").Set(container.ShardingStats.Sharding.AuditRoot.Success)
					swiftContainerSharding.WithLabelValues("audit_shard", "attempted").Set(container.ShardingStats.Sharding.AuditShard.Attempted)
					swiftContainerSharding.WithLabelValues("audit_shard", "failure").Set(container.ShardingStats.Sharding.AuditShard.Failure)
					swiftContainerSharding.WithLabelValues("audit_shard", "attempted").Set(container.ShardingStats.Sharding.AuditShard.Attempted)

					swiftContainerSharding.WithLabelValues("cleaved", "attempted").Set(container.ShardingStats.Sharding.Cleaved.Attempted)
					swiftContainerSharding.WithLabelValues("cleaved", "failure").Set(container.ShardingStats.Sharding.Cleaved.Failure)
					swiftContainerSharding.WithLabelValues("cleaved", "max_time").Set(container.ShardingStats.Sharding.Cleaved.MaxTime)
					swiftContainerSharding.WithLabelValues("cleaved", "min_time").Set(container.ShardingStats.Sharding.Cleaved.MinTime)
					swiftContainerSharding.WithLabelValues("cleaved", "success").Set(container.ShardingStats.Sharding.Cleaved.Success)

					swiftContainerSharding.WithLabelValues("created", "attempted").Set(container.ShardingStats.Sharding.Created.Attempted)
					swiftContainerSharding.WithLabelValues("created", "failure").Set(container.ShardingStats.Sharding.Created.Failure)
					swiftContainerSharding.WithLabelValues("created", "success").Set(container.ShardingStats.Sharding.Created.Success)

					swiftContainerSharding.WithLabelValues("misplaced", "attempted").Set(container.ShardingStats.Sharding.Misplaced.Attempted)
					swiftContainerSharding.WithLabelValues("misplaced", "failure").Set(container.ShardingStats.Sharding.Misplaced.Failure)
					swiftContainerSharding.WithLabelValues("misplaced", "found").Set(container.ShardingStats.Sharding.Misplaced.Found)
					swiftContainerSharding.WithLabelValues("misplaced", "max_time").Set(container.ShardingStats.Sharding.Misplaced.MaxTime)
					swiftContainerSharding.WithLabelValues("misplaced", "min_time").Set(container.ShardingStats.Sharding.Misplaced.MinTime)
					swiftContainerSharding.WithLabelValues("misplaced", "success").Set(container.ShardingStats.Sharding.Misplaced.Success)

					swiftContainerSharding.WithLabelValues("scanned", "attempted").Set(container.ShardingStats.Sharding.Scanned.Attempted)
					swiftContainerSharding.WithLabelValues("scanned", "failure").Set(container.ShardingStats.Sharding.Scanned.Failure)
					swiftContainerSharding.WithLabelValues("scanned", "found").Set(container.ShardingStats.Sharding.Scanned.Found)
					swiftContainerSharding.WithLabelValues("scanned", "max_time").Set(container.ShardingStats.Sharding.Scanned.MaxTime)
					swiftContainerSharding.WithLabelValues("scanned", "min_time").Set(container.ShardingStats.Sharding.Scanned.MinTime)
					swiftContainerSharding.WithLabelValues("scanned", "success").Set(container.ShardingStats.Sharding.Scanned.Success)

					swiftContainerSharding.WithLabelValues("sharding_candidates", "found").Set(container.ShardingStats.Sharding.ShardingCandidates.Found)

					swiftContainerSharding.WithLabelValues("visited", "attempted").Set(container.ShardingStats.Sharding.Visited.Attempted)
					swiftContainerSharding.WithLabelValues("visited", "completed").Set(container.ShardingStats.Sharding.Visited.Completed)
					swiftContainerSharding.WithLabelValues("visited", "failure").Set(container.ShardingStats.Sharding.Visited.Failure)
					swiftContainerSharding.WithLabelValues("visited", "skipped").Set(container.ShardingStats.Sharding.Visited.Skipped)
					swiftContainerSharding.WithLabelValues("visited", "success").Set(container.ShardingStats.Sharding.Visited.Success)
				} else {
					//writeLogFile.Println("You are runnig Swift version that does not have Container Sharding")
				}
			} else {
				//writeLogFile.Print("You are running a very old version ")
			}

		}
		if SwiftRole == "object" {

			var object ObjectSwiftRole
			json.Unmarshal(byteValue, &object)
			//writeLogFile.Println(string(byteValue))
			//fmt.Println(object)

			swiftObjectServer.WithLabelValues("server", "async_pending").Set(object.AsyncPending)
			swiftObjectServer.WithLabelValues("replication", "replication_time").Set(object.ObjectReplicationTime)
			swiftObjectServer.WithLabelValues("reconstructor", "object_reconstruction_time").Set(object.ObjectReconstructionTime)
			swiftObjectServer.WithLabelValues("server", "replication_last").Set(object.ObjectReplicationLast)

			swiftObjectServer.WithLabelValues("auditor_ALL", "audit_time").Set(object.ObjectAuditorStatsALL.AuditTime)
			swiftObjectServer.WithLabelValues("auditor_ALL", "byte_processed").Set(object.ObjectAuditorStatsALL.ByteProcessed)
			swiftObjectServer.WithLabelValues("auditor_ALL", "errors").Set(object.ObjectAuditorStatsALL.Errors)
			swiftObjectServer.WithLabelValues("auditor_ALL", "passes").Set(object.ObjectAuditorStatsALL.Passes)
			swiftObjectServer.WithLabelValues("auditor_ALL", "quarantined").Set(object.ObjectAuditorStatsALL.Quarantined)
			swiftObjectServer.WithLabelValues("auditor_ZBF", "audit_time").Set(object.ObjectAuditorStatsZBF.AuditTime)
			swiftObjectServer.WithLabelValues("auditor_ZBF", "byte_processed").Set(object.ObjectAuditorStatsZBF.ByteProcessed)
			swiftObjectServer.WithLabelValues("auditor_ZBF", "errors").Set(object.ObjectAuditorStatsZBF.Errors)
			swiftObjectServer.WithLabelValues("auditor_ZBF", "passes").Set(object.ObjectAuditorStatsZBF.Passes)

			// If Swift version running in the node is 2.18 or later, parse the replication per disk metrics.
			if swiftMajorVersion >= 2 {
				if swiftMinorVersion >= 18 {
					for swiftDrive := range object.ObjectReplicationPerDisk {
						swiftObjectReplicationPerDisk.WithLabelValues("replication_per_disk", "rsync", swiftDrive).Set(object.ObjectReplicationPerDisk[swiftDrive].ObjectReplicatorStats.Rsync)
						swiftObjectReplicationPerDisk.WithLabelValues("replication_per_disk", "success", swiftDrive).Set(object.ObjectReplicationPerDisk[swiftDrive].ObjectReplicatorStats.Success)
						swiftObjectReplicationPerDisk.WithLabelValues("replication_per_disk", "failure", swiftDrive).Set(object.ObjectReplicationPerDisk[swiftDrive].ObjectReplicatorStats.Failure)
						swiftObjectReplicationPerDisk.WithLabelValues("replication_per_disk", "attempted", swiftDrive).Set(object.ObjectReplicationPerDisk[swiftDrive].ObjectReplicatorStats.Attempted)
						swiftObjectReplicationPerDisk.WithLabelValues("replication_per_disk", "hashmatch", swiftDrive).Set(object.ObjectReplicationPerDisk[swiftDrive].ObjectReplicatorStats.Hashmatch)
						swiftObjectReplicationPerDisk.WithLabelValues("replication_per_disk", "remove", swiftDrive).Set(object.ObjectReplicationPerDisk[swiftDrive].ObjectReplicatorStats.Remove)
						swiftObjectReplicationPerDisk.WithLabelValues("replication_per_disk", "suffix_count", swiftDrive).Set(object.ObjectReplicationPerDisk[swiftDrive].ObjectReplicatorStats.SuffixCount)
						swiftObjectReplicationPerDisk.WithLabelValues("replication_per_disk", "suffix_hash", swiftDrive).Set(object.ObjectReplicationPerDisk[swiftDrive].ObjectReplicatorStats.SuffixHash)
						swiftObjectReplicationPerDisk.WithLabelValues("replication_per_disk", "suffix_sync", swiftDrive).Set(object.ObjectReplicationPerDisk[swiftDrive].ObjectReplicatorStats.SuffixSync)
						swiftObjectReplicationPerDisk.WithLabelValues("replication_per_disk", "replication_last", swiftDrive).Set(object.ObjectReplicationPerDisk[swiftDrive].ObjectReplicationLast)
						swiftObjectReplicationPerDisk.WithLabelValues("replication_per_disk", "replication_time", swiftDrive).Set(object.ObjectReplicationPerDisk[swiftDrive].ReplicationTime)

					}
				} else {
					//writeLogFile.Println("You are running a version prior to Swift 2.18. Per disk replication was not enabled at that point. https://github.com/openstack/swift/commit/c28004deb0e938a9a9532c9c2e0f3197b6e572cb")
				}
			} else {
				//writeLogFile.Println("You are running a very very old version of Swift")
			}
			swiftObjectServer.WithLabelValues("replication", "rsync").Set(object.ObjectReplicatorStats.Rsync)
			swiftObjectServer.WithLabelValues("replication", "success").Set(object.ObjectReplicatorStats.Success)
			swiftObjectServer.WithLabelValues("replication", "failure").Set(object.ObjectReplicatorStats.Failure)
			swiftObjectServer.WithLabelValues("replication", "attempted").Set(object.ObjectReplicatorStats.Attempted)
			swiftObjectServer.WithLabelValues("replication", "suffixes_checked").Set(object.ObjectReplicatorStats.Hashmatch)
			swiftObjectServer.WithLabelValues("replication", "start").Set(object.ObjectReplicatorStats.StartTime)
			swiftObjectServer.WithLabelValues("updater", "object_updater_sweep").Set(object.ObjectUpdaterSweep)

		}
	} else {
		//writeLogFile.Println("ReadReconFile Module DISABLED")
	}

}

// CheckSwiftLogSize Description: this function checks the size of Swift all.log at /var/log/swift/all.log and returns its size.
// Once the data is retrieved, we will put expose it over Prometheus.
func CheckSwiftLogSize(swiftLog string) {

// TODO: fix logging
//	writeLogFile := log.New(swiftExporterLog, "CheckSwiftLogSize: ", log.Ldate|log.Ltime|log.Lshortfile)

	swiftLogFileHandle, err := os.Open(swiftLog)
	if err != nil {
//		writeLogFile.Println("Cannot open this file. Exiting")
		os.Exit(1)
	}
	defer swiftLogFileHandle.Close()

	fileInfo, err := swiftLogFileHandle.Stat()
	swiftLogFileSize.Set(float64(fileInfo.Size()))
//	writeLogFile.Printf("Swift all.log Size: %f", float64(fileInfo.Size()))
}

// GatherStoragePolicyUtilization do a "du -s" across all Swift nodes ("/srv/node") and expose
// actual disk size through the Prometheus.
func GatherStoragePolicyUtilization(GatherStoragePolicyUtilizationEnable bool) {

// TODO: fix logging
//	writeLogFile := log.New(swiftExporterLog, "GatherStoragePolicyUtilization: ", log.Ldate|log.Ltime|log.Lshortfile)

	if GatherStoragePolicyUtilizationEnable {
		//writeLogFile.Println("GatherStoragePolicyUtilization Module ENABLED")
		storagePolicyNameList := GatherStoragePolicyCommonName()
		var storagePolicyName string

		// disk.Partition is from gopsutil library and it returns a structure of
		// PartitionStat - which contains the mountpoint information and others.
		swiftDrive, _ := disk.Partitions(false)

		for n := 0; n < len(swiftDrive); n++ {
			var storagePolicyList []string
			// pull the mountpoint from PartitionStat struct.
			driveLocation := swiftDrive[n].Mountpoint
			// add the drive location to the slice to join the complete swift drive path.
			storagePolicyList = append(storagePolicyList, driveLocation)
			if strings.Contains(driveLocation, "/srv/node") {
				// get the list of directories under "/srv/node", the output is a slice as well
				directories, _ := ioutil.ReadDir(driveLocation)
				// since the output of ioutil.ReadDir is a slice, so we will need to go through each
				// element and scan for any directories that has the name "objects"
				for _, f := range directories {
					if strings.Contains(f.Name(), "objects") {
						matchingStoragePolicy := strings.Split(f.Name(), "-")
						if len(matchingStoragePolicy) == 0 {
							fmt.Println("There is no storage policy. Exiting...")
							break
						} else if len(matchingStoragePolicy) == 1 {
							storagePolicyName = storagePolicyNameList["0"]
						} else {
							//indexNumber, _ := strconv.ParseInt(matchingStoragePolicy[1], 10, 64)
							indexNumber := matchingStoragePolicy[1]
							storagePolicyName = storagePolicyNameList[indexNumber]
						}
						storagePolicyList = append(storagePolicyList, f.Name())
						swiftDrivePath := strings.Join(storagePolicyList, "/")
						// run du -s command against the current swiftDrivePath (for example: /srv/node/d0/object/) to get the size.
						directorySize, _ := exec.Command("du", "-s", swiftDrivePath).Output()
						// split the output using tab as the delimiter. For example: "315160	/srv/node/d5/objects"
						usage := strings.Split(string(directorySize), "\t")
						// convert the outut to float64 from string
						usageFloat, _ := strconv.ParseFloat(usage[0], 64)
						// expose the data out to Prometheus
						swiftStoragePolicyUsage.WithLabelValues(driveLocation, f.Name(), storagePolicyName).Set(usageFloat)
						// Removing the last element from slice "storagePolicyList", to "reset" the slice. Otherwise,
						// data in this entry will be carried over to the next one. Causing error...
						storagePolicyList = storagePolicyList[:len(storagePolicyList)-1]
					}
				}
			}
		}
	} else {
		//writeLogFile.Println("GatherStoragePolicyUtilization Module DISABLED")
	}

}

// CountFilesPerSwiftDrive counts the number of file in each Swift partition in a Swift Drive.
func CountFilesPerSwiftDrive() {
	var accountsDB []string
	var accountsPendingDB []string
	var containersDB []string
	var containersPendingDB []string
	var objectFiles []string
	swiftDrivesRoot := "/srv/node/"

	err := filepath.Walk(swiftDrivesRoot, func(path string, info os.FileInfo, err error) error {
		if strings.Contains(path, "accounts") {
			if strings.Contains(path, ".db") {
				accountsDB = append(accountsDB, path)
			} else if strings.Contains(path, ".pending") {
				accountsPendingDB = append(accountsPendingDB, path)
			}
		} else if strings.Contains(path, "containers") {
			if strings.Contains(path, ".db") {
				containersDB = append(containersDB, path)
			} else if strings.Contains(path, ".pending") {
				containersPendingDB = append(containersPendingDB, path)
			}
		} else if strings.Contains(path, "objects") {
			if strings.Contains(path, ".data") {
				objectFiles = append(objectFiles, path)
			}
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	swiftAccountDBCount.Set(float64(len(accountsDB)))
	swiftAccountDBPendingCount.Set(float64(len(accountsPendingDB)))
	swiftContainerDBCount.Set(float64(len(containersDB)))
	swiftContainerDBPendingCount.Set(float64(len(containersPendingDB)))
	swiftObjectFileCount.Set(float64(len(objectFiles)))
}
