package catalog

import (
	"context"
	"net/http"
	"time"
)

type mapPoint struct {
	Postcode   string  `json:"postcode"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	Quality    int16   `json:"quality"`
	Terminated string  `json:"terminated"`
}

// Maps resolve displayed full postcodes only. Partial source codes are never
// upgraded with raw/hidden values; approximate nearby centres are labelled separately.
func (a *API) mapLocations(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query()["postcode"]
	if len(raw) < 1 || len(raw) > 100 {
		fail(w, 400, "Provide 1–100 full displayed postcodes")
		return
	}
	codes := []string{}
	seen := map[string]bool{}
	for _, input := range raw {
		code := Postcode(input)
		if len(input) > 32 || !fullPostcodeSearch.MatchString(code) {
			fail(w, 400, "Maps accept full displayed postcodes only")
			return
		}
		if !seen[code] {
			codes = append(codes, code)
			seen[code] = true
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	select {
	case a.geographySlots <- struct{}{}:
		defer func() { <-a.geographySlots }()
	default:
		fail(w, 503, "Postcode lookup is busy; retry shortly")
		return
	}
	reference, err := activeGeography(ctx, a.DB)
	if err != nil {
		fail(w, 503, "Maps need an imported ONSPD reference")
		return
	}
	rows, err := a.DB.Query(ctx, `SELECT postcode,latitude,longitude,quality,terminated FROM postcode_locations WHERE release=$1 AND postcode=ANY($2::text[]) AND position IS NOT NULL AND quality NOT IN (6,9) ORDER BY postcode`, reference.Release, codes)
	if err != nil {
		fail(w, 503, "Postcode locations unavailable")
		return
	}
	defer rows.Close()
	points := []mapPoint{}
	for rows.Next() {
		var p mapPoint
		if err = rows.Scan(&p.Postcode, &p.Latitude, &p.Longitude, &p.Quality, &p.Terminated); err != nil {
			fail(w, 503, "Postcode locations unavailable")
			return
		}
		points = append(points, p)
	}
	if rows.Err() != nil {
		fail(w, 503, "Postcode locations unavailable or timed out")
		return
	}
	respond(w, 200, struct {
		Reference GeographyRelease `json:"reference"`
		Points    []mapPoint       `json:"points"`
		Unlocated int              `json:"unlocated"`
		Notice    string           `json:"notice"`
	}{reference, points, len(codes) - len(points), geographyNotice})
}
