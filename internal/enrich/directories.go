package enrich

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"kbo-review/internal/model"
)

// DataBe uses data.be's official API. Contact data is well-structured, but
// its active flag mirrors the registry, so it only carries a weak signal.
type DataBe struct {
	Token   string
	HTTP    *http.Client
	Limiter *Limiter
	Cache   Cache
}

func NewDataBe(token string, cache Cache) *DataBe {
	return &DataBe{Token: token, HTTP: &http.Client{Timeout: 15 * time.Second}, Limiter: NewLimiter(2), Cache: cache}
}

func (d *DataBe) Name() string     { return "databe" }
func (d *DataBe) Ready() bool      { return d != nil && d.Token != "" }
func (d *DataBe) CostEUR() float64 { return 0.01 }

type dataBeCompany struct {
	Name    string `json:"name"`
	Active  *bool  `json:"active"`
	Phone   string `json:"phone"`
	Email   string `json:"email"`
	Website string `json:"website"`
	Address string `json:"address"`
	URL     string `json:"url"`
}

// Run fetches the company by VAT number. The exact schema depends on the
// data.be plan; unknown shapes yield an unknown signal, never a crash.
func (d *DataBe) Run(ctx context.Context, row *Row) error {
	num := EnterpriseDigits(row.Record.Number)
	if num == "" {
		row.Signals = append(row.Signals, Signal{Source: d.Name(), Existence: Unknown, Note: "No valid enterprise number"})
		return nil
	}
	var c dataBeCompany
	if !d.Cache.Get("databe", num, &c) {
		if err := d.Limiter.Wait(ctx, "databe"); err != nil {
			return err
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.data.be/2.0/companies/BE"+num, nil)
		req.Header.Set("Authorization", "Bearer "+d.Token)
		req.Header.Set("Accept", "application/json")
		res, err := d.HTTP.Do(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			row.Signals = append(row.Signals, Signal{Source: d.Name(), Existence: Unknown, Note: fmt.Sprintf("data.be returned %d", res.StatusCode)})
			return nil
		}
		if json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&c) != nil {
			row.Signals = append(row.Signals, Signal{Source: d.Name(), Existence: Unknown, Note: "data.be returned an unreadable response"})
			return nil
		}
		d.Cache.Put("databe", num, c)
	}
	ref := "https://data.be/en/company/BE" + num
	if c.URL != "" {
		ref = c.URL
	}
	if c.Active != nil && !*c.Active {
		row.Signals = append(row.Signals, Signal{Source: d.Name(), Existence: Closed, Weight: 0.5, Note: "data.be lists the company as inactive", URL: ref})
	} else if c.Name != "" {
		row.Signals = append(row.Signals, Signal{Source: d.Name(), Existence: Alive, Weight: 0.3, Note: "data.be has a current record", URL: ref})
	}
	for field, value := range map[string]string{"phone": c.Phone, "email": c.Email, "website": c.Website, "address": c.Address} {
		if strings.TrimSpace(value) != "" {
			row.Findings = append(row.Findings, Finding{Source: d.Name(), Field: field, Value: value, URL: ref, Note: "data.be company record", Confidence: 0.7})
		}
	}
	return nil
}

// Directory is a best-effort HTML search on a business directory. These
// sites forbid automated access in their terms, so the source is off unless
// ENRICH_DIRECTORIES names it, and it is paced at one request per second.
type Directory struct {
	key     string
	enabled bool
	HTTP    *http.Client
	Limiter *Limiter
	Cache   Cache
}

func NewDirectory(key string, enabled bool, cache Cache) *Directory {
	return &Directory{key: key, enabled: enabled, HTTP: &http.Client{Timeout: 10 * time.Second}, Limiter: NewLimiter(1), Cache: cache}
}

func (d *Directory) Name() string     { return d.key }
func (d *Directory) Ready() bool      { return d != nil && d.enabled }
func (d *Directory) CostEUR() float64 { return 0 }

func (d *Directory) searchURL(name, municipality string) string {
	q := url.QueryEscape(name)
	switch d.key {
	case "goldenpages":
		return "https://www.goldenpages.be/search/" + q + "/" + url.QueryEscape(municipality)
	case "trendstop":
		return "https://trendstop.knack.be/en/search.aspx?q=" + q
	}
	return ""
}

var telHrefRe = regexp.MustCompile(`(?i)href="tel:([+0-9 ./-]+)"`)

func (d *Directory) Run(ctx context.Context, row *Row) error {
	u := d.searchURL(row.Record.Name, row.Record.Municipality)
	if u == "" {
		return nil
	}
	var page fetched
	if !d.Cache.Get(d.key, u, &page) {
		if err := d.Limiter.Wait(ctx, d.key); err != nil {
			return err
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; KBO-Review/0.1)")
		req.Header.Set("Accept-Language", "nl-BE,nl,en")
		res, err := d.HTTP.Do(req)
		if err != nil {
			row.Signals = append(row.Signals, Signal{Source: d.key, Existence: Unknown, Note: "Directory could not be reached"})
			return nil
		}
		body, _ := io.ReadAll(io.LimitReader(res.Body, 512<<10))
		res.Body.Close()
		page = fetched{URL: u, Status: res.StatusCode, Text: extractText(string(body))}
		for _, m := range telHrefRe.FindAllStringSubmatch(string(body), -1) {
			page.Text += " " + m[1]
		}
		if res.StatusCode == 200 {
			d.Cache.Put(d.key, u, page)
		}
	}
	if page.Status != 200 {
		row.Signals = append(row.Signals, Signal{Source: d.key, Existence: Unknown, Note: fmt.Sprintf("Directory returned HTTP %d (blocked or changed)", page.Status), URL: u})
		return nil
	}
	if Containment(row.Record.Name, truncate(page.Text, 4000)) < 0.6 {
		row.Signals = append(row.Signals, Signal{Source: d.key, Existence: Unknown, Note: "No matching directory entry", URL: u})
		return nil
	}
	row.Signals = append(row.Signals, Signal{Source: d.key, Existence: Alive, Weight: 0.3, Note: "Listed in the directory (listings can be stale)", URL: u})
	seen := map[string]bool{}
	for _, p := range phoneRe.FindAllString(page.Text, -1) {
		n := Normalize("phone", p)
		if len(n) < 9 || len(n) > 10 || seen[n] {
			continue
		}
		seen[n] = true
		row.Findings = append(row.Findings, Finding{Source: d.key, Field: "phone", Value: strings.TrimSpace(p), URL: u, Note: "Directory listing", Confidence: 0.5})
		break
	}
	row.Pages = append(row.Pages, Page{URL: u, Title: d.key + " search", Text: truncate(page.Text, 4000)})
	return nil
}

// VKBO re-queries the Flemish open registry mirror. Because the premise is
// that registry data is stale, it never produces an existence signal; it only
// notes when the registry itself changed since the export.
type VKBO struct {
	HTTP    *http.Client
	Limiter *Limiter
	Cache   Cache
}

func NewVKBO(cache Cache) *VKBO {
	return &VKBO{HTTP: &http.Client{Timeout: 15 * time.Second}, Limiter: NewLimiter(4), Cache: cache}
}

func (v *VKBO) Name() string     { return "vkbo" }
func (v *VKBO) Ready() bool      { return true }
func (v *VKBO) CostEUR() float64 { return 0 }

func (v *VKBO) Run(ctx context.Context, row *Row) error {
	num := EnterpriseDigits(row.Record.Number)
	if num == "" {
		return nil
	}
	var doc struct {
		Features []struct {
			Properties map[string]any `json:"properties"`
		} `json:"features"`
	}
	if !v.Cache.Get("vkbo", num, &doc) {
		if err := v.Limiter.Wait(ctx, "vkbo"); err != nil {
			return err
		}
		u := "https://geo.api.vlaanderen.be/VKBO/ogc/features/v1/collections/Vkbo/items?f=application%2Fjson&limit=1&filter-lang=cql2-text&filter=" + url.QueryEscape("Ondernemingsnr='"+num+"'")
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		res, err := v.HTTP.Do(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		if res.StatusCode != 200 || json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&doc) != nil {
			return fmt.Errorf("registry returned %d", res.StatusCode)
		}
		v.Cache.Put("vkbo", num, doc)
	}
	if len(doc.Features) == 0 {
		return nil
	}
	p := doc.Features[0].Properties
	ref := "https://kbopub.economie.fgov.be/kbopub/zoeknummerform.html?nummer=" + num
	for field, key := range map[string]string{"status": "Rechtstoestand", "phone": "Telefoonnummer", "email": "Email"} {
		val := strings.TrimSpace(fmt.Sprint(p[key]))
		if val == "" || val == "<nil>" {
			continue
		}
		cur := currentValue(row.Record, field)
		if field == "status" && Normalize(field, cur) == Normalize(field, val) {
			continue
		}
		row.Findings = append(row.Findings, Finding{Source: v.Name(), Field: field, Value: val, URL: ref, Note: "Current registry value", Confidence: 0.5})
	}
	if stop := strings.TrimSpace(fmt.Sprint(p["Datum_stopzetting"])); stop != "" && !strings.HasPrefix(stop, "1900") && stop != "<nil>" {
		row.Findings = append(row.Findings, Finding{Source: v.Name(), Field: "status", Value: "Registry records a cessation date " + stop[:10], URL: ref, Note: "Registry cessation date", Confidence: 0.6})
	}
	return nil
}

var _ = model.VerdictActive
