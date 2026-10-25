// Package store 负责 ~/.veda 下的持久化状态：文件索引、工作区登记、会话、
// 设置、界面布局、文档锁定状态，以及旧数据迁移。
package store

// Store 聚合应用需要持久化的全部状态：最近打开的工作区、设置、
// 界面布局与文档锁定状态。
//
// 这些状态都写在 ~/.veda 下，并在文件重命名/移动时一起改写，
// 所以放在同一个包里，由 RewritePaths 统一加锁后处理。
type Store struct {
	Session  *SessionStore
	Settings *SettingsStore
	Layouts  *LayoutStore
	Locks    *LockStore
}

// New 创建一个指向默认数据目录的存储层。
func New() *Store {
	return &Store{
		Session:  &SessionStore{},
		Settings: &SettingsStore{},
		Layouts:  &LayoutStore{},
		Locks:    &LockStore{},
	}
}

// RewritePaths 在文件或目录移动后，改写 ~/.veda 里所有引用旧路径的 JSON 配置。
// 改写期间会锁住各个子存储，避免并发写入写出半份配置。
func (s *Store) RewritePaths(oldPath, newPath string, extraWorkspaces []string) error {
	if oldPath == newPath {
		return nil
	}
	s.Session.mu.Lock()
	defer s.Session.mu.Unlock()
	s.Settings.mu.Lock()
	defer s.Settings.mu.Unlock()
	s.Locks.mu.Lock()
	defer s.Locks.mu.Unlock()
	s.Layouts.mu.Lock()
	defer s.Layouts.mu.Unlock()

	return rewriteVedaConfigs(oldPath, newPath, extraWorkspaces)
}
