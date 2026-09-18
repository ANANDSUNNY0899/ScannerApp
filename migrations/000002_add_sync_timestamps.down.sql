DROP INDEX IF EXISTS idx_pages_client_updated_at;
DROP INDEX IF EXISTS idx_documents_client_updated_at;
DROP INDEX IF EXISTS idx_folders_client_updated_at;

ALTER TABLE pages
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS client_updated_at;

ALTER TABLE documents
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS client_updated_at;

ALTER TABLE folders
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS client_updated_at;
