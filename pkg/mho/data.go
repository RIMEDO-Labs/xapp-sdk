package mho

import (
	policyAPI "github.com/onosproject/onos-a1-dm/go/policy_schemas/traffic_steering_preference/v2"
	e2sm_v2_ies "github.com/onosproject/onos-e2-sm/servicemodels/e2sm_mho_go/v2/e2sm-v2-ies"
)

type UeData struct {
	UeID            string
	E2NodeID        string
	CGI             *e2sm_v2_ies.Cgi
	CGIString       string
	RrcState        string
	RsrpServing     int32
	RsrpNeighbors   map[string]int32
	FiveQiServing   int32
	FiveQiNeighbors map[string]int32
}

type CellData struct {
	CGI                    *e2sm_v2_ies.Cgi
	CGIString              string
	CumulativeHandoversIn  int
	CumulativeHandoversOut int
	Ues                    map[string]*UeData
}

type PolicyData struct {
	Key        string
	API        *policyAPI.API
	IsEnforced bool
}
