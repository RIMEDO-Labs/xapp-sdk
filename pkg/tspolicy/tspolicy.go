package tspolicy

import (
	"errors"
	"fmt"
	"math"
)

func (t *TsPolicyObject) ReadPolicyObjectFromFileV1(jsonPath string, validator *TsPolicySchemaValidatorV1) error {

	jsonFile, err := LoadTsPolicyJsonFromFile(jsonPath)
	if err != nil {
		log.Error("Couldn't read PolicyObject from file")
		return err // empty struct??
	}

	// TODO don't load json two times
	var ok bool
	ok, err = validator.ValidateTsPolicyJsonSchemaV1(jsonPath)
	if err != nil {
		log.Error("Error validating json scheme")
		return err
	}
	if !ok {
		return errors.New("the json file is invalid")
	}

	err = t.UnmarshalTsPolicyJson(jsonFile)
	if err != nil {
		log.Error("Couldn't read PolicyObject from file")
		return err
	}
	return nil
}

func NewTsPolicySchemaValidatorV1(path string) *TsPolicySchemaValidatorV1 {

	var t TsPolicySchemaValidatorV1
	t.schemePath = path
	return &t
}

func NewTsPolicyMap(schemePath string) *TsPolicyMap {
	var t TsPolicyMap
	t.policies = make(map[string]*TsPolicyObject)
	t.validator = *NewTsPolicySchemaValidatorV1(schemePath)
	return &t
}

func (p *TsPolicyObject) CheckPerUEPolicy(ueScope ScopeV1) bool {
	// Check policy scope according to O-RAN WG2.A1TD v1.00
	// Value 0 identifies empty number field

	// Check if Policy is Per UE
	if p.tsPolicyV1.Scope.UeId == "" {
		return false
	}
	// Check if UE Id match
	if p.tsPolicyV1.Scope.UeId != ueScope.UeId {
		return false
	}

	// Check for Slice Id
	if (p.tsPolicyV1.Scope.SliceId != 0) && (p.tsPolicyV1.Scope.SliceId != ueScope.SliceId) {
		return false
	}

	// Check for QoS Id
	if (p.tsPolicyV1.Scope.QosId != 0) && (p.tsPolicyV1.Scope.QosId != ueScope.QosId) {
		return false
	}

	// check for cell Id
	if (p.tsPolicyV1.Scope.CellId != 0) && (p.tsPolicyV1.Scope.CellId != ueScope.CellId) {
		return false
	}

	return true
}

func (p *TsPolicyObject) CheckPerSlicePolicy(ueScope ScopeV1) bool {
	// Check policy scope according to O-RAN WG2.A1TD v1.00
	// Value 0 identifies empty number field

	// Check if Policy is Per Slice
	if p.tsPolicyV1.Scope.SliceId == 0 {
		return false
	}
	// Check if Slice Id match
	if p.tsPolicyV1.Scope.SliceId != ueScope.SliceId {
		return false
	}

	// Check if UE Id exists
	if p.tsPolicyV1.Scope.UeId != "" {
		return false
	}

	// Check for QoS Id
	if (p.tsPolicyV1.Scope.QosId != 0) && (p.tsPolicyV1.Scope.QosId != ueScope.QosId) {
		return false
	}

	// check for cell Id
	if (p.tsPolicyV1.Scope.CellId != 0) && (p.tsPolicyV1.Scope.CellId != ueScope.CellId) {
		return false
	}
	return true
}

func (t *TsPolicyMap) GetTSResultForUE(ueScope ScopeV1, rsrps []int, cellIds []int) int {

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

func GetPreferenceScore(preference string, rsrp int) float64 {

	return float64(rsrp) + float64(POLICY_WEIGHT[preference])
}

func (p *TsPolicyMap) GetPreference(ueScope ScopeV1, queryCellId int) string {

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

func (p *TsPolicyMap) AddPolicy(policyId string, policyDir string) error {

	var policyObject TsPolicyObject
	policyPath := policyDir + policyId
	err := policyObject.ReadPolicyObjectFromFileV1(policyPath, &p.validator)
	if err != nil {
		log.Error(fmt.Sprintf("Couldn't read PolicyObject from file \n policyId: %s from: %s", policyId, policyPath))
		return err
	}
	p.policies[policyId] = &policyObject
	return nil
}

func (p *TsPolicyMap) EnforcePolicy(policyId string) bool {

	if _, ok := p.policies[policyId]; ok {
		p.policies[policyId].isEnforced = true
		return true
	}
	log.Error(fmt.Sprintf("Policy with policyId: %s, not enforced", policyId))
	return false
}

func (p *TsPolicyMap) DisablePolicy(policyId string) bool {

	if _, ok := p.policies[policyId]; ok {
		p.policies[policyId].isEnforced = false
		return true
	}
	log.Error(fmt.Sprintf("Policy with policyId: %s, not enforced", policyId))
	return false
}

func (p *TsPolicyMap) GetPolicy(policyId string) (*TsPolicyObject, bool) {

	if val, ok := p.policies[policyId]; ok {
		return val, ok
	}
	log.Error(fmt.Sprintf("Policy with policyId: %s, not enforced", policyId))
	return nil, false
}
