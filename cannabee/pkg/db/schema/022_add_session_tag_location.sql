-- Add tag and location columns to sessions table
ALTER TABLE sessions
ADD COLUMN tag VARCHAR(100) DEFAULT NULL,
ADD COLUMN latitude DECIMAL(10, 8) DEFAULT NULL,
ADD COLUMN longitude DECIMAL(10, 8) DEFAULT NULL;

-- Add tag and location columns to guest_sessions table
ALTER TABLE guest_sessions
ADD COLUMN tag VARCHAR(100) DEFAULT NULL,
ADD COLUMN latitude DECIMAL(10, 8) DEFAULT NULL,
ADD COLUMN longitude DECIMAL(10, 8) DEFAULT NULL;
