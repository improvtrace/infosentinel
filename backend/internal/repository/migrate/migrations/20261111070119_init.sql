-- Create "secrets" table
CREATE TABLE `secrets` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `name` text NOT NULL, `value` text NOT NULL);
-- Create index "secrets_name_key" to table: "secrets"
CREATE UNIQUE INDEX `secrets_name_key` ON `secrets` (`name`);
-- Create "settings" table
CREATE TABLE `settings` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `key` text NOT NULL, `value` text NOT NULL);
-- Create index "settings_key_key" to table: "settings"
CREATE UNIQUE INDEX `settings_key_key` ON `settings` (`key`);
-- Create "subscriptions" table
CREATE TABLE `subscriptions` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `name` text NOT NULL, `topics` json NOT NULL, `synthesis` text NOT NULL, `cron` text NOT NULL, `targets` json NOT NULL, `space` text NOT NULL DEFAULT (''));
-- Create index "subscriptions_name_key" to table: "subscriptions"
CREATE UNIQUE INDEX `subscriptions_name_key` ON `subscriptions` (`name`);
-- Create "subscription_runs" table
CREATE TABLE `subscription_runs` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `subscription` text NOT NULL, `trigger` text NOT NULL, `status` text NOT NULL, `started_at` datetime NOT NULL, `finished_at` datetime NULL, `duration` integer NOT NULL DEFAULT (0), `error` text NOT NULL DEFAULT (''), `subject` text NOT NULL DEFAULT (''), `body` text NOT NULL DEFAULT (''), `artifacts` json NULL);
-- Create index "subscriptionrun_subscription_started_at" to table: "subscription_runs"
CREATE INDEX `subscriptionrun_subscription_started_at` ON `subscription_runs` (`subscription`, `started_at`);
-- Create index "subscriptionrun_started_at" to table: "subscription_runs"
CREATE INDEX `subscriptionrun_started_at` ON `subscription_runs` (`started_at`);
-- Create "targets" table
CREATE TABLE `targets` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `name` text NOT NULL, `type` text NOT NULL DEFAULT ('email'), `to` text NOT NULL, `subject_prefix` text NOT NULL DEFAULT (''));
-- Create index "targets_name_key" to table: "targets"
CREATE UNIQUE INDEX `targets_name_key` ON `targets` (`name`);
-- Create "topics" table
CREATE TABLE `topics` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `name` text NOT NULL, `prompt` text NOT NULL);
-- Create index "topics_name_key" to table: "topics"
CREATE UNIQUE INDEX `topics_name_key` ON `topics` (`name`);

-- ============================================================
-- 内置默认配置(种子数据):推送目标 / 主题 / 订阅 / 全局设置。
-- 内置默认值以本文件为唯一来源:调整默认值直接改这里,随后执行
-- `go run ./backend/cmd/migrate-diff -hash` 重算 atlas.sum。
-- ============================================================

-- 推送目标:内置信箱占位(收件地址在 Web 界面修改)
INSERT INTO `targets` (`name`, `type`, `to`, `subject_prefix`) VALUES ('mailbox', 'email', 'me@example.com', '[InfoSentinel]');

-- 内置主题(一段 prompt 即采集指令,信息源由提示词直接描述)
INSERT INTO `topics` (`name`, `prompt`) VALUES
('world-situation', '聚焦全球局势的综合情报观察,重点:
- 局势:地缘政治与国际冲突动态、大国博弈、外交动向、制裁与缓和信号
- 金融:宏观经济数据、汇率与大宗商品异动、金融市场重大事件
- 贸易:进出口政策与数据、关税、产业链与供应链变化
- 人才劳务:国际人才流动、劳务与移民政策变化
- 文化:电影、影视与其他大众文化领域的重大发布或事件
信息源:优先用 fetch_rss 拉取权威通讯社与媒体 RSS(如 Reuters、AP、BBC、NHK、联合早报、澎湃、界面新闻等,地址可自行检索确认),用 search_web 以「关键词 + 平台名」检索 X(推特)、Facebook、小红书、微博等平台,以及各国外交部、商务部、白宫、欧盟委员会、IMF、WTO 等官方部门的公开表态与公告;高价值条目用 fetch_webpage 深读原文。
报告用中文撰写:按「局势 / 金融 / 贸易 / 人才劳务 / 文化」分节,每条 = 一句话摘要 + 原文链接;涉及多方说法时注明消息来源与立场。'),
('ai-development', '聚焦人工智能领域的最新进展,重点:
- 大模型:LLM 与多模态新模型发布、能力评测、价格调整、开源与开源权重动态
- Agent 与 AGI:智能体框架与产品、工具调用与 MCP 生态、通向 AGI 的研究进展
- 自动驾驶:Robotaxi 与辅助驾驶的技术进展、监管与商业化动态
- 公司与平台:OpenAI、Anthropic、Google、Meta、xAI 及国内大厂与创业公司的产品、融资、人事与战略动向
- 治理:各国 AI 监管与行业安全事件的重要变化
信息源:用 fetch_rss 拉取 https://hnrss.org/frontpage?points=100 等科技 RSS,用 fetch_hackernews、fetch_github_trending 补充社区与开源动态,用 search_web 检索各 AI 公司官方博客与权威媒体报道;高价值条目用 fetch_webpage 深读原文。
报告用中文撰写:按要点分组,每条附一句话摘要与原文链接,重要发布可展开 2-3 句。'),
('security', '聚焦网络安全行业的威胁与防御动态,重点:
- 漏洞:新披露的高危漏洞与 CVE、在野利用与 PoC、受影响范围与缓解措施
- 补丁:操作系统、浏览器、中间件、网络设备等厂商安全补丁与升级通告
- 威胁:病毒木马、勒索软件、APT 组织活动、攻击事件与数据泄露
- 防护:堡垒机、WAF、防火墙、EDR 等安全产品重大版本与最佳实践;破解、越狱等对抗动态
信息源:用 fetch_rss 拉取 CISA、NVD、FreeBuf、奇安信、绿盟等安全机构与媒体的 RSS,用 search_web 检索「CVE 编号 / 产品名 + 漏洞」与厂商安全公告;高价值条目用 fetch_webpage 深读通告原文。
报告用中文撰写:按「漏洞 / 补丁 / 威胁 / 防护」分节,每条 = 一句话摘要 + 原文链接;高危漏洞注明 CVSS 评分与处置建议。'),
('policy', '收集并解读各国(以中国为主)的政策变更,重点:
- 中国:全国人大、国务院及各部委、央行、证监会、网信办等官方平台发布的法律法规、部门规章、征求意见稿与配套解读;地方重大政策试点
- 海外:美欧等主要经济体在贸易、科技、金融、移民领域影响重大的政策与监管变化
- 解读要求:每条政策说明「变了什么、生效时间、影响谁、如何应对」
信息源:用 fetch_rss 拉取中国政府网、部委官网与权威媒体(新华社、人民日报、经济日报等)的 RSS,用 search_web 检索「政策名 + 发布/解读」;高价值条目用 fetch_webpage 深读官方原文,链接必须来自官方或权威渠道。
报告用中文撰写:按「中国政策 / 海外政策」分节,每条 = 政策名称 + 要点解读(2-3 句)+ 原文链接。');

-- 内置订阅:关联全部内置主题,每天 8 点执行(合成提示词与 repository.DefaultSynthesis 保持一致)
INSERT INTO `subscriptions` (`name`, `topics`, `synthesis`, `cron`, `targets`, `space`) VALUES
('情报日报', json_array('world-situation','ai-development','security','policy'), '你是资讯编辑。请把多份主题报告合并为一份最终报告:去除重复与冗余,按主题分节或按重要性排序,保留每条的原文链接与一句话摘要,开头用一段导语概括本期整体看点。第一行只写总标题纯文本;从第二行起用中文 HTML 片段撰写正文(<section>/<h2>/<p>/<ul>/<table> 等内容级标签,不含 <html>/<body>/<style>/<script>),链接用 <a> 标签原样保留。', '0 8 * * *', json_array('mailbox'), '默认空间');

-- 全局设置:llm / mailer / server / storage(Web 界面保存后整段覆盖)
INSERT INTO `settings` (`key`, `value`) VALUES
('llm', '{"base_url":"https://api.deepseek.com","model":"deepseek-chat","temperature":0.3,"timeout_sec":300,"max_step":25}'),
('mailer', '{"type":"站内信","smtp":{"port":465}}'),
('server', '{"enabled":true,"addr":"127.0.0.1:8080"}'),
('storage', '{"spaces":[{"name":"默认空间","type":"local","path":"","period_days":1,"max_dirs":30}]}');
