-- One-time requeue of matches that failed for reasons that were ours, not
-- Valve's: deadlock-api rate limits (429, which used to burn attempts) and
-- "has metadata already" (salts are refused when deadlock-api stores the
-- file itself; the syncer now downloads it from there first).
UPDATE matches
SET meta_status = 'pending', meta_attempts = 0, meta_next_try = now()
WHERE meta_status IN ('failed', 'pending')
  AND (meta_error LIKE '%429 Too Many Requests%' OR meta_error LIKE '%has metadata already%');
