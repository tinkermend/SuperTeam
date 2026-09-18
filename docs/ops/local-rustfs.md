# 本地 RustFS（开发用对象存储）

| 字段 | 值 |
|---|---|
| 日期 | 2026-08-11 |
| 状态 | 本机已用 Docker/Podman 拉起；**Control Plane `objectStore` 已切到本机 RustFS**（见 `config.yaml`，gitignore） |
| 上游文档 | [RustFS README_ZH](https://github.com/rustfs/rustfs/blob/main/README_ZH.md) |
| Compose | 仓库根目录 `docker-compose.rustfs.dev.yml` |

## 1. 为什么要本地 RustFS

Control Plane 通过 `objectStore` 走 **S3 协议**（工件 / skill 包 / raw log 等 presign）。  
本地起 RustFS 后，可把原先指向云存储（如火山 TOS）的 endpoint 改到本机，验证私有化对象存储路径，无需改业务代码。

Runtime **不配置**对象存储密钥；上传下载仍由 CP 签发 presigned URL。

## 2. 本机当前实例（关键信息）

| 项 | 值 |
|---|---|
| 容器名 | `superteam-rustfs-dev` |
| 镜像 | `rustfs/rustfs:latest` |
| S3 API | `http://127.0.0.1:9000` |
| 健康检查 | `GET http://127.0.0.1:9000/health` → 200 |
| Web 控制台 | `http://127.0.0.1:9001` |
| 控制台健康 | `GET http://127.0.0.1:9001/rustfs/console/health` → 200 |
| Access Key | `rustfsadmin` |
| Secret Key | `rustfsadmin` |
| 推荐开发桶 | `superteam-artifacts`（本机已 `mb` 创建） |
| 数据 / 日志挂载 | **无**（开发约定：不挂宿主机目录，能用即可；`docker rm -v` 后数据丢） |
| 与官方差异 | 官方示例常挂载 `./data` `./logs` 并 `chown 10001:10001`；开发栈刻意省略 |

> 默认密钥与官方一致，**仅本机开发**。生产必须更换。

### 2.1 Control Plane 配置片段

写入 `apps/control-plane/config/config.yaml` 的 `objectStore`（或等价 env），示例：

```yaml
objectStore:
  # 本机 RustFS（S3 兼容）。本地开发通常需要 path-style。
  endpoint: "http://127.0.0.1:9000"
  region: "us-east-1"
  bucket: "superteam-artifacts"
  accessKeyId: "rustfsadmin"
  secretAccessKey: "rustfsadmin"
  forcePathStyle: true
```

改完后重启 Control Plane。`GET /health` 会 HeadBucket；桶不存在返回 503，`object_store.status=error`。起服前建议：

```bash
./scripts/ops/init-object-store.sh --check
```

**`dev-services.sh` 已把这一步内置成起服闸门**：`start` / `restart control-plane` 会先用与 CP 同一份配置
（`SUPERTEAM_DEV_CONTROL_PLANE_CONFIG`，yaml + `S3_*` 覆盖）跑一次 `object-store-init --check --skip-cors`，
endpoint/凭据/桶任一不可用就**中止启动并打印修法**，不再干等 30s 超时后留一个 `/health` 恒 503 的半死进程。

```bash
# 只判桶不判 CORS：CORS 归云厂商控制台或本文件 §2.3 管，与 CP 健康无关
# 存储确实不可用、但本轮验证不涉及对象存储时：
SUPERTEAM_DEV_SKIP_OBJECT_STORE_CHECK=1 ./scripts/dev-services.sh start control-plane
# 换用别的检查实现（测试桩/预编译二进制）：
SUPERTEAM_DEV_OBJECT_STORE_INIT_CMD=./bin/object-store-init ./scripts/dev-services.sh start control-plane
```

### 2.2 新环境初始化（建桶 + CORS）

Control Plane **不会**在启动时 CreateBucket。系统设置页也没有桶名——桶名只在 `objectStore.bucket`（或 `S3_BUCKET` 覆盖）里。
前缀 `skills/` `artifacts/` `runs/` 不用预建，第一次 PUT 才会出现。

读 **与 CP 进程同一份配置**（yaml + `S3_*` overlay）：

```bash
# 幂等：桶不存在则创建，再写入控制台预览 CORS。不设匿名下载。
./scripts/ops/init-object-store.sh

# 只检查，不写
./scripts/ops/init-object-store.sh --check

# 云厂商控制台管 CORS、或密钥没有 PutBucketCors 时
./scripts/ops/init-object-store.sh --skip-cors
```

配置路径默认 `apps/control-plane/config/config.yaml`，可用 `SUPERTEAM_DEV_CONTROL_PLANE_CONFIG` 覆盖。
若环境里有 `S3_ENDPOINT` / `S3_BUCKET` 等，命令会告警（值覆盖 yaml，与 CP 启动行为一致），**不会打印密钥**。

生产密钥未必有 `s3:CreateBucket`：缺权限时由对象存储管理员建好桶，再跑 `--check`。
不要照搬 `docker-compose.dev.yml` 的 `minio-init`（它会 `anonymous set download`）。

### 2.3 桶 CORS（仅改 CORS）

工件预览走 presigned GET，浏览器对 `127.0.0.1:9000` 跨域请求，桶须放行 Web origin。  
`init-object-store.sh` 已写入同一套规则；只改 CORS、桶已存在时仍可用：

```bash
# 写入（幂等）
go run ./apps/control-plane/cmd/bucket-cors --config apps/control-plane/config/config.yaml

# 只读检查
go run ./apps/control-plane/cmd/bucket-cors --config apps/control-plane/config/config.yaml --check

# 自定义来源
BUCKET_CORS_ORIGINS='http://127.0.0.1:3100,http://localhost:3100' \
  go run ./apps/control-plane/cmd/bucket-cors --config apps/control-plane/config/config.yaml
```

默认 origins：`http://127.0.0.1:3100`、`http://localhost:3100`（与 Vite `DEV_SERVER_PORT` 一致）。  
规则：`GET`/`HEAD`，`AllowedHeaders=*`，`ExposeHeaders=ETag,Content-Type,Content-Length`，`MaxAge=3600`。

### 2.4 真实链路冒烟

**A. 技能上传（CP 直写对象存储）**

```bash
# 前置：rustfs up、CP 已指向本地 objectStore 并重启
node scripts/ops/smoke-local-object-store.mjs
```

期望输出含 `SMOKE_OK`，且 `archive_object_ref` 形如  
`s3://superteam-artifacts/skills/<tenant>/...zip`。

**B. Runtime 真任务工件（CP presign → Runtime 直传桶）**

```bash
# 会临时把 runtime claude binary 指到 fake provider，跑完自动还原
node scripts/ops/smoke-runtime-artifact-rustfs.mjs
```

期望：

- 任务 `completed`
- `project_artifact_refs` 出现 `declared` / `execution_output` / `execution_transcript`，`object_ref` 形如 `artifacts/<tenant>/sha256/...`
- `mc cat` 可从本地桶读回内容（脚本用 `podman`/`docker` 拉 `minio/mc`）

假 Provider：`scripts/e2e/fake-providers/claude-success-with-artifacts.sh`  
（写 `deliverables/` + 按 `.scratch/e2e/fake-produces.json` / `fake-acceptance.json` 填 result_contract）。

### 2.5 与现有 MinIO dev 的冲突

`docker-compose.dev.yml` 里的 **minio 同样默认占用 9000/9001**。  
同一时刻只应启动 **RustFS 或 MinIO 之一**。本文件描述的是 RustFS 路径。

## 3. 启停命令

```bash
# 推荐：Compose（无宿主机 data/logs 挂载）
docker compose -f docker-compose.rustfs.dev.yml up -d
docker compose -f docker-compose.rustfs.dev.yml ps
docker compose -f docker-compose.rustfs.dev.yml logs -f rustfs
docker compose -f docker-compose.rustfs.dev.yml down

# 等价：直接 run（当前本机即用此形态拉起）
docker run -d \
  --name superteam-rustfs-dev \
  -p 9000:9000 -p 9001:9001 \
  -e RUSTFS_ACCESS_KEY=rustfsadmin \
  -e RUSTFS_SECRET_KEY=rustfsadmin \
  -e RUSTFS_ADDRESS=0.0.0.0:9000 \
  -e RUSTFS_CONSOLE_ADDRESS=0.0.0.0:9001 \
  -e RUSTFS_CONSOLE_ENABLE=true \
  -e RUSTFS_CONSOLE_CORS_ALLOWED_ORIGINS='*' \
  -e RUSTFS_VOLUMES=/data \
  -e RUSTFS_OBS_LOGGER_LEVEL=info \
  -e RUSTFS_UNSAFE_BYPASS_DISK_CHECK=true \
  --restart unless-stopped \
  rustfs/rustfs:latest
```

本机 Docker 客户端若是 **Podman 别名**（`docker: aliased to podman`），上述命令同样适用。

## 4. 建桶与冒烟（mc，备用）

优先用 §2.2 的 `init-object-store.sh`（读 CP 配置，幂等建桶 + CORS）。
下面是不经过 Go 命令、直接用 `mc` 的备用路径。

镜像 `minio/mc` 的 entrypoint 是 `mc`，需要覆盖 entrypoint：

```bash
docker run --rm --entrypoint /bin/sh minio/mc:latest -c '
mc alias set rustfs http://host.containers.internal:9000 rustfsadmin rustfsadmin
mc mb -p rustfs/superteam-artifacts
echo hello > /tmp/t.txt
mc cp /tmp/t.txt rustfs/superteam-artifacts/smoke/hello.txt
mc cat rustfs/superteam-artifacts/smoke/hello.txt
'
```

在 Linux 宿主机且容器使用 host 网络时，也可用 `http://127.0.0.1:9000`。

本机 2026-08-11 实测：

- `GET /health` → 200  
- 桶 `superteam-artifacts` 已创建  
- put/get `smoke/hello.txt` 成功  

## 5. 浏览器与 presign 注意

- Console（Web）若通过 **302 跳到对象 URL** 预览，桶可能还需 CORS；产品侧优先走 artifact `format=json` 两步取 URL。  
- Presign URL 里的 host 必须是 **浏览器 / Runtime 能访问** 的地址；本地统一 `127.0.0.1:9000` 最省事。  
- 生产桶 **不要** anonymous download（dev MinIO 曾用过，不可照搬）。

## 6. 常用排障

| 现象 | 处理 |
|---|---|
| 端口占用 | `lsof -nP -iTCP:9000 -sTCP:LISTEN`；停掉 minio 或其它占用者 |
| 容器反复退出 | `docker logs superteam-rustfs-dev`；确认未误挂只读目录 |
| 机器重启后 `dev-services.sh start` 报「对象存储未就绪」 | 容器没被拉回：`podman start superteam-rustfs-dev`（compose 路径为 `docker compose -f docker-compose.rustfs.dev.yml up -d`），再 `./scripts/ops/init-object-store.sh --check` |
| CP 连不上 | 确认 `forcePathStyle: true`、endpoint 无尾斜杠、密钥与桶名一致 |
| HeadBucket 404 / 技能上传 NoSuchBucket | 桶未建：`./scripts/ops/init-object-store.sh`（CP 启动不会自动建桶） |
| 连的是云桶而 yaml 写的是本机 | 检查环境里是否还有 `S3_ENDPOINT`/`S3_BUCKET`；init 命令会告警覆盖 |
| mc 报 `sh is not a recognized command` | 使用 `--entrypoint /bin/sh` |
| 需要持久化数据 | 再改为命名 volume 或宿主机挂载，并保证 UID `10001` 可写（见官方 README） |

## 7. 相关文档

- 私有化差距总览：`docs/ops/private-deploy-ops-maintainability.md`（P0-5 对象存储；P0-4 整站 bootstrap 仍缺）
- CP 配置样例：`apps/control-plane/config/config.example.yaml`
- 桶 CORS 规则：`docs/superpowers/specs/2026-07-19-execution-output-attachments-followups.md` §2
