DROP INDEX IF EXISTS idx_receipt_orders_folder_id;
ALTER TABLE receipt_orders DROP COLUMN IF EXISTS folder_id;