package tspolicy

import (
	"encoding/json"
	"errors"
	"math"

	"github.com/RIMEDO-Labs/xapp-sdk/pkg/mho"
	policyAPI "github.com/onosproject/onos-a1-dm/go/policy_schemas/traffic_steering_preference/v2"
)

func NewPolicyManager(schemePath string, policyMap *map[string]*mho.PolicyData) *PolicyManager {

	return &PolicyManager{
		validator: NewTsPolicySchemaValidatorV1(schemePath),
		policyMap: policyMap,
	}

}

type PolicyManager struct {
	validator *TsPolicySchemaValidatorV1
	policyMap *map[string]*mho.PolicyData
}

func (m *PolicyManager) ReadPolicyObjectFromFile(jsonPath string, policyObject *mho.PolicyData) error {

	jsonFile, err := LoadTsPolicyJsonFromFile(jsonPath)
	if err != nil {
		log.Error("Couldn't read PolicyObject from file")
		return err // empty struct??
	}

	// TODO don't load json two times
	var ok bool
	ok, err = m.validator.ValidateTsPolicyJsonSchemaV1(jsonPath)
	if err != nil {
		log.Error("Error validating json scheme")
		return err
	}
	if !ok {
		return errors.New("the json file is invalid")
	}
	if err = json.Unmarshal(jsonFile, policyObject.API); err != nil {
		log.Error("Couldn't read PolicyObject from file")
		return err
	}

	return nil
}

func (m *PolicyManager) CheckPerUePolicyV2(ueScope policyAPI.Scope, policyObject *mho.PolicyData) bool {
	// Check policy scope according to O-RAN WG2.A1TD v1.00
	// Value 0 identifies empty number field

	// Check if Policy is Per UE
	if *policyObject.API.Scope.UeID == "" {
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

func (p *TsPolicyObject) CheckPerSlicePolicyV2(ueScope policyAPI.Scope, policyObject *mho.PolicyData) bool {
	// Check policy scope according to O-RAN WG2.A1TD v1.00
	// Value 0 identifies empty number field

	// Check if Policy is Per UE
	if policyObject.API.Scope.SliceID == nil {
		return false
	}

	if *policyObject.API.Scope.SliceID.SD == "" ||
		policyObject.API.Scope.SliceID.Sst == 0 ||
		policyObject.API.Scope.SliceID.PlmnID.Mcc == "" ||
		policyObject.API.Scope.SliceID.PlmnID.Mnc == "" ||
		&policyObject.API.Scope.SliceID.PlmnID == nil {
		return false
	}

	// Check if UE Id exists
	if *policyObject.API.Scope.UeID == "" {
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

func (t *TsPolicyMap) GetTsResultForUE(ueScope ScopeV1, rsrps []int, cellIds []int) int {

	bestCell := -1
	bestScore := -math.MaxFloat64
	for i := 0; i < len(rsrps); i++ {
		preferece := t.GetPreference(ueScope, cellIds[i])
		score := GetPreferenceScore(preferece, rsrps[i])

		if score > bestScore {
			bestCell = cellIds[i]
			bestScore = score
		}
	}
	return bestCell
}

func GetPreferenceScores(preference string, rsrp int) float64 {

	return float64(rsrp) + float64(POLICY_WEIGHT[preference])
}

func (p *TsPolicyMap) GetPreferences(ueScope ScopeV1, queryCellId int) string {

	var preference string = "DEFAULT" // TODO: maybe change
	for _, policy := range p.policies {
		if policy.isEnforced {
			// Check if ueScope match the policy Scope
			if policy.CheckPerSlicePolicy(ueScope) || policy.CheckPerUEPolicy(ueScope) {

				// Find cell and related preference
				for _, tspResource := range policy.tsPolicyV1.TspResources {

					for _, cellId := range tspResource.CellIdList {
						if cellId == queryCellId {
							preference = tspResource.Preference
						}
					}
				}
			}
		}
	}
	return preference
}
