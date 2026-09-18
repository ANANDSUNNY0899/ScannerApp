-- Add client_updated_at and deleted_at (tombstones) for LWW reconciliation

ALTER TABLE folders
    ADD COLUMN IF NOT EXISTS client_updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP WITH TIME ZONE NULL;

ALTER TABLE documents
    ADD COLUMN IF NOT EXISTS client_updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP WITH TIME ZONE NULL;

ALTER TABLE pages
    ADD COLUMN IF NOT EXISTS client_updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP WITH TIME ZONE NULL;

CREATE INDEX IF NOT EXISTS idx_folders_client_updated_at ON folders(client_updated_at);
CREATE INDEX IF NOT EXISTS idx_documents_client_updated_at ON documents(client_updated_at);
CREATE INDEX IF NOT EXISTS idx_pages_client_updated_at ON pages(client_updated_at);
