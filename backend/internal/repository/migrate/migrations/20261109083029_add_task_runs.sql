-- Create "task_runs" table
CREATE TABLE `task_runs` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `task` text NOT NULL, `trigger` text NOT NULL, `status` text NOT NULL, `started_at` datetime NOT NULL, `finished_at` datetime NULL, `duration` integer NOT NULL DEFAULT (0), `error` text NOT NULL DEFAULT (''), `subject` text NOT NULL DEFAULT (''), `body` text NOT NULL DEFAULT (''));
-- Create index "taskrun_task_started_at" to table: "task_runs"
CREATE INDEX `taskrun_task_started_at` ON `task_runs` (`task`, `started_at`);
-- Create index "taskrun_started_at" to table: "task_runs"
CREATE INDEX `taskrun_started_at` ON `task_runs` (`started_at`);
