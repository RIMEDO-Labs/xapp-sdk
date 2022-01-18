package southbound

import (
	"context"

	topoapi "github.com/onosproject/onos-api/go/onos/topo"
	e2sm_mho "github.com/onosproject/onos-e2-sm/servicemodels/e2sm_mho/v1/e2sm-mho"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	"github.com/onosproject/onos-mho/pkg/broker"
	e2client "github.com/onosproject/onos-ric-sdk-go/pkg/e2/v1beta1"
)

var log = logging.GetLogger("Main source file [pkg/southbound/e2-manager.go]")

type E2Manager struct {
	e2Client e2client.Client
	streams  broker.Broker
}

func NewE2Manager() (*E2Manager, error) {

	log.Info("Created new E2 manager object")

	serviceModelName := e2client.ServiceModelName("oran-e2sm-mho")
	serviceModelVersion := e2client.ServiceModelVersion("v1")
	appID := "RIMEDO-Lab-xAPP-SDK"
	e2tServiceHost := "onos-e2t:5150"
	e2tServicePort := 5150

	e2client := e2client.NewClient(
		e2client.WithServiceModel(serviceModelName, serviceModelVersion),
		e2client.WithAppID(e2client.AppID(appID)),
		e2client.WithE2TAddress(e2tServiceHost, e2tServicePort))

	e2Manager := &E2Manager{

		e2Client: e2client,
		streams:  broker.NewBroker(),
	}

	return e2Manager, nil

}

func (self *E2Manager) Start() error {

	log.Info("E2 Manager is running.")

	errorChannel := make(chan error)
	go func() {

		context, cancel := context.WithCancel(context.Background())
		defer cancel()

		if err := self.watchE2Connections(context); err != nil {

			log.Warn("Can't watch E2 connections [Start()].", err)
			errorChannel <- err

		}

	}()

	if err := <-errorChannel; err != nil {

		return err

	}

	return nil

}

func (self *E2Manager) createSubscription(context context.Context, e2nodeID topoapi.ID, triggerType e2sm_mho.MhoTriggerType) error {

	return nil

}

func (self *E2Manager) watchE2Connections(context context.Context) error {

	log.Info("Watching of E2 connections has been started.")

	errorChannel := make(chan error)

	// Create the channel to receive the events invoked by the system
	eventChannel := make(chan topoapi.Event)

	// Event observed in topology api, i.e, inside the network from side of base stations
	for topoEvent := range eventChannel {

		// Definition of some relation obejct (associated with topology xApp) - probably relation means connection between e2 nodes (and more)
		relation := topoEvent.Object.Obj.(*topoapi.Object_Relation)
		e2NodeID := relation.Relation.TgtEntityID

		// If the event's type is "Station's been already added" or "none" (probably nothing's changed), then make the instructions below
		if topoEvent.Type == topoapi.EventType_ADDED || topoEvent.Type == topoapi.EventType_NONE {

			// Create the map for event trigger types and enable each of them
			triggers := make(map[e2sm_mho.MhoTriggerType]bool)
			triggers[e2sm_mho.MhoTriggerType_MHO_TRIGGER_TYPE_PERIODIC] = true
			triggers[e2sm_mho.MhoTriggerType_MHO_TRIGGER_TYPE_UPON_RCV_MEAS_REPORT] = true
			triggers[e2sm_mho.MhoTriggerType_MHO_TRIGGER_TYPE_UPON_CHANGE_RRC_STATUS] = true

			for triggerType, enabled := range triggers {

				if enabled {

					go func() {

						if err := self.createSubscription(context, e2NodeID, triggerType); err != nil {

							log.Warn("Can't create a subscription stream [watchE2Connections()].")
							errorChannel <- err

						}

					}()

				}

			}

		} else if topoEvent.Type == topoapi.EventType_REMOVED {

			log.Info("Handling disconnection of E2 node")

			// Should to add something :P

		}

	}

	if err := <-errorChannel; err != nil {

		return err

	}

	return nil

}
