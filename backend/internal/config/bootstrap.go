// Bootstrap 是 config.yaml 的全部内容:只保留部署相关的必要引导项。
//
// 业务配置(主题/目标/任务/数据源/模型/发送器)全部存于 SQLite,
// 通过 Web 管理界面维护 —— 配置文件与数据库之间不再有配置冗余。
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Bootstrap 是启动引导配置。
type Bootstrap struct {
	// Database 是 SQLite 数据库位置。
	Database BootstrapDatabase `yaml:"database"`
	// Log 是日志输出配置。
	Log BootstrapLog `yaml:"log"`
	// Storage 是产物空间引导项(本地磁盘根目录;SFTP/FTP 的基础目录存于数据库全局设置)。
	Storage BootstrapStorage `yaml:"storage"`
	// StateDir 是运行时状态目录(采集去重记录,每个任务一个 <task>.json)。
	StateDir string `yaml:"state_dir"`
}

// BootstrapDatabase 是数据库引导项。
type BootstrapDatabase struct {
	// Path 是 SQLite 数据库文件路径。
	Path string `yaml:"path"`
}

// BootstrapLog 是日志引导项。
type BootstrapLog struct {
	// Path 是日志文件路径(JSON 行,同时输出到 stdout)。
	Path string `yaml:"path"`
	// Level 是日志级别:DEBUG / INFO / WARN / ERROR。
	Level string `yaml:"level"`
}

// BootstrapStorage 是产物空间引导项。
type BootstrapStorage struct {
	// BaseDir 是本地磁盘产物根目录(推荐绝对路径)。
	// 页面配置的产物路径(storage.path)相对本目录。
	BaseDir string `yaml:"base_dir"`
}

// 引导项默认值(相对于工作目录;运行时产物统一收在 .tests/ 下,
// 部署环境在 config.yaml 覆盖为实际路径)。
const (
	DefaultDBPath         = ".tests/data/infosentinel.db"
	DefaultLogPath        = ".tests/logs/infosentinel.log"
	DefaultLogLevel       = "INFO"
	DefaultStorageBaseDir = ".tests/artifacts"
	DefaultStateDir       = ".tests/data/state"
)

// LoadBootstrap 读取并校验引导配置。
func LoadBootstrap(path string) (*Bootstrap, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取引导配置 %s: %w", path, err)
	}
	return ParseBootstrap(raw)
}

// ParseBootstrap 从 YAML 文本解析引导配置,填充默认值。
// 兼容旧版配置文件:business 段会被忽略,不会报错。
func ParseBootstrap(data []byte) (*Bootstrap, error) {
	b := &Bootstrap{}
	if err := yaml.Unmarshal(data, b); err != nil {
		return nil, fmt.Errorf("解析引导配置 YAML: %w", err)
	}
	b.applyDefaults()
	return b, nil
}

func (b *Bootstrap) applyDefaults() {
	if b.Database.Path == "" {
		b.Database.Path = DefaultDBPath
	}
	if b.Log.Path == "" {
		b.Log.Path = DefaultLogPath
	}
	if b.Log.Level == "" {
		b.Log.Level = DefaultLogLevel
	}
	if b.Storage.BaseDir == "" {
		b.Storage.BaseDir = DefaultStorageBaseDir
	}
	if b.StateDir == "" {
		b.StateDir = DefaultStateDir
	}
}
