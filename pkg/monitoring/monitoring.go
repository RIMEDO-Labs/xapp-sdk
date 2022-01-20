package monitoring

import (
	"context"

	"github.com/RIMEDO-Labs/xapp-sdk/pkg/mho"
	e2API "github.com/onosproject/onos-api/go/onos/e2t/e2/v1beta1"
	topoapi "github.com/onosproject/onos-api/go/onos/topo"
	e2sm_mho "github.com/onosproject/onos-e2-sm/servicemodels/e2sm_mho/v1/e2sm-mho"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	"github.com/onosproject/onos-mho/pkg/broker"
	e2ind "github.com/onosproject/onos-ric-sdk-go/pkg/e2/indication"
)

var log = logging.GetLogger("Indication monitor source file [pkg/monitoring/monitoring.go]")

type Monitor struct {
	streamReader      broker.StreamReader
	e2NodeID          topoapi.ID
	indicationChannel chan *mho.E2NodeIndication
	triggerType       e2sm_mho.MhoTriggerType
}

func NewMonitor(streamReader broker.StreamReader, e2NodeID topoapi.ID, indicationChannel chan *mho.E2NodeIndication, triggerType e2sm_mho.MhoTriggerType) *Monitor {

	return &Monitor{

		streamReader:      streamReader,
		e2NodeID:          e2NodeID,
		indicationChannel: indicationChannel,
		triggerType:       triggerType,
	}

}

func (self *Monitor) Start(context context.Context) error {

	errorChannel := make(chan error)

	go func() {

		for {

			indicationMessage, err := self.streamReader.Recv(context)

			if err != nil {

				log.Errorf("Error occured during reading indication stream [Start()], [channelID: %v, streamID: %v, error: %v]", self.streamReader.ChannelID(), self.streamReader.StreamID(), err)
				errorChannel <- err

			}

			err = self.processIndication(context, indicationMessage, self.e2NodeID)

			if err != nil {

				log.Errorf("Error occured during processing indication [Start()].", err)
				errorChannel <- err

			}
		}
	}()

	select {

	case err := <-errorChannel:

		return err

	case <-context.Done():

		return context.Err()

	}

}

func (self *Monitor) processIndication(context context.Context, indication e2API.Indication, e2NodeID topoapi.ID) error {
	log.Debugf("Processing new indication... [nodeID: %v, indication: %v ]", e2NodeID, indication)

	self.indicationChannel <- mho.NewE2NodeIndication(
		string(e2NodeID),
		self.triggerType,
		e2ind.Indication{
			Payload: e2ind.Payload{
				Header:  indication.Header,
				Message: indication.Payload,
			},
		})

	return nil
}
