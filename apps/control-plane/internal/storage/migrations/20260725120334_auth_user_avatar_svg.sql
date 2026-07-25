-- auth_users 存渲染后的头像 SVG(P1-D Step 2 · 2b 预生成头像)。
--
-- dicebear 头像是种子的确定性函数,但后端是 Go、跑不了 dicebear。故"预生成"= 把前端/
-- 构建脚本渲染好的 data-URI 存这里,展示时直接读、不再在浏览器算 dicebear,让 @dicebear
-- 彻底移出入口包。可空:未回填/未自愈的用户为 NULL,前端懒加载 dicebear 兜底渲染。

ALTER TABLE auth_users ADD COLUMN avatar_svg TEXT;

COMMENT ON COLUMN auth_users.avatar_svg IS
    '预渲染的头像 data-URI(dicebear 确定性产物);NULL 表示未生成,前端懒加载 dicebear 兜底并可自愈写回。';
