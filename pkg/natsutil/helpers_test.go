package natsutil

import (
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/pashagolub/pgxmock/v4"
)

// fakeJetStream implementa nats.JetStreamContext embebiendo la interfaz (nil)
// y sobreescribiendo solo los métodos que necesita cada test (consumers,
// streams, publisher).
type fakeJetStream struct {
	nats.JetStreamContext

	consumerInfoResult *nats.ConsumerInfo
	consumerInfoErr    error
	consumerInfoCalls  []string // "stream/name"

	addConsumerErr   error
	addConsumerCalls []*nats.ConsumerConfig

	streamInfoResult *nats.StreamInfo
	streamInfoErr    error
	streamInfoCalls  []string

	addStreamErr   error
	addStreamCalls []*nats.StreamConfig

	publishErr error
	published  []*nats.Msg
	ackSeq     uint64
}

func (f *fakeJetStream) ConsumerInfo(stream, name string, _ ...nats.JSOpt) (*nats.ConsumerInfo, error) {
	f.consumerInfoCalls = append(f.consumerInfoCalls, stream+"/"+name)
	return f.consumerInfoResult, f.consumerInfoErr
}

func (f *fakeJetStream) AddConsumer(_ string, cfg *nats.ConsumerConfig, _ ...nats.JSOpt) (*nats.ConsumerInfo, error) {
	f.addConsumerCalls = append(f.addConsumerCalls, cfg)
	if f.addConsumerErr != nil {
		return nil, f.addConsumerErr
	}
	return &nats.ConsumerInfo{}, nil
}

func (f *fakeJetStream) StreamInfo(stream string, _ ...nats.JSOpt) (*nats.StreamInfo, error) {
	f.streamInfoCalls = append(f.streamInfoCalls, stream)
	return f.streamInfoResult, f.streamInfoErr
}

func (f *fakeJetStream) AddStream(cfg *nats.StreamConfig, _ ...nats.JSOpt) (*nats.StreamInfo, error) {
	f.addStreamCalls = append(f.addStreamCalls, cfg)
	if f.addStreamErr != nil {
		return nil, f.addStreamErr
	}
	return &nats.StreamInfo{}, nil
}

func (f *fakeJetStream) PublishMsg(m *nats.Msg, _ ...nats.PubOpt) (*nats.PubAck, error) {
	f.published = append(f.published, m)
	if f.publishErr != nil {
		return nil, f.publishErr
	}
	return &nats.PubAck{Sequence: f.ackSeq}, nil
}

// newMockPool crea un pool pgxmock y registra su cierre y la verificación de
// expectations al finalizar el test.
func newMockPool(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()
	pool, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool() error = %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		if err := pool.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet pgxmock expectations: %v", err)
		}
	})
	return pool
}
