package server

import "gopkg.in/yaml.v3"

// yamlMarshal / yamlUnmarshal 是 YAML 编解码的内部薄封装。
func yamlMarshal(v any) ([]byte, error)   { return yaml.Marshal(v) }
func yamlUnmarshal(b []byte, v any) error { return yaml.Unmarshal(b, v) }
