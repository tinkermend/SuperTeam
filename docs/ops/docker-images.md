# SuperTeam 生产镜像：Control Plane + Web

| 字段 | 值 |
|---|---|
| 日期 | 2026-08-11 |
| 状态 | Dockerfile 已入库；本机可用 `scripts/build-images.sh` 构建 |
| 范围 | **control-plane**、**web**（Runtime / connector / Temporal 不在本批） |

## 1. 镜像一览

| 镜像 | Dockerfile | 默认端口 | 说明 |
|---|---|---|---|
| `superteam-control-plane` | `apps/control-plane/Dockerfile` | `8080` | 静态 Go 二进制；配置文件挂载；`/migrations` 附带 SQL 供 atlas |
| `superteam-web` | `apps/web/Dockerfile` | `80` | Vite 静态资源 + nginx SPA |

构建上下文均为 **仓库根目录**（需要 `pnpm-lock`、`contracts/` 等）。

## 2. 构建

### 2.1 一键脚本

```bash
# 默认 tag=local；本机 docker 若是 podman 别名亦可
./scripts/build-images.sh

# 自定义
SUPERTEAM_IMAGE_TAG=dev \
VITE_CONTROL_PLANE_URL=http://127.0.0.1:8080 \
./scripts/build-images.sh
```

环境变量：

| 变量 | 默认 | 含义 |
|---|---|---|
| `SUPERTEAM_CONTAINER_CLI` | 自动探测 docker/podman | 容器 CLI |
| `SUPERTEAM_IMAGE_TAG` | `local` | 镜像 tag |
| `SUPERTEAM_CP_IMAGE` | `superteam-control-plane:$TAG` | 覆盖 CP 镜像名 |
| `SUPERTEAM_WEB_IMAGE` | `superteam-web:$TAG` | 覆盖 Web 镜像名 |
| `VITE_CONTROL_PLANE_URL` | `http://127.0.0.1:8080` | **构建期**写入前端的 API 基址 |

### 2.2 手动命令

```bash
# Control Plane
docker build -f apps/control-plane/Dockerfile -t superteam-control-plane:local .

# Web（务必传 API 地址，否则浏览器默认猜 hostname:8080）
docker build -f apps/web/Dockerfile \
  --build-arg VITE_CONTROL_PLANE_URL=http://127.0.0.1:8080 \
  -t superteam-web:local .
```

本机若 `docker` 为 shell alias 而脚本/CI 无 alias，请用 `podman` 或设置 `SUPERTEAM_CONTAINER_CLI`.

## 3. 运行

### 3.1 Control Plane

配置文件必填（与本地 dev 相同契约，见 `apps/control-plane/config/config.example.yaml`）。  
镜像**不会**在启动时自动 migrate——请先对 Postgres 执行 atlas（SQL 在镜像 `/migrations`）。

```bash
# 示例：使用本机已验证过的 config（含本地 RustFS objectStore）
docker run --rm --name superteam-cp \
  -p 8080:8080 \
  -v "$PWD/apps/control-plane/config/config.yaml:/etc/superteam/config.yaml:ro" \
  superteam-control-plane:local

curl -fsS http://127.0.0.1:8080/health
```

注意：若 Postgres/Redis/RustFS 在宿主机端口上，容器内访问 `127.0.0.1` 是容器自己——需改成 `host.containers.internal`（Podman）或 `host.docker.internal`，或共用网络。开发机验证时常见做法是 **config 里写宿主机可达地址**，或使用 `podman run --network host`（Linux）。

### 3.2 Web

```bash
docker run --rm --name superteam-web \
  -p 3100:80 \
  superteam-web:local

# 浏览器打开 http://127.0.0.1:3100
# 页面会请求构建时写入的 VITE_CONTROL_PLANE_URL
```

### 3.3 与本地 RustFS / 已有 dev 栈

对象存储仍按 `docs/ops/local-rustfs.md`：宿主机 rustfs `:9000`，CP 配置 `objectStore.endpoint` 对 **CP 进程网络命名空间** 可达即可。

## 4. 设计取舍

| 点 | 选择 | 原因 |
|---|---|---|
| CP 基础镜像 | `alpine` + `ca-certificates` + `wget` | 需 TLS 出站与简易 healthcheck |
| Web 服务 | nginx 静态 | 无 Node 运行时；SPA `try_files` |
| 迁移 | 拷进镜像但不自动 apply | 与现网 atlas 工作流一致，避免容器反复 migrate 竞态 |
| 前端 API 地址 | 构建参数 `VITE_CONTROL_PLANE_URL` | Vite 编译期注入；同源反代属后续 P0-7 |
| Runtime | 未做镜像 | 执行机依赖 Provider CLI；后续二进制/systemd 优先 |

## 5. 验收清单

- [ ] `./scripts/build-images.sh` 退出码 0  
- [ ] `docker run ... superteam-control-plane` + 有效 config → `GET /health` 200  
- [ ] `docker run -p 3100:80 superteam-web` → 浏览器打开控制台壳  
- [ ] 构建说明与本文件一致  

## 6. 后续（非本批）

- `deploy/compose` 串联 PG/Redis/RustFS/CP/Web  
- Runtime 镜像或安装包  
- nginx 同源反代样例（Console + API 同域）  
- CI 构建并推送镜像仓库  
