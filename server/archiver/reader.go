// SPDX-License-Identifier: AGPL-3.0-or-later

package archiver

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"html/template"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"codeberg.org/readeck/go-readability/v2"
	"github.com/PuerkitoBio/goquery"
	"github.com/microcosm-cc/bluemonday"
)

// readerPolicy keeps article markup and embedded images but no active content.
var readerPolicy = func() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowDataURIImages()
	p.AllowElements("figure", "figcaption", "picture", "mark", "time", "details", "summary")
	p.AllowAttrs("datetime").OnElements("time")
	p.AllowAttrs("controls", "src", "type", "preload").OnElements("video", "audio", "source")
	p.AllowElements("video", "audio", "source")
	p.AllowURLSchemes("http", "https", "mailto", "data")
	p.RequireNoFollowOnLinks(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)
	return p
}()

// Article is the main content extracted from a page.
type Article struct {
	Title     string
	Byline    string
	SiteName  string
	Published string
	Content   string // sanitized HTML
	Excerpt   string
}

// ExtractArticle runs reader extraction on a page and returns its main content.
func ExtractArticle(pageHTML, pageURL string) (*Article, error) {
	u, err := url.Parse(pageURL)
	if err != nil {
		return nil, err
	}
	a, err := readability.FromReader(strings.NewReader(pageHTML), u)
	if err != nil {
		return nil, fmt.Errorf("reader extraction failed: %w", err)
	}
	var content strings.Builder
	if err := a.RenderHTML(&content); err != nil {
		return nil, fmt.Errorf("reader extraction failed: %w", err)
	}
	if strings.TrimSpace(content.String()) == "" {
		return nil, fmt.Errorf("no readable content found")
	}
	art := &Article{
		Title:    a.Title(),
		Byline:   a.Byline(),
		SiteName: a.SiteName(),
		Excerpt:  a.Excerpt(),
		Content:  content.String(),
	}
	if t, err := a.PublishedTime(); err == nil && !t.IsZero() {
		art.Published = t.Format("2006-01-02")
	}
	return art, nil
}

// imageInliner replaces image URLs with data URIs.
type imageInliner struct {
	client    *http.Client
	userAgent string
	maxBytes  int64
	blocklist *Blocklist
	cache     map[string]string
}

func (in *imageInliner) dataURI(ctx context.Context, src string) string {
	if strings.HasPrefix(src, "data:image/") {
		return src
	}
	if v, ok := in.cache[src]; ok {
		return v
	}
	in.cache[src] = ""
	u, err := url.Parse(src)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || in.blocklist.BlocksHost(u.Hostname()) {
		return ""
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return ""
	}
	if in.userAgent != "" {
		req.Header.Set("User-Agent", in.userAgent)
	}
	req.Header.Set("Accept", "image/avif,image/webp,image/png,image/svg+xml,image/*;q=0.8")
	resp, err := in.client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength > in.maxBytes {
		return ""
	}
	mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	data, err := io.ReadAll(io.LimitReader(resp.Body, in.maxBytes+1))
	if err != nil || int64(len(data)) > in.maxBytes || len(data) == 0 {
		return ""
	}
	if !strings.HasPrefix(mediaType, "image/") {
		mediaType = http.DetectContentType(data)
	}
	// SVG can carry scripts; only raster images are embedded.
	if !strings.HasPrefix(mediaType, "image/") || mediaType == "image/svg+xml" {
		return ""
	}
	v := "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(data)
	in.cache[src] = v
	return v
}

// processImages embeds or removes the images of article content.
func processImages(ctx context.Context, content string, keep bool, in *imageInliner) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(content))
	if err != nil {
		return "", err
	}
	if !keep {
		doc.Find("img, picture, svg, figure:not(:has(figcaption))").Remove()
		doc.Find("figure img").Remove()
	} else {
		doc.Find("picture source").Remove()
		doc.Find("img").Each(func(_ int, img *goquery.Selection) {
			img.RemoveAttr("srcset")
			img.RemoveAttr("sizes")
			img.RemoveAttr("loading")
			src, _ := img.Attr("src")
			if src == "" {
				img.Remove()
				return
			}
			if ctx.Err() != nil {
				return
			}
			if data := in.dataURI(ctx, src); data != "" {
				img.SetAttr("src", data)
			} else {
				// Keep the description instead of a broken remote reference.
				if alt, _ := img.Attr("alt"); strings.TrimSpace(alt) != "" {
					img.ReplaceWithHtml("<span>[" + template.HTMLEscapeString(alt) + "]</span>")
				} else {
					img.Remove()
				}
			}
		})
	}
	return doc.Find("body").Html()
}

// Video is a media file kept with an archive.
type Video struct {
	File string
	Type string
}

var readerTemplate = template.Must(template.New("reader").Parse(`<!DOCTYPE html>
<html lang="{{.Lang}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="referrer" content="no-referrer">
<title>{{.Title}}</title>
<style>
:root { color-scheme: light dark; --fg: #1d1d1f; --bg: #fbfaf7; --muted: #6b6b70; --link: #0b6e75; --rule: #e3e1dc; }
@media (prefers-color-scheme: dark) { :root { --fg: #e7e5e0; --bg: #1b1b1d; --muted: #a09fa6; --link: #5fc4c9; --rule: #333336; } }
html { background: var(--bg); color: var(--fg); }
body { margin: 0 auto; max-width: 42rem; padding: 2rem 1rem 4rem; font: 1.125rem/1.7 Georgia, "Iowan Old Style", serif; overflow-wrap: break-word; }
header { border-bottom: 1px solid var(--rule); margin-bottom: 2rem; padding-bottom: 1rem; }
h1 { font-size: 2rem; line-height: 1.2; margin: 0 0 .5rem; }
.meta { color: var(--muted); font: .875rem/1.5 system-ui, sans-serif; }
.meta a, a { color: var(--link); }
img, video { max-width: 100%; height: auto; }
figure { margin: 1.5rem 0; } figcaption { color: var(--muted); font-size: .875rem; }
pre { overflow-x: auto; padding: 1rem; background: rgba(127,127,127,.12); font-size: .875rem; }
code { font-size: .9em; } blockquote { margin: 1rem 0; padding-left: 1rem; border-left: 3px solid var(--rule); color: var(--muted); }
table { border-collapse: collapse; display: block; overflow-x: auto; } td, th { border: 1px solid var(--rule); padding: .25rem .5rem; }
</style>
</head>
<body>
<header>
<h1>{{.Title}}</h1>
<p class="meta">{{if .Byline}}{{.Byline}} · {{end}}{{if .SiteName}}{{.SiteName}} · {{end}}{{if .Published}}Published {{.Published}} · {{end}}Archived {{.Archived}}<br><a href="{{.URL}}" rel="noopener noreferrer nofollow">{{.URL}}</a></p>
</header>
<article>
{{range .Videos}}<video controls preload="metadata" src="{{.File}}"{{if .Type}} type="{{.Type}}"{{end}}></video>
{{end}}{{.Content}}
</article>
</body>
</html>
`))

type readerPage struct {
	Lang, Title, Byline, SiteName, Published, Archived, URL string
	Videos                                                  []Video
	Content                                                 template.HTML
}

// RenderReader builds the standalone archive page.
func RenderReader(art *Article, pageURL, lang string, archived time.Time, videos []Video) ([]byte, error) {
	var buf bytes.Buffer
	err := readerTemplate.Execute(&buf, readerPage{
		Lang:      lang,
		Title:     art.Title,
		Byline:    art.Byline,
		SiteName:  art.SiteName,
		Published: art.Published,
		Archived:  archived.UTC().Format("2006-01-02 15:04 UTC"),
		URL:       pageURL,
		Videos:    videos,
		// Content is sanitized by readerPolicy before rendering.
		Content: template.HTML(art.Content), //nolint:gosec
	})
	return buf.Bytes(), err
}
