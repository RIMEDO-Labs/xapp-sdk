package manager

import (
	"context"
	"net"

	"github.com/RIMEDO-Labs/xapp-sdk/pkg/mho"
	"github.com/RIMEDO-Labs/xapp-sdk/pkg/rnib"
	"github.com/RIMEDO-Labs/xapp-sdk/pkg/southbound/e2"
	"github.com/RIMEDO-Labs/xapp-sdk/pkg/store"
	"github.com/RIMEDO-Labs/xapp-sdk/pkg/tspolicy"
	"github.com/onosproject/onos-lib-go/pkg/logging"
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

	options := e2.Options{
		AppID:       config.AppID,
		E2tAddress:  config.E2tAddress,
		E2tPort:     config.E2tPort,
		TopoAddress: config.TopoAddress,
		TopoPort:    config.TopoPort,
		SMName:      config.SMName,
		SMVersion:   config.SMVersion,
	}

	e2Manager, err := e2.NewManager(options, indCh)
	if err != nil {
		log.Warn(err)
	}

	manager := &Manager{
		e2Manager: e2Manager,
		mhoCtrl:   mho.NewController(indCh, ueStore, cellStore),
		PolicyMap: *policyMap,
		ueStore:   ueStore,
		cellStore: cellStore,
	}
	return manager
}

type Manager struct {
	e2Manager e2.Manager
	mhoCtrl   *mho.Controller
	PolicyMap tspolicy.TsPolicyMap
	ueStore   store.Store
	cellStore store.Store
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

func (m *Manager) GetCellTypes(ctx context.Context) map[string]rnib.Cell {
	return m.e2Manager.GetCellTypes(ctx)
}

func (m *Manager) SetCellType(ctx context.Context, cellID string, cellType string) error {
	return m.e2Manager.SetCellType(ctx, cellID, cellType)
}
