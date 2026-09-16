package enrich

import "kbo-review/internal/model"

// This step deliberately cannot add findings, signals, pages, costs or sources.
// The fixture must never reach the judge or automatic acceptance rules.
func companywebDemo(rec model.Record, tr *tracer) {
	i := tr.start("companyweb_demo")
	number := rec.Enterprise
	if number == "" && rec.Kind == "enterprise" {
		number = rec.Number
	}
	if digits(number) != "0725606718" {
		tr.end(i, model.StepSkipped, "Demo only: no prepared Companyweb snapshot for this enterprise. No live request made.", 0, 0)
		return
	}
	tr.steps[i].Demo = &model.CompanywebDemo{
		URL:        "https://www.companyweb.be/en/0725606718/coja",
		CapturedAt: "2026-09-16",
		Enterprise: "0725606718",
		Name:       "Coja",
		Address:    "Alfons Servaislei 53, 2900 Schoten",
		Status:     "Active",
	}
	tr.end(i, model.StepDone, "Demo snapshot only. No live Companyweb request; no effect on confidence, verdict or saved business fields. Companyweb also uses KBO data.", 0, 0)
}
