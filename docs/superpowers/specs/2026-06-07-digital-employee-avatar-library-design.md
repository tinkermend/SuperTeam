# 数字员工头像库设计

## 1. 背景

数字员工创建时需要选择头像。头像不是执行能力，也不是 Provider 状态；它是数字员工业务身份的一部分，用于 Console 识别、列表扫描和详情页辨认。

本期只支持平台内置头像库，不开放用户上传。头像图片可以先随 Web 静态资源交付，但头像库的可选项、授权信息和选择校验必须归 Control Plane 管理，避免前端硬编码 URL 成为业务事实来源。

## 2. 目标

- 提供 20 张内置 2D 写实头像，男性 10 张、女性 10 张，均为虚构亚洲面孔，年龄 18-40 岁，职业气质贴合计算机行业工程师。
- 创建数字员工时用户只能从平台头像库选择一个头像。
- Control Plane 暴露头像资产列表，并在创建数字员工时校验 `avatar_asset_id`。
- 数字员工列表和详情页展示已选头像；历史数据没有头像时按员工 ID 稳定回退到内置头像，不在每次刷新时随机变化。
- 图片文件不进入数据库。数据库或元数据只保存头像资产 ID 和必要快照。

## 3. 非目标

- 不开放用户上传头像。
- 不实现租户级自定义头像库。
- 不实现头像裁剪 UI。
- 不引入 3D/VRM 头像渲染。
- 不把图片二进制存入 PostgreSQL。

## 4. 架构决策

### 4.1 图片文件

第一阶段将图片放在：

```text
apps/web/public/images/digital-employee-avatars/
```

该目录只是内置静态资源包。未来产品化部署时，同一批资产可迁移到 S3 兼容存储和 CDN，前端仍只消费 Control Plane 返回的 `image_url` 与 `thumbnail_url`。

### 4.2 头像资产事实

Control Plane 定义头像资产注册表，暴露：

```text
GET /api/v1/digital-employee-avatar-assets
```

返回字段包含：

- `id`
- `label`
- `gender`
- `age_range`
- `style`
- `image_url`
- `thumbnail_url`
- `source`
- `license`
- `status`

本期注册表可用代码内置清单承载，不急于创建独立表；但 API 与领域模型按资产注册表设计，后续可平滑替换为 `employee_avatar_assets` 或通用 `media_assets` 表。

### 4.3 数字员工选择结果

创建请求新增：

```json
{
  "avatar_asset_id": "engineer-m-01"
}
```

Control Plane 校验该 ID 存在且状态为 `active`。通过后将稳定快照写入 `digital_employees.metadata`：

```json
{
  "avatar_asset_id": "engineer-m-01",
  "avatar": {
    "id": "engineer-m-01",
    "provider": "superteam-generated",
    "image_url": "/images/digital-employee-avatars/engineer-m-01.webp",
    "thumbnail_url": "/images/digital-employee-avatars/engineer-m-01-256.webp",
    "source": "ai_generated_internal_pack",
    "license": "internal_product_asset"
  }
}
```

元数据快照用于历史展示和审计；资产 ID 用于未来重新解析最新 CDN 地址、下架状态或授权信息。

## 5. Web 交互

创建向导在“身份”步骤展示头像库网格。用户必须选择一个头像；默认可按当前草稿名称或员工类型稳定选中一个头像，但提交前仍显示明确选择状态。

列表页与详情页显示头像图片。没有持久化头像的历史数字员工，前端按员工 ID 稳定选择一个内置头像作为回退，避免刷新后变化。

## 6. 验证

- Web 创建测试覆盖头像选择和 `avatar_asset_id` 提交。
- Web 列表测试覆盖 overview 头像展示。
- Web 详情测试覆盖 metadata 头像展示。
- Control Plane 路由测试覆盖头像资产列表授权与创建请求透传。
- 服务层测试覆盖未知 `avatar_asset_id` 被拒绝，以及有效头像写入 metadata 快照。
