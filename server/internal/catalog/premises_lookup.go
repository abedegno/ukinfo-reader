package catalog

import "context"

// Legacy readers/catalogues keep the full postcode scan until every selected
// edition has an atomically published complete projection and SELECT permission.
func (a *API) premisesIndexReady(ctx context.Context, datasets []premisesDataset) (bool, error) {
	var available bool
	err := a.DB.QueryRow(ctx, `SELECT COALESCE(has_table_privilege(current_user,to_regclass('premises_keys'),'SELECT') AND has_table_privilege(current_user,to_regclass('premises_indexes'),'SELECT'),false)`).Scan(&available)
	if err != nil || !available {
		return false, err
	}
	ids := make([]int16, 0, len(datasets))
	for _, d := range datasets {
		ids = append(ids, d.ID)
	}
	var ready int
	err = a.DB.QueryRow(ctx, `SELECT count(*) FROM premises_indexes i JOIN datasets d ON d.id=i.dataset_id AND d.entries=i.entries WHERE i.dataset_id=ANY($1::smallint[]) AND i.version=1`, ids).Scan(&ready)
	return ready == len(ids), err
}
