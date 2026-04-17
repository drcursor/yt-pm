package youtube

import (
	"crypto/sha1"
	"fmt"
	"strings"
	"time"
)

// ComputeSAPISIDHASH computes a single SAPISIDHASH token.
// It SHA-1s the string "{ts} {sapisid} {origin}" and returns
// "SAPISIDHASH {ts}_{hex}".
//
// The SAPISIDHASH must be recomputed on every request because it embeds the
// current Unix timestamp. The sapisid value itself must never appear in logs;
// only the final opaque token is safe to log.
func ComputeSAPISIDHASH(sapisid, origin string) string {
	ts := fmt.Sprintf("%d", time.Now().Unix())
	input := ts + " " + sapisid + " " + origin
	h := sha1.New()
	h.Write([]byte(input))
	return fmt.Sprintf("SAPISIDHASH %s_%x", ts, h.Sum(nil))
}

// computeSchemeHash is the internal helper that produces a named hash token.
// scheme is one of "SAPISIDHASH", "SAPISID1PHASH", "SAPISID3PHASH".
func computeSchemeHash(scheme, sapisid, origin string) string {
	ts := fmt.Sprintf("%d", time.Now().Unix())
	input := ts + " " + sapisid + " " + origin
	h := sha1.New()
	h.Write([]byte(input))
	return fmt.Sprintf("%s %s_%x", scheme, ts, h.Sum(nil))
}

// BuildAuthHeader returns the full Authorization header value for a session.
// It handles all three SAPISIDHASH variants and space-joins them when the
// corresponding cookies are present:
//
//   - SAPISIDHASH     — uses SAPISID (falls back to __Secure-3PAPISID)
//   - SAPISID1PHASH   — uses __Secure-1PAPISID (only when present)
//   - SAPISID3PHASH   — uses __Secure-3PAPISID (only when present)
//
// Session tokens and cookie values are NEVER written to logs; only the
// computed opaque hash tokens appear in any output.
func BuildAuthHeader(session Session) string {
	origin := session.Origin
	if origin == "" {
		origin = "https://www.youtube.com"
	}

	var parts []string

	// Primary: SAPISIDHASH
	// Prefer SAPISID; fall back to __Secure-3PAPISID per yt-dlp issue #393.
	primarySID := session.Cookies["SAPISID"]
	if primarySID == "" {
		primarySID = session.Cookies["__Secure-3PAPISID"]
	}
	if primarySID != "" {
		parts = append(parts, computeSchemeHash("SAPISIDHASH", primarySID, origin))
	}

	// Optional: SAPISID1PHASH — only when __Secure-1PAPISID cookie is present.
	if sid1p := session.Cookies["__Secure-1PAPISID"]; sid1p != "" {
		parts = append(parts, computeSchemeHash("SAPISID1PHASH", sid1p, origin))
	}

	// Optional: SAPISID3PHASH — only when __Secure-3PAPISID cookie is present.
	if sid3p := session.Cookies["__Secure-3PAPISID"]; sid3p != "" {
		parts = append(parts, computeSchemeHash("SAPISID3PHASH", sid3p, origin))
	}

	return strings.Join(parts, " ")
}
