package tspolicy

func readPolicyObjectFromFileV1(jsonPath string) TsPolicyObject, error {

	var policyObject TsPolicyObject

	jsonFile, err := LoadTsPolicyJsonFromFile(path string)
	if err != nil {
		log.Error("Couldn't read PolicyObject from file")
		return nil, err
	}
	// TODO: validate against scheme from docs

	err, policyObject.UnmarshalTsPolicyJson(jsonFile)
	if err != nil {
		log.Error("Couldn't read PolicyObject from file")
		return nil, err
	}
	return policyObject
}
