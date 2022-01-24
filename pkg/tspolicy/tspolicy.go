package tspolicy

import (
	"fmt"
)

func (t *TsPolicyObject) readPolicyObjectFromFileV1(jsonPath string) error {

	jsonFile, err := LoadTsPolicyJsonFromFile(jsonPath)
	if err != nil {
		log.Error("Couldn't read PolicyObject from file")
		return err // empty struct??
	}
	// TODO: validate against scheme from docs

	err = t.UnmarshalTsPolicyJson(jsonFile)
	if err != nil {
		log.Error("Couldn't read PolicyObject from file")
		return err
	}
	return nil
}

func NewTsPolicyMap() *TsPolicyMap {
	var t TsPolicyMap
	t.policies = make(map[string]*TsPolicyObject)
	return &t
}

func (p *TsPolicyMap) addPolicy(policyId string, policyDir string) error {

	var policyObject TsPolicyObject
	policyPath := policyDir + policyId
	err := policyObject.readPolicyObjectFromFileV1(policyPath)
	if err != nil {
		log.Error(fmt.Sprintf("Couldn't read PolicyObject from file \n policyId: %s from: %s", policyId, policyPath))
		return err
	}
	p.policies[policyId] = &policyObject
	return nil
}

func (p *TsPolicyMap) enforcePolicy(policyId string) bool {

	if _, ok := p.policies[policyId]; ok {
		p.policies[policyId].isEnforced = true
		return true
	}
	log.Error(fmt.Sprintf("Policy with policyId: %s, not enforced", policyId))
	return false
}
