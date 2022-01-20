package mho

import (
	"context"
	"reflect"
	"strconv"
	"sync"

	e2api "github.com/onosproject/onos-api/go/onos/e2t/e2/v1beta1"
	e2sm_mho "github.com/onosproject/onos-e2-sm/servicemodels/e2sm_mho/v1/e2sm-mho"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	appConfig "github.com/onosproject/onos-mho/pkg/config"
	"github.com/onosproject/onos-mho/pkg/store"
	"github.com/onosproject/onos-ric-sdk-go/pkg/e2/indication"
	"google.golang.org/protobuf/proto"
)

var log = logging.GetLogger("MHO controller file [pkg/mho/mho.go]")

type UeData struct {
	UeID                string
	E2NodeID            string
	CGI                 *e2sm_mho.CellGlobalId
	CgiString           string
	RrcState            string
	servingNodeRSRP     int32
	neighbourNodesRSRPs map[string]int32
}

type CellData struct {
	CgiString              string
	CumulativeHandoversIn  int
	CumulativeHandoversOut int
	UEs                    map[string]*UeData
}

type E2NodeIndication struct {
	nodeID            string
	triggerType       e2sm_mho.MhoTriggerType
	indicationMessage indication.Indication
}

func NewE2NodeIndication(nodeID string, triggerType e2sm_mho.MhoTriggerType, indicationMessage indication.Indication) *E2NodeIndication {

	log.Info("New E2 node indication was created")

	return &E2NodeIndication{

		nodeID:            nodeID,
		triggerType:       triggerType,
		indicationMessage: indicationMessage,
	}

}

type MhoController struct {
	indicationChannel      chan *E2NodeIndication
	controlRequestChannels map[string]chan *e2api.ControlMessage
	ueStore                store.Store
	cellStore              store.Store
	mutex                  sync.RWMutex
	cells                  map[string]*CellData
}

func NewMhoController(cfg appConfig.Config, indChan chan *E2NodeIndication, ctrlReqChans map[string]chan *e2api.ControlMessage, ueStore store.Store, cellStore store.Store) *MhoController {

	log.Info("Created new MHO Controller object")

	return &MhoController{

		indicationChannel:      indChan,
		controlRequestChannels: ctrlReqChans,
		ueStore:                ueStore,
		cellStore:              cellStore,
		cells:                  make(map[string]*CellData),
	}

}

func (self *MhoController) Run(context context.Context) {

	log.Info("MHO Controller is running!")
	go self.listenIndicationChannel(context)

}

func (self *MhoController) listenIndicationChannel(context context.Context) {

	for e2NodeIndication := range self.indicationChannel {

		indicationHeaderByte := e2NodeIndication.indicationMessage.Payload.Header
		indicationMessageByte := e2NodeIndication.indicationMessage.Payload.Message
		e2NodeID := e2NodeIndication.nodeID
		indicationHeader := e2sm_mho.E2SmMhoIndicationHeader{}
		if err := proto.Unmarshal(indicationHeaderByte, &indicationHeader); err == nil {

			indicationMessage := e2sm_mho.E2SmMhoIndicationMessage{}
			if err := proto.Unmarshal(indicationMessageByte, &indicationMessage); err == nil {

				switch typeOfIndicationMessage := indicationMessage.E2SmMhoIndicationMessage.(type) {

				case *e2sm_mho.E2SmMhoIndicationMessage_IndicationMessageFormat1:

					if e2NodeIndication.triggerType == e2sm_mho.MhoTriggerType_MHO_TRIGGER_TYPE_UPON_RCV_MEAS_REPORT {

						go self.handleMeasurementReportTrigger(context, indicationHeader.GetIndicationHeaderFormat1(), indicationMessage.GetIndicationMessageFormat1(), e2NodeID)

					} else if e2NodeIndication.triggerType == e2sm_mho.MhoTriggerType_MHO_TRIGGER_TYPE_PERIODIC {

						go self.handlePeriodicMeasurementReportTrigger(context, indicationHeader.GetIndicationHeaderFormat1(), indicationMessage.GetIndicationMessageFormat1(), e2NodeID)

					}

				case *e2sm_mho.E2SmMhoIndicationMessage_IndicationMessageFormat2:

					go self.handleRrcStateChangedTrigger(context, indicationHeader.GetIndicationHeaderFormat1(), indicationMessage.GetIndicationMessageFormat2())

				default:

					log.Warnf("Undefined MHO trigger type of the indication message: %v", typeOfIndicationMessage)

				}

			} else {

				log.Error("Problem with unmarshaling message of the indication [listenIndicationChannel()].")

			}

		} else {

			log.Error("Problem with unmarshaling header of the indication [listenIndicationChannel()].")

		}

	}

}

func (self *MhoController) handleMeasurementReportTrigger(context context.Context, header *e2sm_mho.E2SmMhoIndicationHeaderFormat1, message *e2sm_mho.E2SmMhoIndicationMessageFormat1, e2NodeID string) {

	// Lock the store to make changes and unlock it when the function process are done (defer).
	self.mutex.Lock()
	defer self.mutex.Unlock()

	// Determine the identifier for UE and for handling BS
	ueID := message.GetUeId().GetValue()
	nodeCGI := getCgiFromIndicationHeader(header)
	log.Info("Handling the measurement report for UE with ID: %v, assigned to BS with CGI: %v.", ueID, nodeCGI)

	// Get the UE object from the store or create it when it doesn't exist
	ueData := self.getUe(context, ueID)
	if ueData == nil {

		ueData = self.createUe(context, ueID)
		self.attachUe(context, ueData, nodeCGI)

	} else if ueData.CgiString != nodeCGI {

		log.Warn("UE it's not associated with expected BS node (CGI: %v).", nodeCGI)
		return

	}

	ueData.CGI = header.GetCgi()
	ueData.E2NodeID = e2NodeID
	ueData.servingNodeRSRP, ueData.neighbourNodesRSRPs = getRsrpLevelsFromMeasurementReport(getNciFromCgi(header.GetCgi()), message.GetMeasReport())

	self.setUe(context, ueData)

	// DO THE HANDOVER RIGHT HERE

}

func (self *MhoController) handlePeriodicMeasurementReportTrigger(context context.Context, header *e2sm_mho.E2SmMhoIndicationHeaderFormat1, message *e2sm_mho.E2SmMhoIndicationMessageFormat1, e2NodeID string) {

	// Lock the store to make changes and unlock it when the function process are done (defer).
	self.mutex.Lock()
	defer self.mutex.Unlock()

	// Determine the identifier for UE and for handling BS
	ueID := message.GetUeId().GetValue()
	nodeCGI := getCgiFromIndicationHeader(header)
	log.Info("Handling the periodic measurement reports for UE with ID: %v, assigned to BS with CGI: %v.", ueID, nodeCGI)

	// Get the UE object from the store or create it when it doesn't exist
	newUeCreated := false
	ueData := self.getUe(context, ueID)
	if ueData == nil {

		ueData = self.createUe(context, ueID)
		self.attachUe(context, ueData, nodeCGI)
		newUeCreated = true

	} else if ueData.CgiString != nodeCGI {

		log.Warn("UE it's not associated with expected BS node (CGI: %v).", nodeCGI)
		return

	}

	servingNodeRSRP, neighbourNodesRSRPs := getRsrpLevelsFromMeasurementReport(getNciFromCgi(header.GetCgi()), message.GetMeasReport())

	if !newUeCreated && ueData.servingNodeRSRP == servingNodeRSRP && reflect.DeepEqual(neighbourNodesRSRPs, ueData.neighbourNodesRSRPs) {
		return
	}

	// Update the store
	ueData.servingNodeRSRP, ueData.neighbourNodesRSRPs = servingNodeRSRP, neighbourNodesRSRPs
	self.setUe(context, ueData)

}

func (self *MhoController) handleRrcStateChangedTrigger(context context.Context, header *e2sm_mho.E2SmMhoIndicationHeaderFormat1, message *e2sm_mho.E2SmMhoIndicationMessageFormat2) {

	// Lock the store to make changes and unlock it when the function process are done (defer).
	self.mutex.Lock()
	defer self.mutex.Unlock()

	// Determine the identifier for UE and for handling BS
	ueID := message.GetUeId().GetValue()
	nodeCGI := getCgiFromIndicationHeader(header)
	log.Info("Handling the RRC state change trigger for UE with ID: %v, assigned to BS with CGI: %v.", ueID, nodeCGI)

	// Get the UE object from the store or create it when it doesn't exist
	ueData := self.getUe(context, ueID)
	if ueData == nil {

		ueData = self.createUe(context, ueID)
		self.attachUe(context, ueData, nodeCGI)

	} else if ueData.CgiString != nodeCGI {

		return

	}

	// Set RRC state of the UE object (takes care of attach/detach as well)
	newRrcState := message.GetRrcStatus().String()
	self.setUeRrcState(context, ueData, newRrcState, nodeCGI)

	// Update the store
	self.setUe(context, ueData)

}

func (self *MhoController) createUe(context context.Context, ueID string) *UeData {

	if len(ueID) == 0 {

		panic("Bad data was passed when creating UE object [createUE()].")

	}

	ueData := &UeData{

		UeID:                ueID,
		CgiString:           "",
		RrcState:            e2sm_mho.Rrcstatus_name[int32(e2sm_mho.Rrcstatus_RRCSTATUS_CONNECTED)],
		neighbourNodesRSRPs: make(map[string]int32),
	}

	if _, err := self.ueStore.Put(context, ueID, *ueData); err != nil {

		log.Warn("Error occured when new UE object was put to the store [createUe()].", err)

	}

	return ueData
}

func (self *MhoController) getUe(context context.Context, ueID string) *UeData {

	ue, err := self.ueStore.Get(context, ueID)
	if err != nil || ue == nil {

		return nil

	}

	temp := ue.Value.(UeData)
	ueData := &temp
	if ueData.UeID != ueID {

		panic("Bad data was passed when getting the UE object [getUe()].")

	}

	return ueData

}

func (self *MhoController) setUe(context context.Context, ueData *UeData) {

	if _, err := self.ueStore.Put(context, ueData.UeID, *ueData); err != nil {

		panic("Bad data was passed to the UE store [setUe()].")

	}

}

func (self *MhoController) attachUe(context context.Context, ueData *UeData, CGI string) {

	// detach ue from current cell
	self.detachUe(context, ueData)

	// attach ue to new cell
	ueData.CgiString = CGI
	self.setUe(context, ueData)
	cell := self.getCell(context, CGI)

	if cell == nil {

		cell = self.createCell(context, CGI)

	}

	cell.UEs[ueData.UeID] = ueData
	self.setCell(context, cell)

}

func (self *MhoController) detachUe(context context.Context, ueData *UeData) {

	for _, cell := range self.cells {

		delete(cell.UEs, ueData.UeID)

	}

}

func (self *MhoController) setUeRrcState(context context.Context, ueData *UeData, newRrcState string, CGI string) {

	oldRrcState := ueData.RrcState

	if oldRrcState == e2sm_mho.Rrcstatus_name[int32(e2sm_mho.Rrcstatus_RRCSTATUS_CONNECTED)] &&
		newRrcState == e2sm_mho.Rrcstatus_name[int32(e2sm_mho.Rrcstatus_RRCSTATUS_IDLE)] {

		self.detachUe(context, ueData)

	} else if oldRrcState == e2sm_mho.Rrcstatus_name[int32(e2sm_mho.Rrcstatus_RRCSTATUS_IDLE)] &&
		newRrcState == e2sm_mho.Rrcstatus_name[int32(e2sm_mho.Rrcstatus_RRCSTATUS_CONNECTED)] {

		self.attachUe(context, ueData, CGI)

	}

	ueData.RrcState = newRrcState

}

func (self *MhoController) createCell(context context.Context, CGI string) *CellData {

	if len(CGI) == 0 {

		panic("Bad data was passed when creating new BS Cell object [createCell()].")

	}

	cellData := &CellData{

		CgiString: CGI,
		UEs:       make(map[string]*UeData),
	}

	if _, err := self.cellStore.Put(context, CGI, *cellData); err != nil {

		panic("Bad data was passed to the BS Cell store [createCell()].")

	}

	self.cells[cellData.CgiString] = cellData
	return cellData

}

func (self *MhoController) getCell(context context.Context, CGI string) *CellData {

	cell, err := self.cellStore.Get(context, CGI)
	if err != nil || cell == nil {

		return nil

	}

	temp := cell.Value.(CellData)
	if temp.CgiString != CGI {

		panic("Bad data was passed when getting BS Cell object [getCell()].")

	}

	cellData := &temp
	return cellData

}

func (self *MhoController) setCell(context context.Context, cellData *CellData) {

	if len(cellData.CgiString) == 0 {

		panic("bad data")

	}

	if _, err := self.cellStore.Put(context, cellData.CgiString, *cellData); err != nil {

		panic("Bad data was passed to the BS Cell store [setCell()].")

	}

}

func plmnIdBytesToInt(b []byte) uint64 {

	return uint64(b[2])<<16 | uint64(b[1])<<8 | uint64(b[0])

}

func plmnIdNciToCgi(plmnID uint64, nci uint64) string {

	return strconv.FormatInt(int64(plmnID<<36|(nci&0xfffffffff)), 16)

}

func getNciFromCgi(cellGlobalID *e2sm_mho.CellGlobalId) uint64 {

	return cellGlobalID.GetNrCgi().GetNRcellIdentity().GetValue().GetValue()

}

func getPlmnIDBytesFromCgi(cellGlobalID *e2sm_mho.CellGlobalId) []byte {
	return cellGlobalID.GetNrCgi().GetPLmnIdentity().GetValue()
}

func getCgiFromIndicationHeader(header *e2sm_mho.E2SmMhoIndicationHeaderFormat1) string {

	nci := getNciFromCgi(header.GetCgi())
	plmnIDBytes := getPlmnIDBytesFromCgi(header.GetCgi())
	plmnID := plmnIdBytesToInt(plmnIDBytes)

	return plmnIdNciToCgi(plmnID, nci)

}

func getCgiFromMeasurementReportItem(measReport *e2sm_mho.E2SmMhoMeasurementReportItem) string {

	nci := getNciFromCgi(measReport.GetCgi())
	plmnIDBytes := getPlmnIDBytesFromCgi(measReport.GetCgi())
	plmnID := plmnIdBytesToInt(plmnIDBytes)

	return plmnIdNciToCgi(plmnID, nci)

}

func getRsrpLevelsFromMeasurementReport(servingNci uint64, measReport []*e2sm_mho.E2SmMhoMeasurementReportItem) (int32, map[string]int32) {

	var rsrpServing int32
	rsrpNeighbors := make(map[string]int32)

	for _, measReportItem := range measReport {
		if getNciFromCgi(measReportItem.GetCgi()) == servingNci {
			rsrpServing = measReportItem.GetRsrp().GetValue()
		} else {
			CGIString := getCgiFromMeasurementReportItem(measReportItem)
			rsrpNeighbors[CGIString] = measReportItem.GetRsrp().GetValue()
		}
	}

	return rsrpServing, rsrpNeighbors

}
