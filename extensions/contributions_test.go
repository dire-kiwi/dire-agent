package extensions

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestRegisteredCommandsPromptsHooksAndUI(t *testing.T) {
	connection := newFakeConnection()
	connection.registration = Registration{
		Commands:        []CommandSpec{{Name: "deploy", Description: "Deploy safely."}},
		PromptFragments: []PromptFragment{{ID: "system", Text: "Follow deployment policy.", Priority: 10}},
		Hooks: []HookSpec{
			{ID: "block", Event: HookBeforePrompt, Priority: 20},
			{ID: "prefix", Event: HookBeforePrompt, Priority: 10},
		},
		SettingsSchema: json.RawMessage(`{"type":"object"}`),
		UI:             []UIContribution{{ID: "deploy-status", Kind: "status", Title: "Deploy", Schema: json.RawMessage(`{"type":"object"}`)}},
		ToolRenderers:  []ToolRenderer{{Tool: "echo", Style: "key-value", Label: "Echo"}},
	}
	client := openFake(t, connection, Limits{})
	defer client.Close(context.Background())
	registration := client.Registration()
	if len(registration.Commands) != 1 || len(registration.PromptFragments) != 1 || len(registration.UI) != 1 {
		t.Fatalf("registration = %+v", registration)
	}
	result, err := client.ExecuteCommand(context.Background(), "deploy", "staging")
	if err != nil || result.Output != "command complete" || result.Prompt != "run command" {
		t.Fatalf("command = %+v, %v", result, err)
	}
	payload, veto, err := client.InvokeHooks(context.Background(), HookBeforePrompt, HookPayload{Prompt: "ship"})
	if err != nil || payload.Prompt != "checked: ship" || veto == nil || !veto.Veto || veto.Message != "blocked" {
		t.Fatalf("hooks = payload %+v veto %+v err %v", payload, veto, err)
	}
	status, err := client.Status(context.Background())
	if err != nil || status.Level != "ready" {
		t.Fatalf("status = %+v, %v", status, err)
	}
	if _, err := client.ExecuteCommand(context.Background(), "missing", ""); err == nil {
		t.Fatal("unknown command succeeded")
	}
}

func TestInvalidRegistrationFailsInitialization(t *testing.T) {
	connection := newFakeConnection()
	connection.registration = Registration{Hooks: []HookSpec{{ID: "bad", Event: "unknown"}}}
	_, err := Open(context.Background(), trustedFakeConfig(), OpenOptions{Connector: staticConnector(connection)})
	if !errors.Is(err, errInvalidRegistration) {
		t.Fatalf("error = %v", err)
	}
	if !connection.wasClosed() {
		t.Fatal("invalid extension was not cleaned up")
	}
}
