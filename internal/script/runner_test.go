package script

import (
	"testing"

	"github.com/google/uuid"
)

func TestValidate_Valid(t *testing.T) {
	err := Validate(`function transform(event) { return event; }`)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidate_SyntaxError(t *testing.T) {
	err := Validate(`function transform(event { return event; }`)
	if err == nil {
		t.Fatal("expected error for syntax error")
	}
}

func TestValidate_MissingTransform(t *testing.T) {
	err := Validate(`function process(event) { return event; }`)
	if err != ErrNoTransform {
		t.Fatalf("expected ErrNoTransform, got: %v", err)
	}
}

func TestValidate_NotAFunction(t *testing.T) {
	err := Validate(`var transform = 42;`)
	if err != ErrNoTransform {
		t.Fatalf("expected ErrNoTransform, got: %v", err)
	}
}

func TestValidate_TooLarge(t *testing.T) {
	large := "function transform(e) { return e; }" + string(make([]byte, maxScriptSize+1))
	err := Validate(large)
	if err != ErrScriptTooLarge {
		t.Fatalf("expected ErrScriptTooLarge, got: %v", err)
	}
}

func TestRun_BasicTransform(t *testing.T) {
	script := `function transform(event) {
		event.payload.processed = true;
		return event;
	}`

	input := TransformInput{
		Payload:       map[string]any{"type": "push"},
		Headers:       map[string]string{"Content-Type": "application/json"},
		Subscriptions: []SubscriptionRef{{ID: uuid.New(), TargetURL: "https://example.com"}},
	}

	result, err := Run(script, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Dropped {
		t.Fatal("expected event not to be dropped")
	}
	if result.Payload["processed"] != true {
		t.Fatalf("expected processed=true, got: %v", result.Payload["processed"])
	}
	if result.Payload["type"] != "push" {
		t.Fatalf("expected type=push, got: %v", result.Payload["type"])
	}
}

func TestRun_Drop(t *testing.T) {
	script := `function transform(event) { return null; }`

	input := TransformInput{
		Payload:       map[string]any{"type": "ping"},
		Headers:       map[string]string{},
		Subscriptions: []SubscriptionRef{},
	}

	result, err := Run(script, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Dropped {
		t.Fatal("expected event to be dropped")
	}
}

func TestRun_HeaderModification(t *testing.T) {
	script := `function transform(event) {
		event.headers["X-Processed"] = "true";
		return event;
	}`

	input := TransformInput{
		Payload:       map[string]any{},
		Headers:       map[string]string{"Content-Type": "application/json"},
		Subscriptions: []SubscriptionRef{},
	}

	result, err := Run(script, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Headers["X-Processed"] != "true" {
		t.Fatalf("expected X-Processed header, got: %v", result.Headers)
	}
	if result.Headers["Content-Type"] != "application/json" {
		t.Fatalf("expected Content-Type preserved, got: %v", result.Headers)
	}
}

func TestRun_SubscriptionFiltering(t *testing.T) {
	sub1 := SubscriptionRef{ID: uuid.New(), TargetURL: "https://production.example.com/hook"}
	sub2 := SubscriptionRef{ID: uuid.New(), TargetURL: "https://staging.example.com/hook"}

	script := `function transform(event) {
		event.subscriptions = event.subscriptions.filter(function(s) {
			return s.target_url.indexOf("production") !== -1;
		});
		return event;
	}`

	input := TransformInput{
		Payload:       map[string]any{},
		Headers:       map[string]string{},
		Subscriptions: []SubscriptionRef{sub1, sub2},
	}

	result, err := Run(script, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Subscriptions) != 1 {
		t.Fatalf("expected 1 subscription, got: %d", len(result.Subscriptions))
	}
	if result.Subscriptions[0].ID != sub1.ID {
		t.Fatalf("expected production sub, got: %v", result.Subscriptions[0])
	}
}

func TestRun_Timeout(t *testing.T) {
	script := `function transform(event) { while(true) {} return event; }`

	input := TransformInput{
		Payload:       map[string]any{},
		Headers:       map[string]string{},
		Subscriptions: []SubscriptionRef{},
	}

	_, err := Run(script, input)
	if err != ErrScriptTimeout {
		t.Fatalf("expected ErrScriptTimeout, got: %v", err)
	}
}

func TestRun_SyntaxError(t *testing.T) {
	script := `function transform(event { return event; }`

	input := TransformInput{
		Payload:       map[string]any{},
		Headers:       map[string]string{},
		Subscriptions: []SubscriptionRef{},
	}

	_, err := Run(script, input)
	if err == nil {
		t.Fatal("expected error for syntax error")
	}
}

func TestRun_ConditionalDrop(t *testing.T) {
	script := `function transform(event) {
		if (event.payload.type === "ping") return null;
		event.payload.processed_at = "2024-01-01";
		return event;
	}`

	// Test drop case
	input := TransformInput{
		Payload:       map[string]any{"type": "ping"},
		Headers:       map[string]string{},
		Subscriptions: []SubscriptionRef{},
	}
	result, err := Run(script, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Dropped {
		t.Fatal("expected ping to be dropped")
	}

	// Test pass-through case
	input.Payload = map[string]any{"type": "push"}
	result, err = Run(script, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Dropped {
		t.Fatal("expected push not to be dropped")
	}
	if result.Payload["processed_at"] != "2024-01-01" {
		t.Fatalf("expected processed_at, got: %v", result.Payload)
	}
}
