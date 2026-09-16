// Package parse turns uploaded CSV, JSON and GeoJSON files into records while
// keeping every original source column attached.
package parse

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"kbo-review/internal/model"
	"kbo-review/internal/validate"
)

// MaxRecords is the per-file record limit.
const MaxRecords = 10000

// File reads records from r according to the lower-cased file extension.
func File(r io.Reader, ext string) ([]model.Record, error) {
	var raw []map[string]any
	var geometries []any
	var err error
	switch ext {
	case ".csv":
		raw, err = readCSV(r)
	case ".json", ".geojson":
		raw, geometries, err = readJSON(r)
	default:
		return nil, errors.New("Choose a .csv, .json or .geojson file")
	}
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, errors.New("The file has no records")
	}
	if len(raw) > MaxRecords {
		return nil, errors.New("Upload up to 10,000 records per file")
	}
	records := make([]model.Record, 0, len(raw))
	recognized := false
	for i, m := range raw {
		row := toRecord(i, m)
		if i < len(geometries) {
			row.Geometry = geometries[i]
		}
		if row.Geometry == nil {
			row.Geometry = geometryOf(m)
		}
		if row.Name != "" || row.Number != "" {
			recognized = true
		}
		records = append(records, row)
	}
	if !recognized {
		return nil, errors.New("No business columns found. Include name or Maatschappelijke_naam, and number or Ondernemingsnr. Download the example for a template.")
	}
	return records, nil
}

func readCSV(r io.Reader) ([]map[string]any, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, errors.New("Could not read this CSV")
	}
	text := strings.TrimPrefix(string(b), "\ufeff")
	reader := csv.NewReader(strings.NewReader(text))
	line := strings.SplitN(text, "\n", 2)[0]
	if strings.Count(line, ";") > strings.Count(line, ",") {
		reader.Comma = ';'
	}
	if strings.Count(line, "\t") > strings.Count(line, string(reader.Comma)) {
		reader.Comma = '\t'
	}
	headers, err := reader.Read()
	if err != nil {
		return nil, errors.New("The CSV needs a header row")
	}
	seen := map[string]bool{}
	for i, h := range headers {
		h = strings.TrimSpace(h)
		if h == "" || seen[h] {
			return nil, errors.New("CSV column names must be non-empty and unique")
		}
		headers[i] = h
		seen[h] = true
	}
	var raw []map[string]any
	for {
		values, err := reader.Read()
		if err == io.EOF {
			return raw, nil
		}
		if err != nil {
			return nil, fmt.Errorf("CSV row %d is malformed: check its column count and quotes", len(raw)+2)
		}
		m := map[string]any{}
		for i, h := range headers {
			m[h] = values[i]
		}
		raw = append(raw, m)
		if len(raw) > MaxRecords {
			return nil, errors.New("Upload up to 10,000 records per file")
		}
	}
}

func readJSON(r io.Reader) (raw []map[string]any, geometries []any, err error) {
	decoder := json.NewDecoder(r)
	decoder.UseNumber()
	var data any
	if decoder.Decode(&data) != nil {
		return nil, nil, errors.New("This file is not valid JSON")
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return nil, nil, errors.New("Unexpected content after the JSON document")
	}
	var rows []any
	switch v := data.(type) {
	case []any:
		rows = v
	case map[string]any:
		if v["type"] == "FeatureCollection" {
			features, ok := v["features"].([]any)
			if !ok {
				return nil, nil, errors.New("GeoJSON must contain a features array")
			}
			for _, item := range features {
				f, ok := item.(map[string]any)
				if !ok || f["type"] != "Feature" {
					return nil, nil, errors.New("GeoJSON contains an invalid feature")
				}
				p, ok := f["properties"].(map[string]any)
				if !ok {
					return nil, nil, errors.New("Each feature needs properties")
				}
				raw = append(raw, p)
				geometries = append(geometries, f["geometry"])
			}
			return raw, geometries, nil
		}
		rows, _ = v["records"].([]any)
		if rows == nil {
			rows, _ = v["data"].([]any)
		}
		if rows == nil {
			return nil, nil, errors.New("JSON must be an array, a records object, or a GeoJSON FeatureCollection")
		}
	default:
		return nil, nil, errors.New("JSON must contain records")
	}
	for _, item := range rows {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, nil, errors.New("Each record must be a JSON object")
		}
		raw = append(raw, m)
	}
	return raw, nil, nil
}

// field returns the first non-empty value among the case-insensitive keys.
func field(m map[string]any, keys ...string) string {
	for _, key := range keys {
		for k, v := range m {
			if strings.EqualFold(k, key) && v != nil {
				s := strings.TrimSpace(fmt.Sprint(v))
				if s != "" {
					return s
				}
			}
		}
	}
	return ""
}

func toRecord(i int, m map[string]any) model.Record {
	source := m
	if original, ok := m["source"].(map[string]any); ok {
		source = original
	}
	row := model.Record{
		ID:           strconv.Itoa(i + 1),
		Number:       field(m, "number", "Ondernemingsnr", "EnterpriseNumber", "enterprise_number", "establishmentNumber"),
		Enterprise:   field(m, "enterprise", "Ondernemingsnr_maatsch_zetel"),
		Name:         field(m, "name", "Maatschappelijke_naam", "Commerciele_naam", "Denomination", "business_name"),
		Address:      field(m, "address", "formattedAddress"),
		Municipality: field(m, "municipality", "KBO_Gemeente", "city"),
		Status:       field(m, "status", "Rechtstoestand"),
		Phone:        field(m, "phone", "telephone", "phone_number"),
		Email:        field(m, "email", "email_address"),
		Website:      field(m, "website", "url"),
		Notes:        field(m, "notes"),
		Source:       source,
		Issues:       []string{},
	}
	row.Reviewed, _ = m["reviewed"].(bool)
	if row.Address == "" {
		street := strings.TrimSpace(field(m, "KBO_Straat", "street") + " " + field(m, "KBO_Huisnr", "houseNumber"))
		if box := field(m, "KBO_Busnr", "box"); box != "" {
			street += " bus " + box
		}
		locality := strings.TrimSpace(field(m, "KBO_Postcode", "postcode", "postalCode") + " " + row.Municipality)
		row.Address = strings.Trim(strings.Join([]string{street, locality}, ", "), ", ")
	}
	n := validate.Digits(row.Number)
	row.Kind = "enterprise"
	if len(n) == 10 && n[0] >= '2' {
		row.Kind = "establishment"
	}
	if row.Enterprise == "" && row.Kind == "enterprise" {
		row.Enterprise = row.Number
	}
	if g, ok := m["geometry"]; ok {
		row.Geometry = g
	}
	return row
}

// geometryOf builds a GeoJSON point from longitude/latitude columns, or nil.
// Missing coordinates are never invented.
func geometryOf(m map[string]any) any {
	lon, e1 := strconv.ParseFloat(field(m, "longitude", "lon", "lng"), 64)
	lat, e2 := strconv.ParseFloat(field(m, "latitude", "lat"), 64)
	if e1 == nil && e2 == nil && lon >= -180 && lon <= 180 && lat >= -90 && lat <= 90 {
		return map[string]any{"type": "Point", "coordinates": []float64{lon, lat}}
	}
	return nil
}
