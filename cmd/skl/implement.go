package main

import (
	"fmt"

	"github.com/urfave/cli/v2"
	skilldist "github.com/vicrdguez/skills"
)

// implementationCapabilities is the complete set of established execution
// capabilities. It is never inferred from a harness name.
var implementationCapabilities = map[string]skilldist.ExecutionCapability{
	string(skilldist.UnknownCapability): skilldist.UnknownCapability,
	string(skilldist.ClaudeAgentReview): skilldist.ClaudeAgentReview,
	string(skilldist.PiSubagentReview):  skilldist.PiSubagentReview,
	string(skilldist.SequentialReview):  skilldist.SequentialReview,
}

func implementationCapabilityFlag() cli.Flag {
	return &cli.StringFlag{Name: "capability", Usage: "Established execution capability: claude-agents, pi-subagents, or sequential; omit it when the capability is unknown"}
}

// implementationCapability validates a supplied capability before any
// avoidable backend call, selection, Claim, publication, or result-directory
// creation. An omitted or empty value keeps the runtime choice.
func implementationCapability(value string) (skilldist.ExecutionCapability, error) {
	capability, ok := implementationCapabilities[value]
	if !ok {
		return "", fmt.Errorf("invalid execution capability %q; use claude-agents, pi-subagents, or sequential, or omit the flag when the capability is unknown", value)
	}
	return capability, nil
}

// implementationFormatKind is the complete set of supported transports. Explicit
// JSON preserves the same operation and outcome; it never names a second
// operation or an alternative authority for success.
type implementationFormatKind string

const (
	formatMarkdown implementationFormatKind = "markdown"
	formatJSON     implementationFormatKind = "json"
)

func implementationFormatFlag() cli.Flag {
	return &cli.StringFlag{Name: "format", Value: string(formatMarkdown), Usage: "Output transport: markdown (default) or json"}
}

// implementationFormat validates the requested transport before any avoidable
// backend call, selection, Claim, publication, or result-directory creation.
func implementationFormat(value string) (implementationFormatKind, error) {
	format := implementationFormatKind(value)
	if format != formatMarkdown && format != formatJSON {
		return "", fmt.Errorf("unsupported format %q; use markdown or json", value)
	}
	return format, nil
}
