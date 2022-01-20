package manager

import (
	"github.com/RIMEDO-Labs/xapp-sdk/pkg/southbound"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	"github.com/onosproject/onos-lib-go/pkg/northbound"
	service "github.com/onosproject/onos-mho/pkg/northbound"
	"github.com/onosproject/onos-mho/pkg/store"
	app "github.com/onosproject/onos-ric-sdk-go/pkg/config/app/default"
)

var log = logging.GetLogger("xApp Manager class [pkg/manager/manager.go]")

// Manager configuration object structure
type Config struct {
	CAPath      string
	KeyPath     string
	CertPath    string
	ConfigPath  string
	E2tEndpoint string
	GRPCPort    int
	AppConfig   *app.Config
	SMName      string
	SMVersion   string
}

type Manager struct {
	configuration Config
	e2Manager     *southbound.E2Manager
	ueStore       store.Store
	cellStore     store.Store
}

func NewManager() *Manager {

	// for testing only
	log.Info("Created new app manager object")

	// appConfiguration, err := appConfig.NewConfig(configuration.ConfigPath)
	// if err != nil {

	// 	log.Warn("Some issue with creating new application configuration object [NewManager()].", err)

	// }
	// subscriptionBroker := broker.NewBroker()
	ueStore := store.NewStore()
	cellStore := store.NewStore()
	// indicationChannel := make(chan *mho.E2NodeIndication)
	// controlRequestChannel := make(map[string]chan *e2API.ControlMessage)
	e2Manager, err := southbound.NewE2Manager(
	// configuration.ConfigPath,
	// e2.WithE2TAddress("onos-e2t", 5150),
	// e2.WithServiceModel(e2.ServiceModelName(configuration.SMName),
	// e2.ServiceModelVersion(configuration.SMVersion)),
	// e2.WithAppConfig(appConfiguration),
	// e2.WithAppID("RIMEDO-Labs-xAPP-SDK"),
	// e2.WithBroker(subscriptionBroker),
	// e2.WithIndChan(indicationChannel),
	// e2.WithCtrlReqChs(controlRequestChannel),
	// e2.WithUeStore(ueStore),
	// e2.WithCellStore(cellStore),

	)
	if err != nil {

		log.Warn("Can't create E2 Manager object [NewManager()].")

	}

	manager := &Manager{

		//configuration: configuration,
		e2Manager: e2Manager,
		ueStore:   ueStore,
		cellStore: cellStore,
	}

	return manager

}

func (self *Manager) Run() {

	log.Info("Manager is running")

	if err := self.RunAllControllers(); err != nil {

		log.Fatal("Can't run controllers [Run()].", err)

	}

}

func (self *Manager) RunAllControllers() error {

	if err := self.startNorthboundServer(); err != nil {

		log.Warn("Northbound server can't start [RunAllControllers()].", err)

		return err

	}

	if err := self.e2Manager.Start(); err != nil {

		log.Warn("E2 Manager can't start [RunAllControllers()].", err)

		return err

	}

	// If there's no error return nil
	return nil

}

func (self *Manager) startNorthboundServer() error {

	server := northbound.NewServer(northbound.NewServerCfg(
		self.configuration.CAPath,
		self.configuration.KeyPath,
		self.configuration.CertPath,
		int16(self.configuration.GRPCPort),
		true,
		northbound.SecurityConfig{}))

	//TODO - MHO northbound service
	server.AddService(service.NewService(self.ueStore, self.cellStore))

	errorChannel := make(chan error)
	go func() {

		if err := server.Serve(func(started string) {

			log.Info("Started NBI on ", started)
			close(errorChannel)

		}); err != nil {

			errorChannel <- err

		}

	}()

	return <-errorChannel

}
