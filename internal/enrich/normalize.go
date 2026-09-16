package enrich

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"kbo-review/internal/model"
)

var (
	emailRe   = regexp.MustCompile(`(?i)\b[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}\b`)
	phoneRe   = regexp.MustCompile(`(?:\+32|0032|\b0)[\s./-]?(?:\d[\s./-]?){7,9}\d`)
	vatRe     = regexp.MustCompile(`(?i)\b(?:BE)?\s?0?(\d{4})[.\s]?(\d{3})[.\s]?(\d{3})\b`)
	spaceRe   = regexp.MustCompile(`\s+`)
	protoRe   = regexp.MustCompile(`(?i)^https?://`)
	trailSlRe = regexp.MustCompile(`/+$`)
)

// Normalize returns a comparable form of a field value.
func Normalize(field, v string) string {
	v = strings.TrimSpace(v)
	switch field {
	case "phone":
		d := digits(v)
		if strings.HasPrefix(d, "0032") {
			d = "0" + d[4:]
		} else if strings.HasPrefix(d, "32") && len(d) >= 10 {
			d = "0" + d[2:]
		}
		return d
	case "email":
		return strings.ToLower(v)
	case "website":
		v = strings.ToLower(protoRe.ReplaceAllString(v, ""))
		v = strings.TrimPrefix(v, "www.")
		return trailSlRe.ReplaceAllString(v, "")
	default:
		return strings.ToLower(spaceRe.ReplaceAllString(v, " "))
	}
}

func digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// genericMailboxes are role addresses that identify a mailbox, not a business
// contact; they are never suggested as the business email.
var genericMailboxes = map[string]bool{
	"info": true, "contact": true, "hello": true, "hallo": true, "office": true, "admin": true,
	"administratie": true, "sales": true, "verkoop": true, "support": true, "help": true, "noreply": true,
	"no-reply": true, "donotreply": true, "webmaster": true, "postmaster": true, "mail": true, "email": true,
	"marketing": true, "press": true, "pers": true, "jobs": true, "careers": true, "privacy": true, "gdpr": true,
	"onthaal": true, "secretariaat": true, "reception": true, "welcome": true, "welkom": true, "team": true,
}

// GenericEmail reports whether the local part is a role mailbox.
func GenericEmail(e string) bool {
	e = strings.ToLower(strings.TrimSpace(e))
	at := strings.Index(e, "@")
	if at <= 0 {
		return true
	}
	local := e[:at]
	if genericMailboxes[local] {
		return true
	}
	for prefix := range genericMailboxes {
		if strings.HasPrefix(local, prefix+".") || strings.HasPrefix(local, prefix+"-") || strings.HasPrefix(local, prefix+"_") {
			return true
		}
	}
	return false
}

// TradeName returns the commercial name from the source columns when the
// export carries one. Sole traders are registered under a person's name but
// known to the world by their trade name.
func TradeName(r model.Record) string {
	for _, k := range []string{"Commerciele_naam", "commercial_name", "trade_name", "tradeName", "Afgekorte_naam"} {
		if v, ok := r.Source[k]; ok {
			if s := strings.TrimSpace(fmt.Sprint(v)); s != "" && s != "<nil>" {
				return s
			}
		}
	}
	return ""
}

// LegalForm returns the registered legal form when present.
func LegalForm(r model.Record) string {
	if v, ok := r.Source["Rechtsvorm"]; ok {
		return strings.TrimSpace(fmt.Sprint(v))
	}
	return ""
}

// SearchName is the name most likely to appear on listings.
func SearchName(r model.Record) string {
	if t := TradeName(r); t != "" {
		return t
	}
	return r.Name
}

// BestSimilarity compares a listing name against both registered and trade name.
func BestSimilarity(r model.Record, listing string) float64 {
	sim := Similarity(r.Name, listing)
	if t := TradeName(r); t != "" {
		if s := Similarity(t, listing); s > sim {
			sim = s
		}
	}
	return sim
}

// EnterpriseDigits returns the ten digits of a KBO number, or "".
func EnterpriseDigits(s string) string {
	d := digits(s)
	if len(d) == 10 {
		return d
	}
	return ""
}

// Similarity is token Jaccard on lower-cased words, ignoring legal-form noise.
func Similarity(a, b string) float64 {
	ta, tb := tokens(a), tokens(b)
	if len(ta) == 0 || len(tb) == 0 {
		return 0
	}
	inter := 0
	for t := range ta {
		if tb[t] {
			inter++
		}
	}
	return float64(inter) / float64(len(ta)+len(tb)-inter)
}

// Containment is the share of a's meaningful tokens that appear in b. Use it
// when b is a long text (a web page) and Jaccard would be diluted.
func Containment(a, b string) float64 {
	ta, tb := tokens(a), tokens(b)
	if len(ta) == 0 {
		return 0
	}
	hit := 0
	for t := range ta {
		if tb[t] {
			hit++
		}
	}
	return float64(hit) / float64(len(ta))
}

var noise = map[string]bool{"bv": true, "bvba": true, "nv": true, "vzw": true, "cv": true, "cvba": true, "comm": true, "v": true, "sa": true, "srl": true, "sprl": true, "asbl": true, "de": true, "het": true, "the": true, "en": true, "&": true, "and": true}

func tokens(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if !noise[w] && len(w) > 1 {
			out[w] = true
		}
	}
	return out
}
