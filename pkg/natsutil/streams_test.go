package natsutil

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

// ─── EnsureStreams ─────────────────────────────────────────────────────────────

func TestEnsureStreams_CreatesAllMissingStreams(t *testing.T) {
	js := &fakeJetStream{streamInfoErr: nats.ErrStreamNotFound}

	if err := EnsureStreams(js); err != nil {
		t.Fatalf("EnsureStreams() error = %v", err)
	}

	want := allStreamConfigs()
	if len(js.addStreamCalls) != len(want) {
		t.Fatalf("AddStream calls = %d, want %d", len(js.addStreamCalls), len(want))
	}
	for i, cfg := range js.addStreamCalls {
		if cfg.Name != want[i].Name {
			t.Errorf("call[%d].Name = %q, want %q", i, cfg.Name, want[i].Name)
		}
		if len(cfg.Subjects) != len(want[i].Subjects) || cfg.Subjects[0] != want[i].Subjects[0] {
			t.Errorf("call[%d].Subjects = %v, want %v", i, cfg.Subjects, want[i].Subjects)
		}
		if cfg.MaxAge != time.Duration(want[i].MaxAge)*time.Second {
			t.Errorf("call[%d].MaxAge = %v, want %v", i, cfg.MaxAge, time.Duration(want[i].MaxAge)*time.Second)
		}
		if cfg.Replicas != 1 {
			t.Errorf("call[%d].Replicas = %d, want 1", i, cfg.Replicas)
		}
		if cfg.Storage != nats.FileStorage {
			t.Errorf("call[%d].Storage = %v, want %v", i, cfg.Storage, nats.FileStorage)
		}
		if cfg.Retention != nats.LimitsPolicy {
			t.Errorf("call[%d].Retention = %v, want %v", i, cfg.Retention, nats.LimitsPolicy)
		}
	}
}

func TestEnsureStreams_SkipsExistingStreams(t *testing.T) {
	js := &fakeJetStream{streamInfoResult: &nats.StreamInfo{}}

	if err := EnsureStreams(js); err != nil {
		t.Fatalf("EnsureStreams() error = %v", err)
	}
	if len(js.addStreamCalls) != 0 {
		t.Errorf("AddStream calls = %d, want 0 (todos los streams ya existen)", len(js.addStreamCalls))
	}
	if len(js.streamInfoCalls) != len(allStreamConfigs()) {
		t.Errorf("StreamInfo calls = %d, want %d", len(js.streamInfoCalls), len(allStreamConfigs()))
	}
}

func TestEnsureStreams_StreamInfoError(t *testing.T) {
	js := &fakeJetStream{streamInfoErr: errors.New("connection reset")}

	err := EnsureStreams(js)
	if err == nil || !strings.Contains(err.Error(), "natsutil: stream info") {
		t.Fatalf("error = %v, want it to contain %q", err, "natsutil: stream info")
	}
	if len(js.streamInfoCalls) != 1 {
		t.Errorf("StreamInfo calls = %d, want 1 (debe cortar en el primer error)", len(js.streamInfoCalls))
	}
}

func TestEnsureStreams_AddStreamError(t *testing.T) {
	js := &fakeJetStream{
		streamInfoErr: nats.ErrStreamNotFound,
		addStreamErr:  errors.New("invalid subject"),
	}

	err := EnsureStreams(js)
	if err == nil || !strings.Contains(err.Error(), "natsutil: create stream") {
		t.Fatalf("error = %v, want it to contain %q", err, "natsutil: create stream")
	}
	if len(js.addStreamCalls) != 1 {
		t.Errorf("AddStream calls = %d, want 1 (debe cortar en el primer error)", len(js.addStreamCalls))
	}
}

// ─── allStreamConfigs ──────────────────────────────────────────────────────────

func TestAllStreamConfigs_NoDuplicateNames(t *testing.T) {
	seen := map[string]bool{}
	for _, cfg := range allStreamConfigs() {
		if seen[cfg.Name] {
			t.Errorf("duplicate stream name: %q", cfg.Name)
		}
		seen[cfg.Name] = true
	}
}

func TestAllStreamConfigs_AllFieldsPopulated(t *testing.T) {
	for _, cfg := range allStreamConfigs() {
		if cfg.Name == "" {
			t.Errorf("config %+v has empty Name", cfg)
		}
		if len(cfg.Subjects) == 0 {
			t.Errorf("config %q has no subjects", cfg.Name)
		}
	}
}
