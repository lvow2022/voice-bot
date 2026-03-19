// Package commands provides command registration and execution (stub for voicebot).
package commands

import "context"

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
	Workspace       string
	ListDefinitions []Definition
}

// NewRuntime creates a new runtime.
func NewRuntime(workspace string, defs []Definition) *Runtime {
	return &Runtime{
		Workspace:       workspace,
		ListDefinitions: defs,
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
	Command string
	Args    []string
	Input   string
}

// ExecuteResult is a command execution result.
type ExecuteResult struct {
	Output string
	Error  error
}

// Execute executes a command.
func (e *Executor) Execute(ctx context.Context, req Request) ExecuteResult {
	cmd, ok := e.registry.commands[req.Command]
	if !ok {
		return ExecuteResult{Error: ErrCommandNotFound}
	}
	output, err := cmd.Execute(ctx, req.Args)
	return ExecuteResult{Output: output, Error: err}
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
