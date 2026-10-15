package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/goccy/go-yaml"
)

type ConfigServer struct {
	Port int64 `yaml:"port"`
}

type ConfigCache struct {
	TTL                  string      `yaml:"ttl"`
	StaleWhileRevalidate string      `yaml:"stale_while_revalidate"`
	StaleIfError         string      `yaml:"stale_if_error"`
	MaxBytes             ConfigBytes `yaml:"max_bytes"`
	MaxBodyBytes         ConfigBytes `yaml:"max_body_bytes"`
}

type ConfigSource struct {
	compiledPath       *CompiledPath
	mimeExtensions     map[string]string
	cache              *Cache
	indexParentSlashes int

	Host           string              `yaml:"host"`
	Path           string              `yaml:"path"`
	Target         string              `yaml:"target"`
	Index          string              `yaml:"index,omitempty"`
	Mime           string              `yaml:"mime"`
	MimeExtensions map[string]string   `yaml:"mime_extensions,omitempty"`
	Allowed        map[string][]string `yaml:"allowed"`
	Cache          ConfigCache         `yaml:"cache"`
}

type Config struct {
	sourceMap map[string]*ConfigSource

	Server  ConfigServer   `yaml:"server"`
	Sources []ConfigSource `yaml:"sources"`
}

func DefaultConfig() *Config {
	return &Config{
		Server: ConfigServer{
			Port: 2112,
		},
		Sources: []ConfigSource{
			{
				Host:   "src.example.com",
				Path:   "/<repo>/<path...>",
				Target: "https://raw.githubusercontent.com/github/<repo>/refs/heads/main/<path>",
				Mime:   "text/plain",
				Allowed: map[string][]string{
					"path": {
						"*.md",
						"readme.*",
						"install.sh",
					},
				},
				Cache: ConfigCache{
					TTL:                  "5m",
					StaleWhileRevalidate: "1m",
					StaleIfError:         "24h",
					MaxBytes:             64 << 20,
					MaxBodyBytes:         8 << 20,
				},
			},
		},
	}
}

func LoadConfig() (*Config, error) {
	cfg := DefaultConfig()

	file, err := os.OpenFile("config.yml", os.O_RDONLY, 0)
	if os.IsNotExist(err) {
		err = cfg.Store()
		if err != nil {
			return nil, err
		}
	} else {
		if err != nil {
			return nil, err
		}

		defer file.Close()

		err = yaml.NewDecoder(file).Decode(cfg)
		if err != nil {
			return nil, err
		}
	}

	err = cfg.Init()
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Addr() string {
	return fmt.Sprintf(":%d", c.Server.Port)
}

func (c *Config) Init() error {
	// check if port is valid
	if c.Server.Port <= 0 || c.Server.Port >= 65535 {
		return fmt.Errorf("invalid port %d", c.Server.Port)
	}

	// build source map
	c.sourceMap = make(map[string]*ConfigSource, len(c.Sources))

	for idx := range c.Sources {
		source := &c.Sources[idx]

		err := source.initRouting()
		if err != nil {
			c.Close()

			return fmt.Errorf("source %q: %w", source.Host, err)
		}

		cache, err := NewCache(source.Cache, nil)
		if err != nil {
			c.Close()

			return fmt.Errorf("source %q: %w", source.Host, err)
		}

		source.cache = cache

		c.sourceMap[source.Host] = source
	}

	return nil
}

func (c *Config) Close() {
	for idx := range c.Sources {
		source := &c.Sources[idx]

		if source.cache != nil {
			source.cache.Close()
		}
	}
}

func (c *Config) Store() error {
	var (
		buffer   bytes.Buffer
		comments = yaml.CommentMap{
			"$.sources": {yaml.HeadComment("")},

			"$.server.port": {yaml.HeadComment(" port to serve rawr on (required; default 2112)")},
		}
	)

	err := yaml.NewEncoder(&buffer, yaml.WithComment(comments)).Encode(c)
	if err != nil {
		return err
	}

	body := bytes.ReplaceAll(buffer.Bytes(), []byte("#\n"), []byte("\n"))

	return os.WriteFile("config.yml", body, 0644)
}

func (c *Config) GetSourceFromHost(host string) *ConfigSource {
	source, ok := c.sourceMap[host]
	if !ok {
		return nil
	}

	return source
}
