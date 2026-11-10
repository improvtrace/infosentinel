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

// Build 按全局配置构建产物空间管理器:映射 StorageConfig → 存储驱动参数。
// localBaseDir 为本地磁盘根目录(来自引导配置 storage.base_dir);
// password 为已解析的 SFTP/FTP 登录密码(本地驱动可空)。
func Build(cfg *repository.StorageConfig, localBaseDir, password string) (*Manager, error) {
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

// Save 写入一个产物文件,返回相对路径(<bucket>/<name> 或 <path>/<bucket>/<name>)。
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
	rel := path.Join(m.path, bucket, clean)
	if err := m.store.Save(ctx, rel, data); err != nil {
		return "", err
	}
	if bucket != m.lastBucket {
		m.lastBucket = bucket
		if err := m.prune(ctx, bucket); err != nil {
			return rel, fmt.Errorf("产物已保存,但清理旧分桶失败: %w", err)
		}
	}
	return rel, nil
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
