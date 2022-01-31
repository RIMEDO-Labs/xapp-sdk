package tspolicy

import (
	"encoding/json"
	"io/ioutil"
	"os"

	"github.com/onosproject/onos-lib-go/pkg/logging"
	"github.com/xeipuuv/gojsonschema"
	// MIT License
)

var log = logging.GetLogger("jsonTsPolicy")

func LoadTsPolicyJsonFromFile(path string) ([]byte, error) {

	jsonFile, err := os.Open(path)
	if err != nil {
		log.Error("Failed to open policy JSON File")
		return nil, err
	}

	log.Info("Successfully Opened policy JSON File")
	// defer the closing of our jsonFile so that we can parse it later on
	defer jsonFile.Close()

	// read our opened xmlFile as a byte array.
	byteValue, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		log.Error("Failed to read data from policy JSON File")
		return nil, err
	}
	return byteValue, nil
}

func (t *TsPolicySchemaValidatorV1) ValidateTsPolicyJsonSchemaV1(jsonPath string) (bool, error) {

	schemaLoader := gojsonschema.NewReferenceLoader("file://" + t.schemePath)
	documentLoader := gojsonschema.NewReferenceLoader("file://" + jsonPath)

	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		return false, err
	}
	return result.Valid(), nil
}

func (t *TsPolicyObject) UnmarshalTsPolicyJson(tsPolicyJson []byte) error {

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
