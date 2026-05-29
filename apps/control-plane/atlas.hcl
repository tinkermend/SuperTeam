env "local" {
  src = "file://internal/storage/migrations"
  url = "postgres://superteam:83ab1f233b790e580ba5dae3a26998d78095f780d7067b32@127.0.0.1:5432/superteam?sslmode=disable"
  dev = "docker://postgres/16/dev"
}
