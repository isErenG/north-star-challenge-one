// Package validate runs data-quality checks on records. These are local
// checks, not live KBO registration verification.
package validate

import (
	"net/mail"
	"strconv"
	"strings"

	"kbo-review/internal/model"
)

const duplicateIssue = "Duplicate identifier"

// Digits keeps only the decimal digits of s.
func Digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Number reports whether s is a ten-digit Belgian enterprise or establishment
// number with a valid modulo-97 checksum.
func Number(s string) bool {
	n := Digits(s)
	if len(n) != 10 {
		return false
	}
	a, _ := strconv.Atoi(n[:8])
	b, _ := strconv.Atoi(n[8:])
	return 97-a%97 == b
}

// Email reports whether s is a single plain address without display name.
func Email(s string) bool {
	a, e := mail.ParseAddress(s)
	return e == nil && a.Address == s && len(s) <= 254
}

// Record recomputes r.Issues. A previously set duplicate flag is preserved
// because duplicates are only known across the whole job.
func Record(r *model.Record) {
	duplicate := false
	for _, issue := range r.Issues {
		if issue == duplicateIssue {
			duplicate = true
		}
	}
	r.Issues = []string{}
	if !Number(r.Number) {
		r.Issues = append(r.Issues, "Check identifier")
	}
	if r.Name == "" {
		r.Issues = append(r.Issues, "Missing business name")
	}
	if r.Address == "" {
		r.Issues = append(r.Issues, "Missing address")
	}
	if r.Email != "" && !Email(r.Email) {
		r.Issues = append(r.Issues, "Check email format")
	}
	if r.Geometry == nil {
		r.Issues = append(r.Issues, "No coordinates")
	}
	status := strings.ToLower(r.Status)
	if strings.Contains(status, "stop") || strings.Contains(status, "closed") || strings.Contains(status, "ontbind") || strings.Contains(status, "cess") {
		r.Issues = append(r.Issues, "Check registered status")
	}
	if duplicate {
		r.Issues = append(r.Issues, duplicateIssue)
	}
}

// MarkDuplicates flags every record whose identifier appears more than once.
// Duplicates are flagged and retained, never merged.
func MarkDuplicates(records []model.Record) {
	counts := map[string]int{}
	for _, row := range records {
		if row.Number != "" {
			counts[row.Number]++
		}
	}
	for i := range records {
		if counts[records[i].Number] > 1 {
			records[i].Issues = append(records[i].Issues, duplicateIssue)
		}
	}
}
