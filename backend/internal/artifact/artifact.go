// Package artifact 管理产物空间:在存储驱动(storage.Store)之上实现
// 「每 m 天的产物保存到一个子目录、最多保留 n 个子目录」的分桶与保留策略。
//
// 目录结构:<产物路径 path>/<分桶 bucket>/<产物文件>,其中 bucket 为
// 周期起始日期(UTC,如 2026-01-05)。写入新分桶时自动清理超出数量的最旧分桶。
package artifact

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/tomasgdl/infosentinel/backend/internal/repository"
	"github.com/tomasgdl/infosentinel/backend/pkg/storage"
)

// MaxDirsDefault 是 max_dirs 未配置时的默认保留分桶数。
const MaxDirsDefault = 30

// bucketRe 是合法分桶目录名(周期起始日期)。
var bucketRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// Manager 是产物空间管理器。
type Manager struct {
	store      storage.Store
	path       string // 产物路径(相对存储根目录)
	periodDays int    // 每 m 天一个分桶
	maxDirs    int    // 最多保留的分桶数
	lastBucket string // 上次写入的分桶;变化时触发清理
}

// Options 是创建 Manager 的参数。
type Options struct {
	Store      storage.Store
	Path       string // 产物路径(相对存储根目录;空为根目录)
	PeriodDays int    // 每 m 天一个分桶;<1 按 1 处理
	MaxDirs    int    // 最多保留分桶数;<1 按 MaxDirsDefault 处理
}

// New 创建产物空间管理器。
func New(opts Options) (*Manager, error) {
	if opts.Store == nil {
		return nil, fmt.Errorf("产物存储驱动为空")
	}
	clean, err := storage.CleanRel(opts.Path)
	if err != nil {
		return nil, fmt.Errorf("产物路径不合法: %w", err)
	}
	period := opts.PeriodDays
	if period < 1 {
		period = 1
	}
	maxDirs := opts.MaxDirs
	if maxDirs < 1 {
		maxDirs = MaxDirsDefault
	}
	return &Manager{store: opts.Store, path: clean, periodDays: period, maxDirs: maxDirs}, nil
}

// File 是一个产物文件(含所在分桶)。
type File struct {
	Bucket  string    `json:"bucket"`   // 分桶目录名(周期起始日期)
	Name    string    `json:"name"`     // 文件名
	Size    int64     `json:"size"`     // 字节数
	ModTime time.Time `json:"mod_time"` // 最后修改时间
}

// BucketOf 计算时间 t 所在的分桶名:每 periodDays 天一个,以周期起始日期命名(UTC)。
func BucketOf(t time.Time, periodDays int) string {
	if periodDays < 1 {
		periodDays = 1
	}
	days := int(math.Floor(float64(t.Unix()) / 86400))
	start := days - days%periodDays
	return time.Unix(int64(start)*86400, 0).UTC().Format("2006-01-02")
}

// Build 按单个产物空间配置构建管理器:映射 StorageSpace → 存储驱动参数。
// localBaseDir 为本地磁盘根目录(来自引导配置 storage.base_dir);
// password 为已解析的 SFTP/FTP 登录密码(本地驱动可空)。
func Build(cfg *repository.StorageSpace, localBaseDir, password string) (*Manager, error) {
	scfg := storage.Config{Type: cfg.Type, Password: password}
	switch cfg.Type {
	case repository.StorageTypeLocal:
		scfg.Root = localBaseDir
	case repository.StorageTypeSFTP:
		scfg.Root = cfg.SFTP.BaseDir
		scfg.Host = cfg.SFTP.Host
		scfg.Port = cfg.SFTP.Port
		scfg.Username = cfg.SFTP.Username
		scfg.KeyPath = cfg.SFTP.KeyPath
		scfg.HostKey = cfg.SFTP.HostKey
	case repository.StorageTypeFTP:
		scfg.Root = cfg.FTP.BaseDir
		scfg.Host = cfg.FTP.Host
		scfg.Port = cfg.FTP.Port
		scfg.Username = cfg.FTP.Username
		scfg.TLS = cfg.FTP.TLS
	}
	store, err := storage.New(scfg)
	if err != nil {
		return nil, err
	}
	return New(Options{
		Store:      store,
		Path:       cfg.Path,
		PeriodDays: cfg.PeriodDays,
		MaxDirs:    cfg.MaxDirs,
	})
}

// Save 写入一个产物文件,返回所在分桶名(周期起始日期)。
// 首次写入某个新分桶时按保留策略清理最旧的分桶。
func (m *Manager) Save(ctx context.Context, name string, data []byte) (string, error) {
	clean, err := storage.CleanRel(name)
	if err != nil {
		return "", fmt.Errorf("产物名不合法: %w", err)
	}
	if clean == "" || strings.Contains(clean, "/") {
		return "", fmt.Errorf("产物名必须是单级文件名: %q", name)
	}
	bucket := BucketOf(time.Now(), m.periodDays)
	if err := m.store.Save(ctx, path.Join(m.path, bucket, clean), data); err != nil {
		return "", err
	}
	if bucket != m.lastBucket {
		m.lastBucket = bucket
		if err := m.prune(ctx, bucket); err != nil {
			return bucket, fmt.Errorf("产物已保存,但清理旧分桶失败: %w", err)
		}
	}
	return bucket, nil
}

// List 列出产物空间全部文件(跨分桶,最新修改在前)。
func (m *Manager) List(ctx context.Context) ([]File, error) {
	dirs, err := m.store.List(ctx, m.path)
	if err != nil {
		// 路径尚不存在(还没写过任何产物)视为空
		if isNotFound(err) {
			return []File{}, nil
		}
		return nil, err
	}
	buckets := make([]string, 0, len(dirs))
	for _, d := range dirs {
		if d.Dir && bucketRe.MatchString(d.Name) {
			buckets = append(buckets, d.Name)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(buckets)))

	out := make([]File, 0, len(buckets)*4)
	for _, bucket := range buckets {
		ents, err := m.store.List(ctx, path.Join(m.path, bucket))
		if err != nil {
			if isNotFound(err) {
				continue
			}
			return nil, fmt.Errorf("列出分桶 %s: %w", bucket, err)
		}
		for _, e := range ents {
			if e.Dir {
				continue
			}
			out = append(out, File{Bucket: bucket, Name: e.Name, Size: e.Size, ModTime: e.ModTime})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	return out, nil
}

// Read 读取一个产物文件。
func (m *Manager) Read(ctx context.Context, bucket, name string) ([]byte, error) {
	if !bucketRe.MatchString(bucket) {
		return nil, fmt.Errorf("非法分桶名 %q", bucket)
	}
	clean, err := storage.CleanRel(name)
	if err != nil || clean == "" || strings.Contains(clean, "/") {
		return nil, fmt.Errorf("非法产物名 %q", name)
	}
	return m.store.Read(ctx, path.Join(m.path, bucket, clean))
}

// prune 按保留策略删除超出 maxDirs 的最旧分桶(keep 为刚写入的当前分桶)。
func (m *Manager) prune(ctx context.Context, keep string) error {
	dirs, err := m.store.List(ctx, m.path)
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return err
	}
	buckets := make([]string, 0, len(dirs))
	for _, d := range dirs {
		if d.Dir && bucketRe.MatchString(d.Name) {
			buckets = append(buckets, d.Name)
		}
	}
	if len(buckets) <= m.maxDirs {
		return nil
	}
	// 分桶名即周期起始日期,字典序即时间序;删除最旧的
	sort.Sort(sort.Reverse(sort.StringSlice(buckets)))
	for _, bucket := range buckets[m.maxDirs:] {
		if bucket == keep {
			continue
		}
		if err := m.store.DeleteDir(ctx, path.Join(m.path, bucket)); err != nil && !isNotFound(err) {
			return fmt.Errorf("删除分桶 %s: %w", bucket, err)
		}
	}
	return nil
}

// isNotFound 判断错误是否为路径不存在。
func isNotFound(err error) bool {
	return errors.Is(err, storage.ErrNotFound)
}

// ---- 多产物空间 ----

// SpaceFile 是一个产物文件(附带所属空间名)。
type SpaceFile struct {
	File
	Space string `json:"space"` // 所属空间名
}

// Spaces 是产物空间集合:容纳全局配置的多个空间(按配置顺序),
// 订阅按名引用其中之一;支持跨空间列举与按空间读取。
type Spaces struct {
	names    []string
	managers map[string]*Manager
	types    map[string]string // 空间名 → 驱动类型(供 DSN 前缀)
}

// NewSpaces 创建空集合。
func NewSpaces() *Spaces {
	return &Spaces{managers: map[string]*Manager{}, types: map[string]string{}}
}

// Add 登记一个空间;同名后到者忽略(配置校验已保证唯一)。
func (s *Spaces) Add(space *repository.StorageSpace, m *Manager) {
	if _, ok := s.managers[space.Name]; ok {
		return
	}
	s.names = append(s.names, space.Name)
	s.managers[space.Name] = m
	s.types[space.Name] = space.Type
}

// Len 返回空间数。
func (s *Spaces) Len() int { return len(s.names) }

// Names 按配置顺序返回全部空间名。
func (s *Spaces) Names() []string {
	out := make([]string, len(s.names))
	copy(out, s.names)
	return out
}

// Manager 返回指定空间的管理器;不存在时 ok 为 false。
func (s *Spaces) Manager(name string) (*Manager, bool) {
	m, ok := s.managers[name]
	return m, ok
}

// Type 返回指定空间的驱动类型;不存在返回空串。
func (s *Spaces) Type(name string) string { return s.types[name] }

// List 列出全部空间的文件(空间按配置顺序,空间内最新在前;跨空间按修改时间倒序)。
func (s *Spaces) List(ctx context.Context) ([]SpaceFile, error) {
	out := make([]SpaceFile, 0, 16)
	for _, name := range s.names {
		files, err := s.managers[name].List(ctx)
		if err != nil {
			return nil, fmt.Errorf("列出产物空间 %s: %w", name, err)
		}
		for _, f := range files {
			out = append(out, SpaceFile{File: f, Space: name})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	return out, nil
}

// Read 读取指定空间的一个产物文件。
func (s *Spaces) Read(ctx context.Context, space, bucket, name string) ([]byte, error) {
	m, ok := s.managers[space]
	if !ok {
		return nil, fmt.Errorf("产物空间 %q 不存在: %w", space, storage.ErrNotFound)
	}
	return m.Read(ctx, bucket, name)
}

// ---- 产物 DSN ----
// 产物路径采用 DSN 风格引用:<类型>://<空间名>/<分桶>/<文件名>,
// 如 local://默认空间/2026-11-11/ai-agent-20261111-080000.html,
// 可唯一定位到某空间的某个文件(空间名解析回全局配置)。

// DSN 构造产物路径引用。
func DSN(spaceType, space, bucket, name string) string {
	return fmt.Sprintf("%s://%s/%s/%s", spaceType, space, bucket, name)
}

// ParseDSN 解析产物 DSN 为空间名、分桶与文件名;格式非法返回错误。
func ParseDSN(dsn string) (space, bucket, name string, err error) {
	scheme, rest, ok := strings.Cut(dsn, "://")
	if !ok || scheme == "" {
		return "", "", "", fmt.Errorf("产物路径 %q 不是 DSN 格式(类型://空间名/分桶/文件名)", dsn)
	}
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", fmt.Errorf("产物路径 %q 不是 DSN 格式(类型://空间名/分桶/文件名)", dsn)
	}
	return parts[0], parts[1], parts[2], nil
}
