package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"strconv"
	"sync"

	"github.com/RIMEDO-Labs/xapp-sdk/pkg/broker"
	"github.com/RIMEDO-Labs/xapp-sdk/pkg/manager"
	e2api "github.com/onosproject/onos-api/go/onos/e2t/e2/v1beta1"
	"github.com/onosproject/onos-api/go/onos/topo"
	topoapi "github.com/onosproject/onos-api/go/onos/topo"
	"github.com/onosproject/onos-e2-sm/servicemodels/e2sm_mho/pdubuilder"
	e2sm_mho "github.com/onosproject/onos-e2-sm/servicemodels/e2sm_mho/v1/e2sm-mho"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	e2ind "github.com/onosproject/onos-ric-sdk-go/pkg/e2/indication"
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

type E2NodeIndication struct {
	NodeID      string
	TriggerType e2sm_mho.MhoTriggerType
	IndMsg      e2ind.Indication
}

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

var subscriptionBroker broker.Broker = broker.NewBroker()
var indCh chan *E2NodeIndication = make(chan *E2NodeIndication)
var mu sync.RWMutex

func main() {
	go listenIndChan(context.Background(), indCh)

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

	cellEntityFilter := &topoapi.Filters{
		KindFilter: &topoapi.Filter{
			Filter: &topoapi.Filter_In{
				In: &topoapi.InFilter{
					Values: []string{topo.E2CELL},
				},
			},
		},
	}
	objs, err := topoClient.List(ctx, toposdk.WithListFilters(cellEntityFilter))
	if err != nil {
		log.Warn(err)
	}
	for _, i := range objs {
		cellObject := &topoapi.E2Cell{}
		err = i.GetAspect(cellObject)
		if err != nil {
			log.Warn(err)
		}
		log.Info(cellObject.CellType)
		cellTypeAspect := &topoapi.E2Cell{
			CellType: "Macro",
		}
		err = i.SetAspect(cellTypeAspect)
		if err != nil {
			log.Warn(err)
		}
		topoClient.Update(ctx, &i)
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

			obj, err := topoClient.Get(ctx, e2NodeID)
			if err != nil {
				log.Warn(err)
			}
			log.Info(obj)

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
			_ = channelID
			if err != nil {
				log.Warn(err)
			}

			// streamReader, err := subscriptionBroker.OpenReader(ctx, node, subName, channelID, subSpec)
			//if err != nil {
			//	log.Warn(err)
			//}
			//ch := make(chan e2api.Indication)
			//go sendIndicationOnStream(streamReader.StreamID(), ch)

			//go startMonitoring(ctx, streamReader, e2NodeID, indCh, e2sm_mho.MhoTriggerType_MHO_TRIGGER_TYPE_PERIODIC)

			triggerType := e2sm_mho.MhoTriggerType_MHO_TRIGGER_TYPE_PERIODIC

			log.Info("Sent subscription")
			//log.Debug("Channel ID: " + channelID)
			for ind := range ch_ind {
				indHeaderByte := ind.Header
				indMessageByte := ind.Payload

				indHeader := e2sm_mho.E2SmMhoIndicationHeader{}
				if err = proto.Unmarshal(indHeaderByte, &indHeader); err == nil {
					indMessage := e2sm_mho.E2SmMhoIndicationMessage{}
					if err = proto.Unmarshal(indMessageByte, &indMessage); err == nil {
						switch x := indMessage.E2SmMhoIndicationMessage.(type) {
						case *e2sm_mho.E2SmMhoIndicationMessage_IndicationMessageFormat1:
							if triggerType == e2sm_mho.MhoTriggerType_MHO_TRIGGER_TYPE_UPON_RCV_MEAS_REPORT {
								log.Info("Handle meansurement report")
								//go handleMeasReport(ctx, indHeader.GetIndicationHeaderFormat1(), indMessage.GetIndicationMessageFormat1(), e2NodeID)
							} else if triggerType == e2sm_mho.MhoTriggerType_MHO_TRIGGER_TYPE_PERIODIC {
								log.Info("Handle periodic report")
								go handlePeriodicReport(ctx, indHeader.GetIndicationHeaderFormat1(), indMessage.GetIndicationMessageFormat1())

							}
						case *e2sm_mho.E2SmMhoIndicationMessage_IndicationMessageFormat2:
							//go c.handleRrcState(ctx, indHeader.GetIndicationHeaderFormat1(), indMessage.GetIndicationMessageFormat2())
							log.Info("Handle rrc state")
						default:
							log.Warnf("Unknown MHO indication message format, indication message: %v", x)
						}

					}
				}
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
}

func handlePeriodicReport(ctx context.Context, header *e2sm_mho.E2SmMhoIndicationHeaderFormat1, message *e2sm_mho.E2SmMhoIndicationMessageFormat1) {
	//mu.Lock()
	//defer mu.Unlock()
	ueID := message.GetUeId().GetValue()
	cgi := getCGIFromIndicationHeader(header)
	log.Infof("rx periodic ueID:%v cgi:%v", ueID, cgi)

	// get ue from store (create if it does not exist)
	// var ueData *UeData
	// newUe := false
	// ueData = c.getUe(ctx, ueID)
	// if ueData == nil {
	// 	ueData = c.createUe(ctx, ueID)
	//	c.attachUe(ctx, ueData, cgi)
	//		newUe = true
	//} else if ueData.CGIString != cgi {
	//	return
	//}

	rsrpServing, rsrpNeighbors := getRsrpFromMeasReport(getNciFromCellGlobalID(header.GetCgi()), message.MeasReport)
	log.Infof("rsrp_serving:%v rsrp_neighbors:%v", rsrpServing, rsrpNeighbors)

	//if !newUe && rsrpServing == ueData.RsrpServing && reflect.DeepEqual(rsrpNeighbors, ueData.RsrpNeighbors) {
	//	return
	//}

	// update store
	//ueData.RsrpServing, ueData.RsrpNeighbors = rsrpServing, rsrpNeighbors
	//c.setUe(ctx, ueData)

}

func getRsrpFromMeasReport(servingNci uint64, measReport []*e2sm_mho.E2SmMhoMeasurementReportItem) (int32, map[string]int32) {
	var rsrpServing int32
	rsrpNeighbors := make(map[string]int32)

	for _, measReportItem := range measReport {
		if getNciFromCellGlobalID(measReportItem.GetCgi()) == servingNci {
			rsrpServing = measReportItem.GetRsrp().GetValue()
		} else {
			CGIString := getCGIFromMeasReportItem(measReportItem)
			rsrpNeighbors[CGIString] = measReportItem.GetRsrp().GetValue()
		}
	}

	return rsrpServing, rsrpNeighbors
}

func getCGIFromMeasReportItem(measReport *e2sm_mho.E2SmMhoMeasurementReportItem) string {
	nci := getNciFromCellGlobalID(measReport.GetCgi())
	plmnIDBytes := getPlmnIDBytesFromCellGlobalID(measReport.GetCgi())
	plmnID := plmnIDBytesToInt(plmnIDBytes)
	return plmnIDNciToCGI(plmnID, nci)
}

func getCGIFromIndicationHeader(header *e2sm_mho.E2SmMhoIndicationHeaderFormat1) string {
	nci := getNciFromCellGlobalID(header.GetCgi())
	plmnIDBytes := getPlmnIDBytesFromCellGlobalID(header.GetCgi())
	plmnID := plmnIDBytesToInt(plmnIDBytes)
	return plmnIDNciToCGI(plmnID, nci)
}

func getNciFromCellGlobalID(cellGlobalID *e2sm_mho.CellGlobalId) uint64 {
	return cellGlobalID.GetNrCgi().GetNRcellIdentity().GetValue().GetValue()
}

func getPlmnIDBytesFromCellGlobalID(cellGlobalID *e2sm_mho.CellGlobalId) []byte {
	return cellGlobalID.GetNrCgi().GetPLmnIdentity().GetValue()
}

func plmnIDBytesToInt(b []byte) uint64 {
	return uint64(b[2])<<16 | uint64(b[1])<<8 | uint64(b[0])
}

func plmnIDNciToCGI(plmnID uint64, nci uint64) string {
	return strconv.FormatInt(int64(plmnID<<36|(nci&0xfffffffff)), 16)
}

func sendIndicationOnStream(streamID broker.StreamID, ch chan e2api.Indication) {
	log.Info("In sendIndicationOnStream")
	streamWriter, err := subscriptionBroker.GetWriter(streamID)
	if err != nil {
		log.Error(err)
		return
	}

	for msg := range ch {
		log.Info("In sendIndicationOnStream loop")
		err := streamWriter.Send(msg)
		if err != nil {
			log.Warn(err)
			return
		}
	}
}

func startMonitoring(ctx context.Context, streamReader broker.StreamReader, nodeID topoapi.ID, indChan chan *E2NodeIndication, triggerType e2sm_mho.MhoTriggerType) error {
	log.Info("In startMonitoring")
	errCh := make(chan error)
	go func() {
		for {
			indMsg, err := streamReader.Recv(ctx)
			if err != nil {
				log.Errorf("Error reading indication stream, chanID:%v, streamID:%v, err:%v", streamReader.ChannelID(), streamReader.StreamID(), err)
				errCh <- err
			}
			err = processIndication(ctx, indMsg, nodeID, triggerType)
			if err != nil {
				log.Errorf("Error processing indication, err:%v", err)
				errCh <- err
			}
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func processIndication(ctx context.Context, indication e2api.Indication, nodeID topoapi.ID, triggerType e2sm_mho.MhoTriggerType) error {
	log.Infof("processIndication, nodeID: %v, indication: %v ", nodeID, indication)
	// Debugf
	indCh <- &E2NodeIndication{
		NodeID:      string(nodeID),
		TriggerType: triggerType,
		IndMsg: e2ind.Indication{
			Payload: e2ind.Payload{
				Header:  indication.Header,
				Message: indication.Payload,
			},
		},
	}

	return nil
}

func listenIndChan(ctx context.Context, indChan chan *E2NodeIndication) {
	var err error
	log.Info("Before listenIndChan loop")
	for indMsg := range indChan {
		log.Info("In listenIndChan loop")

		indHeaderByte := indMsg.IndMsg.Payload.Header
		indMessageByte := indMsg.IndMsg.Payload.Message
		e2NodeID := indMsg.NodeID
		_ = e2NodeID

		indHeader := e2sm_mho.E2SmMhoIndicationHeader{}
		if err = proto.Unmarshal(indHeaderByte, &indHeader); err == nil {
			indMessage := e2sm_mho.E2SmMhoIndicationMessage{}
			if err = proto.Unmarshal(indMessageByte, &indMessage); err == nil {
				switch x := indMessage.E2SmMhoIndicationMessage.(type) {
				case *e2sm_mho.E2SmMhoIndicationMessage_IndicationMessageFormat1:
					if indMsg.TriggerType == e2sm_mho.MhoTriggerType_MHO_TRIGGER_TYPE_UPON_RCV_MEAS_REPORT {
						log.Info("Handle meansurement report")
						//go c.handleMeasReport(ctx, indHeader.GetIndicationHeaderFormat1(), indMessage.GetIndicationMessageFormat1(), e2NodeID)
					} else if indMsg.TriggerType == e2sm_mho.MhoTriggerType_MHO_TRIGGER_TYPE_PERIODIC {
						//go c.handlePeriodicReport(ctx, indHeader.GetIndicationHeaderFormat1(), indMessage.GetIndicationMessageFormat1(), e2NodeID)
						log.Info("Handle periodic report")
					}
				case *e2sm_mho.E2SmMhoIndicationMessage_IndicationMessageFormat2:
					//go c.handleRrcState(ctx, indHeader.GetIndicationHeaderFormat1(), indMessage.GetIndicationMessageFormat2())
					log.Info("Handle rrc state")
				default:
					log.Warnf("Unknown MHO indication message format, indication message: %v", x)
				}
			}
		}
		if err != nil {
			log.Error(err)
		}
	}
	log.Info("After listenIndChan loop")
}
