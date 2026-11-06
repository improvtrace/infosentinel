-- Create "secrets" table
CREATE TABLE `secrets` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `name` text NOT NULL, `value` text NOT NULL);
-- Create index "secrets_name_key" to table: "secrets"
CREATE UNIQUE INDEX `secrets_name_key` ON `secrets` (`name`);
-- Create "settings" table
CREATE TABLE `settings` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `key` text NOT NULL, `value` text NOT NULL);
-- Create index "settings_key_key" to table: "settings"
CREATE UNIQUE INDEX `settings_key_key` ON `settings` (`key`);
-- Create "targets" table
CREATE TABLE `targets` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `name` text NOT NULL, `type` text NOT NULL DEFAULT ('email'), `to` text NOT NULL, `subject_prefix` text NOT NULL DEFAULT (''));
-- Create index "targets_name_key" to table: "targets"
CREATE UNIQUE INDEX `targets_name_key` ON `targets` (`name`);
-- Create "tasks" table
CREATE TABLE `tasks` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `name` text NOT NULL, `topic` text NOT NULL, `targets` json NOT NULL, `schedules` json NOT NULL);
-- Create index "tasks_name_key" to table: "tasks"
CREATE UNIQUE INDEX `tasks_name_key` ON `tasks` (`name`);
-- Create "topics" table
CREATE TABLE `topics` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `name` text NOT NULL, `prompt` text NOT NULL);
-- Create index "topics_name_key" to table: "topics"
CREATE UNIQUE INDEX `topics_name_key` ON `topics` (`name`);
