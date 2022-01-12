package mho

import (
	"strconv"

	e2sm_mho "github.com/onosproject/onos-e2-sm/servicemodels/e2sm_mho/v1/e2sm-mho"
)

func plmnIDBytesToInt(b []byte) uint64 {
	return uint64(b[2])<<16 | uint64(b[1])<<8 | uint64(b[0])
}

func plmnIDNciToCGI(plmnID uint64, nci uint64) string {
	return strconv.FormatInt(int64(plmnID<<36|(nci&0xfffffffff)), 16)
}

//func getPlmnIDFromIndicationHeader(header *e2sm_mho.E2SmMhoIndicationHeaderFormat1) uint64 {
//	plmnIDBytes := header.GetCgi().GetNrCgi().GetPLmnIdentity().GetValue()
//	return plmnIDBytesToInt(plmnIDBytes)
//}

func getNciFromCellGlobalID(cellGlobalID *e2sm_mho.CellGlobalId) uint64 {
	return cellGlobalID.GetNrCgi().GetNRcellIdentity().GetValue().GetValue()
}

func getPlmnIDBytesFromCellGlobalID(cellGlobalID *e2sm_mho.CellGlobalId) []byte {
	return cellGlobalID.GetNrCgi().GetPLmnIdentity().GetValue()
}

func getCGIFromIndicationHeader(header *e2sm_mho.E2SmMhoIndicationHeaderFormat1) string {
	nci := getNciFromCellGlobalID(header.GetCgi())
	plmnIDBytes := getPlmnIDBytesFromCellGlobalID(header.GetCgi())
	plmnID := plmnIDBytesToInt(plmnIDBytes)
	return plmnIDNciToCGI(plmnID, nci)
}

func getCGIFromMeasReportItem(measReport *e2sm_mho.E2SmMhoMeasurementReportItem) string {
	nci := getNciFromCellGlobalID(measReport.GetCgi())
	plmnIDBytes := getPlmnIDBytesFromCellGlobalID(measReport.GetCgi())
	plmnID := plmnIDBytesToInt(plmnIDBytes)
	return plmnIDNciToCGI(plmnID, nci)
}

func getRsrpFromMeasReport(servingNci uint64, measReport []*e2sm_mho.E2SmMhoMeasurementReportItem) (int32, map[string]int32) {
	var rsrpServing int32
	rsrpNeighbors := make(map[string]int32)

	for _, measReportItem := range measReport {
		if getNciFromCellGlobalID(measReportItem.GetCgi()) == servingNci {
			rsrpServing = measReportItem.GetRsrp().GetValue()
		} else {
			CGIString := getCGIFromMeasReportItem(measReportItem)
			rsrpNeighbors[CGIString] = measReportItem.GetRsrp().GetValue()
		}
	}

	return rsrpServing, rsrpNeighbors
}
