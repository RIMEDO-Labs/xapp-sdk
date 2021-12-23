package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"strconv"

	"github.com/RIMEDO-Labs/xapp-sdk/pkg/manager"
	e2api "github.com/onosproject/onos-api/go/onos/e2t/e2/v1beta1"
	topoapi "github.com/onosproject/onos-api/go/onos/topo"
	"github.com/onosproject/onos-e2-sm/servicemodels/e2sm_mho/pdubuilder"
	e2sm_mho "github.com/onosproject/onos-e2-sm/servicemodels/e2sm_mho/v1/e2sm-mho"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	e2client "github.com/onosproject/onos-ric-sdk-go/pkg/e2/v1beta1"
	toposdk "github.com/onosproject/onos-ric-sdk-go/pkg/topo"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

var log = logging.GetLogger("main")

const (
	e2t_address  = "onos-e2t"
	e2t_port     = 5150
	topo_address = "onos-topo"
	topo_port    = 5150
	sm_name      = "oran-e2sm-mho"
	sm_ver       = "v1"
)

func server() {
	lis, err := net.Listen("tcp", ":5150")
	if err != nil {
		log.Fatal("Failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatal("Failed to serve: %s", err)
	}
}

func main() {
	e2tAddress := flag.String("e2tAddress", "", "address of onos-e2t")
	e2tPort := flag.Int("e2tPort", 0, "port of onos-e2t")
	topoAddress := flag.String("topoAddress", "", "address of onos-topo")
	topoPort := flag.Int("topoPort", 0, "port of onos-topo")
	flag.Parse()

	go server()

	log.Info("Read from parameters:")
	log.Info("E2T address: " + *e2tAddress)
	log.Info("E2T port: " + strconv.Itoa(*e2tPort))
	log.Info("TOPO address: " + *topoAddress)
	log.Info("TOPO port: " + strconv.Itoa(*topoPort))

	// https://tutorialedge.net/golang/go-grpc-beginners-tutorial/

	manager.NewManager()

	// Select Log Level
	log.SetLevel(logging.DebugLevel)

	log.Info("Starting")

	topoClient, err := toposdk.NewClient(toposdk.WithTopoHost(topo_address), toposdk.WithTopoPort(topo_port))
	if err != nil {
		log.Warn(err)
	}

	e2Client := e2client.NewClient(e2client.WithE2TAddress(e2t_address, e2t_port),
		e2client.WithServiceModel(e2client.ServiceModelName(sm_name),
			e2client.ServiceModelVersion(sm_ver)))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch_topo := make(chan topoapi.Event)

	controlRelationFilter := &topoapi.Filters{
		KindFilter: &topoapi.Filter{
			Filter: &topoapi.Filter_Equal_{
				Equal_: &topoapi.EqualFilter{
					Value: topoapi.CONTROLS,
				},
			},
		},
	}

	log.Info("Watching onos-topo events...")
	err = topoClient.Watch(ctx, ch_topo, toposdk.WithWatchFilters(controlRelationFilter))
	if err != nil {
		log.Warn(err)
	}

	for topoEvent := range ch_topo {
		switch topoEvent.Type {
		case topoapi.EventType_NONE:
			log.Debug("topoEvent.Type=NONE")

			relation := topoEvent.Object.Obj.(*topoapi.Object_Relation)
			e2NodeID := relation.Relation.TgtEntityID
			log.Info("e2NodeID: " + e2NodeID)

			ch_ind := make(chan e2api.Indication)
			node := e2Client.Node(e2client.NodeID(e2NodeID))

			e2smRcEventTriggerDefinition, err := pdubuilder.CreateE2SmMhoEventTriggerDefinition(
				e2sm_mho.MhoTriggerType_MHO_TRIGGER_TYPE_PERIODIC, 1000)
			if err != nil {
				log.Error(err)
			}
			err = e2smRcEventTriggerDefinition.Validate()
			if err != nil {
				log.Error(err)
			}
			protoBytes, err := proto.Marshal(e2smRcEventTriggerDefinition)
			if err != nil {
				log.Error(err)
			}
			eventTriggerData := protoBytes

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

			subSpec := e2api.SubscriptionSpec{
				Actions: actions,
				EventTrigger: e2api.EventTrigger{
					Payload: eventTriggerData,
				},
			}
			log.Info("Creating subscription for E2 node with ID:", e2NodeID)
			subName := fmt.Sprintf("rimedo-mho-subscription-%s", e2sm_mho.MhoTriggerType_MHO_TRIGGER_TYPE_PERIODIC)
			channelID, err := node.Subscribe(ctx, subName, subSpec, ch_ind)
			if err != nil {
				log.Warn(err)
			}
			log.Info("Sent subscription")
			log.Debug("Channel ID: " + channelID)
			for ind := range ch_ind {
				log.Info(ind)
			}

		case topoapi.EventType_ADDED:
			log.Debug("topoEvent.Type=ADDED")
		case topoapi.EventType_REMOVED:
			log.Debug("topoEvent.Type=REMOVED")
		case topoapi.EventType_UPDATED:
			log.Debug("topoEvent.Type=UPDATED")
		default:
			log.Warn("unknown topoEvent.Type")
		}
	}

	/*
		client := e2client.NewClient(e2client.WithE2TAddress("localhost", 5150),
			e2client.WithServiceModel(e2client.ServiceModelName("oran-e2sm-kpm"),
				e2client.ServiceModelVersion("v2")),
			e2client.WithEncoding(e2client.ProtoEncoding))

		e2node := client.Node(e2client.NodeID("e2:1/5153"))

		subName := "onos-kpimon-subscriptionxd"
		var eventTriggerData []byte

		actions := make([]e2api.Action, 0)

		action := &e2api.Action{
			ID:   0,
			Type: e2api.ActionType_ACTION_TYPE_REPORT,
			SubsequentAction: &e2api.SubsequentAction{
				Type:       e2api.SubsequentActionType_SUBSEQUENT_ACTION_TYPE_CONTINUE,
				TimeToWait: e2api.TimeToWait_TIME_TO_WAIT_ZERO,
			},
		}

		actions = append(actions, *action)

		subSpec := e2api.SubscriptionSpec{
			Actions: actions,
			EventTrigger: e2api.EventTrigger{
				Payload: eventTriggerData,
			}}

		// ch := make(chan e2api.Indication)

		// ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		// defer cancel()
		log.Info("Trying to subscribe")
		// channelID, err := e2node.Subscribe(ctx, subName, subSpec, ch)

		if err != nil {
			log.Error(err)
		}

		for ind := range ch {
			_ = ind
			log.Info("we are here")
		}

		_ = channelID
	*/
}
