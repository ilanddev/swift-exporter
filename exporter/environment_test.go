package exporter

import (
	"fmt"
	"testing"
)

// change the location of the ssnode.conf file here for the rest of the test script to work.
const TestssnodeConfFile = ""

func TestGetAPIAddress(t *testing.T) {
	// run GetAPIAddress to get API data back from the test ssnode.conf listed on line #9.
	testAPIAddress, testAPIPort, testAPIHostname, getError := GetAPIAddress(TestssnodeConfFile)
	if getError != nil {
		fmt.Println(getError)
	}
	fmt.Printf("The read variables are:")
	fmt.Printf("API Address: %s \n", testAPIAddress)
	fmt.Printf("API Port: %s \n", testAPIPort)
	fmt.Printf("API Hostname: %s \n", testAPIHostname)
}

func TestGetUUIDAndFQDN(t *testing.T) {
	testUUID, testFQDN, getError := GetUUIDAndFQDN(TestssnodeConfFile)
	if getError != nil {
		fmt.Println(getError)
	}
	fmt.Println("The read variables are:")
	fmt.Printf("FQDN: %s \n", testFQDN)
	fmt.Printf("UUID: %s \n", testUUID)

}
