// SPDX-License-Identifier: AGPL-3.0-or-later

package archiver

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	maxFilterListBytes = 32 << 20
	filterListTTL      = 24 * time.Hour
)

// Blocklist matches request hosts against domain rules from filter lists.
// Only whole domain rules are supported: hosts files ("0.0.0.0 ads.example")
// and Adblock Plus rules of the form "||ads.example^". Cosmetic rules, path
// rules and exceptions are ignored; reader extraction removes most page
// furniture anyway.
type Blocklist struct {
	domains map[string]struct{}
}

// ParseFilterList reads domain rules from a hosts file or Adblock Plus list.
func ParseFilterList(r io.Reader, into map[string]struct{}) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '!' || line[0] == '#' || line[0] == '[' {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "||"); ok {
			end := strings.IndexAny(rest, "^/$*|")
			if end < 0 {
				end = len(rest)
			}
			// Rules with options or paths after the domain only apply to
			// some requests; skip them to avoid breaking pages.
			if end < len(rest) && (rest[end] != '^' || len(rest) > end+1) {
				continue
			}
			addDomain(into, rest[:end])
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && (fields[0] == "0.0.0.0" || fields[0] == "127.0.0.1" || fields[0] == "::") {
			for _, host := range fields[1:] {
				if strings.HasPrefix(host, "#") {
					break
				}
				addDomain(into, host)
			}
		}
	}
	return scanner.Err()
}

func addDomain(into map[string]struct{}, domain string) {
	domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	if domain == "" || domain == "localhost" || !strings.Contains(domain, ".") || strings.ContainsAny(domain, "*/ ") {
		return
	}
	into[domain] = struct{}{}
}

// NewBlocklist builds a blocklist from a set of domains.
func NewBlocklist(domains map[string]struct{}) *Blocklist {
	return &Blocklist{domains: domains}
}

// Len returns the number of blocked domains.
func (b *Blocklist) Len() int {
	if b == nil {
		return 0
	}
	return len(b.domains)
}

// BlocksHost reports whether host or one of its parent domains is blocked.
func (b *Blocklist) BlocksHost(host string) bool {
	if b == nil || len(b.domains) == 0 {
		return false
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for host != "" {
		if _, ok := b.domains[host]; ok {
			return true
		}
		i := strings.IndexByte(host, '.')
		if i < 0 {
			return false
		}
		host = host[i+1:]
	}
	return false
}

// BlocksURL reports whether the URL's host is blocked.
func (b *Blocklist) BlocksURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return b.BlocksHost(u.Hostname())
}

// blocklistCache loads filter lists once and refreshes them daily.
type blocklistCache struct {
	mu      sync.Mutex
	sources string
	loaded  time.Time
	list    *Blocklist
}

func (c *blocklistCache) get(ctx context.Context, sources []string, client *http.Client) *Blocklist {
	key := strings.Join(sources, "\n")
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.list != nil && c.sources == key && time.Since(c.loaded) < filterListTTL {
		return c.list
	}
	domains := make(map[string]struct{})
	for _, source := range sources {
		if err := loadFilterList(ctx, source, client, domains); err != nil {
			logWarn(err, "failed to load ad block list", source)
		}
	}
	c.list, c.sources, c.loaded = NewBlocklist(domains), key, time.Now()
	return c.list
}

func loadFilterList(ctx context.Context, source string, client *http.Client, into map[string]struct{}) error {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status %d", resp.StatusCode)
		}
		return ParseFilterList(io.LimitReader(resp.Body, maxFilterListBytes), into)
	}
	f, err := os.Open(source)
	if err != nil {
		return err
	}
	defer f.Close()
	return ParseFilterList(io.LimitReader(f, maxFilterListBytes), into)
}
