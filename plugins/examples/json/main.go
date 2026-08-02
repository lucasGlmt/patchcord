// Command json is an example plugin contributing basic JSON manipulation
// actions — "json.parse@1", "json.stringify@1", "json.jsonpath@1", and
// "json.merge@1" — the kind of utility operations most real workflows need
// (e.g. pulling a field out of an HTTP response body).
//
// "json.jsonpath@1" implements a deliberately minimal subset of JSONPath:
// the root selector "$", dot access (".key"), bracket access with a quoted
// key ("['key']"/`["key"]`) or a numeric index ("[0]"), and the wildcard
// ("[*]"). Recursive descent ("..") and filter expressions ("[?(...)]") are
// not supported in this first version — a narrower, well-tested slice
// beats a partial implementation of the full spec (cf. ADR-0014's same
// judgment call for the plugin protocol itself).
//
// It depends only on the SDK (sdk/go-plugin), never on any internal/
// package of the agent.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"sort"
	"strconv"
	"strings"

	patchcord "github.com/lucasglmt/patchcord/sdk/go-plugin"
)

type parseAction struct{}

func (parseAction) ID() string { return "json.parse@1" }

func (parseAction) Run(_ context.Context, input patchcord.ActionInput, _ *patchcord.ConnectorConfig) (patchcord.ActionOutput, error) {
	raw, ok := input["value"].(string)
	if !ok {
		return nil, fmt.Errorf("input %q must be a string", "value")
	}

	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}
	return patchcord.ActionOutput{"value": decoded}, nil
}

type stringifyAction struct{}

func (stringifyAction) ID() string { return "json.stringify@1" }

func (stringifyAction) Run(_ context.Context, input patchcord.ActionInput, _ *patchcord.ConnectorConfig) (patchcord.ActionOutput, error) {
	value, present := input["value"]
	if !present {
		return nil, fmt.Errorf("input %q is required", "value")
	}
	pretty, _ := input["pretty"].(bool)

	var (
		encoded []byte
		err     error
	)
	if pretty {
		encoded, err = json.MarshalIndent(value, "", "  ")
	} else {
		encoded, err = json.Marshal(value)
	}
	if err != nil {
		return nil, fmt.Errorf("stringify json: %w", err)
	}
	return patchcord.ActionOutput{"value": string(encoded)}, nil
}

type mergeAction struct{}

func (mergeAction) ID() string { return "json.merge@1" }

func (mergeAction) Run(_ context.Context, input patchcord.ActionInput, _ *patchcord.ConnectorConfig) (patchcord.ActionOutput, error) {
	base, ok := input["base"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("input %q must be a JSON object", "base")
	}
	patch, ok := input["patch"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("input %q must be a JSON object", "patch")
	}

	merged := make(map[string]any, len(base)+len(patch))
	maps.Copy(merged, base)
	maps.Copy(merged, patch)
	return patchcord.ActionOutput{"value": merged}, nil
}

type jsonpathSegmentKind int

const (
	segmentKey jsonpathSegmentKind = iota
	segmentIndex
	segmentWildcard
)

type jsonpathSegment struct {
	kind  jsonpathSegmentKind
	key   string
	index int
}

// parseJSONPath parses the subset of JSONPath documented in this file's
// package comment into a sequence of segments to apply in order.
func parseJSONPath(path string) ([]jsonpathSegment, error) {
	if !strings.HasPrefix(path, "$") {
		return nil, fmt.Errorf("jsonpath %q must start with %q", path, "$")
	}
	rest := path[1:]

	var segments []jsonpathSegment
	for len(rest) > 0 {
		switch rest[0] {
		case '.':
			rest = rest[1:]
			end := 0
			for end < len(rest) && isIdentByte(rest[end]) {
				end++
			}
			if end == 0 {
				return nil, fmt.Errorf("jsonpath %q: expected an identifier after %q", path, ".")
			}
			segments = append(segments, jsonpathSegment{kind: segmentKey, key: rest[:end]})
			rest = rest[end:]

		case '[':
			closeIdx := strings.IndexByte(rest, ']')
			if closeIdx < 0 {
				return nil, fmt.Errorf("jsonpath %q: unterminated %q", path, "[")
			}
			inner := rest[1:closeIdx]
			rest = rest[closeIdx+1:]

			switch {
			case inner == "*":
				segments = append(segments, jsonpathSegment{kind: segmentWildcard})
			case len(inner) >= 2 && isQuoted(inner, '\'') || len(inner) >= 2 && isQuoted(inner, '"'):
				segments = append(segments, jsonpathSegment{kind: segmentKey, key: inner[1 : len(inner)-1]})
			default:
				idx, err := strconv.Atoi(inner)
				if err != nil {
					return nil, fmt.Errorf("jsonpath %q: invalid bracket segment %q", path, inner)
				}
				segments = append(segments, jsonpathSegment{kind: segmentIndex, index: idx})
			}

		default:
			return nil, fmt.Errorf("jsonpath %q: unexpected character %q", path, string(rest[0]))
		}
	}
	return segments, nil
}

func isIdentByte(b byte) bool {
	return b == '_' ||
		(b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9')
}

func isQuoted(s string, quote byte) bool {
	return s[0] == quote && s[len(s)-1] == quote
}

// evaluateJSONPath applies segments to root, threading the current set of
// matches through each segment in turn. A segment that doesn't apply to a
// given value (e.g. an index on a value that isn't a list) simply drops
// that value rather than erroring — an empty result is a legitimate
// "not found", not a failure.
func evaluateJSONPath(segments []jsonpathSegment, root any) []any {
	current := []any{root}
	for _, seg := range segments {
		var next []any
		for _, value := range current {
			switch seg.kind {
			case segmentKey:
				if m, ok := value.(map[string]any); ok {
					if v, ok := m[seg.key]; ok {
						next = append(next, v)
					}
				}

			case segmentIndex:
				if arr, ok := value.([]any); ok {
					idx := seg.index
					if idx < 0 {
						idx += len(arr)
					}
					if idx >= 0 && idx < len(arr) {
						next = append(next, arr[idx])
					}
				}

			case segmentWildcard:
				switch v := value.(type) {
				case []any:
					next = append(next, v...)
				case map[string]any:
					keys := make([]string, 0, len(v))
					for k := range v {
						keys = append(keys, k)
					}
					sort.Strings(keys)
					for _, k := range keys {
						next = append(next, v[k])
					}
				}
			}
		}
		current = next
	}
	return current
}

type jsonpathExtractAction struct{}

func (jsonpathExtractAction) ID() string { return "json.jsonpath@1" }

func (jsonpathExtractAction) Run(_ context.Context, input patchcord.ActionInput, _ *patchcord.ConnectorConfig) (patchcord.ActionOutput, error) {
	path, ok := input["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("input %q must be a non-empty string", "path")
	}
	value, present := input["value"]
	if !present {
		return nil, fmt.Errorf("input %q is required", "value")
	}

	segments, err := parseJSONPath(path)
	if err != nil {
		return nil, err
	}

	matches := evaluateJSONPath(segments, value)
	output := patchcord.ActionOutput{
		"found":  len(matches) > 0,
		"values": matches,
	}
	if len(matches) > 0 {
		output["value"] = matches[0]
	} else {
		output["value"] = nil
	}
	return output, nil
}

func main() {
	plugin := patchcord.Plugin{
		Manifest: patchcord.Manifest{
			ID:      "io.patchcord.example-json",
			Version: "1.0.0",
		},
		Actions: []patchcord.Action{
			parseAction{},
			stringifyAction{},
			jsonpathExtractAction{},
			mergeAction{},
		},
	}

	if err := patchcord.Serve(plugin); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
