package manager

import (
	"context"
	"net"
	"sync"

	"github.com/RIMEDO-Labs/xapp-sdk/pkg/mho"
	"github.com/RIMEDO-Labs/xapp-sdk/pkg/rnib"
	"github.com/RIMEDO-Labs/xapp-sdk/pkg/southbound/e2"
	"github.com/RIMEDO-Labs/xapp-sdk/pkg/store"
	"github.com/RIMEDO-Labs/xapp-sdk/pkg/tspolicy"
	e2tAPI "github.com/onosproject/onos-api/go/onos/e2t/e2"
	e2api "github.com/onosproject/onos-api/go/onos/e2t/e2/v1beta1"
	e2sm_mho "github.com/onosproject/onos-e2-sm/servicemodels/e2sm_mho/v1/e2sm-mho"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	control "github.com/onosproject/onos-mho/pkg/mho"
	"google.golang.org/grpc"
)

var log = logging.GetLogger("manager")

type Config struct {
	AppID              string
	E2tAddress         string
	E2tPort            int
	TopoAddress        string
	TopoPort           int
	SMName             string
	SMVersion          string
	TSPolicySchemePath string
}

func NewManager(config Config) *Manager {

	ueStore := store.NewStore()
	cellStore := store.NewStore()

	policyMap := tspolicy.NewTsPolicyMap(config.TSPolicySchemePath)

	indCh := make(chan *mho.E2NodeIndication)
	ctrlReqChs := make(map[string]chan *e2api.ControlMessage)

	options := e2.Options{
		AppID:       config.AppID,
		E2tAddress:  config.E2tAddress,
		E2tPort:     config.E2tPort,
		TopoAddress: config.TopoAddress,
		TopoPort:    config.TopoPort,
		SMName:      config.SMName,
		SMVersion:   config.SMVersion,
	}

	e2Manager, err := e2.NewManager(options, indCh, ctrlReqChs)
	if err != nil {
		log.Warn(err)
	}

	manager := &Manager{
		e2Manager:  e2Manager,
		mhoCtrl:    mho.NewController(indCh, ueStore, cellStore),
		PolicyMap:  *policyMap,
		ueStore:    ueStore,
		cellStore:  cellStore,
		ctrlReqChs: ctrlReqChs,
	}
	return manager
}

type Manager struct {
	e2Manager  e2.Manager
	mhoCtrl    *mho.Controller
	PolicyMap  tspolicy.TsPolicyMap
	ueStore    store.Store
	cellStore  store.Store
	ctrlReqChs map[string]chan *e2api.ControlMessage
	mutex      sync.RWMutex
}

func (m *Manager) Run() {
	//log.Info("Running Manager")
	if err := m.start(); err != nil {
		log.Fatal("Unable to run Manager", err)
	}
}

func (m *Manager) start() error {
	go m.startNorthboundServer()
	err := m.e2Manager.Start()
	if err != nil {
		log.Warn(err)
		return err
	}

	go m.mhoCtrl.Run(context.Background())

	/*
		go func() {
			ctx := context.Background()
			for {
				time.Sleep(2 * time.Second)
				log.Info("trying to get something...")
				chEntries := make(chan *store.Entry, 1024)
				err = m.ueStore.Entries(ctx, chEntries)
				if err != nil {
					log.Warn(err)
				}
				for entry := range chEntries {
					ueData := entry.Value.(mho.UeData)
					log.Infof("UeID:%v, CGI: %v, RSRP: %v", ueData.UeID, ueData.CGIString, ueData.RsrpServing)
					log.Info("Neighbours:")
					for key, ele := range ueData.RsrpNeighbors {
						log.Infof(" - CGI:%v, RSRP:%v", key, ele)
					}
					log.Info(" ")
				}

			}
		}()
	*/

	return nil
}

func (m *Manager) startNorthboundServer() error {
	// ONLY FOR NOW TO LISTEN ON PORT
	lis, err := net.Listen("tcp", ":5150")
	if err != nil {
		log.Fatal("Failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatal("Failed to serve: %s", err)
	}
	return nil
}

func (m *Manager) GetUEs(ctx context.Context) map[string]mho.UeData {
	output := make(map[string]mho.UeData)
	chEntries := make(chan *store.Entry, 1024)
	err := m.ueStore.Entries(ctx, chEntries)
	if err != nil {
		log.Warn(err)
		return output
	}
	for entry := range chEntries {
		ueData := entry.Value.(mho.UeData)
		output[ueData.UeID] = ueData
	}
	return output
}

func (m *Manager) GetCells(ctx context.Context) map[string]mho.CellData {
	output := make(map[string]mho.CellData)
	chEntries := make(chan *store.Entry, 1024)
	err := m.cellStore.Entries(ctx, chEntries)
	if err != nil {
		log.Warn(err)
		return output
	}
	for entry := range chEntries {
		cellData := entry.Value.(mho.CellData)
		output[cellData.CGIString] = cellData
	}
	return output
}

func (m *Manager) GetCellTypes(ctx context.Context) map[string]rnib.Cell {
	return m.e2Manager.GetCellTypes(ctx)
}

func (m *Manager) SetCellType(ctx context.Context, cellID string, cellType string) error {
	return m.e2Manager.SetCellType(ctx, cellID, cellType)
}

func (m *Manager) GetCell(ctx context.Context, CGI string) *mho.CellData {

	return m.mhoCtrl.GetCell(ctx, CGI)

}

func (m *Manager) SetCell(ctx context.Context, cell *mho.CellData) {

	m.mhoCtrl.SetCell(ctx, cell)

}

func (m *Manager) AttachUe(ctx context.Context, ue *mho.UeData, CGI string, cgiObject *e2sm_mho.CellGlobalId) {

	m.mhoCtrl.AttachUe(ctx, ue, CGI, cgiObject)

}

func (m *Manager) GetUe(ctx context.Context, ueID string) *mho.UeData {

	return m.mhoCtrl.GetUe(ctx, ueID)

}

func (m *Manager) SetUe(ctx context.Context, ueData *mho.UeData) {

	m.mhoCtrl.SetUe(ctx, ueData)

}

func (m *Manager) GetControlChannelsMap(ctx context.Context) map[string]chan *e2api.ControlMessage {
	return m.ctrlReqChs
}

func (m *Manager) SwitchUeBetweenCells(ctx context.Context, ueID string, targetCellCGI string, controlChannelsMap map[string]chan *e2api.ControlMessage) {

	m.mutex.Lock()
	defer m.mutex.Unlock()

	availableUes := m.GetUEs(ctx)
	chosenUe := availableUes[ueID]

	targetCell := m.GetCell(ctx, targetCellCGI)
	servingCell := m.GetCell(ctx, chosenUe.CGIString)

	targetCell.CumulativeHandoversOut++
	servingCell.CumulativeHandoversIn++

	m.AttachUe(ctx, &chosenUe, targetCellCGI, targetCell.CGI)

	m.SetCell(ctx, targetCell)
	m.SetCell(ctx, servingCell)

	controlChannel := controlChannelsMap[chosenUe.E2NodeID]

	controlHandler := &control.E2SmMhoControlHandler{
		NodeID:            chosenUe.E2NodeID,
		ControlAckRequest: e2tAPI.ControlAckRequest_NO_ACK,
	}

	ueIdentity := e2sm_mho.UeIdentity{
		Value: chosenUe.UeID,
	}

	servingPlmnIDBytes := servingCell.CGI.GetNrCgi().GetPLmnIdentity().GetValue()
	servingNCI := servingCell.CGI.GetNrCgi().GetNRcellIdentity().GetValue().GetValue()
	servingNCILen := servingCell.CGI.GetNrCgi().GetNRcellIdentity().GetValue().GetLen()

	log.Info("Creating Control Message and sending it...")
	var err error
	go func() {
		if controlHandler.ControlHeader, err = controlHandler.CreateMhoControlHeader(servingNCI, servingNCILen, 1, servingPlmnIDBytes); err == nil {

			if controlHandler.ControlMessage, err = controlHandler.CreateMhoControlMessage(servingCell.CGI, &ueIdentity, targetCell.CGI); err == nil {

				if controlRequest, err := controlHandler.CreateMhoControlRequest(); err == nil {

					controlChannel <- controlRequest
					log.Info("Control message: ", <-controlChannel)

				} else {
					log.Warn("Control request problem :(", err)
				}
			} else {
				log.Warn("Control message problem :(", err)
			}
		} else {
			log.Warn("Control header problem :(", err)
		}
	}()

}
