package tspolicy

func readPolicyObjectFromFileV1(jsonPath string) (TsPolicyObject, error) {

	var policyObject TsPolicyObject

	jsonFile, err := LoadTsPolicyJsonFromFile(jsonPath)
	if err != nil {
		log.Error("Couldn't read PolicyObject from file")
		return TsPolicyObject{}, err // empty struct??
	}
	// TODO: validate against scheme from docs

	err = policyObject.UnmarshalTsPolicyJson(jsonFile)
	if err != nil {
		log.Error("Couldn't read PolicyObject from file")
		return TsPolicyObject{}, err
	}
	return policyObject, nil
}
