# themis

Discord App to allow EU4 players to take claims on regions and provinces.

## Setup

### Requirements

To develop:
- [Go](https://go.dev/) version 1.19 or higher installed locally
- `sqlite3` installed locally (already ships by default on most OSes)

To deploy:
- Register for a [free account on Fly](https://fly.io)
- An [AWS account](https://console.aws.amazon.com)

### Steps

To work with the core modules that simply interact with the database, or make
sure that everything compiles correctly, you can run the following commands:

```bash
go test ./...
go build -o ./bin/ ./cmd/...
```

## Operations

This application is deployed via [Fly](https://fly.io) on a single virtual
machine instance. It uses an embed SQLite database as its database engine, and
uses [Litestream](https://litestream.io) to replicate the database to an S3
bucket.

### Application Configuration

The deployment configurations can be found in the [fly.toml](/fly.toml) file and
you can find more information on the configuration options [in the official Fly
documentation](https://fly.io/docs/reference/configuration/).

The virtual image is based off of the [Dockerfile](/Dockerfile) which is a
multi-stage build that builds the main application binary, downloads Litestream
and packages everything on top of an Ubuntu 22.04 base.

### Entrypoint

The application is started using a [custom entrypoint shell script](/start.sh)
that is in charge of first restoring the database file through Litestream and
then starting the main application as a [child process of Litestream's
replication process](https://litestream.io/reference/replicate/#arguments).

It's a very simple script but is necessary since the application doesn't have a
persistent volume to rely on and must rehydrate its database file after every
deployment.

### Environment Variables

| Env Var                 | Defined At            |
| ----------------------- | --------------------- |
| `DISCORD_TOKEN`         | fly secret            |
| `DISCORD_APP_ID`        | [fly.toml](/fly.toml) |
| `DISCORD_GUILD_ID`      | [fly.toml](fly.toml)  |
| `AWS_ACCESS_KEY_ID`     | fly secret            |
| `AWS_SECRET_ACCESS_KEY` | fly secret            |

## Local Development

### Application Entrypoint

- `./cmd/themis-server` This is the main application entrypoint
- `./cmd/themis-repl` _coming soon_

### Core Functions

The core database functions can be developed and tested locally easily, just run
`go test ./...` to test your changes locally.

You can also load a test database easily by using the `init.sql` script. You can
use the cli sqlite3 client or connect using whatever SQL editor you prefer that
can open sqlite3 connections.

```bash
sqlite3 local.db < migrations/init.sql
sqlite3 local.db
# interactive SQLite session
```

### Discord Integration

This is a work in progress, but I am currently using a dedicated Discord server
with this application already signed into it. You can contact me directly to be
invited into that server. From there you only need to set the following
environment variables before launching the `./cmd/themis-server` entrypoint:

```bash
export DISCORD_APP_ID="1014881815921705030"
export DISCORD_GUILD_ID="[test server id goes here]"
```

### Litestream replication

Litestream is _not a necessary component_ to run the `./cmd/themis-server`
entrypoint and can be safely ignored when developing locally. Still, if you wish
to try it out, you can find the Litestream commands used in production in the
[start.sh](/start.sh) script. As per the Litestream docs, it should work fine
with [Minio](https://min.io/) but I have not tested it yet nor are there any
scripts provided to run it (yet).

## SQLite

### Importing From CSV

This is a neat feature built-in to SQLite. Using the source file at
[data/eu4-provinces.csv](/data/eu4-provinces.csv) you can import the data
directly into a SQLite database using the following command:

```bash
$ sqlite3
# ...
sqlite> .mode csv
sqlite> .import data/eu4-provinces.csv provinces
sqlite> .schema provinces
CREATE TABLE provinces(
  "ID" TEXT,
  "Name" TEXT,
  "Development" TEXT,
  "BT" TEXT,
  "BP" TEXT,
  "BM" TEXT,
  "Trade good" TEXT,
  "Trade node" TEXT,
  "Modifiers" TEXT,
  "Type" TEXT,
  "Continent" TEXT,
  "Superregion" TEXT,
  "Region" TEXT,
  "Area" TEXT
);
sqlite> select count(1) from provinces;
3925
```

### Creating an SQL Dump From Imported Data

The init script at [migrations/init.sql](/migrations/init.sql) was initially
created after importing the CSV data and running the following commands:

```bash
# In the same SQlite session as last section
sqlite> .output dump.sql
sqlite> .dump
```

Note: The column names were _edited manually_ to remove capital letters and
spaces to make it easier to work with.

### Claims Schema

```sql
CREATE TABLE claim_types (
    claim_type TEXT PRIMARY KEY
);

CREATE TABLE claims (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    player TEXT,
    claim_type TEXT,
    val TEXT,
    FOREIGN KEY(claim_type) REFERENCES claim_types(claim_type)
);
```
