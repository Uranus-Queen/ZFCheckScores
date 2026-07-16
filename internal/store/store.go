package store

import (
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
)

const (
	DataDir     = "data"
	InfoFile    = "info.txt"
	GradeFile   = "grade.txt"
	OldGradeFile = "old_grade.txt"
)

// Store manages file-based state: first-run detection, grade snapshotting,
// and MD5 comparison between current and previous runs.
type Store struct {
	dir string
}

// New creates a Store with the given data directory (relative or absolute).
func New(dir string) *Store {
	return &Store{dir: dir}
}

// EnsureDir creates the data directory if it doesn't exist.
func (s *Store) EnsureDir() error {
	return os.MkdirAll(s.dir, 0755)
}

func (s *Store) path(name string) string {
	return filepath.Join(s.dir, name)
}

// IsFirstRun returns true if info.txt doesn't exist or its content
// differs from the encrypted info passed in.
func (s *Store) IsFirstRun(encryptedInfo string) bool {
	data, err := os.ReadFile(s.path(InfoFile))
	if err != nil {
		return true
	}
	return string(data) != encryptedInfo
}

// SaveInfo writes the encrypted info to info.txt.
func (s *Store) SaveInfo(encryptedInfo string) error {
	return os.WriteFile(s.path(InfoFile), []byte(encryptedInfo), 0644)
}

// SnapshotGrade copies grade.txt content to old_grade.txt.
// Creates an empty grade.txt if it doesn't exist yet.
func (s *Store) SnapshotGrade() error {
	gp := s.path(GradeFile)
	if _, err := os.Stat(gp); os.IsNotExist(err) {
		if err := os.WriteFile(gp, []byte{}, 0644); err != nil {
			return err
		}
	}
	data, err := os.ReadFile(gp)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path(OldGradeFile), data, 0644)
}

// WriteGrade computes the MD5 digest of content and writes it to grade.txt.
func (s *Store) WriteGrade(content string) error {
	h := md5.Sum([]byte(content))
	return os.WriteFile(s.path(GradeFile), []byte(fmt.Sprintf("%x", h)), 0644)
}

// GradeContent returns the content of grade.txt (raw, typically an MD5 hex).
func (s *Store) GradeContent() (string, error) {
	data, err := os.ReadFile(s.path(GradeFile))
	if os.IsNotExist(err) {
		return "", nil
	}
	return string(data), err
}

// OldGradeContent returns the content of old_grade.txt.
func (s *Store) OldGradeContent() (string, error) {
	data, err := os.ReadFile(s.path(OldGradeFile))
	if os.IsNotExist(err) {
		return "", nil
	}
	return string(data), err
}

// MD5 returns the hex-encoded MD5 digest of s.
func MD5(s string) string {
	h := md5.Sum([]byte(s))
	return fmt.Sprintf("%x", h)
}
