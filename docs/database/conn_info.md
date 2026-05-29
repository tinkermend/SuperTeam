# SuperTeam Local Database Connection Info

## PostgreSQL

- Host: `115.190.247.9`
- Port: `35432`
- Application database: `superteam`
- Application schema: `superteam`
- Application user: `superteam`
- Application password: `83ab1f233b790e580ba5dae3a26998d78095f780d7067b32`
- JDBC-style URL: `postgresql://superteam:83ab1f233b790e580ba5dae3a26998d78095f780d7067b32@115.190.247.9:35432/superteam?sslmode=disable`
- DATABASE_URL: `postgres://superteam:83ab1f233b790e580ba5dae3a26998d78095f780d7067b32@115.190.247.9:35432/superteam?sslmode=disable`

## Redis

- Docker container: `superteam-redis`
- Image: `redis:7`
- Host: `115.190.247.9`
- Port: `6379`
- Password: `d862d604a7d5adb0d2f800e72ca68a38aa2cf8edb5b0b0fa`
- REDIS_URL: `redis://:d862d604a7d5adb0d2f800e72ca68a38aa2cf8edb5b0b0fa@115.190.247.9:6379/0`

## S3-Compatible Object Storage

- S3_ENDPOINT: local MinIO or S3-compatible endpoint
- S3_REGION: `us-east-1`
- S3_BUCKET: `superteam-artifacts`
- S3_ACCESS_KEY_ID: object-store access key
- S3_SECRET_ACCESS_KEY: object-store secret key
- S3_FORCE_PATH_STYLE: `true` for MinIO/localstack-style local development

## Verification

```bash
redis-cli -h 115.190.247.9 -p 6379 -a 'd862d604a7d5adb0d2f800e72ca68a38aa2cf8edb5b0b0fa' --no-auth-warning ping
PGPASSWORD='83ab1f233b790e580ba5dae3a26998d78095f780d7067b32' psql -h 115.190.247.9 -p 35432 -U superteam -d superteam -c 'select current_user, current_database(), current_schema();'
```

## Control Plane Local Config

Copy `apps/control-plane/config/config.example.yaml` to `apps/control-plane/config/local.yaml` for local development and put real local secrets there. `local.yaml` is ignored by Git.

Environment variables such as `DATABASE_URL`, `REDIS_URL`, and `S3_BUCKET` still override YAML values for container deployment.
