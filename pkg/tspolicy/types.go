package tspolicy

var POLICY_WEIGHT = map[string]int{
	"DEFAULT": 0.0,
	"PREFER":  8.0,
	"AVOID":   -8.0,
	"SHALL":   1000.0,
	"FORBID":  -1000.0,
}

type ScopeV1 struct {
	UeId    string `json:"ueId,omitempty"` // required UeId, ot SliceId for further validation
	SliceId int    `json:"sliceId,omitempty"`
	QosId   int    `json:"qosId,omitempty"`
	CellId  int    `json:"cellId,omitempty"`
}

type TspResourceV1 struct {
	CellIdList []int  `json:"cellIdList"`
	Preference string `json:"preference"`
	Primary    bool   `json:"primary,omitempty"`
}

type TsPolicyV1 struct {
	Scope        ScopeV1         `json:"scope"`
	TspResources []TspResourceV1 `json:"tspResources"`
}

type TsPolicyObject struct {
	tsPolicyV1 TsPolicyV1
	isEnforced bool
}

type TsPolicyMap struct {
	// Key - string policyId, value - TSPolicyObject
	policies  map[string]*TsPolicyObject
	validator TsPolicySchemaValidatorV1
}

type TsPolicySchemaValidatorV1 struct {
	schemePath string
}
