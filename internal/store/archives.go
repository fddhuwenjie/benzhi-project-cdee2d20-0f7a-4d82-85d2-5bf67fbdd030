package store

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/mission"
)

func (s *Store) ListArchived(ctx context.Context, f mission.ArchiveFilter) ([]mission.ArchiveCandidate, error) {
	site := strings.ToLower(strings.Join(strings.Fields(f.CaveSite), " "))
	rows, err := s.db.QueryContext(ctx, `SELECT id,cave_site,archived_at FROM missions WHERE status=? AND (?='' OR cave_site_key=?) AND (?='' OR archived_at>=?) AND (?='' OR archived_at<=?) ORDER BY archived_at,id`, mission.StatusArchived, site, site, timeValue(f.From), timeValue(f.From), timeValue(f.To), timeValue(f.To))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []mission.ArchiveCandidate
	for rows.Next() {
		var id, cave string
		var archived sql.NullString
		if err := rows.Scan(&id, &cave, &archived); err != nil {
			return nil, err
		}
		if !archived.Valid {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, archived.String)
		if err != nil {
			return nil, err
		}
		out = append(out, mission.ArchiveCandidate{ID: id, CaveSite: cave, ArchivedAt: at})
	}
	return out, rows.Err()
}
