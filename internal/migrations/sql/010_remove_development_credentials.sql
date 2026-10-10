-- Remove credentials created by the original migration 007. Fresh databases do
-- not create these rows; this cleanup exists only for safe upgrades.
DELETE FROM principals
WHERE name IN ('local-admin', 'local-operator', 'local-viewer');
