package archiver

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/asciimoo/hister/config"
)

func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const articleBody = `<p>The first paragraph of the article explains the topic in enough words for reader extraction to keep it.</p>
<p>A second paragraph continues the explanation with more sentences, so the content clearly looks like an article body.</p>
<p>A third paragraph closes the article and mentions details that a person might search for later on.</p>`

// testSite serves an article page with an inline image, an ad script and an
// image on a blocked host.
func testSite(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	pic := pngBytes(t)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/article":
			w.Header().Set("Content-Type", "text/html")
			ua := r.UserAgent()
			fmt.Fprintf(w, `<html><head><title>Test Article</title><meta name="description" content="desc"></head><body>
<nav><a href="/">Home</a> <a href="/about">About</a></nav>
<div class="ad">Buy things now</div>
<article><h1>Test Article</h1>%s
<p><img src="/pic.png" alt="a red pixel"></p>
<p><img src="http://ads.blocked.test/banner.png" alt="banner"></p>
<p id="ua">UA: %s</p>
<p id="cookie">Cookie: %s</p>
<script>document.write('<p>scripted</p>')</script>
</article><footer>Copyright</footer></body></html>`, articleBody, ua, r.Header.Get("Cookie"))
		case "/pic.png":
			hits.Add(1)
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pic)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestArchiveStoredKeepsMainTextAndEmbedsImages(t *testing.T) {
	var hits atomic.Int32
	srv := testSite(t, &hits)
	resp, err := http.Get(srv.URL + "/article")
	if err != nil {
		t.Fatal(err)
	}
	var page bytes.Buffer
	_, _ = page.ReadFrom(resp.Body)
	resp.Body.Close()

	out := t.TempDir()
	a := New(config.Archiver{Engine: config.ArchiverEngineStored, KeepImages: true}, "hister-test")
	res, err := a.Archive(context.Background(), Request{URL: srv.URL + "/article", StoredHTML: page.String(), OutputDir: out}, Options{Engine: "stored", KeepImages: true})
	if err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile(filepath.Join(out, IndexFile))
	if err != nil {
		t.Fatal(err)
	}
	s := string(html)
	if res.Title != "Test Article" || !strings.Contains(s, "<title>Test Article</title>") {
		t.Errorf("title = %q", res.Title)
	}
	if !strings.Contains(s, "The first paragraph of the article") || !strings.Contains(s, "data:image/png;base64,") {
		t.Errorf("archive lacks text or embedded image:\n%s", s)
	}
	for _, unwanted := range []string{"<script", "Buy things now", "Copyright", "/pic.png", "<nav"} {
		if strings.Contains(s, unwanted) {
			t.Errorf("archive contains %q", unwanted)
		}
	}
	if hits.Load() != 1 {
		t.Errorf("image fetched %d times", hits.Load())
	}

	out2 := t.TempDir()
	if _, err := a.Archive(context.Background(), Request{URL: srv.URL + "/article", StoredHTML: page.String(), OutputDir: out2}, Options{Engine: "stored"}); err != nil {
		t.Fatal(err)
	}
	html, _ = os.ReadFile(filepath.Join(out2, IndexFile))
	if strings.Contains(string(html), "<img") || !strings.Contains(string(html), "second paragraph") {
		t.Errorf("archive without images:\n%s", html)
	}

	if _, err := a.Archive(context.Background(), Request{URL: srv.URL, OutputDir: out2}, Options{Engine: "stored"}); err != ErrNoStoredHTML {
		t.Errorf("missing stored HTML error = %v", err)
	}
}

func TestParseFilterList(t *testing.T) {
	domains := make(map[string]struct{})
	list := `! comment
[Adblock Plus 2.0]
||ads.example.com^
||tracker.example.net^$third-party
||cdn.example.org/ads/*
0.0.0.0 doubleclick.test # comment
127.0.0.1 localhost
example.com##.ad
`
	if err := ParseFilterList(strings.NewReader(list), domains); err != nil {
		t.Fatal(err)
	}
	b := NewBlocklist(domains)
	for host, want := range map[string]bool{
		"ads.example.com": true, "x.ads.example.com": true, "example.com": false,
		"doubleclick.test": true, "tracker.example.net": false, "cdn.example.org": false, "localhost": false,
	} {
		if got := b.BlocksHost(host); got != want {
			t.Errorf("BlocksHost(%q) = %v, want %v", host, got, want)
		}
	}
	if !b.BlocksURL("https://ads.example.com/x.js") || b.Len() != 2 {
		t.Errorf("blocklist = %v", domains)
	}
}

func TestParseCookies(t *testing.T) {
	netscape := "# Netscape HTTP Cookie File\n.example.com\tTRUE\t/\tTRUE\t0\tsid\tabc\n#HttpOnly_example.com\tFALSE\t/a\tFALSE\t1\th\tv\n"
	cookies, err := ParseCookies(strings.NewReader(netscape))
	if err != nil || len(cookies) != 2 || cookies[0].Name != "sid" || !cookies[0].Secure || !cookies[1].HTTPOnly || cookies[1].Path != "/a" {
		t.Fatalf("cookies = %+v, %v", cookies, err)
	}
	cookies, err = ParseCookies(strings.NewReader(`[{"domain":".e.com","name":"a","value":"b","path":"/"}]`))
	if err != nil || len(cookies) != 1 || cookies[0].Value != "b" {
		t.Fatalf("json cookies = %+v, %v", cookies, err)
	}
	if _, err := ParseCookies(strings.NewReader("nothing")); err == nil {
		t.Error("invalid cookies accepted")
	}
}

func chromiumPath(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("HISTER_TEST_CHROMIUM"); p != "" {
		return p
	}
	for _, pattern := range []string{"/opt/pw-browsers/chromium-*/chrome-linux*/chrome", "/usr/bin/chromium", "/usr/bin/chromium-browser", "/usr/bin/google-chrome"} {
		if matches, _ := filepath.Glob(pattern); len(matches) > 0 {
			return matches[0]
		}
	}
	t.Skip("no Chromium found; set HISTER_TEST_CHROMIUM")
	return ""
}

func TestArchiveWithBrowser(t *testing.T) {
	browser := chromiumPath(t)
	var hits atomic.Int32
	srv := testSite(t, &hits)
	dir := t.TempDir()
	lists := filepath.Join(dir, "hosts.txt")
	if err := os.WriteFile(lists, []byte("0.0.0.0 ads.blocked.test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cookies := filepath.Join(dir, "cookies.txt")
	host := strings.TrimPrefix(srv.URL, "http://")
	host = host[:strings.IndexByte(host, ':')]
	if err := os.WriteFile(cookies, []byte(host+"\tFALSE\t/\tFALSE\t0\tsession\tsecret123\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := New(config.Archiver{BrowserPath: browser, AdblockLists: []string{lists}, Timeout: 60}, "hister-test-agent")
	out := t.TempDir()
	res, err := a.Archive(context.Background(), Request{
		URL: srv.URL + "/article", OutputDir: out, CookiesFile: cookies, ProfileDir: filepath.Join(dir, "profile"),
	}, Options{Engine: "browser", KeepImages: true, Mobile: true, Adblock: true})
	if err != nil {
		t.Fatal(err)
	}
	html, _ := os.ReadFile(filepath.Join(out, IndexFile))
	s := string(html)
	if res.Engine != "browser" || !strings.Contains(s, "Android") || !strings.Contains(s, "session=secret123") {
		t.Errorf("mobile user agent or cookie missing:\n%s", s)
	}
	if !strings.Contains(s, "data:image/png;base64,") || strings.Contains(s, "banner.png") {
		t.Errorf("images not embedded or blocked image kept:\n%s", s)
	}
	if strings.Contains(s, "<script") {
		t.Error("archive keeps scripts")
	}
	if _, err := os.Stat(filepath.Join(dir, "profile")); err != nil {
		t.Errorf("profile directory not created: %v", err)
	}
}

func TestArchiveWithSingleFile(t *testing.T) {
	singleFile := os.Getenv("HISTER_TEST_SINGLEFILE")
	if singleFile == "" {
		t.Skip("set HISTER_TEST_SINGLEFILE to the single-file CLI to run this test")
	}
	browser := chromiumPath(t)
	var hits atomic.Int32
	srv := testSite(t, &hits)
	a := New(config.Archiver{BrowserPath: browser, SingleFilePath: singleFile, Timeout: 90}, "hister-test-agent")
	out := t.TempDir()
	res, err := a.Archive(context.Background(), Request{URL: srv.URL + "/article", OutputDir: out},
		Options{Engine: "singlefile", KeepImages: true, Mobile: true})
	if err != nil {
		t.Fatal(err)
	}
	html, _ := os.ReadFile(filepath.Join(out, IndexFile))
	s := string(html)
	if res.Engine != "singlefile" || !strings.Contains(s, "The first paragraph of the article") {
		t.Errorf("archive lacks article text:\n%s", s)
	}
	if !strings.Contains(s, "data:image/png;base64,") || !strings.Contains(s, "Android") {
		t.Errorf("SingleFile image or mobile emulation missing:\n%s", s)
	}
	if strings.Contains(s, "<script") || strings.Contains(s, "Copyright") {
		t.Error("archive keeps scripts or page furniture")
	}
}

func TestArchiverProxy(t *testing.T) {
	var proxied atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A plain HTTP forward proxy receives absolute request URLs.
		proxied.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes(t))
	}))
	defer proxy.Close()
	a := New(config.Archiver{Proxy: proxy.URL}, "")
	page := `<html><head><title>P</title></head><body><article>` + articleBody + `<p><img src="http://images.invalid/a.png"></p></article></body></html>`
	out := t.TempDir()
	if _, err := a.Archive(context.Background(), Request{URL: "http://site.invalid/p", StoredHTML: page, OutputDir: out}, Options{Engine: "stored", KeepImages: true}); err != nil {
		t.Fatal(err)
	}
	html, _ := os.ReadFile(filepath.Join(out, IndexFile))
	if proxied.Load() != 1 || !strings.Contains(string(html), "data:image/png;base64,") {
		t.Errorf("image was not fetched through the proxy: %d requests", proxied.Load())
	}
	args := strings.Join(a.singleFileArgs(Request{URL: "http://site.invalid/p"}, Options{}), " ")
	if !strings.Contains(args, `--proxy-server=`+proxy.URL) {
		t.Errorf("SingleFile arguments lack the proxy: %s", args)
	}
	if New(config.Archiver{Proxy: "ftp://x"}, "").cfg.ProxyURL() != nil {
		t.Error("invalid proxy accepted")
	}
}
