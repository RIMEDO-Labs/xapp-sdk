package broker

import (
	"context"
	"io"
	"sync"

	e2api "github.com/onosproject/onos-api/go/onos/e2t/e2/v1beta1"
	"github.com/onosproject/onos-lib-go/pkg/errors"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	e2client "github.com/onosproject/onos-ric-sdk-go/pkg/e2/v1beta1"
)

var log = logging.GetLogger("broker")

type streamBroker struct {
	subs     map[e2api.ChannelID]Stream
	streams  map[StreamID]Stream
	streamID StreamID
	mu       sync.RWMutex
}

type Broker interface {
	io.Closer
	OpenReader(ctx context.Context, node e2client.Node, subName string, id e2api.ChannelID, subSpec e2api.SubscriptionSpec) (StreamReader, error)
	CloseStream(ctx context.Context, id e2api.ChannelID) (StreamReader, error)
	GetWriter(id StreamID) (StreamWriter, error)
	ChannelIDs() []e2api.ChannelID
}

func (b *streamBroker) OpenReader(ctx context.Context, node e2client.Node, subName string, channelID e2api.ChannelID, subSpec e2api.SubscriptionSpec) (StreamReader, error) {
	b.mu.RLock()
	stream, ok := b.subs[channelID]
	b.mu.RUnlock()
	if ok {
		return stream, nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.streamID++
	streamID := b.streamID
	stream = newBufferedStream(node, subName, streamID, channelID, subSpec)
	b.subs[channelID] = stream
	b.streams[streamID] = stream
	//log.Infof("Opened new stream %d for subscription channel '%s'", streamID, channelID)
	return stream, nil
}

func (b *streamBroker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	var err error
	for _, stream := range b.streams {
		if e := stream.Close(); e != nil {
			err = e
		}
	}
	return err
}

func (b *streamBroker) CloseStream(ctx context.Context, id e2api.ChannelID) (StreamReader, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	stream, ok := b.subs[id]
	if !ok {
		return nil, errors.NewNotFound("subscription '%s' not found", id)
	}

	log.Debugf("Deleting Subscription: %s", stream.SubscriptionName())
	err := stream.Node().Unsubscribe(ctx, stream.SubscriptionName())
	if err != nil {
		return nil, err
	}

	delete(b.subs, stream.ChannelID())
	delete(b.streams, stream.StreamID())

	//log.Infof("Closed stream %d for subscription '%s'", stream.StreamID(), id)
	return stream, stream.Close()
}

func (b *streamBroker) GetWriter(id StreamID) (StreamWriter, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	stream, ok := b.streams[id]
	if !ok {
		return nil, errors.NewNotFound("stream %d not found", id)
	}
	return stream, nil
}

func (b *streamBroker) ChannelIDs() []e2api.ChannelID {
	b.mu.Lock()
	defer b.mu.Unlock()
	channelIDs := make([]e2api.ChannelID, len(b.subs))
	for channelID := range b.subs {
		channelIDs = append(channelIDs, channelID)
	}
	return channelIDs
}

func NewBroker() Broker {
	return &streamBroker{
		subs:    make(map[e2api.ChannelID]Stream),
		streams: make(map[StreamID]Stream),
	}
}

var _ Broker = &streamBroker{}
