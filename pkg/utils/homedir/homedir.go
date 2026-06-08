package homedir

import (
	"os"
	"path/filepath"
	"runtime"
)

// HomeDir returns the home directory for the current user.
// 返回当前用户的主目录路径。
//
// On Windows:
// 1. the first of %HOME%, %HOMEDRIVE%%HOMEPATH%, %USERPROFILE% containing a `.apimachinery\config` file is returned.
// 2. if none of those locations contain a `.apimachinery\config` file, the first of
// %HOME%, %USERPROFILE%, %HOMEDRIVE%%HOMEPATH% that exists and is writeable is returned.
// 3. if none of those locations are writeable, the first of %HOME%, %USERPROFILE%,
// %HOMEDRIVE%%HOMEPATH% that exists is returned.
// 4. if none of those locations exists, the first of %HOME%, %USERPROFILE%,
// %HOMEDRIVE%%HOMEPATH% that is set is returned.
//
// Windows 下：
// 1. 依次检查 %HOME%、%HOMEDRIVE%%HOMEPATH%、%USERPROFILE%，返回首个存在 `.apimachinery\config` 的路径。
// 2. 若上述路径均不含该文件，则在 %HOME%、%USERPROFILE%、%HOMEDRIVE%%HOMEPATH% 中返回首个存在且可写的路径。
// 3. 若均不可写，则返回其中首个存在的路径。
// 4. 若均不存在，则返回其中首个已设置（非空）的路径。
func HomeDir() string {
	if runtime.GOOS != "windows" {
		return os.Getenv("HOME")
	}
	home := os.Getenv("HOME")
	homeDriveHomePath := ""
	if homeDrive, homePath := os.Getenv("HOMEDRIVE"), os.Getenv("HOMEPATH"); len(homeDrive) > 0 && len(homePath) > 0 {
		homeDriveHomePath = homeDrive + homePath
	}
	userProfile := os.Getenv("USERPROFILE")

	// Return the first of %HOME%, %HOMEDRIVE%/%HOMEPATH%, %USERPROFILE% that contains `.apimachinery\config`.
	// %HOMEDRIVE%/%HOMEPATH% is listed before %USERPROFILE% for backward compatibility.
	// 返回 %HOME%、%HOMEDRIVE%/%HOMEPATH%、%USERPROFILE% 中首个包含 `.apimachinery\config` 的路径。
	// 为保持向后兼容，%HOMEDRIVE%/%HOMEPATH% 排在 %USERPROFILE% 之前。
	for _, p := range []string{home, homeDriveHomePath, userProfile} {
		if len(p) == 0 {
			continue
		}
		if _, err := os.Stat(filepath.Join(p, ".apimachinery", "config")); err != nil {
			continue
		}
		return p
	}

	firstSetPath := ""
	firstExistingPath := ""

	// Prefer %USERPROFILE% over %HOMEDRIVE%/%HOMEPATH% for compatibility with other tools that write auth data.
	// 为与其他写入认证数据的工具兼容，优先使用 %USERPROFILE% 而非 %HOMEDRIVE%/%HOMEPATH%。
	for _, p := range []string{home, userProfile, homeDriveHomePath} {
		if len(p) == 0 {
			continue
		}
		if len(firstSetPath) == 0 {
			// Remember the first non-empty candidate path.
			// 记录首个非空的候选路径。
			firstSetPath = p
		}
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if len(firstExistingPath) == 0 {
			// Remember the first path that exists on disk.
			// 记录磁盘上首个存在的路径。
			firstExistingPath = p
		}
		if info.IsDir() && info.Mode().Perm()&(1<<(uint(7))) != 0 {
			// Return the first writable directory among candidates.
			// 在候选路径中返回首个可写目录。
			return p
		}
	}

	// If none are writable, return the first path that exists.
	// 若均不可写，则返回首个存在的路径。
	if len(firstExistingPath) > 0 {
		return firstExistingPath
	}

	// If none exist, return the first environment value that was set.
	// 若路径均不存在，则返回首个已设置的环境变量对应值。
	if len(firstSetPath) > 0 {
		return firstSetPath
	}

	// No usable home directory could be determined.
	// 无法确定可用的主目录。
	return ""
}
