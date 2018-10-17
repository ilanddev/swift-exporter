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
	ObjectReplicationTime    float64                       `json:"object_replication_time"`
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
	accountServer = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "account_server",
		Help: "Account Server Metrics",
	}, []string{"service_name", "metrics_name", "FQDN", "UUID"})
	accountDBCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "account_db",
		Help: "Number of Account DBs",
	}, []string{"FQDN", "UUID"})
	accountDBPendingCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "account_db_pending",
		Help: "Number of Pending Account DBs",
	}, []string{"FQDN", "UUID"})
	containerServer = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "container_server",
		Help: "Container Server Metrics",
	}, []string{"service_name", "metrics_name", "FQDN", "UUID"})
	containerDBCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "container_db",
		Help: "Number of Container DBs",
	}, []string{"FQDN", "UUID"})
	containerDBPendingCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "container_db_pending",
		Help: "Number of Pending Container DBs",
	}, []string{"FQDN", "UUID"})
	objectServer = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "object_server",
		Help: "Object Server Metrics",
	}, []string{"service_name", "metrics_name", "FQDN", "UUID"})
	objectFileCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "object_file_count",
		Help: "Number of Object Files",
	}, []string{"FQDN", "UUID"})
	swiftObjectReplicationPerDisk = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_object_replication_per_disk",
		Help: "Swift Object Replication Per Disk Metrics",
	}, []string{"service_name", "metrics_type", "swift_disk", "FQDN", "UUID"})
	swiftObjectReplicationEstimate = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_object_replication_estimate",
		Help: "Swift Object Server - Replication Estimate in seconds (s) and parts/second (/sec)",
	}, []string{"metrics_type", "FQDN", "UUID"})
	swiftObjectReplicationPerDiskEstimate = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_object_replication_per_disk_estimate",
		Help: "Swift Object Server - Replication Per Disk Estimate in seconds(s) and parts/second (/sec)",
	}, []string{"metrics_type", "swift_disk", "FQDN", "UUID"})
	swiftContainerReplicationEstimate = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_container_replication_estimate",
		Help: "Swift Container Server - Replicator Estimate in parts/second (/sec)",
	}, []string{"metrics_type", "FQDN", "UUID"})
	swiftContainerSharding = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_container_sharding",
		Help: "Swift Container Sharding",
	}, []string{"metric_name", "parameter", "FQDN", "UUID"})
	swiftAccountReplicationEstimate = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_account_replication_estimage",
		Help: "Swift Account Server - Replicatin Estimage in parts/second (/sec)",
	}, []string{"metric_type", "FQDN", "UUID"})
	swiftDrivePrimaryParitions = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_drive_primary_partitions",
		Help: "Swift Drive Primary Partitions - the number of primary partition, no specific unit.",
	}, []string{"FQDN", "UUID", "swift_drive_label", "storage_policy", "swift_role", "drive_type"})
	swiftDriveHandoffPartitions = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_drive_handoff_partitions",
		Help: "Swift Drive Handoff Partitions - the number of handoff partition, no specific unit.",
	}, []string{"FQDN", "UUID", "swift_drive_label", "storage_policy", "swift_role", "drive_type"})
	swiftLogFileSize = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "swift_log_file_size",
		Help: "Size of swift all.log",
	})
	swiftServiceStatus = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_service_status",
		Help: "Swift Main Service Status - Services like 'Accouts', 'Containers', and 'Objects' are recorded here",
	}, []string{"FQDN", "UUID", "SwiftServiceName"})
	swiftSubServiceStatus = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_sub_service_status",
		Help: "Swift Sub Service Status - Services like 'Auditors', 'Replicator', and 'Expirer'...etc are recorded here",
	}, []string{"FQDN", "UUID", "SwiftSubServiceName"})
)

func init() {
	prometheus.MustRegister(accountServer)
	prometheus.MustRegister(containerServer)
	prometheus.MustRegister(objectServer)
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
	prometheus.MustRegister(accountDBCount)
	prometheus.MustRegister(accountDBPendingCount)
	prometheus.MustRegister(containerDBCount)
	prometheus.MustRegister(containerDBPendingCount)
	prometheus.MustRegister(objectFileCount)
	prometheus.MustRegister(swiftServiceStatus)
	prometheus.MustRegister(swiftSubServiceStatus)
}

// GatherStoragePolicyCommonName reads throught /etc/swift/swift.conf file to get the storage policy name.
// Once it gets the policy name, it will put them into a slice and then share with other modules that needs to
// relate the storage policy name with the policy number.
func GatherStoragePolicyCommonName() map[string]string {

	writeLogFile := log.New(swiftExporterLog, "GatherStoragePolicyCommonName: ", log.Ldate|log.Ltime|log.Lshortfile)

	openFile, err := os.Open(swiftConfig)
	StoragePolicyName := make(map[string]string)

	if err != nil {
		writeLogFile.Fatal(err)
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
			//StoragePolicyName = append(StoragePolicyName, strings.TrimLeft(readStoragePolicyName[1], " "))
		}
	}
	return StoragePolicyName
}

// ReadReconFile parses the .recon files, put them into the struct defined above and expose them out
//in prometheus. This function takes in 2 argument - ReconFile is the const configured above that reflects
//the exact location of the .recon file in Swift nodes.
func ReadReconFile(ReconFile string, SwiftRole string, ReadReconFileEnable bool) {

	writeLogFile := log.New(swiftExporterLog, "ReadReconFile: ", log.Ldate|log.Ltime|log.Lshortfile)
	hostFQDN, hostUUID, _ := GetUUIDAndFQDN(ssnodeConfFile)
	swiftParameter := GetSwiftEnvironmentParameters()
	swiftVersion := strings.Split(swiftParameter.Swift.Version, ".")
	swiftMajorVersion, _ := strconv.ParseInt(swiftVersion[0], 10, 64)
	swiftMinorVersion, _ := strconv.ParseInt(swiftVersion[1], 10, 64)

	if ReadReconFileEnable {
		writeLogFile.Println("ReadReconFile Module ENABLED")
		jsonFile, err := os.Open(ReconFile)
		if err != nil {
			log.Println(err)
		}

		defer jsonFile.Close()

		byteValue, _ := ioutil.ReadAll(jsonFile)

		if SwiftRole == "account" {
			var account AccountSwiftRole
			json.Unmarshal(byteValue, &account)
			writeLogFile.Println(string(byteValue))
			//fmt.Println(&account)

			accountServer.WithLabelValues("auditor", "passed", hostFQDN, hostUUID).Set(account.AccountAuditsPassed)
			accountServer.WithLabelValues("auditor", "failed", hostFQDN, hostUUID).Set(account.AccountAuditsFailed)
			accountServer.WithLabelValues("auditor", "passed_completed", hostFQDN, hostUUID).Set(account.PassCompleted)
			accountServer.WithLabelValues("replicator", "remote_merge", hostFQDN, hostUUID).Set(account.AccountReplicator.RemoteMerge)
			accountServer.WithLabelValues("replicator", "diff", hostFQDN, hostUUID).Set(account.AccountReplicator.Diff)
			accountServer.WithLabelValues("replicator", "diff_capped", hostFQDN, hostUUID).Set(account.AccountReplicator.DiffCapped)
			accountServer.WithLabelValues("replicator", "no_change", hostFQDN, hostUUID).Set(account.AccountReplicator.NoChange)
			accountServer.WithLabelValues("replicator", "ts_repl", hostFQDN, hostUUID).Set(account.AccountReplicator.TsRepl)
			accountServer.WithLabelValues("replicator", "replication_time", hostFQDN, hostUUID).Set(account.ReplicationTime)

			accountServer.WithLabelValues("replicator", "rsync", hostFQDN, hostUUID).Set(account.AccountReplicator.Rsync)
			accountServer.WithLabelValues("replicator", "success", hostFQDN, hostUUID).Set(account.AccountReplicator.Success)
			accountServer.WithLabelValues("replicator", "failure", hostFQDN, hostUUID).Set(account.AccountReplicator.Failure)
			accountServer.WithLabelValues("replicator", "attempted", hostFQDN, hostUUID).Set(account.AccountReplicator.Attempted)
			accountServer.WithLabelValues("replicator", "hashmatch", hostFQDN, hostUUID).Set(account.AccountReplicator.Hashmatch)

			accountReplicationPartsPerSecond := account.AccountReplicator.Attempted / account.ReplicationTime
			swiftAccountReplicationEstimate.WithLabelValues("parts_per_second", hostFQDN, hostUUID).Set(accountReplicationPartsPerSecond)
		}
		if SwiftRole == "container" {
			var container ContainerSwiftRole
			json.Unmarshal(byteValue, &container)
			writeLogFile.Println(string(byteValue))

			containerServer.WithLabelValues("auditor", "passed", hostFQDN, hostUUID).Set(container.ContainerAuditsPassed)
			containerServer.WithLabelValues("auditor", "failed", hostFQDN, hostUUID).Set(container.ContainerAuditsFailed)
			containerServer.WithLabelValues("auditor", "passed_completed", hostFQDN, hostUUID).Set(container.ContainerAuditorPassCompleted)
			containerServer.WithLabelValues("replicator", "remote_merge", hostFQDN, hostUUID).Set(container.ContainerReplicator.RemoteMerge)
			containerServer.WithLabelValues("replicator", "diff", hostFQDN, hostUUID).Set(container.ContainerReplicator.Diff)
			containerServer.WithLabelValues("replicator", "diff_capped", hostFQDN, hostUUID).Set(container.ContainerReplicator.DiffCapped)
			containerServer.WithLabelValues("replicator", "no_change", hostFQDN, hostUUID).Set(container.ContainerReplicator.NoChange)
			containerServer.WithLabelValues("replicator", "ts_repl", hostFQDN, hostUUID).Set(container.ContainerReplicator.TsRepl)
			containerServer.WithLabelValues("replicator", "replication_time", hostFQDN, hostUUID).Set(container.ReplicationTime)

			containerServer.WithLabelValues("replicator", "rsync", hostFQDN, hostUUID).Set(container.ContainerReplicator.Rsync)
			containerServer.WithLabelValues("replicator", "success", hostFQDN, hostUUID).Set(container.ContainerReplicator.Success)
			containerServer.WithLabelValues("replicator", "failure", hostFQDN, hostUUID).Set(container.ContainerReplicator.Failure)
			containerServer.WithLabelValues("replicator", "attempted", hostFQDN, hostUUID).Set(container.ContainerReplicator.Attempted)
			containerServer.WithLabelValues("replicator", "hashmatch", hostFQDN, hostUUID).Set(container.ContainerReplicator.Hashmatch)

			if swiftMajorVersion >= 2 {
				if swiftMinorVersion >= 15 {
					swiftContainerSharding.WithLabelValues("sharding_stats", "attempted", hostFQDN, hostUUID).Set(container.ShardingStats.Attempted)
					swiftContainerSharding.WithLabelValues("sharding_stats", "deffered", hostFQDN, hostUUID).Set(container.ShardingStats.Deferred)
					swiftContainerSharding.WithLabelValues("sharding_stats", "diff", hostFQDN, hostUUID).Set(container.ShardingStats.Diff)
					swiftContainerSharding.WithLabelValues("sharding_stats", "diff_capped", hostFQDN, hostUUID).Set(container.ShardingStats.DiffCapped)
					swiftContainerSharding.WithLabelValues("sharding_stats", "empty", hostFQDN, hostUUID).Set(container.ShardingStats.Empty)
					swiftContainerSharding.WithLabelValues("sharding_stats", "failure", hostFQDN, hostUUID).Set(container.ShardingStats.Failure)
					swiftContainerSharding.WithLabelValues("sharding_stats", "hashmatch", hostFQDN, hostUUID).Set(container.ShardingStats.Hashmatch)
					swiftContainerSharding.WithLabelValues("sharding_stats", "no_change", hostFQDN, hostUUID).Set(container.ShardingStats.NoChange)
					swiftContainerSharding.WithLabelValues("sharding_stats", "remote_merge", hostFQDN, hostUUID).Set(container.ShardingStats.RemoteMerge)
					swiftContainerSharding.WithLabelValues("sharding_stats", "remove", hostFQDN, hostUUID).Set(container.ShardingStats.Remove)
					swiftContainerSharding.WithLabelValues("sharding_stats", "rsync", hostFQDN, hostUUID).Set(container.ShardingStats.Rsync)

					swiftContainerSharding.WithLabelValues("audit_root", "attempted", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.AuditRoot.Attempted)
					swiftContainerSharding.WithLabelValues("audit_root", "failure", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.AuditRoot.Failure)
					swiftContainerSharding.WithLabelValues("audit_root", "success", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.AuditRoot.Success)
					swiftContainerSharding.WithLabelValues("audit_shard", "attempted", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.AuditShard.Attempted)
					swiftContainerSharding.WithLabelValues("audit_shard", "failure", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.AuditShard.Failure)
					swiftContainerSharding.WithLabelValues("audit_shard", "attempted", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.AuditShard.Attempted)

					swiftContainerSharding.WithLabelValues("cleaved", "attempted", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.Cleaved.Attempted)
					swiftContainerSharding.WithLabelValues("cleaved", "failure", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.Cleaved.Failure)
					swiftContainerSharding.WithLabelValues("cleaved", "max_time", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.Cleaved.MaxTime)
					swiftContainerSharding.WithLabelValues("cleaved", "min_time", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.Cleaved.MinTime)
					swiftContainerSharding.WithLabelValues("cleaved", "success", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.Cleaved.Success)

					swiftContainerSharding.WithLabelValues("created", "attempted", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.Created.Attempted)
					swiftContainerSharding.WithLabelValues("created", "failure", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.Created.Failure)
					swiftContainerSharding.WithLabelValues("created", "success", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.Created.Success)

					swiftContainerSharding.WithLabelValues("misplaced", "attempted", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.Misplaced.Attempted)
					swiftContainerSharding.WithLabelValues("misplaced", "failure", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.Misplaced.Failure)
					swiftContainerSharding.WithLabelValues("misplaced", "found", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.Misplaced.Found)
					swiftContainerSharding.WithLabelValues("misplaced", "max_time", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.Misplaced.MaxTime)
					swiftContainerSharding.WithLabelValues("misplaced", "min_time", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.Misplaced.MinTime)
					swiftContainerSharding.WithLabelValues("misplaced", "success", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.Misplaced.Success)

					swiftContainerSharding.WithLabelValues("scanned", "attempted", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.Scanned.Attempted)
					swiftContainerSharding.WithLabelValues("scanned", "failure", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.Scanned.Failure)
					swiftContainerSharding.WithLabelValues("scanned", "found", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.Scanned.Found)
					swiftContainerSharding.WithLabelValues("scanned", "max_time", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.Scanned.MaxTime)
					swiftContainerSharding.WithLabelValues("scanned", "min_time", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.Scanned.MinTime)
					swiftContainerSharding.WithLabelValues("scanned", "success", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.Scanned.Success)

					swiftContainerSharding.WithLabelValues("sharding_candidates", "found", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.ShardingCandidates.Found)

					swiftContainerSharding.WithLabelValues("visited", "attempted", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.Visited.Attempted)
					swiftContainerSharding.WithLabelValues("visited", "completed", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.Visited.Completed)
					swiftContainerSharding.WithLabelValues("visited", "failure", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.Visited.Failure)
					swiftContainerSharding.WithLabelValues("visited", "skipped", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.Visited.Skipped)
					swiftContainerSharding.WithLabelValues("visited", "success", hostFQDN, hostUUID).Set(container.ShardingStats.Sharding.Visited.Success)
				} else {
					writeLogFile.Println("You are runnig Swift version that does not have Container Sharding")
				}
			} else {
				writeLogFile.Print("You are running a very old version ")
			}

			containerReplicationPartsPerSecond := container.ContainerReplicator.Attempted / container.ReplicationTime
			swiftContainerReplicationEstimate.WithLabelValues("parts_per_second", hostFQDN, hostUUID).Set(containerReplicationPartsPerSecond)
		}
		if SwiftRole == "object" {

			var object ObjectSwiftRole
			json.Unmarshal(byteValue, &object)
			writeLogFile.Println(string(byteValue))
			//fmt.Println(object)

			objectServer.WithLabelValues("server", "async_pending", hostFQDN, hostUUID).Set(object.AsyncPending)
			objectServer.WithLabelValues("replicator", "object_replication_time", hostFQDN, hostUUID).Set(object.ObjectReplicationTime)
			objectServer.WithLabelValues("reconstructor", "object_reconstruction_time", hostFQDN, hostUUID).Set(object.ObjectReconstructionTime)
			objectServer.WithLabelValues("server", "replication_last", hostFQDN, hostUUID).Set(object.ObjectReplicationLast)

			objectServer.WithLabelValues("auditor_ALL", "audit_time", hostFQDN, hostUUID).Set(object.ObjectAuditorStatsALL.AuditTime)
			objectServer.WithLabelValues("auditor_ALL", "byte_processed", hostFQDN, hostUUID).Set(object.ObjectAuditorStatsALL.ByteProcessed)
			objectServer.WithLabelValues("auditor_ALL", "errors", hostFQDN, hostUUID).Set(object.ObjectAuditorStatsALL.Errors)
			objectServer.WithLabelValues("auditor_ALL", "passes", hostFQDN, hostUUID).Set(object.ObjectAuditorStatsALL.Passes)
			objectServer.WithLabelValues("auditor_ALL", "quarantined", hostFQDN, hostUUID).Set(object.ObjectAuditorStatsALL.Quarantined)
			objectServer.WithLabelValues("auditor_ZBF", "audit_time", hostFQDN, hostUUID).Set(object.ObjectAuditorStatsZBF.AuditTime)
			objectServer.WithLabelValues("auditor_ZBF", "byte_processed", hostFQDN, hostUUID).Set(object.ObjectAuditorStatsZBF.ByteProcessed)
			objectServer.WithLabelValues("auditor_ZBF", "errors", hostFQDN, hostUUID).Set(object.ObjectAuditorStatsZBF.Errors)
			objectServer.WithLabelValues("auditor_ZBF", "passes", hostFQDN, hostUUID).Set(object.ObjectAuditorStatsZBF.Passes)

			// If Swift version running in the node is 2.15 or after, go ahead and parse the replication per disk metrics.
			if swiftMajorVersion >= 2 {
				if swiftMinorVersion >= 15 {
					for swiftDrive := range object.ObjectReplicationPerDisk {
						swiftObjectReplicationPerDisk.WithLabelValues("replicator_per_disk", "rsync", swiftDrive, hostFQDN, hostUUID).Set(object.ObjectReplicationPerDisk[swiftDrive].ObjectReplicatorStats.Rsync)
						swiftObjectReplicationPerDisk.WithLabelValues("replicator_per_disk", "success", swiftDrive, hostFQDN, hostUUID).Set(object.ObjectReplicationPerDisk[swiftDrive].ObjectReplicatorStats.Success)
						swiftObjectReplicationPerDisk.WithLabelValues("replicator_per_disk", "failure", swiftDrive, hostFQDN, hostUUID).Set(object.ObjectReplicationPerDisk[swiftDrive].ObjectReplicatorStats.Failure)
						swiftObjectReplicationPerDisk.WithLabelValues("replicator_per_disk", "attempted", swiftDrive, hostFQDN, hostUUID).Set(object.ObjectReplicationPerDisk[swiftDrive].ObjectReplicatorStats.Attempted)
						swiftObjectReplicationPerDisk.WithLabelValues("replicator_per_disk", "hashmatch", swiftDrive, hostFQDN, hostUUID).Set(object.ObjectReplicationPerDisk[swiftDrive].ObjectReplicatorStats.Hashmatch)
						swiftObjectReplicationPerDisk.WithLabelValues("replicator_per_disk", "remove", swiftDrive, hostFQDN, hostUUID).Set(object.ObjectReplicationPerDisk[swiftDrive].ObjectReplicatorStats.Remove)
						swiftObjectReplicationPerDisk.WithLabelValues("replicator_per_disk", "suffix_count", swiftDrive, hostFQDN, hostUUID).Set(object.ObjectReplicationPerDisk[swiftDrive].ObjectReplicatorStats.SuffixCount)
						swiftObjectReplicationPerDisk.WithLabelValues("replicator_per_disk", "suffix_hash", swiftDrive, hostFQDN, hostUUID).Set(object.ObjectReplicationPerDisk[swiftDrive].ObjectReplicatorStats.SuffixHash)
						swiftObjectReplicationPerDisk.WithLabelValues("replicator_per_disk", "suffix_sync", swiftDrive, hostFQDN, hostUUID).Set(object.ObjectReplicationPerDisk[swiftDrive].ObjectReplicatorStats.SuffixSync)
						swiftObjectReplicationPerDisk.WithLabelValues("replicator_per_disk", "replication_last", swiftDrive, hostFQDN, hostUUID).Set(object.ObjectReplicationPerDisk[swiftDrive].ObjectReplicationLast)
						swiftObjectReplicationPerDisk.WithLabelValues("replication_per_disk", "replication_time", swiftDrive, hostFQDN, hostUUID).Set(object.ObjectReplicationPerDisk[swiftDrive].ReplicationTime)

						partitionReplicatedPerDisk := object.ObjectReplicationPerDisk[swiftDrive].ObjectReplicatorStats.Attempted
						replicationTimeUsedPerDisk := object.ObjectReplicationPerDisk[swiftDrive].ReplicationTime * 60
						replicationPartPerSecondPerDisk := partitionReplicatedPerDisk / replicationTimeUsedPerDisk

						swiftObjectReplicationPerDiskEstimate.WithLabelValues("parts_per_second_per_disk", swiftDrive, hostFQDN, hostUUID).Set(replicationPartPerSecondPerDisk)
						swiftObjectReplicationPerDiskEstimate.WithLabelValues("time_used_per_disk", swiftDrive, hostFQDN, hostUUID).Set(replicationTimeUsedPerDisk)
					}
				} else {
					writeLogFile.Println("You are running Swift 2.15 or below. This is not meant for you")
				}
			} else {
				writeLogFile.Println("You are running a very very old version of Swift")
			}
			objectServer.WithLabelValues("replicator", "rsync", hostFQDN, hostUUID).Set(object.ObjectReplicatorStats.Rsync)
			objectServer.WithLabelValues("replicator", "success", hostFQDN, hostUUID).Set(object.ObjectReplicatorStats.Success)
			objectServer.WithLabelValues("replicator", "failure", hostFQDN, hostUUID).Set(object.ObjectReplicatorStats.Failure)
			objectServer.WithLabelValues("replicator", "attempted", hostFQDN, hostUUID).Set(object.ObjectReplicatorStats.Attempted)
			objectServer.WithLabelValues("replicator", "suffixes_checked", hostFQDN, hostUUID).Set(object.ObjectReplicatorStats.Hashmatch)
			objectServer.WithLabelValues("replicator", "start", hostFQDN, hostUUID).Set(object.ObjectReplicatorStats.StartTime)
			objectServer.WithLabelValues("updater", "object_updater_sweep", hostFQDN, hostUUID).Set(object.ObjectUpdaterSweep)

			partitionReplicated := object.ObjectReplicatorStats.Attempted
			replicationTimeUsed := object.ObjectReplicationTime * 60
			replicationPartPerSecond := partitionReplicated / replicationTimeUsed

			swiftObjectReplicationEstimate.WithLabelValues("parts_per_second", hostFQDN, hostUUID).Set(replicationPartPerSecond)
			swiftObjectReplicationEstimate.WithLabelValues("time_used", hostFQDN, hostUUID).Set(replicationTimeUsed)

		}
	} else {
		writeLogFile.Println("ReadReconFile Module DISABLED")
	}

}

// GrabSwiftPartition reads the /opt/ss/var/lib/replication_progress.json file and gets the
// primary and handoff partition, then expose them to the prometheus.
func GrabSwiftPartition(replicationProgressFile string, GrabSwiftPartitionEnable bool) {

	writeLogFile := log.New(swiftExporterLog, "GrabSwiftPartition: ", log.Ldate|log.Ltime|log.Lshortfile)
	nodeHostname, nodeUUID, _ := GetUUIDAndFQDN(ssnodeConfFile) // getting node FQDN and UUID

	if GrabSwiftPartitionEnable {
		writeLogFile.Println("GrabSwiftPartition Module ENABLED")
		storagePolicyNameList := GatherStoragePolicyCommonName()
		var parts = make(map[string]map[string]PartCounts) // do NOT remove!!
		drivesAvailable, _ := disk.Partitions(false)       // List out all the drives detected in OS.

		jsonFile, err := os.Open(replicationProgressFile)
		if err != nil {
			writeLogFile.Println(err)
		}
		defer jsonFile.Close()
		byteValue, _ := ioutil.ReadAll(jsonFile)
		err = json.Unmarshal(byteValue, &parts)
		writeLogFile.Println(string(byteValue))

		for i := 0; i < len(drivesAvailable); i++ {

			swiftMountPoint := drivesAvailable[i].Mountpoint
			swiftDriveLabel := drivesAvailable[i].Device
			driveType := HddOrSSD(swiftDriveLabel)
			if strings.Contains(swiftMountPoint, "/srv/node") {
				swiftMountPoint := strings.Split(swiftMountPoint, "/")[3]
				swiftDrivePrimaryParitions.WithLabelValues(nodeHostname, nodeUUID, swiftMountPoint, "Account & Container", "account", driveType).Set(parts[swiftMountPoint]["accounts"].Primary)
				swiftDrivePrimaryParitions.WithLabelValues(nodeHostname, nodeUUID, swiftMountPoint, "Account & Container", "container", driveType).Set(parts[swiftMountPoint]["containers"].Primary)
				//swiftDrivePrimaryParitions.WithLabelValues(swiftMountPoint, "object").Set(parts[swiftMountPoint].ObjectPartCount.Primary)
				swiftDriveHandoffPartitions.WithLabelValues(nodeHostname, nodeUUID, swiftMountPoint, "Account & Container", "account", driveType).Set(parts[swiftMountPoint]["accounts"].Handoff)
				swiftDriveHandoffPartitions.WithLabelValues(nodeHostname, nodeUUID, swiftMountPoint, "Account & Container", "container", driveType).Set(parts[swiftMountPoint]["containers"].Handoff)
				//swiftDriveHandoffPartitions.WithLabelValues(swiftMountPoint, "object").Set(parts[swiftMountPoint].ObjectPartCount.Handoff)
				if len(storagePolicyNameList) > 0 {
					for j := 0; j < len(storagePolicyNameList); j++ {
						objectDirectoryBody := []string{"objects"}
						storagePolicyIndex := strconv.FormatInt(int64(j), 10)
						if j == 0 {
							swiftDrivePrimaryParitions.WithLabelValues(nodeHostname, nodeUUID, swiftMountPoint, storagePolicyNameList[storagePolicyIndex], "objects", driveType).Set(parts[swiftMountPoint]["objects"].Primary)
							swiftDriveHandoffPartitions.WithLabelValues(nodeHostname, nodeUUID, swiftMountPoint, storagePolicyNameList[storagePolicyIndex], "objects", driveType).Set(parts[swiftMountPoint]["objects"].Handoff)
						} else {
							objectDirectoryBody = append(objectDirectoryBody, storagePolicyIndex)
							objectDirectoryComplete := strings.Join(objectDirectoryBody, "-")
							swiftDrivePrimaryParitions.WithLabelValues(nodeHostname, nodeUUID, swiftMountPoint, storagePolicyNameList[storagePolicyIndex], objectDirectoryComplete, driveType).Set(parts[swiftMountPoint][objectDirectoryComplete].Primary)
							swiftDriveHandoffPartitions.WithLabelValues(nodeHostname, nodeUUID, swiftMountPoint, storagePolicyNameList[storagePolicyIndex], objectDirectoryComplete, driveType).Set(parts[swiftMountPoint][objectDirectoryComplete].Handoff)
							objectDirectoryBody = objectDirectoryBody[:len(objectDirectoryBody)-1]
						}
					}
				}
			} else {
				writeLogFile.Println("Not a Swift Partition")
				continue
			}
		}
	} else {
		writeLogFile.Println("GrabSwiftPartition Module DISABLED")
	}
}

// CheckSwiftLogSize Description: this function checks the size of Swift all.log at /var/log/swift/all.log and returns its size.
// Once the data is retrieved, we will put expose it over Prometheus.
func CheckSwiftLogSize(swiftLog string) {

	writeLogFile := log.New(swiftExporterLog, "CheckSwiftLogSize: ", log.Ldate|log.Ltime|log.Lshortfile)

	swiftLogFileHandle, err := os.Open(swiftLog)
	if err != nil {
		writeLogFile.Println("Cannot open this file. Exiting")
		os.Exit(1)
	}
	defer swiftLogFileHandle.Close()

	fileInfo, err := swiftLogFileHandle.Stat()
	swiftLogFileSize.Set(float64(fileInfo.Size()))
	writeLogFile.Printf("Swift all.log Size: %f", float64(fileInfo.Size()))
}

// GatherStoragePolicyUtilization do a "du -s" across all Swift nodes ("/srv/node") and expose
// actual disk size through the Prometheus.
func GatherStoragePolicyUtilization(GatherStoragePolicyUtilizationEnable bool) {

	writeLogFile := log.New(swiftExporterLog, "GatherStoragePolicyUtilization: ", log.Ldate|log.Ltime|log.Lshortfile)

	if GatherStoragePolicyUtilizationEnable {
		writeLogFile.Println("GatherStoragePolicyUtilization Module ENABLED")
		storagePolicyNameList := GatherStoragePolicyCommonName()
		hostFQDN, hostUUID, _ := GetUUIDAndFQDN(ssnodeConfFile)
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
						swiftStoragePolicyUsage.WithLabelValues(driveLocation, f.Name(), storagePolicyName, hostFQDN, hostUUID).Set(usageFloat)
						// Removing the last element from slice "storagePolicyList", to "reset" the slice. Otherwise,
						// data in this entry will be carried over to the next one. Causing error...
						storagePolicyList = storagePolicyList[:len(storagePolicyList)-1]
					}
				}
			}
		}
	} else {
		writeLogFile.Println("GatherStoragePolicyUtilization Module DISABLED")
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

	nodeHostname, nodeUUID, _ := GetUUIDAndFQDN(ssnodeConfFile) // getting node FQDN and UUID

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
	accountDBCount.WithLabelValues(nodeHostname, nodeUUID).Set(float64(len(accountsDB)))
	accountDBPendingCount.WithLabelValues(nodeHostname, nodeUUID).Set(float64(len(accountsPendingDB)))
	containerDBCount.WithLabelValues(nodeHostname, nodeUUID).Set(float64(len(containersDB)))
	containerDBPendingCount.WithLabelValues(nodeHostname, nodeUUID).Set(float64(len(containersPendingDB)))
	objectFileCount.WithLabelValues(nodeHostname, nodeUUID).Set(float64(len(objectFiles)))
}

// CheckSwiftService is a service check on all Swift / Swift-related services running in a node.
func CheckSwiftService() {
	nodeHostname, nodeUUID, _ := GetUUIDAndFQDN(ssnodeConfFile) // getting node FQDN and UUID
	swiftServices := [4]string{"ssswift-proxy", "ssswift-account@server", "ssswift-container@server", "ssswift-object@server"}
	swiftSubServices := []string{"ssswift-object-replication@server", "ssswift-object-replication@reconstructor.service",
		"ssswift-object-replication@replicator", "ssswift-object@updater", "ssswift-object@auditor", "ssswift-container-replication@sharder",
		"ssswift-container-replication@replicator", "ssswift-container-replication@server", "ssswift-container@updater", "ssswift-container@auditor",
		"ssswift-account-replication@replicator", "ssswift-account-replication@server", "ssswift-account@reaper", "ssswift-account@auditor"}

	for i := 0; i < len(swiftServices); i++ {
		cmd := exec.Command("systemctl", "check", swiftServices[i])
		out, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Println("Cannot find main process")
			fmt.Println("Process Not Running: ", swiftServices[i])
			fmt.Println("Service Status: ", string(out))
			swiftServiceStatus.WithLabelValues(nodeHostname, nodeUUID, swiftServices[i]).Set(float64(0))
		} else {
			if strings.TrimRight(string(out), "\n") == "active" {
				fmt.Println("Now Checking: ", swiftServices[i])
				fmt.Println("Service Status: ", string(out))
				swiftServiceStatus.WithLabelValues(nodeHostname, nodeUUID, swiftServices[i]).Set(float64(1))
			}
		}
	}

	for j := 0; j < len(swiftSubServices); j++ {
		cmd := exec.Command("systemctl", "check", swiftSubServices[j])
		out, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Println("Cannot find process")
			fmt.Println("Process Not Running: ", swiftServices[j])
			fmt.Println("Service Status: ", string(out))
			swiftServiceStatus.WithLabelValues(nodeHostname, nodeUUID, swiftServices[j]).Set(float64(0))

		} else {
			if strings.TrimRight(string(out), "\n") == "active" {
				fmt.Println("Now Checking: ", swiftServices[j])
				fmt.Println("Service Status: ", string(out))
				swiftServiceStatus.WithLabelValues(nodeHostname, nodeUUID, swiftServices[j]).Set(float64(1))
			}
		}
	}
}
