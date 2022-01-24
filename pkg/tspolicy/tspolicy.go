package tspolicy

import (
	"fmt"
)

func (t *TsPolicyObject) ReadPolicyObjectFromFileV1(jsonPath string) error {

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

func (p *TsPolicyMap) AddPolicy(policyId string, policyDir string) error {

	var policyObject TsPolicyObject
	policyPath := policyDir + policyId
	err := policyObject.ReadPolicyObjectFromFileV1(policyPath)
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
