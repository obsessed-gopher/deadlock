-- Average rank badge of each team's players (tier * 10 + subtier, 0 = not
-- reported). Valve writes no per-player rank into metadata, so the lobby
-- level is the only rank signal we have.
ALTER TABLE matches
    ADD COLUMN badge_team0    smallint NOT NULL DEFAULT 0,
    ADD COLUMN badge_team1    smallint NOT NULL DEFAULT 0,
    -- Version of the tables derived from meta_raw (db.DeriveVersion). Rows
    -- below the current one are re-derived from meta_raw on start.
    ADD COLUMN derive_version integer  NOT NULL DEFAULT 0;
CREATE INDEX matches_stale_derive ON matches (match_id) WHERE meta_status = 'ok';
