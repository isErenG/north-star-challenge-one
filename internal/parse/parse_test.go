package parse

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestSchotenSchema(t *testing.T) {
	// Synthetic values, same field names as the supplied export. No private dataset fixture.
	input := "Ondernemingsnr,Maatschappelijke_naam,Ondernemingsnr_maatsch_zetel,KBO_Straat,KBO_Huisnr,KBO_Busnr,KBO_Postcode,KBO_Gemeente,longitude,latitude,Extra\n2285533695,Example,0719273014,Example Street,42,B,2900,Schoten,4.50,51.25,keep me\n"
	rows, err := File(strings.NewReader(input), ".csv")
	if err != nil {
		t.Fatal(err)
	}
	r := rows[0]
	if r.Number != "2285533695" || r.Enterprise != "0719273014" || r.Kind != "establishment" {
		t.Fatalf("identifier mapping: %+v", r)
	}
	if r.Address != "Example Street 42 bus B, 2900 Schoten" || r.Source["Extra"] != "keep me" || r.Geometry == nil {
		t.Fatalf("lost source data: %+v", r)
	}
}

func TestFormats(t *testing.T) {
	cases := []struct{ ext, input string }{
		{".csv", "\ufeffnumber;name;address\n0123456749;\"One; Two\";Belgium\n"},
		{".json", `[{"number":"0123456749","name":"One; Two","address":"Belgium","custom":{"a":1}}]`},
		{".geojson", `{"type":"FeatureCollection","features":[{"type":"Feature","properties":{"number":"0123456749","name":"One; Two"},"geometry":{"type":"Point","coordinates":[4.5,51.2]}}]}`},
	}
	for _, c := range cases {
		t.Run(c.ext, func(t *testing.T) {
			rows, err := File(strings.NewReader(c.input), c.ext)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 || rows[0].Number != "0123456749" || rows[0].Name != "One; Two" {
				t.Fatalf("bad parsing: %+v", rows)
			}
			if c.ext == ".geojson" && rows[0].Geometry == nil {
				t.Fatal("geometry lost")
			}
		})
	}
}

func TestRejectsMalformedInput(t *testing.T) {
	for _, c := range []struct{ ext, input string }{{".csv", "name,name\nx,y"}, {".csv", "name,number\nx,y,z"}, {".json", "{}"}, {".json", "[]"}, {".json", `[{"name":"a"}] {}`}, {".geojson", `{"type":"FeatureCollection","features":[null]}`}, {".csv", "unknown\nx"}, {".txt", "x"}} {
		if _, err := File(strings.NewReader(c.input), c.ext); err == nil {
			t.Errorf("accepted %s", c.input)
		}
	}
}

func TestRoundTripSourceAndGeometry(t *testing.T) {
	original := `[{"number":"0123456749","name":"Original","custom":"untouched","longitude":4.5,"latitude":51.2}]`
	rows, err := File(strings.NewReader(original), ".json")
	if err != nil {
		t.Fatal(err)
	}
	rows[0].Name = "Edited"
	rows[0].Reviewed = true
	encoded, _ := json.Marshal(map[string]any{"records": rows})
	again, err := File(bytes.NewReader(encoded), ".json")
	if err != nil {
		t.Fatal(err)
	}
	if again[0].Name != "Edited" || again[0].Source["name"] != "Original" || again[0].Source["custom"] != "untouched" || !again[0].Reviewed || again[0].Geometry == nil {
		t.Fatalf("round trip lost data: %+v", again[0])
	}
}
