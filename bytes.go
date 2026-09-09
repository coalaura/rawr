package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
)

type ConfigBytes int64

func (c *ConfigBytes) UnmarshalYAML(data []byte) error {
	var value any

	err := yaml.Unmarshal(data, &value)
	if err != nil {
		return err
	}

	var text string

	switch value := value.(type) {
	case string:
		text = value
	case int, int64, uint, uint64:
		text = fmt.Sprint(value)
	default:
		return fmt.Errorf("invalid byte size %v", value)
	}

	size, err := parseConfigBytes(text)
	if err != nil {
		return err
	}

	*c = size

	return nil
}

func parseConfigBytes(value string) (ConfigBytes, error) {
	text := strings.TrimSpace(value)

	var shift uint

	if len(text) != 0 {
		switch text[len(text)-1] {
		case 'k', 'K':
			shift = 10
		case 'm', 'M':
			shift = 20
		case 'g', 'G':
			shift = 30
		case 't', 'T':
			shift = 40
		}

		if shift != 0 {
			text = text[:len(text)-1]
		}
	}

	count, err := strconv.ParseUint(text, 10, 63)
	if err != nil {
		return 0, fmt.Errorf("invalid byte size %q: %w", value, err)
	}

	if count > (1<<63-1)>>shift {
		return 0, fmt.Errorf("byte size %q exceeds int64", value)
	}

	return ConfigBytes(count << shift), nil
}
