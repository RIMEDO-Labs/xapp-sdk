package manager

import (
	"github.com/RIMEDO-Labs/xapp-sdk/pkg/southbound"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	"github.com/onosproject/onos-mho/pkg/store"
)

var log = logging.GetLogger("xApp Manager class [pkg/manager/manager.go]")

type Manager struct {
	ID        int
	e2Manager southbound.E2Manager
	ueStore   store.Store
	cellStore store.Store
}

func NewManager(ID int) *Manager {
	// for testing only
	log.Info("Created new app manager object")

	manager := &Manager{
		ID: ID,
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

	if err := self.e2Manager.Start(); err != nil {

		log.Warn("E2 Manager can't start [RunAllControllers()].", err)

		return err

	}

	// If there's no error return nil
	return nil

}
