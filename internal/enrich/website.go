package enrich

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Website fetches the business's own site and extracts contact facts without
// any AI. It needs no key.
type Website struct {
	HTTP    *http.Client
	Limiter *Limiter
	Cache   Cache
	Agent   string
}

func NewWebsite(cache Cache) *Website {
	return &Website{
		HTTP: &http.Client{Timeout: 6 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			return nil
		}},
		Limiter: NewLimiter(1),
		Cache:   cache,
		Agent:   "KBO-Review/0.1 (+local data-quality tool; contact info extraction)",
	}
}

func (w *Website) Name() string     { return "website" }
func (w *Website) Ready() bool      { return true }
func (w *Website) CostEUR() float64 { return 0 }

var pagePaths = []string{"", "/contact", "/contact-us", "/contacteer-ons", "/over-ons", "/about", "/impressum", "/algemene-voorwaarden"}

// contactLinkRe finds same-site links whose path or text mentions contact,
// so sites with unusual contact URLs (/nl/contacteer, /praktijk/contact) are
// still covered.
var contactLinkRe = regexp.MustCompile(`(?i)href="([^"#?]*(?:contact|contacteer|kontakt|bereik)[^"#?]*)"`)

var (
	tagRe     = regexp.MustCompile(`(?is)<(script|style|noscript|svg|head)[^>]*>.*?</\s*(script|style|noscript|svg|head)\s*>`)
	titleRe   = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	anyTagRe  = regexp.MustCompile(`(?s)<[^>]+>`)
	mailtoRe  = regexp.MustCompile(`(?i)mailto:([^"'?\s>]+)`)
	entityRe  = regexp.MustCompile(`&(amp|nbsp|#160|quot|#39|lt|gt);`)
	closureRe = regexp.MustCompile(`(?i)permanently closed|definitief gesloten|is gesloten|deuren gesloten|stopgezet|activiteiten stopgezet|handelszaak (is )?overgelaten|ceased trading|out of business|fermé définitivement|a cessé ses activités`)
	parkedRe  = regexp.MustCompile(`(?i)domain (name )?(is )?for sale|this domain (has been|is) (registered|parked)|buy this domain|domein (is )?te koop|parked free|sedo|godaddy|hostinger|register\.be|combell.*(parked|tijdelijke)|website coming soon|under construction|in opbouw`)
)

type fetched struct {
	URL, Title, Text string
	Status           int
	Err              string
	HTML             string `json:"-"` // raw markup, kept only while discovering links
}

func (w *Website) Run(ctx context.Context, row *Row) error {
	site, trusted := row.Website(), row.Record.Website != ""
	if !trusted {
		for _, f := range row.Findings {
			if f.Field == "website" && f.Value == site && f.Confidence >= 0.5 {
				trusted = true
			}
		}
	}
	if site == "" {
		row.Signals = append(row.Signals, Signal{Source: w.Name(), Existence: Unknown, Note: "No website known for this business"})
		return nil
	}
	base, err := url.Parse(withScheme(site))
	if err != nil || base.Host == "" {
		row.Signals = append(row.Signals, Signal{Source: w.Name(), Existence: Unknown, Note: "Website address could not be parsed"})
		return nil
	}
	var pages []fetched
	if !w.Cache.Get("website", base.Host, &pages) {
		seen := map[string]bool{}
		paths := append([]string{}, pagePaths...)
		for i := 0; i < len(paths) && len(pages) < 6; i++ {
			p := paths[i]
			if seen[p] {
				continue
			}
			seen[p] = true
			if err := w.Limiter.Wait(ctx, base.Host); err != nil {
				return err
			}
			f := w.fetch(ctx, base.Scheme+"://"+base.Host+p)
			if p == "" {
				if f.Err != "" || f.Status >= 400 {
					pages = append(pages, f)
					break
				}
				// Discover the site's own contact links from the home page.
				for _, m := range contactLinkRe.FindAllStringSubmatch(f.HTML, -1) {
					if u, err := url.Parse(m[1]); err == nil {
						if u.Host == "" || strings.EqualFold(u.Host, base.Host) {
							if path := strings.TrimRight(u.Path, "/"); path != "" && !seen[path] {
								paths = append(paths[:i+1], append([]string{path}, paths[i+1:]...)...)
							}
						}
					}
				}
			}
			if f.Err == "" && f.Status == 200 {
				pages = append(pages, f)
			}
		}
		for i := range pages {
			pages[i].HTML = ""
		}
		w.Cache.Put("website", base.Host, pages)
	}
	w.interpret(row, base, pages, trusted)
	return nil
}

func withScheme(s string) string {
	if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
		return "https://" + s
	}
	return s
}

func (w *Website) fetch(ctx context.Context, u string) fetched {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fetched{URL: u, Err: err.Error()}
	}
	req.Header.Set("User-Agent", w.Agent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "nl-BE,nl,fr-BE,fr,en")
	res, err := w.HTTP.Do(req)
	if err != nil {
		return fetched{URL: u, Err: err.Error()}
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 512<<10))
	html := string(body)
	title := ""
	if m := titleRe.FindStringSubmatch(html); m != nil {
		title = strings.TrimSpace(cleanText(m[1]))
	}
	return fetched{URL: res.Request.URL.String(), Title: title, Text: extractText(html), Status: res.StatusCode, HTML: html}
}

// extractText strips markup while keeping mailto targets visible.
func extractText(html string) string {
	var mails []string
	for _, m := range mailtoRe.FindAllStringSubmatch(html, -1) {
		mails = append(mails, m[1])
	}
	html = tagRe.ReplaceAllString(html, " ")
	html = anyTagRe.ReplaceAllString(html, " ")
	return cleanText(html + " " + strings.Join(mails, " "))
}

func cleanText(s string) string {
	s = entityRe.ReplaceAllStringFunc(s, func(e string) string {
		switch e {
		case "&amp;":
			return "&"
		case "&quot;":
			return `"`
		case "&#39;":
			return "'"
		case "&lt;":
			return "<"
		case "&gt;":
			return ">"
		}
		return " "
	})
	return strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
}

// interpret turns fetched pages into signals and findings. trusted is false
// when the site was discovered through a listing whose name did not match;
// then only an enterprise-number or name match on the site restores trust,
// otherwise every finding is scaled down.
func (w *Website) interpret(row *Row, base *url.URL, pages []fetched, trusted bool) {
	src := w.Name()
	if len(pages) == 0 || (pages[0].Err != "" || pages[0].Status >= 400) {
		note := "Website did not respond"
		if len(pages) > 0 && pages[0].Status >= 400 {
			note = fmt.Sprintf("Website returned HTTP %d", pages[0].Status)
		}
		row.Signals = append(row.Signals, Signal{Source: src, Existence: Closed, Weight: 0.4, Note: note, URL: base.String()})
		return
	}
	home := pages[0]
	all := strings.Builder{}
	for _, p := range pages {
		all.WriteString(" ")
		all.WriteString(p.Text)
		row.Pages = append(row.Pages, Page{URL: p.URL, Title: p.Title, Text: truncate(p.Text, 8000)})
	}
	text := all.String()
	if parkedRe.MatchString(home.Title+" "+truncate(home.Text, 1500)) && len(home.Text) < 2500 {
		row.Signals = append(row.Signals, Signal{Source: src, Existence: Closed, Weight: 0.5, Note: "Domain looks parked or unused", URL: home.URL})
		return
	}
	if m := closureRe.FindString(text); m != "" {
		row.Signals = append(row.Signals, Signal{Source: src, Existence: Closed, Weight: 0.7, Note: "Site contains a closure notice: “" + m + "”", URL: home.URL})
	}
	// scale weights findings by how sure we are the site is this business's:
	// proven (enterprise number or name on the site) 1.15, plausible 1.0,
	// discovered via a mismatched listing 0.4.
	scale := 1.0
	if num := EnterpriseDigits(row.Record.Number); num != "" && vatMatches(text, num) {
		scale = 1.15
		row.Signals = append(row.Signals, Signal{Source: src, Existence: Alive, Weight: 0.9, Note: "Site shows this enterprise number", URL: home.URL})
	} else if sample := home.Title + " " + truncate(home.Text, 3000); Containment(row.Record.Name, sample) >= 0.5 || (TradeName(row.Record) != "" && Containment(TradeName(row.Record), sample) >= 0.5) {
		scale = 1.15
		row.Signals = append(row.Signals, Signal{Source: src, Existence: Alive, Weight: 0.5, Note: "Live site mentions the business name", URL: home.URL})
	} else if trusted {
		row.Signals = append(row.Signals, Signal{Source: src, Existence: Unknown, Weight: 0, Note: "Site is live but does not clearly name this business", URL: home.URL})
	} else {
		scale = 0.4
		row.Signals = append(row.Signals, Signal{Source: src, Existence: Unknown, Weight: 0, Note: "Site found via a listing with a different name and does not name this business; its details may belong to another business", URL: home.URL})
	}
	seen := map[string]bool{}
	for _, e := range emailRe.FindAllString(text, -1) {
		e = strings.ToLower(strings.Trim(e, ".,;:"))
		if seen[e] || GenericEmail(e) || strings.HasSuffix(e, ".png") || strings.HasSuffix(e, ".jpg") || strings.Contains(e, "example.") || strings.Contains(e, "sentry") || strings.Contains(e, "wixpress") {
			continue
		}
		seen[e] = true
		conf := 0.6
		if strings.HasSuffix(e, "@"+strings.TrimPrefix(base.Host, "www.")) {
			conf = 0.8
		}
		row.Findings = append(row.Findings, Finding{Source: src, Field: "email", Value: e, URL: home.URL, Note: "Found on the business website", Confidence: clamp(conf * scale)})
		if len(seen) >= 3 {
			break
		}
	}
	phones := map[string]bool{}
	vat := map[string]bool{}
	for _, m := range vatRe.FindAllStringSubmatch(text, -1) {
		vat["0"+m[1]+m[2]+m[3]] = true
	}
	for _, p := range phoneRe.FindAllString(text, -1) {
		n := Normalize("phone", p)
		if len(n) < 9 || len(n) > 10 || phones[n] || vat[n] || n == EnterpriseDigits(row.Record.Number) {
			continue
		}
		phones[n] = true
		row.Findings = append(row.Findings, Finding{Source: src, Field: "phone", Value: strings.TrimSpace(p), URL: home.URL, Note: "Found on the business website", Confidence: clamp(0.6 * scale)})
		if len(phones) >= 2 {
			break
		}
	}
	row.Findings = append(row.Findings, Finding{Source: src, Field: "website", Value: base.Scheme + "://" + base.Host, URL: home.URL, Note: "Site responded", Confidence: clamp(0.6 * scale)})
}

func vatMatches(text, digits10 string) bool {
	for _, m := range vatRe.FindAllStringSubmatch(text, -1) {
		if m[1]+m[2]+m[3] == digits10[1:] || "0"+m[1]+m[2]+m[3] == digits10 {
			return true
		}
	}
	return strings.Contains(digitsOnly(text), digits10)
}

func digitsOnly(s string) string { return digits(s) }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
