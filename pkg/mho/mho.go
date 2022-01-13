package mho

import (
	"context"
	"reflect"
	"sync"

	"github.com/RIMEDO-Labs/xapp-sdk/pkg/store"
	e2sm_mho "github.com/onosproject/onos-e2-sm/servicemodels/e2sm_mho/v1/e2sm-mho"
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

func NewController(indChan chan *E2NodeIndication, ueStore store.Store, cellStore store.Store) *Controller {
	// log.Info("Init MhoController")
	return &Controller{
		IndChan:   indChan,
		ueStore:   ueStore,
		cellStore: cellStore,
		cells:     make(map[string]*CellData),
	}
}

type Controller struct {
	IndChan   chan *E2NodeIndication
	ueStore   store.Store
	cellStore store.Store
	mu        sync.RWMutex
	cells     map[string]*CellData
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
					go c.handleRrcState(ctx, indHeader.GetIndicationHeaderFormat1(), indMessage.GetIndicationMessageFormat2())
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
	ueID := message.GetUeId().GetValue()
	cgi := getCGIFromIndicationHeader(header)
	log.Debugf("rx periodic ueID:%v cgi:%v", ueID, cgi)

	// get ue from store (create if it does not exist)
	var ueData *UeData
	newUe := false
	ueData = c.getUe(ctx, ueID)
	if ueData == nil {
		ueData = c.createUe(ctx, ueID)
		c.attachUe(ctx, ueData, cgi)
		newUe = true
	} else if ueData.CGIString != cgi {
		return
	}

	rsrpServing, rsrpNeighbors := getRsrpFromMeasReport(getNciFromCellGlobalID(header.GetCgi()), message.MeasReport)

	if !newUe && rsrpServing == ueData.RsrpServing && reflect.DeepEqual(rsrpNeighbors, ueData.RsrpNeighbors) {
		return
	}

	// update store
	ueData.RsrpServing, ueData.RsrpNeighbors = rsrpServing, rsrpNeighbors
	c.setUe(ctx, ueData)

}

func (c *Controller) handleMeasReport(ctx context.Context, header *e2sm_mho.E2SmMhoIndicationHeaderFormat1, message *e2sm_mho.E2SmMhoIndicationMessageFormat1, e2NodeID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ueID := message.GetUeId().GetValue()
	cgi := getCGIFromIndicationHeader(header)
	log.Debugf("rx a3 ueID:%v cgi:%v", ueID, cgi)

	// get ue from store (create if it does not exist)
	var ueData *UeData
	ueData = c.getUe(ctx, ueID)
	if ueData == nil {
		ueData = c.createUe(ctx, ueID)
		c.attachUe(ctx, ueData, cgi)
	} else if ueData.CGIString != cgi {
		return
	}

	// update info needed by control() later
	ueData.CGI = header.GetCgi()
	ueData.E2NodeID = e2NodeID

	// update rsrp
	ueData.RsrpServing, ueData.RsrpNeighbors = getRsrpFromMeasReport(getNciFromCellGlobalID(header.GetCgi()), message.MeasReport)

	// update store
	c.setUe(ctx, ueData)

	// do the real HO processing
	// c.HoCtrl.Input(ctx, header, message)

}

func (c *Controller) handleRrcState(ctx context.Context, header *e2sm_mho.E2SmMhoIndicationHeaderFormat1, message *e2sm_mho.E2SmMhoIndicationMessageFormat2) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ueID := message.GetUeId().GetValue()
	cgi := getCGIFromIndicationHeader(header)
	log.Debugf("rx rrc ueID:%v cgi:%v", ueID, cgi)

	// get ue from store (create if it does not exist)
	var ueData *UeData
	ueData = c.getUe(ctx, ueID)
	if ueData == nil {
		ueData = c.createUe(ctx, ueID)
		c.attachUe(ctx, ueData, cgi)
	} else if ueData.CGIString != cgi {
		return
	}

	// set rrc state (takes care of attach/detach as well)
	newRrcState := message.GetRrcStatus().String()
	c.setUeRrcState(ctx, ueData, newRrcState, cgi)

	// update store
	c.setUe(ctx, ueData)

}

func (c *Controller) createUe(ctx context.Context, ueID string) *UeData {
	if len(ueID) == 0 {
		panic("bad data")
	}
	ueData := &UeData{
		UeID:          ueID,
		CGIString:     "",
		RrcState:      e2sm_mho.Rrcstatus_name[int32(e2sm_mho.Rrcstatus_RRCSTATUS_CONNECTED)],
		RsrpNeighbors: make(map[string]int32),
	}
	_, err := c.ueStore.Put(ctx, ueID, *ueData)
	if err != nil {
		log.Warn(err)
	}

	return ueData
}

func (c *Controller) getUe(ctx context.Context, ueID string) *UeData {
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

func (c *Controller) setUe(ctx context.Context, ueData *UeData) {
	_, err := c.ueStore.Put(ctx, ueData.UeID, *ueData)
	if err != nil {
		panic("bad data")
	}
}

func (c *Controller) attachUe(ctx context.Context, ueData *UeData, cgi string) {
	// detach ue from current cell
	c.detachUe(ctx, ueData)

	// attach ue to new cell
	ueData.CGIString = cgi
	c.setUe(ctx, ueData)
	cell := c.getCell(ctx, cgi)
	if cell == nil {
		cell = c.createCell(ctx, cgi)
	}
	cell.Ues[ueData.UeID] = ueData
	c.setCell(ctx, cell)
}

func (c *Controller) detachUe(ctx context.Context, ueData *UeData) {
	for _, cell := range c.cells {
		delete(cell.Ues, ueData.UeID)
	}
}

func (c *Controller) setUeRrcState(ctx context.Context, ueData *UeData, newRrcState string, cgi string) {
	oldRrcState := ueData.RrcState

	if oldRrcState == e2sm_mho.Rrcstatus_name[int32(e2sm_mho.Rrcstatus_RRCSTATUS_CONNECTED)] &&
		newRrcState == e2sm_mho.Rrcstatus_name[int32(e2sm_mho.Rrcstatus_RRCSTATUS_IDLE)] {
		c.detachUe(ctx, ueData)
	} else if oldRrcState == e2sm_mho.Rrcstatus_name[int32(e2sm_mho.Rrcstatus_RRCSTATUS_IDLE)] &&
		newRrcState == e2sm_mho.Rrcstatus_name[int32(e2sm_mho.Rrcstatus_RRCSTATUS_CONNECTED)] {
		c.attachUe(ctx, ueData, cgi)
	}
	ueData.RrcState = newRrcState
}

func (c *Controller) createCell(ctx context.Context, cgi string) *CellData {
	if len(cgi) == 0 {
		panic("bad data")
	}
	cellData := &CellData{
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

func (c *Controller) getCell(ctx context.Context, cgi string) *CellData {
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

func (c *Controller) setCell(ctx context.Context, cellData *CellData) {
	if len(cellData.CGIString) == 0 {
		panic("bad data")
	}
	_, err := c.cellStore.Put(ctx, cellData.CGIString, *cellData)
	if err != nil {
		panic("bad data")
	}
}
