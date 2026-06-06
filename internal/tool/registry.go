package tool

import (
	"context"
	"fmt"
	"sync"

	"github.com/cloudwego/eino/schema"

	"github.com/lsxiaoxin/GoClaw/internal/permission"
)

// Call is one model-requested tool invocation.
type Call struct {
	Name      string
	Arguments string
}

// Result is one tool result. Results retain the same order as calls.
type Result struct {
	Output string
	Err    error
}

// Registry owns tool definitions and dispatch.
type Registry struct {
	tools  map[string]Tool
	infos  []*schema.ToolInfo
	policy *permission.Engine
}

// NewRegistry creates a registry and rejects invalid or duplicate tools.
func NewRegistry(tools ...Tool) (*Registry, error) {
	registry := &Registry{
		tools:  make(map[string]Tool, len(tools)),
		infos:  make([]*schema.ToolInfo, 0, len(tools)),
		policy: permission.New(),
	}
	for _, registered := range tools {
		if registered == nil {
			return nil, fmt.Errorf("tool is required")
		}
		info := registered.Info()
		if info == nil || info.Name == "" {
			return nil, fmt.Errorf("tool name is required")
		}
		if _, exists := registry.tools[info.Name]; exists {
			return nil, fmt.Errorf("duplicate tool %q", info.Name)
		}
		registry.tools[info.Name] = registered
		registry.infos = append(registry.infos, info)
	}
	return registry, nil
}

// Permission evaluates one call without executing it.
func (r *Registry) Permission(call Call) permission.Decision {
	registered, exists := r.tools[call.Name]
	if !exists {
		return permission.Decision{
			Behavior: permission.Invalid,
			Reason:   fmt.Sprintf("tool %q is not available", call.Name),
		}
	}
	return r.policy.Decide(call.Name, call.Arguments, registered.Validate)
}

// Infos returns model-facing tool definitions in registration order.
func (r *Registry) Infos() []*schema.ToolInfo {
	return append([]*schema.ToolInfo(nil), r.infos...)
}

// Execute runs consecutive concurrency-safe tools in parallel and all other
// tools sequentially. Returned results always match the original call order.
func (r *Registry) Execute(ctx context.Context, calls []Call) []Result {
	results := make([]Result, len(calls))
	for start := 0; start < len(calls); {
		if !r.concurrencySafe(calls[start].Name) {
			results[start] = r.executeOne(ctx, calls[start])
			start++
			continue
		}

		end := start + 1
		for end < len(calls) && r.concurrencySafe(calls[end].Name) {
			end++
		}
		r.executeConcurrent(ctx, calls[start:end], results[start:end])
		start = end
	}
	return results
}

func (r *Registry) concurrencySafe(name string) bool {
	registered, exists := r.tools[name]
	return exists && registered.ConcurrencySafe()
}

func (r *Registry) executeConcurrent(ctx context.Context, calls []Call, results []Result) {
	var group sync.WaitGroup
	group.Add(len(calls))
	for index := range calls {
		go func() {
			defer group.Done()
			results[index] = r.executeOne(ctx, calls[index])
		}()
	}
	group.Wait()
}

func (r *Registry) executeOne(ctx context.Context, call Call) Result {
	if err := ctx.Err(); err != nil {
		return Result{Err: err}
	}
	registered, exists := r.tools[call.Name]
	if !exists {
		return Result{Err: fmt.Errorf("tool %q is not available", call.Name)}
	}
	output, err := registered.Run(ctx, call.Arguments)
	return Result{Output: output, Err: err}
}
