// Package storage 提供产物存储的抽象访问接口:本地磁盘、SFTP、FTP 只是不同驱动。
// 业务层(artifact.Manager 等)只依赖 Store 接口,不感知具体协议。
//
// 路径约定:Root 为存储根目录(本地=磁盘绝对路径;SFTP/FTP=远端基础目录,
// 相对路径时基于登录目录);其余入参一律为根目录下的相对路径,使用 / 分隔,
// 由 CleanRel 清洗(拒绝 .. 与绝对路径),驱动内部负责拼接与平台差异。
package storage

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"
)

// 存储驱动类型。
const (
	TypeLocal = "local"
	TypeSFTP  = "sftp"
	TypeFTP   = "ftp"
)

// ErrNotFound 表示目标路径不存在。
var ErrNotFound = errors.New("storage: not found")

// Entry 是目录列表中的一项。
type Entry struct {
	Name    string    // 条目名(不含路径)
	Size    int64     // 字节数(目录为 0)
	Dir     bool      // 是否为目录
	ModTime time.Time // 最后修改时间
}

// Store 是产物存储的抽象访问接口。实现需保证:
//   - Save 自动创建缺失的父目录;
//   - 不存在的路径读取/列出返回包装了 ErrNotFound 的错误;
//   - 所有入参为 Root 下的相对路径(/ 分隔)。
type Store interface {
	// Save 写入文件(创建缺失父目录)。
	Save(ctx context.Context, p string, data []byte) error
	// Read 读取文件。
	Read(ctx context.Context, p string) ([]byte, error)
	// List 列出目录下的一级条目(不递归)。
	List(ctx context.Context, dir string) ([]Entry, error)
	// Delete 删除文件。
	Delete(ctx context.Context, p string) error
	// DeleteDir 递归删除目录(含全部内容)。
	DeleteDir(ctx context.Context, dir string) error
}

// Config 是创建存储驱动的参数(与具体配置层解耦)。
type Config struct {
	// Type 是驱动类型:local / sftp / ftp。
	Type string
	// Root 是存储根目录(见包注释的路径约定)。
	Root string
	// Host 是远端主机(SFTP/FTP)。
	Host string
	// Port 是远端端口;0 时取协议默认值(SFTP 22 / FTP 21)。
	Port int
	// Username 是登录用户名(可选,空按匿名处理 FTP)。
	Username string
	// Password 是登录密码(SFTP 可为空改用私钥)。
	Password string
	// KeyPath 是 SFTP 私钥文件路径(可选)。
	KeyPath string
	// HostKey 是 SFTP 期望的远端公钥(authorized_keys 单行格式,可选);
	// 为空时不校验主机指纹(InsecureIgnoreHostKey)。
	HostKey string
	// TLS 是 FTP 是否使用 FTPS(显式 TLS)。
	TLS bool
	// TimeoutSec 是单次连接/操作超时;0 取默认值。
	TimeoutSec int
}

// New 按配置创建存储驱动。
func New(cfg Config) (Store, error) {
	switch cfg.Type {
	case TypeLocal:
		return newLocal(cfg.Root)
	case TypeSFTP:
		return newSFTP(cfg)
	case TypeFTP:
		return newFTP(cfg)
	default:
		return nil, fmt.Errorf("不支持的存储类型: %q(仅支持 local/sftp/ftp)", cfg.Type)
	}
}

// CleanRel 清洗相对路径:拒绝 ..、绝对路径与空字节,返回 / 分隔的干净路径;
// 空串表示根目录本身。
func CleanRel(p string) (string, error) {
	if p == "" {
		return "", nil
	}
	if strings.ContainsRune(p, 0) {
		return "", fmt.Errorf("路径包含空字节: %q", p)
	}
	p = strings.ReplaceAll(p, "\\", "/")
	var out []string
	for _, part := range strings.Split(p, "/") {
		switch part {
		case "", ".":
			continue
		case "..":
			return "", fmt.Errorf("路径不允许包含 ..: %q", p)
		default:
			out = append(out, part)
		}
	}
	return strings.Join(out, "/"), nil
}

// timeoutOf 返回有效超时(秒数 <= 0 时取默认 15s)。
func timeoutOf(sec int) time.Duration {
	if sec <= 0 {
		return 15 * time.Second
	}
	return time.Duration(sec) * time.Second
}

// joinRoot 拼接根目录与已清洗的相对路径。
// 根目录保留其绝对性(远端绝对路径很常见),仅做规范化;空根目录表示登录目录。
func joinRoot(root, rel string) string {
	root = path.Clean(strings.ReplaceAll(root, "\\", "/"))
	if root == "." {
		root = ""
	}
	switch {
	case rel == "":
		return root
	case root == "":
		return rel
	default:
		return root + "/" + rel
	}
}
