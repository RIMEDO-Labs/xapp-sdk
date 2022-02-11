package mho

import (
	"strconv"

	e2sm_mho "github.com/onosproject/onos-e2-sm/servicemodels/e2sm_mho/v1/e2sm-mho"
)

func PlmnIDBytesToInt(b []byte) uint64 {
	return uint64(b[2])<<16 | uint64(b[1])<<8 | uint64(b[0])
}

func PlmnIDNciToCGI(plmnID uint64, nci uint64) string {
	return strconv.FormatInt(int64(plmnID<<36|(nci&0xfffffffff)), 16)
}

//func getPlmnIDFromIndicationHeader(header *e2sm_mho.E2SmMhoIndicationHeaderFormat1) uint64 {
//	plmnIDBytes := header.GetCgi().GetNrCgi().GetPLmnIdentity().GetValue()
//	return plmnIDBytesToInt(plmnIDBytes)
//}

func GetNciFromCellGlobalID(cellGlobalID *e2sm_mho.CellGlobalId) uint64 {
	return cellGlobalID.GetNrCgi().GetNRcellIdentity().GetValue().GetValue()
}

func GetPlmnIDBytesFromCellGlobalID(cellGlobalID *e2sm_mho.CellGlobalId) []byte {
	return cellGlobalID.GetNrCgi().GetPLmnIdentity().GetValue()
}

func GetCGIFromIndicationHeader(header *e2sm_mho.E2SmMhoIndicationHeaderFormat1) string {
	nci := GetNciFromCellGlobalID(header.GetCgi())
	plmnIDBytes := GetPlmnIDBytesFromCellGlobalID(header.GetCgi())
	plmnID := PlmnIDBytesToInt(plmnIDBytes)
	return PlmnIDNciToCGI(plmnID, nci)
}

func GetCGIFromMeasReportItem(measReport *e2sm_mho.E2SmMhoMeasurementReportItem) string {
	nci := GetNciFromCellGlobalID(measReport.GetCgi())
	plmnIDBytes := GetPlmnIDBytesFromCellGlobalID(measReport.GetCgi())
	plmnID := PlmnIDBytesToInt(plmnIDBytes)
	return PlmnIDNciToCGI(plmnID, nci)
}
