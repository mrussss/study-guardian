package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// config-helper performs narrowly-scoped YAML edits while retaining all
// unrelated configuration keys. It is used by the Windows PowerShell helpers
// so they do not need a YAML parser or fragile text replacement.
func main() {
	configPath := flag.String("config", "", "path to config.yaml")
	migrate := flag.Bool("migrate", false, "migrate legacy ai fields to schema v2")
	provider := flag.String("provider", "", "text provider")
	model := flag.String("model", "", "text model")
	baseURL := flag.String("base-url", "", "text provider base URL")
	apiKeyEnv := flag.String("api-key-env", "", "environment variable containing the API key")
	apiKeyFile := flag.String("api-key-file", "", "file containing the API key")
	visionProvider := flag.String("vision-provider", "", "vision provider")
	visionModel := flag.String("vision-model", "", "vision model")
	visionBaseURL := flag.String("vision-base-url", "", "vision provider base URL")
	visionEnabled := flag.Bool("vision-enabled", false, "enable vision endpoint")
	developerMode := flag.Bool("developer-mode", false, "enable developer-only providers")
	flag.Parse()
	if *configPath == "" {
		fatal("-config is required")
	}
	data, err := os.ReadFile(*configPath)
	if err != nil {
		fatal("read config: %v", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		fatal("parse config: %v", err)
	}
	root := doc.Content
	if len(root) == 0 || root[0].Kind != yaml.MappingNode {
		fatal("config root must be a YAML mapping")
	}
	ai := ensureMap(root[0], "ai")
	if *migrate {
		migrateLegacy(ai)
	}
	if *provider != "" {
		text := ensureMap(ai, "text")
		setScalar(text, "provider", *provider)
		if *model != "" {
			setScalar(text, "model", *model)
		}
		if *baseURL != "" {
			setScalar(text, "base_url", *baseURL)
		}
		if *apiKeyEnv != "" {
			setScalar(text, "api_key_env", *apiKeyEnv)
		}
		if *apiKeyFile != "" {
			setScalar(text, "api_key_file", *apiKeyFile)
		}
		setScalar(ai, "enabled", *provider != "none")
		setScalar(ai, "schema_version", 2)
	}
	if *visionProvider != "" {
		vision := ensureMap(ai, "vision")
		setScalar(vision, "provider", *visionProvider)
		if *visionModel != "" {
			setScalar(vision, "model", *visionModel)
		}
		if *visionBaseURL != "" {
			setScalar(vision, "base_url", *visionBaseURL)
		}
		setScalar(vision, "enabled", *visionEnabled)
	}
	if *developerMode {
		setScalar(ai, "developer_mode", true)
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		fatal("write config: %v", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(*configPath), ".studyguardian-config-*.yaml")
	if err != nil {
		fatal("create temporary config: %v", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		fatal("write temporary config: %v", err)
	}
	if err := tmp.Close(); err != nil {
		fatal("close temporary config: %v", err)
	}
	if err := os.Rename(tmpName, *configPath); err != nil {
		fatal("replace config: %v", err)
	}
}

func migrateLegacy(ai *yaml.Node) {
	if value(ai, "text") != nil || value(ai, "vision") != nil {
		return
	}
	text := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	copyScalar(text, "provider", value(ai, "provider"), "none")
	copyScalar(text, "model", value(ai, "model"), "")
	copyScalar(text, "base_url", value(ai, "endpoint"), "")
	setScalar(text, "timeout_seconds", 6)
	setScalar(text, "json_mode", "auto")
	setNode(ai, "text", text)
	vision := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	setScalar(vision, "enabled", false)
	setScalar(vision, "provider", "none")
	setScalar(vision, "timeout_seconds", 8)
	setScalar(vision, "json_mode", "auto")
	setNode(ai, "vision", vision)
	setScalar(ai, "schema_version", 2)
}

func value(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}
func ensureMap(m *yaml.Node, key string) *yaml.Node {
	if v := value(m, key); v != nil && v.Kind == yaml.MappingNode {
		return v
	}
	v := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	setNode(m, key, v)
	return v
}
func setNode(m *yaml.Node, key string, v *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = v
			return
		}
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, v)
}
func setScalar(m *yaml.Node, key string, v interface{}) {
	setNode(m, key, &yaml.Node{Kind: yaml.ScalarNode, Tag: scalarTag(v), Value: fmt.Sprint(v)})
}
func copyScalar(dst *yaml.Node, key string, src *yaml.Node, fallback string) {
	if src != nil && src.Kind == yaml.ScalarNode {
		setNode(dst, key, &yaml.Node{Kind: yaml.ScalarNode, Tag: src.Tag, Value: src.Value})
		return
	}
	setScalar(dst, key, fallback)
}
func scalarTag(v interface{}) string {
	switch v.(type) {
	case bool:
		return "!!bool"
	case int:
		return "!!int"
	default:
		return "!!str"
	}
}
func fatal(format string, args ...interface{}) {
	panic(strings.TrimSpace(fmt.Sprintf(format, args...)))
}
