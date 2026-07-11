package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDisabledAndUntrustedServersAreNotContacted(t *testing.T) {
	var calls atomic.Int32
	client, err := New([]ServerConfig{
		{Name: "off", Enabled: false, Trusted: true, Transport: TransportStdio, Command: "off"},
		{Name: "unsafe", Enabled: true, Trusted: false, Transport: TransportStdio, Command: "unsafe"},
	}, Options{TransportFactory: TransportFactoryFunc(func(context.Context, ServerConfig) (mcp.Transport, error) {
		calls.Add(1)
		return nil, errors.New("should not be called")
	})})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatalf("factory called %d times", calls.Load())
	}
	statuses := client.ServerStatuses()
	if statuses[0].State != StateDisabled || statuses[1].State != StateUntrusted {
		t.Fatalf("unexpected statuses: %#v", statuses)
	}
	if len(client.AgentTools()) != 0 {
		t.Fatal("inactive servers exposed tools")
	}
}

func TestStatusAndErrorsRedactConnectionSecrets(t *testing.T) {
	const headerSecret = "header-token-very-secret"
	const envSecret = "environment-token-very-secret"
	const querySecret = "query-token-very-secret"
	client, err := New([]ServerConfig{{
		Name: "remote", Enabled: true, Trusted: true, Transport: TransportStreamableHTTP,
		Endpoint:    "https://example.test/mcp?token=" + querySecret,
		Headers:     map[string]string{"Authorization": "Bearer " + headerSecret},
		Environment: map[string]string{"TOKEN": envSecret},
	}}, Options{TransportFactory: TransportFactoryFunc(func(_ context.Context, cfg ServerConfig) (mcp.Transport, error) {
		return nil, errors.New(cfg.Endpoint + " " + cfg.Headers["Authorization"] + " " + cfg.Environment["TOKEN"])
	})})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	err = client.Connect(context.Background())
	if err == nil {
		t.Fatal("Connect succeeded unexpectedly")
	}
	encoded, marshalErr := json.Marshal(struct {
		Error  string         `json:"error"`
		Status []ServerStatus `json:"status"`
	}{err.Error(), client.ServerStatuses()})
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	text := string(encoded)
	for _, secret := range []string{headerSecret, envSecret, querySecret} {
		if strings.Contains(text, secret) {
			t.Fatalf("secret %q leaked in %s", secret, text)
		}
	}
	if !strings.Contains(text, "[redacted]") {
		t.Fatalf("redaction marker missing from %s", text)
	}
}

func TestConfigurationIsDefensivelyCopied(t *testing.T) {
	headers := map[string]string{"Authorization": "original"}
	cfg := ServerConfig{Name: "copy", Enabled: false, Trusted: true, Transport: TransportStreamableHTTP,
		Endpoint: "https://example.test/mcp", Headers: headers}
	client, err := New([]ServerConfig{cfg}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	headers["Authorization"] = "mutated"
	if got := client.servers["copy"].config.Headers["Authorization"]; got != "original" {
		t.Fatalf("stored header = %q", got)
	}
}
