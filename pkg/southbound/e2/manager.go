package e2

import (
	"context"
	"fmt"

	"github.com/RIMEDO-Labs/xapp-sdk/pkg/broker"
	"github.com/RIMEDO-Labs/xapp-sdk/pkg/mho"
	"github.com/RIMEDO-Labs/xapp-sdk/pkg/monitoring"
	"github.com/RIMEDO-Labs/xapp-sdk/pkg/rnib"
	e2api "github.com/onosproject/onos-api/go/onos/e2t/e2/v1beta1"
	topoapi "github.com/onosproject/onos-api/go/onos/topo"
	"github.com/onosproject/onos-e2-sm/servicemodels/e2sm_mho/pdubuilder"
	e2sm_mho "github.com/onosproject/onos-e2-sm/servicemodels/e2sm_mho/v1/e2sm-mho"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	e2client "github.com/onosproject/onos-ric-sdk-go/pkg/e2/v1beta1"
	"google.golang.org/protobuf/proto"
)

var log = logging.GetLogger("e2", "manager")

type Options struct {
	AppID       string
	E2tAddress  string
	E2tPort     int
	TopoAddress string
	TopoPort    int
	SMName      string
	SMVersion   string
}

func NewManager(options Options, indCh chan *mho.E2NodeIndication) (Manager, error) {
	log.Info("Init E2Manager")

	smName := e2client.ServiceModelName(options.SMName)
	smVer := e2client.ServiceModelVersion(options.SMVersion)
	appID := e2client.AppID(options.AppID)
	e2Client := e2client.NewClient(
		e2client.WithAppID(appID),
		e2client.WithServiceModel(smName, smVer),
		e2client.WithE2TAddress(options.E2tAddress, options.E2tPort),
	)

	rnibOptions := rnib.Options{
		TopoAddress: options.TopoAddress,
		TopoPort:    options.TopoPort,
	}

	rnibClient, err := rnib.NewClient(rnibOptions)
	if err != nil {
		return Manager{}, err
	}

	return Manager{
		e2client:   e2Client,
		rnibClient: rnibClient,
		streams:    broker.NewBroker(),
		indCh:      indCh,
	}, nil
}

type Manager struct {
	e2client   e2client.Client
	rnibClient rnib.Client
	streams    broker.Broker
	indCh      chan *mho.E2NodeIndication
}

func (m *Manager) Start() error {
	log.Info("Start E2Manager")
	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		err := m.watchE2Connections(ctx)
		if err != nil {
			return
		}
	}()

	return nil
}

func (m *Manager) watchE2Connections(ctx context.Context) error {
	log.Info("Start monitoring E2 connections")
	ch := make(chan topoapi.Event)
	err := m.rnibClient.WatchE2Connections(ctx, ch)
	if err != nil {
		log.Warn(err)
		return err
	}

	for topoEvent := range ch {
		if topoEvent.Type == topoapi.EventType_ADDED || topoEvent.Type == topoapi.EventType_NONE {
			relation := topoEvent.Object.Obj.(*topoapi.Object_Relation)
			e2NodeID := relation.Relation.TgtEntityID

			// move "flags" somewhere else
			triggers := make(map[e2sm_mho.MhoTriggerType]bool)
			triggers[e2sm_mho.MhoTriggerType_MHO_TRIGGER_TYPE_PERIODIC] = true
			triggers[e2sm_mho.MhoTriggerType_MHO_TRIGGER_TYPE_UPON_RCV_MEAS_REPORT] = true
			triggers[e2sm_mho.MhoTriggerType_MHO_TRIGGER_TYPE_UPON_CHANGE_RRC_STATUS] = true

			for triggerType, enabled := range triggers {
				if enabled {
					go func(triggerType e2sm_mho.MhoTriggerType) {
						log.Infof("tx subscription, e2NodeID:%v, triggerType:%v", e2NodeID, e2sm_mho.MhoTriggerType_name[int32(triggerType)])
						err := m.createSubscription(ctx, e2NodeID, triggerType)
						if err != nil {
							log.Warn(err)
						}
					}(triggerType)
				}
			}
		}
	}

	return nil
}

func (m *Manager) createSubscription(ctx context.Context, e2nodeID topoapi.ID, triggerType e2sm_mho.MhoTriggerType) error {
	eventTriggerData, err := m.createEventTrigger(triggerType)
	if err != nil {
		log.Warn(err)
		return err
	}

	actions := m.createSubscriptionActions()

	ch := make(chan e2api.Indication)
	node := m.e2client.Node(e2client.NodeID(e2nodeID))
	subName := fmt.Sprintf("xapp-sdk-subscription-%s", triggerType)
	subSpec := e2api.SubscriptionSpec{
		Actions: actions,
		EventTrigger: e2api.EventTrigger{
			Payload: eventTriggerData,
		},
	}

	channelID, err := node.Subscribe(ctx, subName, subSpec, ch)
	if err != nil {
		log.Warn(err)
		return err
	}

	streamReader, err := m.streams.OpenReader(ctx, node, subName, channelID, subSpec)
	if err != nil {
		return err
	}
	go m.sendIndicationOnStream(streamReader.StreamID(), ch)

	monitor := monitoring.NewMonitor(streamReader, e2nodeID, m.indCh, triggerType)

	err = monitor.Start(ctx)
	if err != nil {
		log.Warn(err)
	}

	return nil
}

func (m *Manager) createEventTrigger(triggerType e2sm_mho.MhoTriggerType) ([]byte, error) {
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

func (m *Manager) createSubscriptionActions() []e2api.Action {
	actions := make([]e2api.Action, 0)
	action := &e2api.Action{
		ID:   int32(0),
		Type: e2api.ActionType_ACTION_TYPE_REPORT,
		SubsequentAction: &e2api.SubsequentAction{
			Type:       e2api.SubsequentActionType_SUBSEQUENT_ACTION_TYPE_CONTINUE,
			TimeToWait: e2api.TimeToWait_TIME_TO_WAIT_ZERO,
		},
	}
	actions = append(actions, *action)
	return actions
}

func (m *Manager) sendIndicationOnStream(streamID broker.StreamID, ch chan e2api.Indication) {
	streamWriter, err := m.streams.GetWriter(streamID)
	if err != nil {
		log.Error(err)
		return
	}

	for msg := range ch {
		err := streamWriter.Send(msg)
		if err != nil {
			log.Warn(err)
			return
		}
	}
}
