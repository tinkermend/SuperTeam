# SuperTeam Local Database Connection Info

## PostgreSQL

- Host: `127.0.0.1`
- Port: `5432`
- Application database: `superteam`
- Application schema: `superteam`
- Application user: `superteam`
- Application password: `83ab1f233b790e580ba5dae3a26998d78095f780d7067b32`
- JDBC-style URL: `postgresql://superteam:83ab1f233b790e580ba5dae3a26998d78095f780d7067b32@127.0.0.1:5432/superteam?sslmode=disable`
- DATABASE_URL: `postgres://superteam:83ab1f233b790e580ba5dae3a26998d78095f780d7067b32@127.0.0.1:5432/superteam?sslmode=disable`

## Redis

- Docker container: `superteam-redis`
- Image: `redis:7`
- Host: `127.0.0.1`
- Port: `6379`
- Password: `d862d604a7d5adb0d2f800e72ca68a38aa2cf8edb5b0b0fa`
- REDIS_URL: `redis://:d862d604a7d5adb0d2f800e72ca68a38aa2cf8edb5b0b0fa@127.0.0.1:6379/0`

## Verification

```bash
docker exec superteam-redis redis-cli -a 'd862d604a7d5adb0d2f800e72ca68a38aa2cf8edb5b0b0fa' ping
PGPASSWORD='83ab1f233b790e580ba5dae3a26998d78095f780d7067b32' psql -h 127.0.0.1 -p 5432 -U superteam -d superteam -c 'select current_user, current_database(), current_schema();'
```
