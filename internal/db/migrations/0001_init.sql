-- Players we track (sync bookkeeping).
CREATE TABLE players (
    account_id     bigint PRIMARY KEY,
    sync_state     text        NOT NULL DEFAULT 'never', -- never | running | idle | error
    sync_error     text        NOT NULL DEFAULT '',
    last_synced_at timestamptz,
    created_at     timestamptz NOT NULL DEFAULT now()
);

-- One row per match; header comes from match history, the rest from metadata.
CREATE TABLE matches (
    match_id        bigint PRIMARY KEY,
    start_time      timestamptz NOT NULL,
    duration_s      integer     NOT NULL,
    game_mode       integer     NOT NULL,
    match_mode      integer     NOT NULL,
    winning_team    smallint    NOT NULL,

    meta_status     text        NOT NULL DEFAULT 'pending', -- pending | ok | expired | failed
    meta_error      text        NOT NULL DEFAULT '',
    meta_attempts   integer     NOT NULL DEFAULT 0,
    meta_next_try   timestamptz NOT NULL DEFAULT now(),
    meta_fetched_at timestamptz,
    meta_raw        bytea,      -- .meta file exactly as downloaded (zstd); lets us re-derive tables
    meta_version    integer
);
CREATE INDEX matches_meta_queue ON matches (meta_next_try) WHERE meta_status = 'pending';

-- A tracked player's row in a match, from match history (exists even without metadata).
CREATE TABLE player_matches (
    account_id       bigint   NOT NULL,
    match_id         bigint   NOT NULL REFERENCES matches ON DELETE CASCADE,
    hero_id          integer  NOT NULL,
    team             smallint NOT NULL,
    won              boolean  NOT NULL,
    kills            integer  NOT NULL,
    deaths           integer  NOT NULL,
    assists          integer  NOT NULL,
    net_worth        integer  NOT NULL,
    last_hits        integer  NOT NULL,
    denies           integer  NOT NULL,
    hero_level       integer  NOT NULL,
    abandoned_time_s integer  NOT NULL,
    PRIMARY KEY (account_id, match_id)
);
CREATE INDEX player_matches_match ON player_matches (match_id);
CREATE INDEX player_matches_hero ON player_matches (account_id, hero_id);

-- Everything below is derived from metadata and covers all 12 players.

CREATE TABLE match_players (
    match_id       bigint   NOT NULL REFERENCES matches ON DELETE CASCADE,
    slot           smallint NOT NULL,
    account_id     bigint   NOT NULL,
    team           smallint NOT NULL,
    hero_id        integer  NOT NULL,
    lane           smallint NOT NULL,
    kills          integer  NOT NULL,
    deaths         integer  NOT NULL,
    assists        integer  NOT NULL,
    net_worth      integer  NOT NULL,
    last_hits      integer  NOT NULL,
    denies         integer  NOT NULL,
    level          integer  NOT NULL,
    player_damage  integer  NOT NULL,
    player_healing integer  NOT NULL,
    abandon_time_s integer  NOT NULL,
    PRIMARY KEY (match_id, slot)
);
CREATE INDEX match_players_account ON match_players (account_id);
CREATE INDEX match_players_hero ON match_players (hero_id);

CREATE TABLE player_stats (
    match_id            bigint   NOT NULL REFERENCES matches ON DELETE CASCADE,
    slot                smallint NOT NULL,
    t                   integer  NOT NULL,
    net_worth           integer  NOT NULL,
    level               integer  NOT NULL,
    kills               integer  NOT NULL,
    deaths              integer  NOT NULL,
    assists             integer  NOT NULL,
    creep_kills         integer  NOT NULL,
    neutral_kills       integer  NOT NULL,
    denies              integer  NOT NULL,
    player_damage       integer  NOT NULL,
    player_damage_taken integer  NOT NULL,
    player_healing      integer  NOT NULL,
    self_healing        integer  NOT NULL,
    gold_player         integer  NOT NULL,
    gold_lane_creep     integer  NOT NULL,
    gold_neutral_creep  integer  NOT NULL,
    gold_boss           integer  NOT NULL,
    gold_treasure       integer  NOT NULL,
    gold_death_loss     integer  NOT NULL,
    shots_hit           integer  NOT NULL,
    shots_missed        integer  NOT NULL,
    PRIMARY KEY (match_id, slot, t)
);

CREATE TABLE item_events (
    match_id   bigint   NOT NULL REFERENCES matches ON DELETE CASCADE,
    slot       smallint NOT NULL,
    seq        smallint NOT NULL,
    t          integer  NOT NULL,
    item_id    bigint   NOT NULL, -- uint32 hash
    sold_t     integer  NOT NULL,
    upgrade_id bigint   NOT NULL,
    flags      integer  NOT NULL,
    PRIMARY KEY (match_id, slot, seq)
);
CREATE INDEX item_events_item ON item_events (item_id);

CREATE TABLE deaths (
    match_id    bigint   NOT NULL REFERENCES matches ON DELETE CASCADE,
    slot        smallint NOT NULL,
    seq         smallint NOT NULL,
    t           integer  NOT NULL,
    killer_slot smallint NOT NULL,
    x real NOT NULL, y real NOT NULL, z real NOT NULL,
    killer_x real NOT NULL, killer_y real NOT NULL, killer_z real NOT NULL,
    respawn_s   integer  NOT NULL,
    ttk_s       real     NOT NULL,
    PRIMARY KEY (match_id, slot, seq)
);
