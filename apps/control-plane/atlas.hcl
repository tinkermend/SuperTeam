env "local" {
  src = "file://internal/storage/migrations"
  url = getenv("DATABASE_URL")
  dev = "docker://postgres/16/dev"
}
