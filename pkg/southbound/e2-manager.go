package southbound

import (
	"context"
	"fmt"

	"github.com/RIMEDO-Labs/xapp-sdk/pkg/mho"
	"github.com/RIMEDO-Labs/xapp-sdk/pkg/monitoring"
	e2API "github.com/onosproject/onos-api/go/onos/e2t/e2/v1beta1"
	topoapi "github.com/onosproject/onos-api/go/onos/topo"
	"github.com/onosproject/onos-e2-sm/servicemodels/e2sm_mho/pdubuilder"
	e2sm_mho "github.com/onosproject/onos-e2-sm/servicemodels/e2sm_mho/v1/e2sm-mho"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	"github.com/onosproject/onos-mho/pkg/broker"
	"github.com/onosproject/onos-mho/pkg/rnib"
	e2client "github.com/onosproject/onos-ric-sdk-go/pkg/e2/v1beta1"
	"google.golang.org/protobuf/proto"
)

var log = logging.GetLogger("E2 Manager source file [pkg/southbound/e2-manager.go]")

type E2Manager struct {
	e2Client   e2client.Client
	streams    broker.Broker
	rnibClient rnib.Client
	//appConfiguration       appConfig.Config
	indicationChannel      chan *mho.E2NodeIndication
	controlRequestChannels map[string]chan *e2API.ControlMessage
}

func NewE2Manager() (*E2Manager, error) {

	log.Info("Created new E2 manager object")

	// appConfiguration, err := appConfig.NewConfig(configurationPath)
	// if err != nil {

	// 	log.Warn("Some issue with creating new application configuration object [NewManager()].", err)

	// }

	serviceModelName := e2client.ServiceModelName("oran-e2sm-mho")
	serviceModelVersion := e2client.ServiceModelVersion("v1")
	appID := "RIMEDO-Labs-xAPP-SDK"
	e2tServiceHost := "onos-e2t:5150"
	e2tServicePort := 5150
	indicationChannel := make(chan *mho.E2NodeIndication)
	controlRequestChannels := make(map[string]chan *e2API.ControlMessage)
	subscriptionBroker := broker.NewBroker()
	e2client := e2client.NewClient(
		e2client.WithServiceModel(serviceModelName, serviceModelVersion),
		e2client.WithAppID(e2client.AppID(appID)),
		e2client.WithE2TAddress(e2tServiceHost, e2tServicePort))
	rnibClient, err := rnib.NewClient()
	if err != nil {

		return &E2Manager{}, err

	}

	e2Manager := &E2Manager{

		e2Client:   e2client,
		streams:    subscriptionBroker,
		rnibClient: rnibClient,
		//appConfiguration:       appConfiguration,
		indicationChannel:      indicationChannel,
		controlRequestChannels: controlRequestChannels,
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

func (self *E2Manager) sendIndicationOnStream(streamID broker.StreamID, channel chan e2API.Indication) {

	streamWriter, err := self.streams.GetWriter(streamID)
	if err != nil {

		log.Error("Problem with getting stream writer [sendIndicationOnStream()].", err)
		return

	}

	for message := range channel {

		err := streamWriter.Send(message)
		if err != nil {

			log.Warn("Problem with sending message [sendIndicationOnStream()].", err)
			return

		}

	}

}

func (self *E2Manager) createSubscription(context context.Context, e2nodeID topoapi.ID, triggerType e2sm_mho.MhoTriggerType) error {

	eventTriggerData, err := self.createEventTrigger(triggerType)

	if err != nil {

		log.Error("Can't create event trigger data [createSubscription()].", err)

		//log.Warn(err)
		return err

	}

	subscriptionActions := self.createSubscriptionActions()
	subscriptionChannel := make(chan e2API.Indication)
	e2Node := self.e2Client.Node(e2client.NodeID(e2nodeID))
	subscriptionName := fmt.Sprintf("rimedo-xapp-sdk-subscription-%s", triggerType)
	subscriptionSpecifications := e2API.SubscriptionSpec{
		Actions: subscriptionActions,
		EventTrigger: e2API.EventTrigger{
			Payload: eventTriggerData,
		},
	}

	channelID, err := e2Node.Subscribe(context, subscriptionName, subscriptionSpecifications, subscriptionChannel)
	if err != nil {

		log.Warn("Problem with subscription creating [createSubscription()].", err)
		return err

	}

	streamReader, err := self.streams.OpenReader(context, e2Node, subscriptionName, channelID, subscriptionSpecifications)
	if err != nil {

		return err

	}

	go self.sendIndicationOnStream(streamReader.StreamID(), subscriptionChannel)

	monitor := monitoring.NewMonitor(streamReader, e2nodeID, self.indicationChannel, triggerType)

	if err = monitor.Start(context); err != nil {

		log.Warn("Monitor can't be started [createSubscription()].", err)

	}

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

func (self *E2Manager) createEventTrigger(triggerType e2sm_mho.MhoTriggerType) ([]byte, error) {

	var reportPeriodMs int32
	reportingPeriod := 1000

	if triggerType == e2sm_mho.MhoTriggerType_MHO_TRIGGER_TYPE_PERIODIC {

		reportPeriodMs = int32(reportingPeriod)

	} else {

		reportPeriodMs = 0

	}

	e2smRcEventTriggerDefinition, err := pdubuilder.CreateE2SmMhoEventTriggerDefinition(triggerType, reportPeriodMs)

	if err != nil {

		return []byte{}, err

	}

	err = e2smRcEventTriggerDefinition.Validate()

	if err != nil {

		return []byte{}, err

	}

	protoBytes, err := proto.Marshal(e2smRcEventTriggerDefinition)

	if err != nil {

		return []byte{}, err

	}

	return protoBytes, err

}

func (self *E2Manager) createSubscriptionActions() []e2API.Action {

	actions := make([]e2API.Action, 0)
	action := &e2API.Action{

		ID:   int32(0),
		Type: e2API.ActionType_ACTION_TYPE_REPORT,
		SubsequentAction: &e2API.SubsequentAction{
			Type:       e2API.SubsequentActionType_SUBSEQUENT_ACTION_TYPE_CONTINUE,
			TimeToWait: e2API.TimeToWait_TIME_TO_WAIT_ZERO,
		},
	}

	actions = append(actions, *action)
	return actions

}
