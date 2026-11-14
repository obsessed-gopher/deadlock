-- Steam GC bots: their request budgets (token bucket + rate-limit pause per
-- kind of request) and which process runs which bot. Both used to live in
-- memory: a restart forgot the budget, and two processes would log the same
-- Steam account on and kick each other off.
CREATE TABLE steam_bot_budgets (
    bot          text             NOT NULL,
    kind         text             NOT NULL,
    tokens       double precision NOT NULL,
    refilled_at  timestamptz      NOT NULL,
    paused_until timestamptz      NOT NULL,
    strikes      integer          NOT NULL,
    updated_at   timestamptz      NOT NULL DEFAULT now(),
    PRIMARY KEY (bot, kind)
);

CREATE TABLE steam_bot_leases (
    bot        text PRIMARY KEY,
    owner      text        NOT NULL,
    expires_at timestamptz NOT NULL
);
