// SPDX-License-Identifier: AGPL-3.0-or-later

package archiver

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
)

// Cookie is one browser cookie.
type Cookie struct {
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Secure   bool    `json:"secure"`
	HTTPOnly bool    `json:"httpOnly"`
	Expires  float64 `json:"expirationDate"`
}

// MaxCookiesFileBytes bounds an uploaded cookies file.
const MaxCookiesFileBytes = 4 << 20

// ParseCookies reads a Netscape cookies.txt file or a JSON array of cookies
// as exported by common browser extensions.
func ParseCookies(r io.Reader) ([]Cookie, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxCookiesFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxCookiesFileBytes {
		return nil, errors.New("cookies file is too large")
	}
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "[") {
		var cookies []Cookie
		if err := json.Unmarshal([]byte(trimmed), &cookies); err != nil {
			return nil, err
		}
		return cookies, nil
	}
	var cookies []Cookie
	scanner := bufio.NewScanner(strings.NewReader(trimmed))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		httpOnly := false
		if rest, ok := strings.CutPrefix(line, "#HttpOnly_"); ok {
			line, httpOnly = rest, true
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 7 {
			continue
		}
		expires, _ := strconv.ParseFloat(f[4], 64)
		cookies = append(cookies, Cookie{
			Domain:   f[0],
			Path:     f[2],
			Secure:   strings.EqualFold(f[3], "TRUE"),
			Expires:  expires,
			Name:     f[5],
			Value:    f[6],
			HTTPOnly: httpOnly,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(cookies) == 0 {
		return nil, errors.New("no cookies found; use a Netscape cookies.txt or JSON export")
	}
	return cookies, nil
}

func (c Cookie) expired(now time.Time) bool {
	return c.Expires > 0 && c.Expires < float64(now.Unix())
}
