-- Steam names and avatars of everyone we have seen (tracked players and all
-- players of stored matches), from deadlock-api. Refreshed in the background
-- (syncer.RunProfiles); a row with found = false remembers that the API had
-- no profile, so it is not asked again before the next refresh.
CREATE TABLE steam_profiles (
    account_id    bigint PRIMARY KEY,
    found         boolean     NOT NULL,
    persona_name  text        NOT NULL DEFAULT '',
    profile_url   text        NOT NULL DEFAULT '',
    avatar        text        NOT NULL DEFAULT '', -- 32 px
    avatar_medium text        NOT NULL DEFAULT '', -- 64 px
    avatar_full   text        NOT NULL DEFAULT '', -- 184 px
    country       text        NOT NULL DEFAULT '',
    fetched_at    timestamptz NOT NULL
);
CREATE INDEX steam_profiles_fetched ON steam_profiles (fetched_at);

-- Steam friends of tracked players only: marks real friends among allies.
CREATE TABLE steam_friends (
    account_id   bigint      NOT NULL,
    friend_id    bigint      NOT NULL,
    friend_since timestamptz,
    PRIMARY KEY (account_id, friend_id)
);
