package urlnorm

import (
	"errors"
	"math"
	"net/url"
	"sort"
	"strings"
)

var (
	ErrEmptyURL         = errors.New("empty url")
	ErrMissingScheme    = errors.New("missing url scheme")
	ErrDangerousScheme  = errors.New("dangerous url scheme")
	sensitiveQueryKeys  = map[string]struct{}{
		"token": {}, "access_token": {}, "refresh_token": {}, "id_token": {},
		"code": {}, "state": {}, "session": {}, "sid": {}, "password": {},
		"passwd": {}, "secret": {}, "api_key": {}, "key": {}, "auth": {},
		"jwt": {}, "signature": {}, "sig": {},
	}
	dangerousSchemes = map[string]struct{}{
		"file": {}, "javascript": {}, "data": {}, "vbscript": {}, "about": {},
		"chrome": {}, "edge": {}, "moz-extension": {},
	}
)

func NormalizeAndRedact(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ErrEmptyURL
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme == "" {
		return "", ErrMissingScheme
	}
	if _, blocked := dangerousSchemes[scheme]; blocked {
		return "", ErrDangerousScheme
	}

	u.Scheme = scheme
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	u.Path = maskSensitivePath(u.Path)
	u.RawQuery = normalizeAndRedactQuery(u.Query())

	return u.String(), nil
}

func normalizeAndRedactQuery(values url.Values) string {
	if len(values) == 0 {
		return ""
	}

	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := url.Values{}
	for _, key := range keys {
		originalVals := values[key]
		if len(originalVals) == 0 {
			out.Set(key, "")
			continue
		}

		lowerKey := strings.ToLower(key)
		if isSensitiveKey(lowerKey) {
			out.Set(key, "<redacted>")
			continue
		}

		redacted := make([]string, 0, len(originalVals))
		for _, v := range originalVals {
			if isHighEntropy(v) {
				redacted = append(redacted, "<redacted>")
			} else {
				redacted = append(redacted, v)
			}
		}
		out[key] = redacted
	}

	return out.Encode()
}

func isSensitiveKey(lowerKey string) bool {
	_, ok := sensitiveQueryKeys[lowerKey]
	return ok
}

func isHighEntropy(s string) bool {
	if len(s) < 24 {
		return false
	}

	freq := map[rune]float64{}
	for _, r := range s {
		freq[r]++
	}

	length := float64(len([]rune(s)))
	if length == 0 {
		return false
	}

	entropy := 0.0
	for _, c := range freq {
		p := c / length
		entropy += -p * math.Log2(p)
	}

	return entropy >= 3.5
}

func maskSensitivePath(path string) string {
	if path == "" {
		return path
	}

	segments := strings.Split(path, "/")
	for i, seg := range segments {
		s := strings.ToLower(seg)
		if s == "reset" || s == "invite" || s == "magic-link" || s == "oauth" {
			if i+1 < len(segments) {
				segments[i+1] = "<redacted>"
			}
		}
	}
	return strings.Join(segments, "/")
}
