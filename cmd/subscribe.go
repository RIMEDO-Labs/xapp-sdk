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

// Defining log context
var log = logging.GetLogger("main")

// Parameters - probably for e2t manager
const (
	e2t_address  = "onos-e2t"
	e2t_port     = 5150
	topo_address = "onos-topo"
	topo_port    = 5150
	sm_name      = "oran-e2sm-mho"
	sm_ver       = "v1"
)

// Defining structure which represents indication from e2 node (cgi, trigger type and the message)
type E2NodeIndication struct {
	NodeID      string
	TriggerType e2sm_mho.MhoTriggerType
	IndMsg      e2ind.Indication
}

// Server func - indication listening and serving ?
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

// Broker (WGO?)
var subscriptionBroker broker.Broker = broker.NewBroker()

// Channel to recieve indications
var indCh chan *E2NodeIndication = make(chan *E2NodeIndication)
var mu sync.RWMutex

// Main func (to be removed)
func main() {

	// Continuous listening ("go") of indication channel
	go listenIndChan(context.Background(), indCh)

	// Definition for e2t parameters
	e2tAddress := flag.String("e2tAddress", "", "address of onos-e2t")
	e2tPort := flag.Int("e2tPort", 0, "port of onos-e2t")
	topoAddress := flag.String("topoAddress", "", "address of onos-topo")
	topoPort := flag.Int("topoPort", 0, "port of onos-topo")
	flag.Parse()

	// Continuous serving of indication
	go server()

	log.Info("Read from parameters:")
	log.Info("E2T address: " + *e2tAddress)
	log.Info("E2T port: " + strconv.Itoa(*e2tPort))
	log.Info("TOPO address: " + *topoAddress)
	log.Info("TOPO port: " + strconv.Itoa(*topoPort))

	// https://tutorialedge.net/golang/go-grpc-beginners-tutorial/

	// Manager creating
	manager.NewManager()

	// Select Log Level
	log.SetLevel(logging.DebugLevel)

	log.Info("Starting")

	// Definiton of topology client
	topoClient, err := toposdk.NewClient(toposdk.WithTopoHost(topo_address), toposdk.WithTopoPort(topo_port))
	if err != nil {
		log.Warn(err)
	}

	// Definition of e2 client
	e2Client := e2client.NewClient(e2client.WithE2TAddress(e2t_address, e2t_port),
		e2client.WithServiceModel(e2client.ServiceModelName(sm_name),
			e2client.ServiceModelVersion(sm_ver)))

	// Definition of app context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Definiton of some channel for topology event
	ch_topo := make(chan topoapi.Event)

	// Filter for indication i guess
	controlRelationFilter := &topoapi.Filters{
		KindFilter: &topoapi.Filter{
			Filter: &topoapi.Filter_Equal_{
				Equal_: &topoapi.EqualFilter{
					Value: topoapi.CONTROLS,
				},
			},
		},
	}

	// Filter for e2 cell
	cellEntityFilter := &topoapi.Filters{
		KindFilter: &topoapi.Filter{
			Filter: &topoapi.Filter_In{
				In: &topoapi.InFilter{
					Values: []string{topo.E2CELL},
				},
			},
		},
	}

	// List topo objects
	objs, err := topoClient.List(ctx, toposdk.WithListFilters(cellEntityFilter))
	if err != nil {
		log.Warn(err)
	}

	// For all objects set aspect (something like setting type of cell?)
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

	// Watiching channel by topo client with defined filters
	log.Info("Watching onos-topo events...")
	err = topoClient.Watch(ctx, ch_topo, toposdk.WithWatchFilters(controlRelationFilter))
	if err != nil {
		log.Warn(err)
	}

	// For all event from topo channel: first check the
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

			// Setting constant trigger type for periodic reports
			triggerType := e2sm_mho.MhoTriggerType_MHO_TRIGGER_TYPE_PERIODIC

			log.Info("Sent subscription")
			//log.Debug("Channel ID: " + channelID)

			// For every received indication
			for ind := range ch_ind {

				// Determine the header
				indHeaderByte := ind.Header
				// Determine the message
				indMessageByte := ind.Payload

				indHeader := e2sm_mho.E2SmMhoIndicationHeader{}
				if err = proto.Unmarshal(indHeaderByte, &indHeader); err == nil {
					indMessage := e2sm_mho.E2SmMhoIndicationMessage{}

					// Probably some conversion of the data
					if err = proto.Unmarshal(indMessageByte, &indMessage); err == nil {

						// Checking indication is in Format 1 or 2
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
							go handleRrcState(ctx, indHeader.GetIndicationHeaderFormat1(), indMessage.GetIndicationMessageFormat2())
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

// Handling of periodic report
func handlePeriodicReport(ctx context.Context, header *e2sm_mho.E2SmMhoIndicationHeaderFormat1, message *e2sm_mho.E2SmMhoIndicationMessageFormat1) {

	//Getting cgi and idnetifier of the UE
	ueID := message.GetUeId().GetValue()
	cgi := getCGIFromIndicationHeader(header)
	log.Infof("rx periodic ueID:%v cgi:%v", ueID, cgi)

	// Getting RSRP from serving BS and neighbour BS
	rsrpServing, rsrpNeighbors := getRsrpFromMeasReport(getNciFromCellGlobalID(header.GetCgi()), message.MeasReport)
	log.Infof("rsrp_serving:%v rsrp_neighbors:%v", rsrpServing, rsrpNeighbors)

}

func handleRrcState(ctx context.Context, header *e2sm_mho.E2SmMhoIndicationHeaderFormat1, message *e2sm_mho.E2SmMhoIndicationMessageFormat2) {

	ueID := message.GetUeId().GetValue()
	cgi := getCGIFromIndicationHeader(header)
	log.Infof("rx rrc ueID:%v cgi:%v", ueID, cgi)

	// set rrc state (takes care of attach/detach as well)
	newRrcState := message.GetRrcStatus().String()
	log.Info("rsrp_serving:%v", newRrcState)

}

// Getting RSRP form measuremnet periodic report
func getRsrpFromMeasReport(servingNci uint64, measReport []*e2sm_mho.E2SmMhoMeasurementReportItem) (int32, map[string]int32) {

	// RSRP value of serving node
	var rsrpServing int32

	// Channel for RSPR from neighbour nodes
	rsrpNeighbors := make(map[string]int32)

	// For every measuremnt report item specify the RSRP from serving node and from neighbour nodes with CGI
	for _, measReportItem := range measReport {
		if getNciFromCellGlobalID(measReportItem.GetCgi()) == servingNci {
			rsrpServing = measReportItem.GetRsrp().GetValue()
		} else {
			CGIString := getCGIFromMeasReportItem(measReportItem)
			rsrpNeighbors[CGIString] = measReportItem.GetRsrp().GetValue()
		}
	}

	// Returning of RSRP values
	return rsrpServing, rsrpNeighbors
}

// Getting CGI from measurement report item
func getCGIFromMeasReportItem(measReport *e2sm_mho.E2SmMhoMeasurementReportItem) string {

	// Node NCI
	nci := getNciFromCellGlobalID(measReport.GetCgi())

	// Getting bytes from CGI
	plmnIDBytes := getPlmnIDBytesFromCellGlobalID(measReport.GetCgi())

	// Converting bytes to integer
	plmnID := plmnIDBytesToInt(plmnIDBytes)

	// Converting to CGI
	return plmnIDNciToCGI(plmnID, nci)
}

// Getting CGI from indication header (same as above func but from indication header)
func getCGIFromIndicationHeader(header *e2sm_mho.E2SmMhoIndicationHeaderFormat1) string {
	nci := getNciFromCellGlobalID(header.GetCgi())
	plmnIDBytes := getPlmnIDBytesFromCellGlobalID(header.GetCgi())
	plmnID := plmnIDBytesToInt(plmnIDBytes)
	return plmnIDNciToCGI(plmnID, nci)
}

// Getting NCI from CGI
func getNciFromCellGlobalID(cellGlobalID *e2sm_mho.CellGlobalId) uint64 {
	return cellGlobalID.GetNrCgi().GetNRcellIdentity().GetValue().GetValue()
}

// Getting  proto buffer (bytes) from CGI
func getPlmnIDBytesFromCellGlobalID(cellGlobalID *e2sm_mho.CellGlobalId) []byte {
	return cellGlobalID.GetNrCgi().GetPLmnIdentity().GetValue()
}

// Converting bytes to integer
func plmnIDBytesToInt(b []byte) uint64 {
	return uint64(b[2])<<16 | uint64(b[1])<<8 | uint64(b[0])
}

// Getting CGI from bytes and NCI
func plmnIDNciToCGI(plmnID uint64, nci uint64) string {
	return strconv.FormatInt(int64(plmnID<<36|(nci&0xfffffffff)), 16)
}

// Send indication on stream
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

// Starting monitoring indications on channel
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

// Listening indication channel
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
