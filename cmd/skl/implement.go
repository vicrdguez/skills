package main

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

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
