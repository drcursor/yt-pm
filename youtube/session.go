package youtube

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadSessionFromCookieFile parses a Netscape/Mozilla cookie file (the format
// exported by browser extensions like "Get cookies.txt") and returns a Session
// populated from the cookies found in that file.
//
// The Netscape format uses tab-separated lines:
//
//	domain  httpOnly  path  secure  expiry  name  value
//
// Lines starting with '#' or empty lines are skipped.
func LoadSessionFromCookieFile(path string) (Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return Session{}, fmt.Errorf("opening cookie file %q: %w", path, err)
	}
	defer f.Close()

	cookies := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		// Skip blank lines and comments (including the "# Netscape HTTP…" header).
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Netscape format: 7 tab-separated fields.
		// Some exporters (and clipboard paste via textarea) produce space-separated
		// output instead of tab-separated. Handle both.
		var fields []string
		if strings.Contains(line, "\t") {
			fields = strings.Split(line, "\t")
		} else {
			// Space-separated: split on any whitespace. The value field (index 6)
			// may itself contain spaces; rejoin everything from index 6 onward.
			fields = strings.Fields(line)
		}
		if len(fields) < 7 {
			continue
		}

		name := strings.TrimSpace(fields[5])
		value := strings.TrimSpace(strings.Join(fields[6:], " "))
		if name != "" {
			cookies[name] = value
		}
	}
	if err := scanner.Err(); err != nil {
		return Session{}, fmt.Errorf("reading cookie file %q: %w", path, err)
	}

	return Session{
		Cookies: cookies,
		Origin:  "https://www.youtube.com",
	}, nil
}

// savedSession is the on-disk representation used by SaveSession / LoadSession.
type savedSession struct {
	Cookies     map[string]string `json:"cookies"`
	Origin      string            `json:"origin"`
	VisitorData string            `json:"visitor_data,omitempty"`
}

// SaveSession writes s to path atomically by first writing to a temporary file
// in the same directory and then renaming it into place.  This prevents
// partial writes from corrupting an existing session file.
//
// Session tokens are written to disk in their raw form — ensure the file has
// restrictive permissions (0600).
func SaveSession(path string, s Session) error {
	data := savedSession{
		Cookies:     s.Cookies,
		Origin:      s.Origin,
		VisitorData: s.VisitorData,
	}

	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling session: %w", err)
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".yt-session-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file for session: %w", err)
	}
	tmpName := tmp.Name()

	// Best-effort cleanup on any error path.
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("setting permissions on temp session file: %w", err)
	}

	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing session to temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp session file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("renaming temp session file to %q: %w", path, err)
	}

	success = true
	return nil
}

// LoadSession reads a session previously written by SaveSession.
func LoadSession(path string) (Session, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Session{}, fmt.Errorf("reading session file %q: %w", path, err)
	}

	var data savedSession
	if err := json.Unmarshal(b, &data); err != nil {
		return Session{}, fmt.Errorf("parsing session file %q: %w", path, err)
	}

	return Session{
		Cookies:     data.Cookies,
		Origin:      data.Origin,
		VisitorData: data.VisitorData,
	}, nil
}
