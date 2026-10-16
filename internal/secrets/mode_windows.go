package secrets

import "io/fs"

// checkMode accepts any secrets file: Windows has no mode bits (Go reports
// 0666), and the file lives in the user's profile, whose ACL keeps others
// out.
func checkMode(string, fs.FileInfo) error { return nil }
