package main

import (
	"testing"

	"github.com/goccy/go-yaml"
)

type configBytesTest struct {
	name    string
	value   string
	want    ConfigBytes
	invalid bool
}

func TestConfigBytes(t *testing.T) {
	t.Parallel()

	tests := []configBytesTest{
		{"bytes", "1024", 1024, false},
		{"quoted bytes", `"1024"`, 1024, false},
		{"kilobytes", "1K", 1 << 10, false},
		{"megabytes", "12M", 12 << 20, false},
		{"gigabytes", `"1G"`, 1 << 30, false},
		{"terabytes", "1T", 1 << 40, false},
		{"lowercase", "12m", 12 << 20, false},
		{"surrounding spaces", `" 12M "`, 12 << 20, false},
		{"zero", "0", 0, false},
		{"maximum", "9223372036854775807", 1<<63 - 1, false},
		{"empty", `""`, 0, true},
		{"negative bytes", "-1", 0, true},
		{"negative units", "-1G", 0, true},
		{"fractional bytes", "1.5", 0, true},
		{"fractional units", "1.5G", 0, true},
		{"missing count", "G", 0, true},
		{"invalid suffix", "12Q", 0, true},
		{"overflow", "9223372036854775808", 0, true},
		{"scaled overflow", "8388608T", 0, true},
		{"boolean", "true", 0, true},
		{"sequence", "[1, 2]", 0, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var cfg ConfigCache

			data := "max_bytes: " + test.value + "\nmax_body_bytes: " + test.value + "\n"

			err := yaml.Unmarshal([]byte(data), &cfg)

			if test.invalid {
				if err == nil {
					t.Fatal("expected configuration error")
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			if cfg.MaxBytes != test.want {
				t.Errorf("max_bytes = %d, want %d", cfg.MaxBytes, test.want)
			}

			if cfg.MaxBodyBytes != test.want {
				t.Errorf("max_body_bytes = %d, want %d", cfg.MaxBodyBytes, test.want)
			}
		})
	}
}
