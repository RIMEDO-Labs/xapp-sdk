package mho

import (
	e2sm_mho "github.com/onosproject/onos-e2-sm/servicemodels/e2sm_mho/v1/e2sm-mho"
)

type UeData struct {
	UeID          string
	E2NodeID      string
	CGI           *e2sm_mho.CellGlobalId
	CGIString     string
	RrcState      string
	RsrpServing   int32
	RsrpNeighbors map[string]int32
}

type CellData struct {
	CGI                    *e2sm_mho.CellGlobalId
	CGIString              string
	CumulativeHandoversIn  int
	CumulativeHandoversOut int
	Ues                    map[string]*UeData
}
