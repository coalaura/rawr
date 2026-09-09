package main

import (
	"fmt"
	"strings"
)

type CompiledPath struct {
	parts     []PathPart
	variables int
}

type PathPart struct {
	prefix   string
	name     string
	allowed  []PathRule
	catchAll bool
}

type PathRule struct {
	value string
	kind  uint8
}

const (
	pathRuleExact uint8 = iota
	pathRulePrefix
	pathRuleSuffix
)

func CompilePath(pattern string, allowed map[string][]string) (*CompiledPath, error) {
	if len(pattern) == 0 || pattern[0] != '/' {
		return nil, fmt.Errorf("pattern must start with '/'")
	}

	pattern = trimTrailingSlashes(pattern)

	var parts []PathPart

	names := make(map[string]int)

	literalStart := 0
	segmentStart := 1

	for segment := range strings.SplitSeq(pattern[1:], "/") {
		segmentEnd := segmentStart + len(segment)

		if strings.ContainsAny(segment, "<>") {
			if len(segment) < 3 || segment[0] != '<' || segment[len(segment)-1] != '>' {
				return nil, fmt.Errorf("invalid variable segment %q", segment)
			}

			name, catchAll := strings.CutSuffix(segment[1:len(segment)-1], "...")
			if len(name) == 0 || !validPathChars(name) {
				return nil, fmt.Errorf("invalid variable name %q", name)
			}

			if catchAll && segmentEnd != len(pattern) {
				return nil, fmt.Errorf("catch-all variable %q must be at the end", name)
			}

			if _, ok := names[name]; ok {
				return nil, fmt.Errorf("duplicate variable %q", name)
			}

			names[name] = len(parts)

			parts = append(parts, PathPart{
				prefix:   pattern[literalStart:segmentStart],
				name:     name,
				catchAll: catchAll,
			})

			literalStart = segmentEnd
		} else if !validPathChars(segment) {
			return nil, fmt.Errorf("invalid literal segment %q", segment)
		}

		segmentStart = segmentEnd + 1
	}

	if literalStart < len(pattern) {
		parts = append(parts, PathPart{prefix: pattern[literalStart:]})
	}

	for name, patterns := range allowed {
		index, ok := names[name]
		if !ok {
			return nil, fmt.Errorf("unknown allowed variable %q", name)
		}

		rules, err := compilePathRules(name, patterns)
		if err != nil {
			return nil, err
		}

		parts[index].allowed = rules
	}

	return &CompiledPath{
		parts:     parts,
		variables: len(names),
	}, nil
}

func compilePathRules(name string, patterns []string) ([]PathRule, error) {
	rules := make([]PathRule, len(patterns))

	for i, pattern := range patterns {
		if pattern == "" {
			return nil, fmt.Errorf("invalid allowed rule %q for variable %q", pattern, name)
		}

		wildcard := strings.IndexByte(pattern, '*')

		switch {
		case wildcard == -1:
			rules[i] = PathRule{
				value: pattern,
				kind:  pathRuleExact,
			}
		case wildcard == len(pattern)-1:
			rules[i] = PathRule{
				value: pattern[:wildcard],
				kind:  pathRulePrefix,
			}
		case wildcard == 0 && strings.IndexByte(pattern[1:], '*') == -1:
			rules[i] = PathRule{
				value: pattern[1:],
				kind:  pathRuleSuffix,
			}
		default:
			return nil, fmt.Errorf("invalid allowed rule %q for variable %q", pattern, name)
		}
	}

	return rules, nil
}

func (p *CompiledPath) Match(path string) (map[string]string, bool) {
	return p.MatchInto(path, nil)
}

func (p *CompiledPath) MatchInto(path string, dst map[string]string) (map[string]string, bool) {
	if len(path) == 0 || path[0] != '/' {
		return dst, false
	}

	path = trimTrailingSlashes(path)

	if !p.matches(path) {
		return dst, false
	}

	if dst == nil && p.variables != 0 {
		dst = make(map[string]string, p.variables)
	}

	clear(dst)

	for i := range p.parts {
		part := &p.parts[i]

		if part.name == "" {
			break
		}

		path = path[len(part.prefix):]

		if i == len(p.parts)-1 {
			dst[part.name] = path

			break
		}

		end := strings.IndexByte(path, '/')

		dst[part.name] = path[:end]
		path = path[end:]
	}

	return dst, true
}

func (p *CompiledPath) matches(path string) bool {
	for i := range p.parts {
		part := &p.parts[i]

		if !strings.HasPrefix(path, part.prefix) {
			return false
		}

		path = path[len(part.prefix):]

		if part.name == "" {
			return path == ""
		}

		if i == len(p.parts)-1 {
			if path == "" || !part.catchAll && strings.IndexByte(path, '/') != -1 {
				return false
			}

			return validPathChars(path) && part.allows(path)
		}

		end := strings.IndexByte(path, '/')
		if end <= 0 || !validPathChars(path[:end]) || !part.allows(path[:end]) {
			return false
		}

		path = path[end:]
	}

	return false
}

func (p *PathPart) allows(value string) bool {
	if p.allowed == nil {
		return true
	}

	for _, rule := range p.allowed {
		switch rule.kind {
		case pathRuleExact:
			if value == rule.value {
				return true
			}
		case pathRulePrefix:
			if strings.HasPrefix(value, rule.value) {
				return true
			}
		case pathRuleSuffix:
			if strings.HasSuffix(value, rule.value) {
				return true
			}
		}
	}

	return false
}

func trimTrailingSlashes(path string) string {
	for len(path) > 1 && path[len(path)-1] == '/' {
		path = path[:len(path)-1]
	}

	return path
}

func validPathChars(path string) bool {
	for i := 0; i < len(path); i++ {
		char := path[i]

		switch char {
		case '/', '-', '.', '_', '~', '!', '$', '&', '\'', '(', ')', '*', '+', ',', ';', '=', ':', '@':
		case '%':
			if i+2 >= len(path) || !isHex(path[i+1]) || !isHex(path[i+2]) {
				return false
			}

			i += 2
		default:
			if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') {
				return false
			}
		}
	}

	return true
}

func isHex(char byte) bool {
	return '0' <= char && char <= '9' || 'a' <= char && char <= 'f' || 'A' <= char && char <= 'F'
}
