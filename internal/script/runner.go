package script

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dop251/goja"
	"github.com/google/uuid"
)

const (
	maxScriptSize = 64 * 1024 // 64KB
	execTimeout   = 500 * time.Millisecond
)

var (
	ErrScriptTooLarge = errors.New("script exceeds 64KB limit")
	ErrScriptTimeout  = errors.New("script execution timed out")
	ErrNoTransform    = errors.New("script must define a 'transform' function")
	ErrNoProcess      = errors.New("script must define a 'process' function")
)

// ActionRef is a lightweight action reference passed into/out of scripts.
type ActionRef struct {
	ID        uuid.UUID `json:"id"`
	TargetURL string    `json:"target_url"`
}

// TransformInput is the data passed to the transform function.
type TransformInput struct {
	Payload map[string]any    `json:"payload"`
	Headers map[string]string `json:"headers"`
	Actions []ActionRef       `json:"actions"`
}

// TransformResult is the output of the transform function.
type TransformResult struct {
	Payload map[string]any    `json:"payload"`
	Headers map[string]string `json:"headers"`
	Actions []ActionRef       `json:"actions"`
	Dropped bool              `json:"dropped"`
}

// Validate checks that the script compiles and exports a 'transform' function.
func Validate(scriptBody string) error {
	if len(scriptBody) > maxScriptSize {
		return ErrScriptTooLarge
	}

	vm := goja.New()
	_, err := vm.RunString(scriptBody)
	if err != nil {
		return fmt.Errorf("script compilation error: %w", err)
	}

	fn := vm.Get("transform")
	if fn == nil || fn == goja.Undefined() || fn == goja.Null() {
		return ErrNoTransform
	}
	if _, ok := goja.AssertFunction(fn); !ok {
		return ErrNoTransform
	}

	return nil
}

// Run executes the transform function with the given input.
// Returns nil result with Dropped=true if the script returns null/undefined.
func Run(scriptBody string, input TransformInput) (result *TransformResult, err error) {
	if len(scriptBody) > maxScriptSize {
		return nil, ErrScriptTooLarge
	}

	// Recover from goja panics (e.g., from vm.Interrupt)
	defer func() {
		if r := recover(); r != nil {
			if interrupted, ok := r.(*goja.InterruptedError); ok {
				_ = interrupted
				result = nil
				err = ErrScriptTimeout
			} else {
				result = nil
				err = fmt.Errorf("script panic: %v", r)
			}
		}
	}()

	vm := goja.New()

	// Set up timeout
	timer := time.AfterFunc(execTimeout, func() {
		vm.Interrupt("timeout")
	})
	defer timer.Stop()

	_, err = vm.RunString(scriptBody)
	if err != nil {
		return nil, fmt.Errorf("script compilation error: %w", err)
	}

	transformFn := vm.Get("transform")
	if transformFn == nil || transformFn == goja.Undefined() || transformFn == goja.Null() {
		return nil, ErrNoTransform
	}

	callable, ok := goja.AssertFunction(transformFn)
	if !ok {
		return nil, ErrNoTransform
	}

	// Build the event object for JS
	eventObj := map[string]any{
		"payload": input.Payload,
		"headers": input.Headers,
	}
	actionsForJS := make([]map[string]any, len(input.Actions))
	for i, a := range input.Actions {
		actionsForJS[i] = map[string]any{
			"id":         a.ID.String(),
			"target_url": a.TargetURL,
		}
	}
	eventObj["actions"] = actionsForJS

	arg := vm.ToValue(eventObj)
	ret, err := callable(goja.Undefined(), arg)
	if err != nil {
		// Check if this was a timeout interrupt
		var interrupted *goja.InterruptedError
		if errors.As(err, &interrupted) {
			return nil, ErrScriptTimeout
		}
		return nil, fmt.Errorf("script execution error: %w", err)
	}

	// null/undefined return means drop the event
	if ret == nil || ret == goja.Undefined() || ret == goja.Null() {
		return &TransformResult{Dropped: true}, nil
	}

	// Marshal the result back through JSON to get clean Go types
	exported := ret.Export()
	jsonBytes, err := json.Marshal(exported)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal script result: %w", err)
	}

	var raw struct {
		Payload map[string]any         `json:"payload"`
		Headers map[string]interface{} `json:"headers"`
		Actions []struct {
			ID        string `json:"id"`
			TargetURL string `json:"target_url"`
		} `json:"actions"`
	}
	if err := json.Unmarshal(jsonBytes, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal script result: %w", err)
	}

	// Convert headers to map[string]string
	headers := make(map[string]string, len(raw.Headers))
	for k, v := range raw.Headers {
		headers[k] = fmt.Sprintf("%v", v)
	}

	// Convert actions
	actions := make([]ActionRef, 0, len(raw.Actions))
	for _, a := range raw.Actions {
		id, err := uuid.Parse(a.ID)
		if err != nil {
			continue // skip invalid action IDs
		}
		actions = append(actions, ActionRef{ID: id, TargetURL: a.TargetURL})
	}

	return &TransformResult{
		Payload: raw.Payload,
		Headers: headers,
		Actions: actions,
	}, nil
}

// ValidateAction checks that the script compiles and exports a 'process' function.
func ValidateAction(scriptBody string) error {
	if len(scriptBody) > maxScriptSize {
		return ErrScriptTooLarge
	}

	vm := goja.New()
	_, err := vm.RunString(scriptBody)
	if err != nil {
		return fmt.Errorf("script compilation error: %w", err)
	}

	fn := vm.Get("process")
	if fn == nil || fn == goja.Undefined() || fn == goja.Null() {
		return ErrNoProcess
	}
	if _, ok := goja.AssertFunction(fn); !ok {
		return ErrNoProcess
	}

	return nil
}

// RunAction executes a per-action JS script's process(event) function.
// Returns the result as a JSON string.
func RunAction(scriptBody string, payload map[string]any, headers map[string]string) (result string, err error) {
	if len(scriptBody) > maxScriptSize {
		return "", ErrScriptTooLarge
	}

	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(*goja.InterruptedError); ok {
				result = ""
				err = ErrScriptTimeout
			} else {
				result = ""
				err = fmt.Errorf("script panic: %v", r)
			}
		}
	}()

	vm := goja.New()

	timer := time.AfterFunc(execTimeout, func() {
		vm.Interrupt("timeout")
	})
	defer timer.Stop()

	_, err = vm.RunString(scriptBody)
	if err != nil {
		return "", fmt.Errorf("script compilation error: %w", err)
	}

	processFn := vm.Get("process")
	if processFn == nil || processFn == goja.Undefined() || processFn == goja.Null() {
		return "", ErrNoProcess
	}

	callable, ok := goja.AssertFunction(processFn)
	if !ok {
		return "", ErrNoProcess
	}

	eventObj := map[string]any{
		"payload": payload,
		"headers": headers,
	}

	arg := vm.ToValue(eventObj)
	ret, err := callable(goja.Undefined(), arg)
	if err != nil {
		var interrupted *goja.InterruptedError
		if errors.As(err, &interrupted) {
			return "", ErrScriptTimeout
		}
		return "", fmt.Errorf("script execution error: %w", err)
	}

	if ret == nil || ret == goja.Undefined() || ret == goja.Null() {
		return "null", nil
	}

	exported := ret.Export()
	jsonBytes, err := json.Marshal(exported)
	if err != nil {
		return "", fmt.Errorf("failed to marshal action script result: %w", err)
	}

	return string(jsonBytes), nil
}
