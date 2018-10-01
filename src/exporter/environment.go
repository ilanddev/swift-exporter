package exporter

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

type formpostParameter struct {
}

type sloParameter struct {
	MaxManifestSegments int `json:"max_manifest_segments"`
	MaxManifestSize     int `json:"max_manifest_size"`
	MinSegmentSize      int `json:"min_segment_size"`
}

type swiftParameter struct {
	AccountAutoCreate      bool     `json:"account_autocreate"`
	AccountListingLimit    int      `json:"account_listing_limit"`
	AllowAccountManagement bool     `json:"allow_account_management"`
	ContainerListingLimit  int      `json:"container_listing_limit"`
	ExtraHeaderConunt      int      `json:"extra_header_count"`
	MaxAccountNameLength   int      `json:"max_account_name_length"`
	MaxContainerNameLength int      `json:"max_container_name_length"`
	MaxFileSize            int      `json:"max_file_size"`
	MaxHeaderSize          int      `json:"max_header_size"`
	MaxMetaCount           int      `json:"max_meta_count"`
	MaxMetaNameLength      int      `json:"max_meta_name_count"`
	MaxMetaOverallSize     int      `json:"max_meta_overall_size"`
	MaxMetaValueLength     int      `json:"max_meta_value_length"`
	MaxObjectNameLength    int      `json:"max_object_name_length"`
	Policies               []string `json:"policies"`
	StrictCorsMode         bool     `json:"strict_core_mode"`
	Version                string   `json:"version"`
}

type swift3Parameter struct {
	AllowMultipartUpload bool   `json:"allow_multipart_upload"`
	MaxBucketListing     int    `json:"max_bucket_listing"`
	MaxMultiDeleteObject int    `json:"max_multi_delete_object"`
	MaxPartsListing      int    `json:"max_parts_listing"`
	MaxUploadPartNum     int    `json:"max_upload_part_num"`
	Version              string `json:"version"`
}

type swiftstackAuthParameter struct {
	AccountACL bool `json:"account_acl"`
}

type swiftstackAuthen struct {
}

type tempURLParameter struct {
	IncomingAllowHeaders  []string `json:"incoming_allow_headers"`
	IncomingRemoveHeaders []string `json:"incoming_remove_headers"`
	Methods               []string `json:"method"`
	OutgoingAllowHeaders  []string `json:"outgoing_allow_headers"`
	OutgoingRemoveHeaders []string `json:"outgoing_remove_headers"`
}

type NodeSwiftSetting struct {
	Formpost         formpostParameter       `json:"formpost"`
	SLO              sloParameter            `json:"slo"`
	Swift            swiftParameter          `json:"swift"`
	Swift3           swift3Parameter         `json:"swift3"`
	SwiftStackAuth   swiftstackAuthParameter `json:"swiftstack_auth"`
	SwiftStackAuthen swiftstackAuthen        `json:"swiftstack_authen"`
	TempURL          tempURLParameter        `json:"tempurl"`
}

// GetSwiftEnvironmentParameters - this function runs a curl call to http://<node_ipaddress>/info to get the
// node parameter of the system. Environment variables like Swift version, S3 version...etc will be expose and
// reference in other modules in the script.
func GetSwiftEnvironmentParameters() (swiftEnvironmentParameters NodeSwiftSetting) {
	apiIP, apiPort, apiHostname := GetAPIAddress()
	var targetEndpoint string
	var read NodeSwiftSetting
	var target []string

	if len(apiHostname) != 0 {
		target = []string{"http://", apiHostname}
	} else if len(apiIP) != 0 {
		target = []string{"http://", apiIP}
	}

	if apiPort == "443" {
		target[0] = "https://"
	}

	target[0] = strings.Join(target, "")
	target[1] = "/info"
	targetEndpoint = strings.Join(target, "")

	resp, err := http.Get(targetEndpoint)
	if err != nil {
		fmt.Println(err)
		fmt.Println("Error here!!")
	}

	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	err2 := json.Unmarshal(body, &read)

	if err2 == nil {
		fmt.Println(err2)
		fmt.Println("ERROR")
	}

	swiftEnvironmentParameters = read
	return swiftEnvironmentParameters
}

func GetAPIAddress() (apiAddress string, apiPort string, apiHostname string) {

	openFile, err := os.Open("/etc/ssnode.conf")
	if err != nil {
		fmt.Println("I cannot read this file")
	}
	defer openFile.Close()

	readFile := bufio.NewScanner(openFile)
	for readFile.Scan() {
		if strings.Contains(readFile.Text(), "api_ip") {
			apiAddress = strings.TrimSpace(strings.Split(readFile.Text(), "=")[1])
		}
		if strings.Contains(readFile.Text(), "api_port") {
			apiPort = strings.TrimSpace(strings.Split(readFile.Text(), "=")[1])
		}
		if strings.Contains(readFile.Text(), "api_hostname") {
			apiHostname = strings.TrimSpace(strings.Split(readFile.Text(), "=")[1])
		}
	}

	return apiAddress, apiPort, apiHostname
}

func GetUUIDAndFQDN() (UUID string, FQDN string) {
	// to get this module to run, please do he following:
	// read /etc/ssnode.conf to get the UUID of the node
	// run hostnamectl to get the FQDN of the node

	var nodeUUID string
	var hostName string

	openFile, err := os.Open("/etc/ssnode.conf")

	if err != nil {
		fmt.Println("I cannot read this file")
	}
	defer openFile.Close()

	readFile := bufio.NewScanner(openFile)
	for readFile.Scan() {
		if strings.Contains(readFile.Text(), "node_uuid") {
			nodeUUID = strings.TrimSpace(strings.Split(readFile.Text(), "=")[1])
		}
	}

	runCommand, _ := exec.Command("hostname", "-f").Output()
	hostName = strings.TrimRight(string(runCommand), "\n")
	return hostName, nodeUUID
}
