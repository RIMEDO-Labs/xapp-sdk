package tspolicy

type ScopeV1 struct {
	UeId    string `json:"ueId,omitempty"` // required UeId, ot SliceId for further validation
	SliceId int    `json:"sliceId,omitempty"`
	QosId   int    `json:"qosId,omitempty"`
	CellId  int    `json:cellId,omitempty`
}

type TspResourceV1 struct {
	CellIdList []int  `json:cellIdList`
	Preference string `json:preference`
	Primary    bool   `json:primary,omitempty`
}

type TsPolicyV1 struct {
	Scope        ScopeV1         `json:"scope"`
	TspResources []TspResourceV1 `json:"tspResources"`
}

type TsPolicyObject struct {
	tsPolicyV1 TsPolicyV1
	isEnforced bool
}
