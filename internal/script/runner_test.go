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
		Payload: map[string]any{"type": "push"},
		Headers: map[string]string{"Content-Type": "application/json"},
		Actions: []ActionRef{{ID: uuid.New(), TargetURL: "https://example.com"}},
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
		Payload: map[string]any{"type": "ping"},
		Headers: map[string]string{},
		Actions: []ActionRef{},
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
		Payload: map[string]any{},
		Headers: map[string]string{"Content-Type": "application/json"},
		Actions: []ActionRef{},
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

func TestRun_ActionFiltering(t *testing.T) {
	action1 := ActionRef{ID: uuid.New(), TargetURL: "https://production.example.com/hook"}
	action2 := ActionRef{ID: uuid.New(), TargetURL: "https://staging.example.com/hook"}

	script := `function transform(event) {
		event.actions = event.actions.filter(function(a) {
			return a.target_url.indexOf("production") !== -1;
		});
		return event;
	}`

	input := TransformInput{
		Payload: map[string]any{},
		Headers: map[string]string{},
		Actions: []ActionRef{action1, action2},
	}

	result, err := Run(script, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Actions) != 1 {
		t.Fatalf("expected 1 action, got: %d", len(result.Actions))
	}
	if result.Actions[0].ID != action1.ID {
		t.Fatalf("expected production action, got: %v", result.Actions[0])
	}
}

func TestRun_Timeout(t *testing.T) {
	script := `function transform(event) { while(true) {} return event; }`

	input := TransformInput{
		Payload: map[string]any{},
		Headers: map[string]string{},
		Actions: []ActionRef{},
	}

	_, err := Run(script, input)
	if err != ErrScriptTimeout {
		t.Fatalf("expected ErrScriptTimeout, got: %v", err)
	}
}

func TestRun_SyntaxError(t *testing.T) {
	script := `function transform(event { return event; }`

	input := TransformInput{
		Payload: map[string]any{},
		Headers: map[string]string{},
		Actions: []ActionRef{},
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
		Payload: map[string]any{"type": "ping"},
		Headers: map[string]string{},
		Actions: []ActionRef{},
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

// Tests for action scripts (RunAction / ValidateAction)

func TestValidateAction_Valid(t *testing.T) {
	err := ValidateAction(`function process(event) { return {result: true}; }`)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateAction_MissingProcess(t *testing.T) {
	err := ValidateAction(`function transform(event) { return event; }`)
	if err != ErrNoProcess {
		t.Fatalf("expected ErrNoProcess, got: %v", err)
	}
}

func TestValidateAction_SyntaxError(t *testing.T) {
	err := ValidateAction(`function process(event { return event; }`)
	if err == nil {
		t.Fatal("expected error for syntax error")
	}
}

func TestRunAction_Basic(t *testing.T) {
	scriptBody := `function process(event) {
		return {processed: true, type: event.payload.type};
	}`

	result, err := RunAction(scriptBody, map[string]any{"type": "push"}, map[string]string{"Content-Type": "application/json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}
	if result == "null" {
		t.Fatal("expected non-null result")
	}
}

func TestRunAction_ReturnsNull(t *testing.T) {
	scriptBody := `function process(event) { return null; }`

	result, err := RunAction(scriptBody, map[string]any{}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "null" {
		t.Fatalf("expected 'null', got: %v", result)
	}
}

func TestRunAction_Timeout(t *testing.T) {
	scriptBody := `function process(event) { while(true) {} }`

	_, err := RunAction(scriptBody, map[string]any{}, map[string]string{})
	if err != ErrScriptTimeout {
		t.Fatalf("expected ErrScriptTimeout, got: %v", err)
	}
}

func TestRunAction_MissingProcess(t *testing.T) {
	scriptBody := `function transform(event) { return event; }`

	_, err := RunAction(scriptBody, map[string]any{}, map[string]string{})
	if err != ErrNoProcess {
		t.Fatalf("expected ErrNoProcess, got: %v", err)
	}
}
