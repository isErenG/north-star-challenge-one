package validate

import (
	"strings"
	"testing"

	"kbo-review/internal/model"
)

func TestNumberValidation(t *testing.T) {
	if !Number("0123.456.749") {
		t.Error("valid leading-zero enterprise number rejected")
	}
	if Number("0123456748") || Number("123") {
		t.Error("invalid identifier accepted")
	}
}

func TestDuplicateFlagSurvivesRevalidation(t *testing.T) {
	rows := []model.Record{{Number: "0123456749", Name: "A"}, {Number: "0123456749", Name: "B"}}
	MarkDuplicates(rows)
	Record(&rows[0])
	if !strings.Contains(strings.Join(rows[0].Issues, ","), "Duplicate identifier") {
		t.Fatal("editing cleared duplicate flag")
	}
	if !strings.Contains(strings.Join(rows[0].Issues, ","), "No coordinates") {
		t.Fatal("missing coordinates not flagged")
	}
}
