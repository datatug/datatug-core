package filestore

import (
	"os"
	"path"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// reservedQueryTxnDirName is the entry a revisioned query transaction
// reserves at the queries root for its lock file, journal and staging area
// (see query_txn.go). No folder path segment or query ID may use it:
// validateQuerySegmentReason rejects it explicitly, on top of the
// leading-"." rule it would already fall under, so a caller can never
// address it as an ordinary query location and the recursive query-tree
// loader can skip it by exact name once it has checked the entry's type
// and permissions (see queries_tree.go).
const reservedQueryTxnDirName = ".dt-query-txn"

// windowsIllegalChars are characters Windows forbids in a file/directory
// name, held here as the common subset every OS validates against: a
// query location must be portable to the platforms datatug-core supports
// (including Windows, see the plan's lock-primitive platform check), not
// merely legal on the OS a given `datatug serve` happens to run on.
const windowsIllegalChars = `<>:"/\|?*`

// maxQuerySegmentLength bounds a single folder/ID segment well under every
// common file system's per-component limit (255 bytes on ext4/APFS/NTFS),
// leaving room for the "<id>.query.<ext>" suffix this store appends.
const maxQuerySegmentLength = 200

// windowsReservedNames are device names Windows reserves regardless of
// extension, compared case-insensitively against a segment's name before
// its first "." (with trailing spaces trimmed, which Windows ignores there
// too - "CON .txt" still opens the console). The set follows Microsoft's
// "Naming Files, Paths, and Namespaces" list: CON, PRN, AUX, NUL, COM0-COM9,
// LPT0-LPT9, the superscript-digit variants COM¹-COM³/LPT¹-LPT³ (Windows
// treats ¹²³ as digits in these names), and the console aliases CONIN$ and
// CONOUT$.
var windowsReservedNames = func() map[string]bool {
	names := map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true, "CONIN$": true, "CONOUT$": true}
	for _, digit := range []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "¹", "²", "³"} {
		names["COM"+digit] = true
		names["LPT"+digit] = true
	}
	return names
}()

// unsafeNameRuneReason reports why r may not appear in a query folder
// segment or ID, or ("", false) when it may. Control characters (Unicode
// category Cc: C0, DEL and C1) can corrupt terminal output, logs and git
// porcelain; the Unicode Bidi_Control characters (U+061C, U+200E, U+200F,
// U+202A-U+202E, U+2066-U+2069) can make a name display as something it is
// not ("Trojan Source"-style spoofing in a file listing or a diff). NUL is
// reported separately by validateQuerySegmentReason.
func unsafeNameRuneReason(r rune) (reason string, bad bool) {
	switch {
	case unicode.IsControl(r):
		return "must not contain control characters", true
	case unicode.Is(unicode.Bidi_Control, r):
		return "must not contain bidirectional-text control characters", true
	}
	return "", false
}

func invalidQueryLocation(folderPath, id, reason string) error {
	return &datatug.InvalidQueryLocationError{FolderPath: folderPath, ID: id, Reason: reason}
}

// validateQuerySegmentReason applies the safety rules shared by every
// folder path segment and the query ID: non-empty; not "." or ".."; no NUL
// byte; none of windowsIllegalChars (also excludes a stray "/" or "\\", so
// a segment can never smuggle in an extra path component); not the
// reserved transaction namespace; not starting with "." (reserves hidden
// names generally); not ending with "." or a space (illegal on Windows);
// not a Windows-reserved device name; within maxQuerySegmentLength.
func validateQuerySegmentReason(segment string) (reason string, ok bool) {
	switch {
	case segment == "":
		return "must not be empty", false
	case segment == "." || segment == "..":
		return `must not be "." or ".."`, false
	case strings.ContainsRune(segment, 0):
		return "must not contain a NUL byte", false
	case !utf8.ValidString(segment):
		return "must be valid UTF-8", false
	}
	for _, r := range segment {
		if reason, bad := unsafeNameRuneReason(r); bad {
			return reason, false
		}
	}
	switch {
	case strings.ContainsAny(segment, windowsIllegalChars):
		return "must not contain any of " + windowsIllegalChars, false
	case segment == reservedQueryTxnDirName:
		return "reserved for the query transaction namespace", false
	case strings.HasPrefix(segment, "."):
		return `must not start with "."`, false
	case strings.HasSuffix(segment, ".") || strings.HasSuffix(segment, " "):
		return `must not end with "." or a space (illegal on Windows)`, false
	case len(segment) > maxQuerySegmentLength:
		return "exceeds maximum segment length", false
	}
	base := segment
	if i := strings.IndexByte(base, '.'); i > 0 {
		base = base[:i]
	}
	base = strings.TrimRight(base, " ")
	if windowsReservedNames[strings.ToUpper(base)] {
		return "is a Windows-reserved device name", false
	}
	return "", true
}

// validateQueryFolderPath validates a query's on-disk folder path: "" (the
// queries root) or a "/"-separated list of safe relative segments. It
// never touches the file system.
func validateQueryFolderPath(folderPath string) error {
	if folderPath == "" {
		return nil
	}
	for _, seg := range strings.Split(folderPath, "/") {
		if reason, ok := validateQuerySegmentReason(seg); !ok {
			return invalidQueryLocation(folderPath, "", "folder path segment "+strconv.Quote(seg)+": "+reason)
		}
	}
	return nil
}

// validateQueryID validates a query ID: one portable filename segment. It
// never touches the file system.
func validateQueryID(id string) error {
	if strings.ContainsAny(id, "/\\") {
		return invalidQueryLocation("", id, "id: must not contain a path separator")
	}
	if reason, ok := validateQuerySegmentReason(id); !ok {
		return invalidQueryLocation("", id, "id: "+reason)
	}
	return nil
}

// lstatSegmentIssue reports why segmentPath cannot be a query location
// directory, or ("", false) when it may be one: either it does not exist
// yet (the normal case - it will be created on write) or it already exists
// as an ordinary directory. It never follows a symlink to see what it
// points to; any symlink along a query location is rejected outright,
// which is what proves containment without needing to resolve where a
// symlink actually leads.
func lstatSegmentIssue(segmentPath string) (reason string, bad bool) {
	info, err := os.Lstat(segmentPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false
		}
		return err.Error(), true
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "resolves through a symlink", true
	}
	if !info.IsDir() {
		return "a path component exists and is not a directory", true
	}
	return "", false
}

// resolveQueryLocation validates folderPath and id, then resolves and
// returns the absolute directory their pair's files live in: the
// project's canonical "queries/" root plus every validated folder
// segment. It proves containment by Lstat-checking every segment that
// already exists on disk as it extends the path, rejecting a symlink or
// non-directory before returning - so a caller can join the returned
// directory with a file name and know the result cannot resolve outside
// the queries root. It performs no I/O beyond those Lstat calls: nothing
// is created, and a rejected request leaves the file system untouched.
func (s fsQueriesStore) resolveQueryLocation(folderPath, id string) (string, error) {
	if err := validateQueryFolderPath(folderPath); err != nil {
		return "", err
	}
	if err := validateQueryID(id); err != nil {
		return "", err
	}
	dir := s.dirPath
	if folderPath != "" {
		for _, seg := range strings.Split(folderPath, "/") {
			dir = path.Join(dir, seg)
			if reason, bad := lstatSegmentIssue(dir); bad {
				return "", invalidQueryLocation(folderPath, id, reason)
			}
		}
	}
	return dir, nil
}
