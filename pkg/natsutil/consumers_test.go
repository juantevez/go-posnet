package natsutil

import (
	"errors"
	"strings"
	"testing"

	"github.com/nats-io/nats.go"
)

// ─── EnsureConsumers ───────────────────────────────────────────────────────────

func TestEnsureConsumers_CreatesAllMissingConsumers(t *testing.T) {
	js := &fakeJetStream{consumerInfoErr: nats.ErrConsumerNotFound}

	if err := EnsureConsumers(js); err != nil {
		t.Fatalf("EnsureConsumers() error = %v", err)
	}

	want := allConsumerConfigs()
	if len(js.addConsumerCalls) != len(want) {
		t.Fatalf("AddConsumer calls = %d, want %d", len(js.addConsumerCalls), len(want))
	}
	for i, cfg := range js.addConsumerCalls {
		if cfg.Durable != want[i].Name {
			t.Errorf("call[%d].Durable = %q, want %q", i, cfg.Durable, want[i].Name)
		}
		if cfg.FilterSubject != want[i].FilterSubject {
			t.Errorf("call[%d].FilterSubject = %q, want %q", i, cfg.FilterSubject, want[i].FilterSubject)
		}
		if cfg.MaxDeliver != want[i].MaxDeliver {
			t.Errorf("call[%d].MaxDeliver = %d, want %d", i, cfg.MaxDeliver, want[i].MaxDeliver)
		}
		if cfg.AckWait != want[i].AckWait {
			t.Errorf("call[%d].AckWait = %v, want %v", i, cfg.AckWait, want[i].AckWait)
		}
		if cfg.AckPolicy != nats.AckExplicitPolicy {
			t.Errorf("call[%d].AckPolicy = %v, want %v", i, cfg.AckPolicy, nats.AckExplicitPolicy)
		}
		if cfg.DeliverPolicy != nats.DeliverNewPolicy {
			t.Errorf("call[%d].DeliverPolicy = %v, want %v", i, cfg.DeliverPolicy, nats.DeliverNewPolicy)
		}
		if cfg.ReplayPolicy != nats.ReplayInstantPolicy {
			t.Errorf("call[%d].ReplayPolicy = %v, want %v", i, cfg.ReplayPolicy, nats.ReplayInstantPolicy)
		}
	}
}

func TestEnsureConsumers_SkipsExistingConsumers(t *testing.T) {
	js := &fakeJetStream{consumerInfoResult: &nats.ConsumerInfo{Name: "already-exists"}}

	if err := EnsureConsumers(js); err != nil {
		t.Fatalf("EnsureConsumers() error = %v", err)
	}
	if len(js.addConsumerCalls) != 0 {
		t.Errorf("AddConsumer calls = %d, want 0 (all consumers already exist)", len(js.addConsumerCalls))
	}
	if len(js.consumerInfoCalls) != len(allConsumerConfigs()) {
		t.Errorf("ConsumerInfo calls = %d, want %d (one check per configured consumer)", len(js.consumerInfoCalls), len(allConsumerConfigs()))
	}
}

func TestEnsureConsumers_ConsumerInfoError(t *testing.T) {
	js := &fakeJetStream{consumerInfoErr: errors.New("connection reset")}

	err := EnsureConsumers(js)
	if err == nil || !strings.Contains(err.Error(), "natsutil: consumer info") {
		t.Fatalf("error = %v, want it to contain %q", err, "natsutil: consumer info")
	}
	if len(js.consumerInfoCalls) != 1 {
		t.Errorf("ConsumerInfo calls = %d, want 1 (debe cortar en el primer error)", len(js.consumerInfoCalls))
	}
	if len(js.addConsumerCalls) != 0 {
		t.Errorf("AddConsumer calls = %d, want 0", len(js.addConsumerCalls))
	}
}

func TestEnsureConsumers_AddConsumerError(t *testing.T) {
	js := &fakeJetStream{
		consumerInfoErr: nats.ErrConsumerNotFound,
		addConsumerErr:  errors.New("stream not found"),
	}

	err := EnsureConsumers(js)
	if err == nil || !strings.Contains(err.Error(), "natsutil: create consumer") {
		t.Fatalf("error = %v, want it to contain %q", err, "natsutil: create consumer")
	}
	if len(js.addConsumerCalls) != 1 {
		t.Errorf("AddConsumer calls = %d, want 1 (debe cortar en el primer error)", len(js.addConsumerCalls))
	}
}

// ─── allConsumerConfigs ────────────────────────────────────────────────────────

func TestAllConsumerConfigs_NoDuplicateNames(t *testing.T) {
	seen := map[string]bool{}
	for _, cfg := range allConsumerConfigs() {
		if seen[cfg.Name] {
			t.Errorf("duplicate consumer name: %q", cfg.Name)
		}
		seen[cfg.Name] = true
	}
}

func TestAllConsumerConfigs_AllFieldsPopulated(t *testing.T) {
	for _, cfg := range allConsumerConfigs() {
		if cfg.Name == "" {
			t.Errorf("config %+v has empty Name", cfg)
		}
		if cfg.Stream == "" {
			t.Errorf("config %q has empty Stream", cfg.Name)
		}
		if cfg.FilterSubject == "" {
			t.Errorf("config %q has empty FilterSubject", cfg.Name)
		}
		if cfg.MaxDeliver <= 0 {
			t.Errorf("config %q has non-positive MaxDeliver = %d", cfg.Name, cfg.MaxDeliver)
		}
		if cfg.AckWait <= 0 {
			t.Errorf("config %q has non-positive AckWait = %v", cfg.Name, cfg.AckWait)
		}
	}
}
