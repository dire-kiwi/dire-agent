package daemon

import (
	"context"
	"testing"

	"github.com/dire-kiwi/dire-agent/threadstore"
)

func TestTakeFollowUpOrSettleClosesPromptStartWindow(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	runtime := &threadRuntime{
		running: true,
		cancel:  cancel,
		thread: threadstore.Thread{
			Status:       "running",
			FollowUpMode: "all",
		},
	}
	t.Cleanup(cancel)

	if next := runtime.takeFollowUpOrSettle(); next != "" {
		t.Fatalf("next follow-up = %q, want empty", next)
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if !runtime.finishing {
		t.Fatal("runtime did not enter finishing before releasing the settlement lock")
	}
	if !runtime.running {
		t.Fatal("runtime stopped running before its finalizer")
	}
	if runtime.cancel != nil {
		t.Fatal("runtime retained cancellation after entering settlement")
	}
	if runtime.thread.Status != "running" {
		t.Fatalf("thread status = %q, want running until finalization", runtime.thread.Status)
	}
}

func TestTakeFollowUpOrSettleKeepsRunOpenForQueuedFollowUp(t *testing.T) {
	runtime := &threadRuntime{
		running:   true,
		followUps: []string{"first", "second"},
		thread: threadstore.Thread{
			Status:       "running",
			FollowUpMode: "one-at-a-time",
		},
	}

	if next := runtime.takeFollowUpOrSettle(); next != "first" {
		t.Fatalf("next follow-up = %q, want first", next)
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.finishing {
		t.Fatal("runtime entered finishing with a queued follow-up")
	}
	if !runtime.running || runtime.thread.Status != "running" {
		t.Fatalf("runtime state after follow-up: running=%v status=%q", runtime.running, runtime.thread.Status)
	}
	if len(runtime.followUps) != 1 || runtime.followUps[0] != "second" {
		t.Fatalf("remaining follow-ups = %#v, want second", runtime.followUps)
	}
}
