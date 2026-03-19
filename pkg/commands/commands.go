// Package commands provides command registration and execution (stub for voicebot).
package commands

import (
	"context"
	"strings"
)

// Definition represents a command definition.
type Definition struct {
	Name        string
	Description string
	Usage       string
}

// BuiltinDefinitions returns built-in command definitions.
func BuiltinDefinitions() []Definition {
	return []Definition{
		{Name: "help", Description: "Show help"},
		{Name: "exit", Description: "Exit the session"},
	}
}

// HasCommandPrefix checks if the text starts with a command prefix.
func HasCommandPrefix(text string) bool {
	return strings.HasPrefix(text, "/") || strings.HasPrefix(text, "!")
}

// ParseCommand parses a command from text.
func ParseCommand(text string) (cmd string, args []string, ok bool) {
	text = strings.TrimSpace(text)
	if !HasCommandPrefix(text) {
		return "", nil, false
	}
	text = text[1:] // Remove prefix
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return "", nil, false
	}
	return parts[0], parts[1:], true
}

// Outcome represents the result of command execution.
type Outcome int

const (
	OutcomeHandled Outcome = iota
	OutcomeNotHandled
	OutcomeError
)

// Registry is a command registry.
type Registry struct {
	commands    map[string]Command
	definitions []Definition
}

// Command represents a command.
type Command interface {
	Name() string
	Description() string
	Execute(ctx context.Context, args []string) (string, error)
}

// NewRegistry creates a new command registry.
func NewRegistry(defs ...[]Definition) *Registry {
	r := &Registry{
		commands:    make(map[string]Command),
		definitions: BuiltinDefinitions(),
	}
	if len(defs) > 0 {
		r.definitions = append(r.definitions, defs[0]...)
	}
	return r
}

// Register registers a command.
func (r *Registry) Register(cmd Command) {
	r.commands[cmd.Name()] = cmd
}

// Definitions returns all command definitions.
func (r *Registry) Definitions() []Definition {
	return r.definitions
}

// Runtime provides runtime context for command execution.
type Runtime struct {
	Workspace          string
	ListDefinitions    []Definition
	Config             interface{}                     // Config reference
	ListAgentIDs       func() []string                 // Function to list agent IDs
	GetEnabledChannels func() []string                 // Function to get enabled channels
	SwitchChannel      func(string) error              // Function to switch channel
	GetModelInfo       func() (string, string)         // Function to get model info (model, provider)
	SwitchModel        func(string) (string, error)    // Function to switch model (returns old model)
	ClearHistory       func() error                    // Function to clear history
}

// NewRuntime creates a new runtime.
func NewRuntime(workspace string, defs []Definition) *Runtime {
	return &Runtime{
		Workspace:       workspace,
		ListDefinitions: defs,
		// Provide default no-op implementations
		GetModelInfo:  func() (string, string) { return "unknown", "unknown" },
		SwitchModel:   func(string) (string, error) { return "", nil },
		ClearHistory:  func() error { return nil },
		SwitchChannel: func(string) error { return nil },
	}
}

// Executor executes commands.
type Executor struct {
	registry *Registry
	runtime  *Runtime
}

// NewExecutor creates a new command executor.
func NewExecutor(registry *Registry, runtime *Runtime) *Executor {
	return &Executor{
		registry: registry,
		runtime:  runtime,
	}
}

// Request is a command execution request.
type Request struct {
	Command  string
	Args     []string
	Input    string
	Channel  string
	ChatID   string
	SenderID string
	Text     string
	Reply    func(text string) error
}

// ExecuteResult is a command execution result.
type ExecuteResult struct {
	Output   string
	Error    error
	Outcome  Outcome
	Command  string
	Err      error
}

// Execute executes a command.
func (e *Executor) Execute(ctx context.Context, req Request) ExecuteResult {
	cmd, ok := e.registry.commands[req.Command]
	if !ok {
		return ExecuteResult{Error: ErrCommandNotFound, Outcome: OutcomeNotHandled, Command: req.Command}
	}
	output, err := cmd.Execute(ctx, req.Args)
	outcome := OutcomeHandled
	if err != nil {
		outcome = OutcomeError
	}
	return ExecuteResult{Output: output, Error: err, Outcome: outcome, Command: cmd.Name(), Err: err}
}

// ErrCommandNotFound is returned when a command is not found.
var ErrCommandNotFound = &CommandError{Message: "command not found"}

// CommandError is a command error.
type CommandError struct {
	Message string
}

func (e *CommandError) Error() string {
	return e.Message
}
