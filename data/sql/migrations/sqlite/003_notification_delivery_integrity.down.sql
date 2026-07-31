-- This compatibility repair is intentionally forward-only on SQLite. SQLite
-- cannot remove the added columns without another lossy table rebuild, and
-- restoring UNIQUE(code) would reject valid channel/locale variants.
SELECT 1;
