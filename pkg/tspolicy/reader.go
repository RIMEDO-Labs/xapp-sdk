package tspolicy

import {
	"os"
	"io/ioutil"
	"encoding/json"

	"github.com/onosproject/onos-lib-go/pkg/logging"
}

var log = logging.GetLogger("jsonTsPolicy")


func LoadTsPolicyJsonFromFile(path string) []byte, error {

	jsonFile, err := os.Open(path)
    if err != nil {
		log.Error("Failed to open policy JSON File")
		return nil, err
    }

	log.Debug("Successfully Opened policy JSON File")
    // defer the closing of our jsonFile so that we can parse it later on
    defer jsonFile.Close()

    // read our opened xmlFile as a byte array.
    byteValue, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		log.Error("Failed to open policy JSON File")
		return nil, err
    }
	return byteValue, nil
}

func (t* TsPolicyObject) UnmarshalTsPolicyJson(tsPolicyJson []byte) error {

	err := json.Unmarshal(tsPolicyJson, &t.tsPolicyV1)
	if err != nil {
		log.Error("Failed to unmarshal policy JSON")
		// MH: Consider whjat next xApp should not crash
		return err
	}
	log.Debug("Successfully read the policy {tsPolicy}")
	t.isEnforced = false // by default set policy to be not enforced
	return nil
}