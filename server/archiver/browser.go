// SPDX-License-Identifier: AGPL-3.0-or-later

package archiver

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// Chromium locks a profile directory, so archives sharing one run in turn.
var profileLocks sync.Map

func lockProfile(dir string) func() {
	if dir == "" {
		return func() {}
	}
	v, _ := profileLocks.LoadOrStore(dir, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// browserOptions are the Chromium flags shared by the browser engine and login.
func (a *Archiver) browserOptions(profileDir string, headless bool, mobile bool) []chromedp.ExecAllocatorOption {
	opts := []chromedp.ExecAllocatorOption{
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("mute-audio", true),
	}
	if headless {
		opts = append(opts, chromedp.Headless, chromedp.Flag("hide-scrollbars", true))
	}
	if os.Geteuid() == 0 {
		opts = append(opts, chromedp.NoSandbox)
	}
	if a.cfg.BrowserPath != "" {
		opts = append(opts, chromedp.ExecPath(a.cfg.BrowserPath))
	}
	if profileDir != "" {
		opts = append(opts, chromedp.UserDataDir(profileDir))
	}
	if proxy := a.cfg.ProxyURL(); proxy != nil {
		opts = append(opts, chromedp.ProxyServer(proxy.String()))
	}
	if mobile {
		opts = append(opts, chromedp.WindowSize(412, 915), chromedp.UserAgent(MobileUserAgent))
	} else {
		opts = append(opts, chromedp.WindowSize(1280, 1024))
		if a.userAgent != "" {
			opts = append(opts, chromedp.UserAgent(a.userAgent))
		}
	}
	return opts
}

// fetchWithBrowser loads a page in headless Chromium and returns its rendered HTML.
func (a *Archiver) fetchWithBrowser(ctx context.Context, req Request, opts Options, blocklist *Blocklist) (string, error) {
	unlock := lockProfile(req.ProfileDir)
	defer unlock()
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, a.browserOptions(req.ProfileDir, true, opts.Mobile)...)
	defer cancelAlloc()
	taskCtx, cancelTask := chromedp.NewContext(allocCtx)
	defer cancelTask()

	if _, err := chromedp.Call(taskCtx, network.Enable, network.EnableParams{}); err != nil {
		return "", fmt.Errorf("browser setup failed: %w", err)
	}
	if opts.Mobile {
		if err := chromedp.Do(taskCtx, chromedp.EmulateViewport(412, 915,
			chromedp.EmulateScale(2.625), chromedp.EmulatePortrait, chromedp.EmulateMobile, chromedp.EmulateTouch)); err != nil {
			return "", fmt.Errorf("browser setup failed: %w", err)
		}
	}
	if blocklist.Len() > 0 {
		paused := make(chan *fetch.EventRequestPaused, 256)
		// The listener must not run commands; requests are answered elsewhere.
		if err := chromedp.Listen(taskCtx, func(ev any) {
			if e, ok := ev.(*fetch.EventRequestPaused); ok {
				select {
				case paused <- e:
				case <-taskCtx.Done():
				}
			}
		}); err != nil {
			return "", fmt.Errorf("browser setup failed: %w", err)
		}
		go func() {
			for {
				select {
				case <-taskCtx.Done():
					return
				case e := <-paused:
					if blocklist.BlocksURL(e.Request.URL) {
						_, _ = chromedp.Call(taskCtx, fetch.FailRequest, fetch.FailRequestParams{
							RequestID: e.RequestID, ErrorReason: network.ErrorReasonBlockedByClient})
					} else {
						_, _ = chromedp.Call(taskCtx, fetch.ContinueRequest, fetch.ContinueRequestParams{RequestID: e.RequestID})
					}
				}
			}
		}()
		if _, err := chromedp.Call(taskCtx, fetch.Enable, fetch.EnableParams{}); err != nil {
			return "", fmt.Errorf("browser setup failed: %w", err)
		}
	}
	if req.CookiesFile != "" {
		if err := setBrowserCookies(taskCtx, req.CookiesFile); err != nil {
			return "", err
		}
	}
	if err := chromedp.Do(taskCtx,
		chromedp.Navigate(req.URL),
		chromedp.WaitReady(chromedp.CSS("body")),
	); err != nil {
		return "", fmt.Errorf("loading the page failed: %w", err)
	}
	// Scroll through the page so lazily loaded images get real sources.
	_, _ = chromedp.Run(taskCtx, chromedp.Evaluate[bool](`(async () => {
		for (let y = 0; y < document.body.scrollHeight && y < 40000; y += innerHeight) {
			scrollTo(0, y); await new Promise(r => setTimeout(r, 120));
		}
		scrollTo(0, 0); return true;
	})()`, chromedp.EvalAwaitPromise))
	html, err := chromedp.Run(taskCtx, chromedp.OuterHTML(chromedp.CSS("html")))
	if err != nil {
		return "", fmt.Errorf("reading the page failed: %w", err)
	}
	return html, nil
}

// Login opens a visible browser with the user's profile so the user can sign
// in to sites. It returns when the browser window is closed.
func (a *Archiver) Login(ctx context.Context, profileDir, startURL string) error {
	if profileDir == "" {
		return fmt.Errorf("archiver.profile_dir is not configured")
	}
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		return err
	}
	unlock := lockProfile(profileDir)
	defer unlock()
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, a.browserOptions(profileDir, false, false)...)
	defer cancelAlloc()
	taskCtx, cancelTask := chromedp.NewContext(allocCtx)
	defer cancelTask()
	if startURL == "" {
		startURL = "about:blank"
	}
	if err := chromedp.Do(taskCtx, chromedp.Navigate(startURL)); err != nil {
		return err
	}
	<-chromedp.FromContext(taskCtx).Browser.LostConnection
	return nil
}

func setBrowserCookies(ctx context.Context, path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	cookies, err := ParseCookies(f)
	_ = f.Close()
	if err != nil {
		return fmt.Errorf("invalid cookies file: %w", err)
	}
	now := time.Now()
	params := make([]*network.CookieParam, 0, len(cookies))
	for _, ck := range cookies {
		if ck.expired(now) || ck.Name == "" {
			continue
		}
		path := ck.Path
		if path == "" {
			path = "/"
		}
		secure, httpOnly := ck.Secure, ck.HTTPOnly
		params = append(params, &network.CookieParam{
			Name: ck.Name, Value: ck.Value, Domain: ck.Domain, Path: path,
			Secure: secure, HTTPOnly: httpOnly, Expires: cdp.TimeSinceEpoch(ck.Expires),
		})
	}
	if len(params) == 0 {
		return nil
	}
	if _, err := chromedp.Call(ctx, network.SetCookies, network.SetCookiesParams{Cookies: params}); err != nil {
		return fmt.Errorf("setting cookies failed: %w", err)
	}
	return nil
}
