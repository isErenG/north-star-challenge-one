package dedupe

import (
	"strings"
	"testing"

	"kbo-review/internal/model"
)

func hasIssue(r model.Record, issue string) bool {
	return strings.Contains(strings.Join(r.Issues, ","), issue)
}

func TestClusterMergesNearDuplicate(t *testing.T) {
	records := []model.Record{
		{ID: "a", Number: "0123456749", Name: "Voorbeeld Atelier BVBA", Address: "Voorbeeldstraat 12", Municipality: "Schoten", Email: "contact@example.be"},
		{ID: "b", Number: "", Name: "Voorbeeld Attelier BVBA", Address: "Voorbeeldstraat 12", Municipality: "Schoten"},
	}
	Cluster(records)

	if records[1].MergedInto != records[0].ID {
		t.Fatalf("expected record b merged into a, got MergedInto=%q", records[1].MergedInto)
	}
	if records[0].MergedInto != "" {
		t.Fatal("canonical record should not itself be marked merged")
	}
	if records[0].DuplicateGroup != records[0].ID || records[1].DuplicateGroup != records[0].ID {
		t.Fatal("both records should share the canonical's ID as duplicate group")
	}
	if len(records[0].MergedFrom) != 1 || records[0].MergedFrom[0] != "Voorbeeld Attelier BVBA" {
		t.Fatalf("expected canonical MergedFrom to list record b's name (it had no number), got %v", records[0].MergedFrom)
	}
	if !hasIssue(records[0], possibleDuplicate) || !hasIssue(records[1], possibleDuplicate) {
		t.Fatal("both records should carry the possible-duplicate issue")
	}
}

func TestClusterKeepsDistinctBranchesSeparate(t *testing.T) {
	records := []model.Record{
		{ID: "a", Name: "Bakkerij De Zon", Address: "Kerkstraat 3", Municipality: "Antwerpen"},
		{ID: "b", Name: "Bakkerij De Zon", Address: "Meirplein 90", Municipality: "Antwerpen"},
	}
	Cluster(records)

	if records[0].MergedInto != "" || records[1].MergedInto != "" {
		t.Fatal("records at different addresses must never be merged")
	}
	if !hasIssue(records[0], recurringName) || !hasIssue(records[1], recurringName) {
		t.Fatal("same name at a different address should be flagged as recurring, not silently ignored")
	}
}

func TestClusterLeavesUnrelatedRecordsAlone(t *testing.T) {
	records := []model.Record{
		{ID: "a", Name: "Slagerij Janssens", Address: "Marktplein 1", Municipality: "Gent"},
		{ID: "b", Name: "Apotheek Verhoeven", Address: "Dorpsstraat 22", Municipality: "Leuven"},
	}
	Cluster(records)

	for _, r := range records {
		if r.MergedInto != "" || len(r.Issues) != 0 {
			t.Fatalf("unrelated record %q should be untouched, got issues=%v mergedInto=%q", r.ID, r.Issues, r.MergedInto)
		}
	}
}

func TestNormalizeFoldsAccentsPunctuationAndCase(t *testing.T) {
	if normalize("Café  De Kroon!") != normalize("cafe de kroon") {
		t.Fatalf("normalize should fold accents, punctuation and case: got %q vs %q", normalize("Café  De Kroon!"), normalize("cafe de kroon"))
	}
}
