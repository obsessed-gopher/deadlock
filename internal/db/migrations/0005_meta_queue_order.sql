-- The metadata queue is taken newest first (ClaimPendingMeta: ORDER BY
-- match_id DESC over pending matches). With millions of matches a partial
-- index in that order keeps the claim a short index scan.
CREATE INDEX matches_meta_pending_order ON matches (match_id DESC) WHERE meta_status = 'pending';
