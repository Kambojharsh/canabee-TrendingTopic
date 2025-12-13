-- Fix UTF-8 encoding for emoji support in chats and session_summaries tables
ALTER TABLE chats MODIFY COLUMN message TEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
ALTER TABLE session_summaries MODIFY COLUMN summary TEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci; 