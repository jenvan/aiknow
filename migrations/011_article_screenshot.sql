-- articles 表增加截图 URL 列（防源网址失效的归档截图）
--
-- 背景：URL 采集的文章现通过无头浏览器渲染时顺带截取全页截图，上传腾讯云 COS
-- 后把可访问 URL 存此列。源站失效/改版时仍可凭截图回溯原文样貌。
--
-- 仅 source_type='url' 且有截图时才有值，故可空。
-- 不在 001 原地补列：001 已记录为"已应用"，改动不会重跑。

ALTER TABLE articles ADD COLUMN IF NOT EXISTS screenshot_url TEXT;
