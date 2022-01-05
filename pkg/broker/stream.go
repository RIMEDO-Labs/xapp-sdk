package broker

import (
	"container/list"
	"context"
	"io"
	"sync"

	e2api "github.com/onosproject/onos-api/go/onos/e2t/e2/v1beta1"
	"github.com/onosproject/onos-lib-go/pkg/errors"
	e2client "github.com/onosproject/onos-ric-sdk-go/pkg/e2/v1beta1"
)

const bufferMaxSize = 10000

type StreamID int

type StreamIO interface {
	io.Closer
	ChannelID() e2api.ChannelID
	StreamID() StreamID
	SubscriptionName() string
	Subscription() e2api.SubscriptionSpec
	Node() e2client.Node
}

func (s *bufferedReader) Recv(ctx context.Context) (e2api.Indication, error) {
	select {
	case ind, ok := <-s.ch:
		if !ok {
			return e2api.Indication{}, io.EOF
		}
		return ind, nil
	case <-ctx.Done():
		return e2api.Indication{}, ctx.Err()
	}
}

type StreamReader interface {
	StreamIO
	Recv(context.Context) (e2api.Indication, error)
}

func (s *bufferedWriter) Send(ind e2api.Indication) error {
	s.cond.L.Lock()
	defer s.cond.L.Unlock()
	if s.closed {
		return io.EOF
	}
	if s.buffer.Len() == bufferMaxSize {
		return errors.NewUnavailable("cannot append indication to stream: maximum buffer size has been reached")
	}
	s.buffer.PushBack(ind)
	s.cond.Signal()
	return nil
}

type StreamWriter interface {
	StreamIO
	Send(indication e2api.Indication) error
}

type Stream interface {
	StreamIO
	StreamReader
	StreamWriter
}

func (s *bufferedWriter) next() (e2api.Indication, bool) {
	s.cond.L.Lock()
	defer s.cond.L.Unlock()
	for s.buffer.Len() == 0 {
		if s.closed {
			return e2api.Indication{}, false
		}
		s.cond.Wait()
	}
	result := s.buffer.Front().Value.(e2api.Indication)
	s.buffer.Remove(s.buffer.Front())
	return result, true
}

func (s *bufferedWriter) drain() {
	for {
		ind, ok := s.next()
		if !ok {
			close(s.ch)
			break
		}
		s.ch <- ind
	}
}

func (s *bufferedWriter) open() {
	go s.drain()
}

type bufferedReader struct {
	ch <-chan e2api.Indication
}

func newBufferedReader(ch <-chan e2api.Indication) *bufferedReader {
	return &bufferedReader{
		ch: ch,
	}
}

type bufferedWriter struct {
	ch     chan<- e2api.Indication
	buffer *list.List
	cond   *sync.Cond
	closed bool
}

func newBufferedWriter(ch chan<- e2api.Indication) *bufferedWriter {
	writer := &bufferedWriter{
		ch:     ch,
		buffer: list.New(),
		cond:   sync.NewCond(&sync.Mutex{}),
	}
	writer.open()
	return writer
}

type bufferedIO struct {
	subSepc   e2api.SubscriptionSpec
	streamID  StreamID
	channelID e2api.ChannelID
	node      e2client.Node
	subName   string
}

type bufferedStream struct {
	*bufferedIO
	*bufferedReader
	*bufferedWriter
}

func (s *bufferedIO) ChannelID() e2api.ChannelID {
	return s.channelID
}

func (s *bufferedWriter) Close() error {
	s.cond.L.Lock()
	defer s.cond.L.Unlock()
	s.closed = true
	return nil
}

func (s *bufferedIO) Node() e2client.Node {
	return s.node
}

func (s *bufferedIO) StreamID() StreamID {
	return s.streamID
}

func (s *bufferedIO) Subscription() e2api.SubscriptionSpec {
	return s.subSepc
}

func (s *bufferedIO) SubscriptionName() string {
	return s.subName
}

func newBufferedStream(node e2client.Node, subName string, streamID StreamID, channelID e2api.ChannelID, subSpec e2api.SubscriptionSpec) Stream {
	ch := make(chan e2api.Indication)
	return &bufferedStream{
		bufferedIO: &bufferedIO{
			streamID:  streamID,
			channelID: channelID,
			subSepc:   subSpec,
			node:      node,
			subName:   subName,
		},
		bufferedReader: newBufferedReader(ch),
		bufferedWriter: newBufferedWriter(ch),
	}
}
