#!/usr/bin/env bash
litestream restore -o prod.db s3://themis-database-prod/prod.db
litestream replicate -exec='themis-server -db prod.db' prod.db s3://themis-database-prod/prod.db