package tspolicy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"os"

	"github.com/RIMEDO-Labs/xapp-sdk/pkg/mho"
	policyAPI "github.com/onosproject/onos-a1-dm/go/policy_schemas/traffic_steering_preference/v2"
	"github.com/xeipuuv/gojsonschema"
)

func NewPolicySchemaValidatorV2(path string) *PolicySchemaValidatorV2 {

	return &PolicySchemaValidatorV2{
		schemePath: path,
	}

}

type PolicySchemaValidatorV2 struct {
	schemePath string
}

func NewPolicyManager(schemePath string, policyMap *map[string]*mho.PolicyData) *PolicyManager {

	var POLICY_WEIGHTS = map[string]int{
		"DEFAULT": 0.0,
		"PREFER":  8.0,
		"AVOID":   -8.0,
		"SHALL":   1000.0,
		"FORBID":  -1000.0,
	}

	return &PolicyManager{
		validator:     NewPolicySchemaValidatorV2(schemePath),
		policyMap:     policyMap,
		preferenceMap: POLICY_WEIGHTS,
	}

}

type PolicyManager struct {
	validator     *PolicySchemaValidatorV2
	policyMap     *map[string]*mho.PolicyData
	preferenceMap map[string]int
}

func (m *PolicyManager) ReadPolicyObjectFromFileV2(jsonPath string, policyObject *mho.PolicyData) error {

	jsonFile, err := m.LoadTsPolicyJsonFromFileV2(jsonPath)
	if err != nil {
		log.Error("Couldn't read PolicyObject from file")
		return err // empty struct??
	}

	// TODO don't load json two times
	var ok bool
	ok, err = m.ValidateTsPolicyJsonSchemaV2(jsonPath)
	if err != nil {
		log.Error("Error validating json scheme")
		return err
	}
	if !ok {
		return errors.New("the json file is invalid")
	}
	if err = m.UnmarshalTsPolicyJsonV2(jsonFile, policyObject); err != nil {
		log.Error("Error unmarshaling json file")
		return err
	}

	return nil
}

func (m *PolicyManager) CheckPerUePolicyV2(ueScope policyAPI.Scope, policyObject *mho.PolicyData) bool {
	// Check policy scope according to O-RAN WG2.A1TD v1.00
	// Value 0 identifies empty number field
	log.Info("Scope: ", ueScope)
	log.Info("UeIDL ", policyObject.API.Scope)
	// Check if Policy is Per UE
	if policyObject.API.Scope.UeID == nil || *policyObject.API.Scope.UeID == "" {
		return false
	}
	// Check if UE Id match
	if *policyObject.API.Scope.UeID != *ueScope.UeID {
		return false
	}

	// Check for Slice Id
	if (policyObject.API.Scope.SliceID != nil) && (*policyObject.API.Scope.SliceID != *ueScope.SliceID) {
		return false
	}

	// Check for QoS Id
	if (policyObject.API.Scope.QosID != nil) && (*policyObject.API.Scope.QosID != *ueScope.QosID) {
		return false
	}

	// check for cell Id
	if (policyObject.API.Scope.CellID != nil) && (*policyObject.API.Scope.CellID != *ueScope.CellID) {
		return false
	}

	return true
}

func (m *PolicyManager) CheckPerSlicePolicyV2(ueScope policyAPI.Scope, policyObject *mho.PolicyData) bool {
	// Check policy scope according to O-RAN WG2.A1TD v1.00
	// Value 0 identifies empty number field

	// Check if Policy is Per UE
	if policyObject.API.Scope.SliceID == nil {
		return false
	}

	if *(policyObject.API.Scope.SliceID.SD) == "" ||
		policyObject.API.Scope.SliceID.Sst == 0 ||
		policyObject.API.Scope.SliceID.PlmnID.Mcc == "" ||
		policyObject.API.Scope.SliceID.PlmnID.Mnc == "" ||
		&policyObject.API.Scope.SliceID.PlmnID == nil {
		return false
	}

	// Check if UE Id exists
	if (policyObject.API.Scope.UeID != nil) && (*policyObject.API.Scope.UeID == "") {
		return false
	}

	// Check for QoS Id
	if (policyObject.API.Scope.QosID != nil) && (*policyObject.API.Scope.QosID != *ueScope.QosID) {
		return false
	}

	// check for cell Id
	if (policyObject.API.Scope.CellID != nil) && (*policyObject.API.Scope.CellID != *ueScope.CellID) {
		return false
	}

	return true
}

func (m *PolicyManager) GetTsResultForUEV2(ueScope policyAPI.Scope, rsrps []int, cellIds []policyAPI.CellID) policyAPI.CellID {

	log.Info("CELL iD List in getTs:", cellIds)
	var bestCell policyAPI.CellID
	bestScore := -math.MaxFloat64
	for i := 0; i < len(rsrps); i++ {
		//TEMP - TO REMOVE
		preferece := m.GetPreferenceV2(ueScope, cellIds[i])
		score := m.GetPreferenceScoresV2(preferece, rsrps[i])

		if score > bestScore {
			bestCell = cellIds[i]
			bestScore = score
		}
	}
	log.Info("The best one:", bestCell)
	return bestCell
}

func (m *PolicyManager) GetPreferenceScoresV2(preference string, rsrp int) float64 {

	return float64(rsrp) + float64(m.preferenceMap[preference])
}

func (m *PolicyManager) GetPreferenceV2(ueScope policyAPI.Scope, queryCellId policyAPI.CellID) string {

	var preference string = "DEFAULT" // TODO: maybe change
	for _, policy := range *m.policyMap {
		if policy.IsEnforced {
			log.Info("Policy Map:", *m.policyMap)
			log.Infof("Policy in get pref [Pointer:%v, Value:%v, API:%v]", policy, *policy, *policy.API)
			// Check if ueScope match the policy Scope
			log.Info("Ue Scope:", *ueScope.UeID)
			log.Info("Ue Policy:", *policy.API.Scope.UeID)
			log.Info("Slice check:", m.CheckPerSlicePolicyV2(ueScope, policy))
			log.Info("Ue check:", m.CheckPerUePolicyV2(ueScope, policy))
			if m.CheckPerSlicePolicyV2(ueScope, policy) || m.CheckPerUePolicyV2(ueScope, policy) {
				log.Info("Query CellID:", queryCellId)
				// Find cell and related preference
				for _, tspResource := range policy.API.TSPResources {

					for _, cellId := range tspResource.CellIDList {
						log.Info("CellId inside: ", cellId)
						// log.Info("If result: ", cellId == queryCellId)
						if *cellId.CID.NcI == *queryCellId.CID.NcI &&
							cellId.PlmnID.Mcc == queryCellId.PlmnID.Mcc &&
							cellId.PlmnID.Mnc == queryCellId.PlmnID.Mnc {
							log.Info("Inside if!")
							preference = string(tspResource.Preference)
						}
					}
				}
			}
			log.Info("Preference:", preference)
		}
	}
	return preference
}

func (m *PolicyManager) AddPolicyV2(policyId string, policyDir string, policyObject *mho.PolicyData) error {

	policyPath := policyDir + policyId
	err := m.ReadPolicyObjectFromFileV2(policyPath, policyObject)
	if err != nil {
		log.Error(fmt.Sprintf("Couldn't read PolicyObject from file \n policyId: %s from: %s", policyId, policyPath))
		return err
	}
	// (*m.policyMap)[policyObject.Key] = policyObject
	return nil
}

func (m *PolicyManager) EnforcePolicyV2(policyId string) bool {

	if _, ok := (*m.policyMap)[policyId]; ok {
		(*m.policyMap)[policyId].IsEnforced = true
		return true
	}
	log.Error(fmt.Sprintf("Policy with policyId: %s, not enforced", policyId))
	return false
}

func (m *PolicyManager) DisablePolicyV2(policyId string) bool {

	if _, ok := (*m.policyMap)[policyId]; ok {
		(*m.policyMap)[policyId].IsEnforced = false
		return true
	}
	log.Error(fmt.Sprintf("Policy with policyId: %s, not enforced", policyId))
	return false
}

func (m *PolicyManager) GetPolicyV2(policyId string) (*mho.PolicyData, bool) {

	if val, ok := (*m.policyMap)[policyId]; ok {
		return val, ok
	}
	log.Error(fmt.Sprintf("Policy with policyId: %s, not enforced", policyId))
	return nil, false
}

func (m *PolicyManager) ValidateTsPolicyJsonSchemaV2(jsonPath string) (bool, error) {

	schemaLoader := gojsonschema.NewReferenceLoader("file://" + m.validator.schemePath)
	documentLoader := gojsonschema.NewReferenceLoader("file://" + jsonPath)

	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		return false, err
	}
	return result.Valid(), nil
}

func (m *PolicyManager) UnmarshalTsPolicyJsonV2(jsonFile []byte, policyObject *mho.PolicyData) error {

	if err := json.Unmarshal(jsonFile, policyObject.API); err != nil {
		log.Error("Couldn't read PolicyObject from file")
		return err
	}
	log.Debug("Successfully read the policy {tsPolicy}")
	// policyObject.IsEnforced = false // by default set policy to be not enforced
	return nil

}

func (m *PolicyManager) LoadTsPolicyJsonFromFileV2(path string) ([]byte, error) {

	jsonFile, err := os.Open(path)
	if err != nil {
		log.Error("Failed to open policy JSON File")
		if os.IsNotExist(err) {
			fmt.Print("File Does Not Exist: ")
		} else if os.IsExist(err) {
			fmt.Print("File Exists: ")
		}
		fmt.Println(err)
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
