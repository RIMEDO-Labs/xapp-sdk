package mho

import (
	"context"
	"reflect"
	"strconv"
	"sync"

	"github.com/RIMEDO-Labs/xapp-sdk/pkg/store"
	policyAPI "github.com/onosproject/onos-a1-dm/go/policy_schemas/traffic_steering_preference/v2"
	e2sm_mho "github.com/onosproject/onos-e2-sm/servicemodels/e2sm_mho_go/v2/e2sm-mho-go"
	e2sm_v2_ies "github.com/onosproject/onos-e2-sm/servicemodels/e2sm_mho_go/v2/e2sm-v2-ies"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	"github.com/onosproject/onos-ric-sdk-go/pkg/e2/indication"
	"google.golang.org/protobuf/proto"
)

var log = logging.GetLogger("mho")

type E2NodeIndication struct {
	NodeID      string
	TriggerType e2sm_mho.MhoTriggerType
	IndMsg      indication.Indication
}

func NewController(indChan chan *E2NodeIndication, ueStore store.Store, cellStore store.Store, onosPolicyStore store.Store, policies map[string]*PolicyData) *Controller {
	// log.Info("Init MhoController")

	return &Controller{
		IndChan:         indChan,
		ueStore:         ueStore,
		cellStore:       cellStore,
		onosPolicyStore: onosPolicyStore,
		cells:           make(map[string]*CellData),
		policies:        policies,
		// policyManager:   tspolicy.NewPolicyManager("", &policies),
	}
}

type Controller struct {
	IndChan         chan *E2NodeIndication
	ueStore         store.Store
	cellStore       store.Store
	onosPolicyStore store.Store
	mu              sync.RWMutex
	cells           map[string]*CellData
	policies        map[string]*PolicyData
	// policyManager   *tspolicy.PolicyManager
}

func (c *Controller) Run(ctx context.Context) {
	// log.Info("Start MhoController")
	go c.listenIndChan(ctx)
}

func (c *Controller) listenIndChan(ctx context.Context) {
	var err error
	for indMsg := range c.IndChan {

		indHeaderByte := indMsg.IndMsg.Payload.Header
		indMessageByte := indMsg.IndMsg.Payload.Message
		e2NodeID := indMsg.NodeID

		indHeader := e2sm_mho.E2SmMhoIndicationHeader{}
		if err = proto.Unmarshal(indHeaderByte, &indHeader); err == nil {
			indMessage := e2sm_mho.E2SmMhoIndicationMessage{}
			if err = proto.Unmarshal(indMessageByte, &indMessage); err == nil {
				switch x := indMessage.E2SmMhoIndicationMessage.(type) {
				case *e2sm_mho.E2SmMhoIndicationMessage_IndicationMessageFormat1:
					if indMsg.TriggerType == e2sm_mho.MhoTriggerType_MHO_TRIGGER_TYPE_UPON_RCV_MEAS_REPORT {
						go c.handleMeasReport(ctx, indHeader.GetIndicationHeaderFormat1(), indMessage.GetIndicationMessageFormat1(), e2NodeID)
					} else if indMsg.TriggerType == e2sm_mho.MhoTriggerType_MHO_TRIGGER_TYPE_PERIODIC {
						go c.handlePeriodicReport(ctx, indHeader.GetIndicationHeaderFormat1(), indMessage.GetIndicationMessageFormat1(), e2NodeID)
					}
				case *e2sm_mho.E2SmMhoIndicationMessage_IndicationMessageFormat2:
					go c.handleRrcState(ctx, indHeader.GetIndicationHeaderFormat1(), indMessage.GetIndicationMessageFormat2(), e2NodeID)
				default:
					log.Warnf("Unknown MHO indication message format, indication message: %v", x)
				}
			}
		}
		if err != nil {
			log.Error(err)
		}
	}
}

func (c *Controller) handlePeriodicReport(ctx context.Context, header *e2sm_mho.E2SmMhoIndicationHeaderFormat1, message *e2sm_mho.E2SmMhoIndicationMessageFormat1, e2NodeID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ueID, err := GetUeID(message.GetUeId())
	if err != nil {
		log.Errorf("handlePeriodicReport() couldn't extract UeID: %v", err)
	}
	cgi := GetCGIFromIndicationHeader(header)
	cgiObject := header.GetCgi()
	log.Debugf("rx periodic ueID:%v cgi:%v", ueID, cgi)

	// get ue from store (create if it does not exist)
	var ueData *UeData
	newUe := false
	ueData = c.GetUe(ctx, strconv.Itoa(int(ueID)))
	if ueData == nil {
		ueData = c.CreateUe(ctx, strconv.Itoa(int(ueID)))
		c.AttachUe(ctx, ueData, cgi, cgiObject)
		newUe = true
	} else if ueData.CGIString != cgi {
		return
	}

	ueData.CGI = cgiObject
	ueData.E2NodeID = e2NodeID

	rsrpServing, rsrpNeighbors := c.GetRsrpFromMeasReport(ctx, GetNciFromCellGlobalID(header.GetCgi()), message.MeasReport)

	// update fiveQi
	ueData.FiveQiServing, ueData.FiveQiNeighbors = c.GetFiveQiFromMeasReport(ctx, GetNciFromCellGlobalID(header.GetCgi()), message.MeasReport)

	if !newUe && rsrpServing == ueData.RsrpServing && reflect.DeepEqual(rsrpNeighbors, ueData.RsrpNeighbors) {
		return
	}

	// update store
	ueData.RsrpServing, ueData.RsrpNeighbors = rsrpServing, rsrpNeighbors
	c.SetUe(ctx, ueData)

}

func (c *Controller) handleMeasReport(ctx context.Context, header *e2sm_mho.E2SmMhoIndicationHeaderFormat1, message *e2sm_mho.E2SmMhoIndicationMessageFormat1, e2NodeID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ueID, err := GetUeID(message.GetUeId())
	if err != nil {
		log.Errorf("handleMeasReport() couldn't extract UeID: %v", err)
	}
	cgi := GetCGIFromIndicationHeader(header)
	cgiObject := header.GetCgi()
	log.Debugf("rx a3 ueID:%v cgi:%v", ueID, cgi)

	// get ue from store (create if it does not exist)
	var ueData *UeData
	ueData = c.GetUe(ctx, strconv.Itoa(int(ueID)))
	if ueData == nil {
		ueData = c.CreateUe(ctx, strconv.Itoa(int(ueID)))
		c.AttachUe(ctx, ueData, cgi, cgiObject)
	} else if ueData.CGIString != cgi {
		return
	}

	// update info needed by control() later
	ueData.CGI = cgiObject
	ueData.E2NodeID = e2NodeID

	// update rsrp
	ueData.RsrpServing, ueData.RsrpNeighbors = c.GetRsrpFromMeasReport(ctx, GetNciFromCellGlobalID(header.GetCgi()), message.MeasReport)

	// update fiveQi
	ueData.FiveQiServing, ueData.FiveQiNeighbors = c.GetFiveQiFromMeasReport(ctx, GetNciFromCellGlobalID(header.GetCgi()), message.MeasReport)

	// update store
	c.SetUe(ctx, ueData)

	// do the real HO processing
	// c.HoCtrl.Input(ctx, header, message)

}

func (c *Controller) handleRrcState(ctx context.Context, header *e2sm_mho.E2SmMhoIndicationHeaderFormat1, message *e2sm_mho.E2SmMhoIndicationMessageFormat2, e2NodeID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ueID, err := GetUeID(message.GetUeId())
	if err != nil {
		log.Errorf("handleRrcState() couldn't extract UeID: %v", err)
	}
	cgi := GetCGIFromIndicationHeader(header)
	cgiObject := header.GetCgi()
	log.Debugf("rx rrc ueID:%v cgi:%v", ueID, cgi)

	// get ue from store (create if it does not exist)
	var ueData *UeData
	ueData = c.GetUe(ctx, strconv.Itoa(int(ueID)))
	if ueData == nil {
		ueData = c.CreateUe(ctx, strconv.Itoa(int(ueID)))
		c.AttachUe(ctx, ueData, cgi, cgiObject)
	} else if ueData.CGIString != cgi {
		return
	}

	ueData.CGI = cgiObject
	ueData.E2NodeID = e2NodeID

	// set rrc state (takes care of attach/detach as well)
	newRrcState := message.GetRrcStatus().String()
	c.SetUeRrcState(ctx, ueData, newRrcState, cgi, cgiObject)

	// update store
	c.SetUe(ctx, ueData)

}

func (c *Controller) CreateUe(ctx context.Context, ueID string) *UeData {
	if len(ueID) == 0 {
		panic("bad data")
	}
	ueData := &UeData{
		UeID:          ueID,
		CGIString:     "",
		RrcState:      e2sm_mho.Rrcstatus_name[int32(e2sm_mho.Rrcstatus_RRCSTATUS_CONNECTED)],
		RsrpNeighbors: make(map[*e2sm_v2_ies.Cgi]int32),
	}
	_, err := c.ueStore.Put(ctx, ueID, *ueData)
	if err != nil {
		log.Warn(err)
	}

	return ueData
}

func (c *Controller) GetUe(ctx context.Context, ueID string) *UeData {
	var ueData *UeData
	u, err := c.ueStore.Get(ctx, ueID)
	if err != nil || u == nil {
		return nil
	}
	t := u.Value.(UeData)
	ueData = &t
	if ueData.UeID != ueID {
		panic("bad data")
	}

	return ueData
}

func (c *Controller) SetUe(ctx context.Context, ueData *UeData) {
	_, err := c.ueStore.Put(ctx, ueData.UeID, *ueData)
	if err != nil {
		panic("bad data")
	}
}

func (c *Controller) AttachUe(ctx context.Context, ueData *UeData, cgi string, cgiObject *e2sm_v2_ies.Cgi) {
	// detach ue from current cell
	c.DetachUe(ctx, ueData)

	// attach ue to new cell
	ueData.CGIString = cgi
	c.SetUe(ctx, ueData)
	cell := c.GetCell(ctx, cgi)
	if cell == nil {
		cell = c.CreateCell(ctx, cgi, cgiObject)
	}
	cell.Ues[ueData.UeID] = ueData
	c.SetCell(ctx, cell)
}

func (c *Controller) DetachUe(ctx context.Context, ueData *UeData) {
	for _, cell := range c.cells {
		delete(cell.Ues, ueData.UeID)
	}
}

func (c *Controller) SetUeRrcState(ctx context.Context, ueData *UeData, newRrcState string, cgi string, cgiObject *e2sm_v2_ies.Cgi) {
	oldRrcState := ueData.RrcState

	if oldRrcState == e2sm_mho.Rrcstatus_name[int32(e2sm_mho.Rrcstatus_RRCSTATUS_CONNECTED)] &&
		newRrcState == e2sm_mho.Rrcstatus_name[int32(e2sm_mho.Rrcstatus_RRCSTATUS_IDLE)] {
		c.DetachUe(ctx, ueData)
	} else if oldRrcState == e2sm_mho.Rrcstatus_name[int32(e2sm_mho.Rrcstatus_RRCSTATUS_IDLE)] &&
		newRrcState == e2sm_mho.Rrcstatus_name[int32(e2sm_mho.Rrcstatus_RRCSTATUS_CONNECTED)] {
		c.AttachUe(ctx, ueData, cgi, cgiObject)
	}
	ueData.RrcState = newRrcState
}

func (c *Controller) CreateCell(ctx context.Context, cgi string, cgiObject *e2sm_v2_ies.Cgi) *CellData {
	if len(cgi) == 0 {
		panic("bad data")
	}
	cellData := &CellData{
		CGI:       cgiObject,
		CGIString: cgi,
		Ues:       make(map[string]*UeData),
	}
	_, err := c.cellStore.Put(ctx, cgi, *cellData)
	if err != nil {
		panic("bad data")
	}
	c.cells[cellData.CGIString] = cellData
	return cellData
}

func (c *Controller) GetCell(ctx context.Context, cgi string) *CellData {
	var cellData *CellData
	cell, err := c.cellStore.Get(ctx, cgi)
	if err != nil || cell == nil {
		return nil
	}
	t := cell.Value.(CellData)
	if t.CGIString != cgi {
		panic("bad data")
	}
	cellData = &t
	return cellData
}

func (c *Controller) SetCell(ctx context.Context, cellData *CellData) {
	if len(cellData.CGIString) == 0 {
		panic("bad data")
	}
	_, err := c.cellStore.Put(ctx, cellData.CGIString, *cellData)
	if err != nil {
		panic("bad data")
	}
	c.cells[cellData.CGIString] = cellData
}

func (c *Controller) GetFiveQiFromMeasReport(ctx context.Context, servingNci uint64, measReport []*e2sm_mho.E2SmMhoMeasurementReportItem) (int32, map[*e2sm_v2_ies.Cgi]int32) {
	var fiveQiServing int32
	fiveQiNeighbors := make(map[*e2sm_v2_ies.Cgi]int32)

	for _, measReportItem := range measReport {

		if GetNciFromCellGlobalID(measReportItem.GetCgi()) == servingNci {
			fiveQi := measReportItem.GetFiveQi()
			if fiveQi != nil {
				fiveQiServing = fiveQi.GetValue()
			} else {
				fiveQiServing = -1
			}
		} else {
			CGIString := GetCGIFromMeasReportItem(measReportItem)
			fiveQi := measReportItem.GetFiveQi()
			if fiveQi != nil {
				fiveQiNeighbors[measReportItem.GetCgi()] = fiveQi.GetValue()
			} else {
				fiveQiNeighbors[measReportItem.GetCgi()] = -1
			}
			cell := c.GetCell(ctx, CGIString)
			if cell == nil {
				cell = c.CreateCell(ctx, CGIString, measReportItem.GetCgi())
				c.SetCell(ctx, cell)
			}
		}
	}

	return fiveQiServing, fiveQiNeighbors
}

func (c *Controller) GetRsrpFromMeasReport(ctx context.Context, servingNci uint64, measReport []*e2sm_mho.E2SmMhoMeasurementReportItem) (int32, map[*e2sm_v2_ies.Cgi]int32) {
	var rsrpServing int32
	rsrpNeighbors := make(map[*e2sm_v2_ies.Cgi]int32)

	for _, measReportItem := range measReport {
		/*
			five := measReportItem.GetFiveQi()
			if five != nil {
				log.Info("COS JEST") // .GetValue()
			} else {
				log.Info("NIC NIE MA")
			}
		*/

		if GetNciFromCellGlobalID(measReportItem.GetCgi()) == servingNci {
			rsrpServing = measReportItem.GetRsrp().GetValue()
		} else {
			CGIString := GetCGIFromMeasReportItem(measReportItem)
			rsrpNeighbors[measReportItem.GetCgi()] = measReportItem.GetRsrp().GetValue()
			cell := c.GetCell(ctx, CGIString)
			if cell == nil {
				cell = c.CreateCell(ctx, CGIString, measReportItem.GetCgi())
				c.SetCell(ctx, cell)
			}
		}
	}

	return rsrpServing, rsrpNeighbors
}

func (c *Controller) CreatePolicy(ctx context.Context, key string, policy *policyAPI.API) *PolicyData {
	if len(key) == 0 {
		panic("bad data")
	}
	policyData := &PolicyData{
		Key:        key,
		API:        policy,
		IsEnforced: false,
	}
	_, err := c.onosPolicyStore.Put(ctx, key, *policyData)
	if err != nil {
		log.Panic("bad data")
	}
	c.policies[policyData.Key] = policyData
	return policyData
}

func (c *Controller) GetPolicy(ctx context.Context, key string) *PolicyData {
	var policy *PolicyData
	p, err := c.onosPolicyStore.Get(ctx, key)
	if err != nil || p == nil {
		return nil
	}
	t := p.Value.(PolicyData)
	if t.Key != key {
		panic("bad data")
	}
	policy = &t

	return policy
}

func (c *Controller) SetPolicy(ctx context.Context, key string, policy *PolicyData) {
	_, err := c.onosPolicyStore.Put(ctx, key, *policy)
	if err != nil {
		panic("bad data")
	}
	c.policies[policy.Key] = policy
}

func (c *Controller) DeletePolicy(ctx context.Context, key string) {
	if err := c.onosPolicyStore.Delete(ctx, key); err != nil {
		panic("bad data")
	}
}

func (c *Controller) GetPolicyStore() *store.Store {
	return &c.onosPolicyStore
}
