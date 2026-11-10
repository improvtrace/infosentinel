-- Create "subscriptions" table
CREATE TABLE `subscriptions` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `topic` text NOT NULL, `cron` text NOT NULL, `targets` json NOT NULL);
-- Create index "subscriptions_topic_key" to table: "subscriptions"
CREATE UNIQUE INDEX `subscriptions_topic_key` ON `subscriptions` (`topic`);
-- Create "subscription_runs" table
CREATE TABLE `subscription_runs` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `topic` text NOT NULL, `trigger` text NOT NULL, `status` text NOT NULL, `started_at` datetime NOT NULL, `finished_at` datetime NULL, `duration` integer NOT NULL DEFAULT (0), `error` text NOT NULL DEFAULT (''), `subject` text NOT NULL DEFAULT (''), `body` text NOT NULL DEFAULT (''));
-- Create index "subscriptionrun_topic_started_at" to table: "subscription_runs"
CREATE INDEX `subscriptionrun_topic_started_at` ON `subscription_runs` (`topic`, `started_at`);
-- Create index "subscriptionrun_started_at" to table: "subscription_runs"
CREATE INDEX `subscriptionrun_started_at` ON `subscription_runs` (`started_at`);
-- Drop "tasks" / "task_runs" tables(订阅模型替代任务模型;历史数据不保留)。
-- 说明:ent 的 diff 只生成建表类变更,不会自动生成 DROP,删除实体需在此手动补齐。
DROP TABLE IF EXISTS `tasks`;
DROP TABLE IF EXISTS `task_runs`;
