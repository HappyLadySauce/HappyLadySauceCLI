package homedir

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestHomeDirNonWindowsReturnsHomeEnv checks non-Windows builds return $HOME verbatim.
// TestHomeDirNonWindowsReturnsHomeEnv 校验非 Windows 下直接返回 $HOME。
func TestHomeDirNonWindowsReturnsHomeEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("non-Windows only")
	}
	t.Setenv("HOME", "/tmp/x")
	if got, want := HomeDir(), "/tmp/x"; got != want {
		t.Errorf("HomeDir() = %q, want %q", got, want)
	}
}

// TestHomeDirNonWindowsEmptyHomeReturnsEmpty checks empty HOME yields empty result on Unix-like OS.
// TestHomeDirNonWindowsEmptyHomeReturnsEmpty 校验非 Windows 下空 HOME 返回空字符串。
func TestHomeDirNonWindowsEmptyHomeReturnsEmpty(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("non-Windows only")
	}
	t.Setenv("HOME", "")
	if got := HomeDir(); got != "" {
		t.Errorf("HomeDir() = %q, want empty string", got)
	}
}

// TestHomeDirWindowsApimachineryConfigPriority returns the first candidate containing `.apimachinery\config`.
// TestHomeDirWindowsApimachineryConfigPriority 校验 Windows 下哨兵文件路径优先级。
func TestHomeDirWindowsApimachineryConfigPriority(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	homeA := t.TempDir()
	homeB := t.TempDir()
	homeC := t.TempDir()
	sentinel := filepath.Join(homeB, ".apimachinery", "config")
	if err := os.MkdirAll(filepath.Dir(sentinel), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v, want nil", filepath.Dir(sentinel), err)
	}
	if err := os.WriteFile(sentinel, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v, want nil", sentinel, err)
	}

	t.Setenv("HOME", homeA)
	vol := filepath.VolumeName(homeB)
	rest := homeB[len(vol):]
	t.Setenv("HOMEDRIVE", vol)
	t.Setenv("HOMEPATH", rest)
	t.Setenv("USERPROFILE", homeC)

	if got, want := HomeDir(), homeB; got != want {
		t.Errorf("HomeDir() = %q, want %q", got, want)
	}
}

// TestHomeDirWindowsPrefersUserProfileOverHomeDriveHomePath picks USERPROFILE when all lack the sentinel file.
// TestHomeDirWindowsPrefersUserProfileOverHomeDriveHomePath 在无哨兵文件时优先可写的 USERPROFILE。
func TestHomeDirWindowsPrefersUserProfileOverHomeDriveHomePath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	tmp1 := t.TempDir()
	tmp2 := t.TempDir()
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", tmp1)
	v2 := filepath.VolumeName(tmp2)
	t.Setenv("HOMEDRIVE", v2)
	t.Setenv("HOMEPATH", tmp2[len(v2):])

	if got, want := HomeDir(), tmp1; got != want {
		t.Errorf("HomeDir() = %q, want %q", got, want)
	}
}

// TestHomeDirWindowsFallsBackToFirstSetWhenNoneExists returns the first set path when nothing exists on disk.
// TestHomeDirWindowsFallsBackToFirstSetWhenNoneExists 在磁盘上均不存在时返回首个已设置路径。
func TestHomeDirWindowsFallsBackToFirstSetWhenNoneExists(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	nonexistent := `C:\no\such\directory\beehive-homedir-test-999999`
	t.Setenv("HOME", nonexistent)
	t.Setenv("USERPROFILE", "")
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")

	if got, want := HomeDir(), nonexistent; got != want {
		t.Errorf("HomeDir() = %q, want %q", got, want)
	}
}

// TestHomeDirWindowsReturnsEmptyWhenAllUnset yields empty string when no home-related env vars are set.
// TestHomeDirWindowsReturnsEmptyWhenAllUnset 在相关环境变量均未设置时返回空字符串。
func TestHomeDirWindowsReturnsEmptyWhenAllUnset(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")

	if got := HomeDir(); got != "" {
		t.Errorf("HomeDir() = %q, want empty string", got)
	}
}
