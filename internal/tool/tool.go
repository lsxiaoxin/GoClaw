// Package tool implements model tools and their registry.
package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/cloudwego/eino/schema"
)

// Tool is one model-callable operation.
type Tool interface {
	Info() *schema.ToolInfo
	ConcurrencySafe() bool
	Validate(string) error
	Run(context.Context, string) (string, error)
}

func decodeArguments(arguments string, target any) error {
	decoder := json.NewDecoder(strings.NewReader(arguments))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode arguments: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode arguments: multiple JSON values")
		}
		return fmt.Errorf("decode arguments: %w", err)
	}
	return nil
}
