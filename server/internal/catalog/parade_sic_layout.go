package catalog

import "context"

// Only measured SIC families opt into the second layout. Discovery is skipped
// entirely when disabled/ineligible; the original main index remains sufficient.
func paradeSICLayoutEligible(q query) bool {
	return q.Kind == "business" && q.SIC != "" && q.Postcode == "" && q.Near == "" && q.Group == "" && (q.Sort == "" || q.Sort == "name") && paradeEligible(q)
}

func (a *API) paradeSICLayout(ctx context.Context, q query, editions []paradeEdition) ([]paradeEdition, bool, error) {
	if !a.ExperimentalSICUnpartitioned || !paradeSICLayoutEligible(q) || len(editions) == 0 {
		return editions, false, nil
	}
	var exists bool
	if err := a.ParadeDB.QueryRow(ctx, "SELECT to_regclass('ukinfo_search.sic_editions') IS NOT NULL").Scan(&exists); err != nil {
		return nil, false, err
	}
	if !exists {
		return editions, false, nil
	}
	selected := make([]paradeEdition, len(editions))
	for i, d := range editions {
		alias := "ukinfo_search." + d.table() + "_sic"
		var ready, present bool
		err := a.ParadeDB.QueryRow(ctx, `SELECT EXISTS(SELECT FROM ukinfo_search.sic_editions WHERE dataset_id=$1 AND generation=$2 AND entries=$3 AND version=$4 AND ready AND last_id=entries),to_regclass($5) IS NOT NULL`, d.ID, d.generation(), d.Entries, paradeVersion, alias).Scan(&ready, &present)
		if err != nil {
			return nil, false, err
		}
		if !ready || !present {
			return editions, false, nil
		}
		selected[i] = d
		selected[i].searchTable = d.table() + "_sic"
	}
	return selected, true, nil
}
