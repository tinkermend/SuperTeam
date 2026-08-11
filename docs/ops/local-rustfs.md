# 本地 RustFS（开发用对象存储）

| 字段 | 值 |
|---|---|
| 日期 | 2026-08-11 |
| 状态 | 本机已用 Docker/Podman 拉起，健康检查通过 |
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

改完后重启 Control Plane，再跑一次技能上传或任务工件上传做冒烟。

### 2.2 与现有 MinIO dev 的冲突

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

## 4. 建桶与冒烟（mc）

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
| CP 连不上 | 确认 `forcePathStyle: true`、endpoint 无尾斜杠、密钥与桶名一致 |
| mc 报 `sh is not a recognized command` | 使用 `--entrypoint /bin/sh` |
| 需要持久化数据 | 再改为命名 volume 或宿主机挂载，并保证 UID `10001` 可写（见官方 README） |

## 7. 相关文档

- 私有化差距总览：`docs/ops/private-deploy-ops-maintainability.md`（P0-5 对象存储）  
- CP 配置样例：`apps/control-plane/config/config.example.yaml`  
- 生产桶 CORS 债：`TODO.md`、`docs/superpowers/specs/2026-07-19-execution-output-attachments-followups.md`  
