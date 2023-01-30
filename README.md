# `conduit`

![alt text][logo]

The Cloud Foundry cli plugin that makes it easy to directly connect to your remote service instances.

## Overview

* Create tunnels to remote service instances running on Cloud Foundry to allow direct access.
* Provides a way to invoke cli tools such as `psql` or `mysqldump` for [supported service](#running-database-tools) types.
* [_experimental_] Enables running local Cloud Foundry application processes against live service instances by setting up a tunneled VCAP_SERVICES environment.

## Installation

`cf-conduit` is a Cloud Foundry CLI Plugin. Cloud Foundry plugins are binaries that you download and install using the `cf install-plugin` command. For more general information on installing and using Cloud Foundry CLI Plugins please see [Using CF CLI Plugins](https://docs.cloudfoundry.org/cf-cli/use-cli-plugins.html#plugin-install)

To install `cf-conduit`:

1. Run the following code from the command line:

    ```
    cf install-plugin conduit
    ```

2. Your plugin should now be installed and you can use via:

    ```
    cf conduit --help
    ```

See the [usage](#usage) and [running database tools](#running-database-tools) section for examples.

## Building from source

Alternatively, you can build from source. You'll need Go 1.13 or higher.

```
go get -u -d github.com/alphagov/paas-cf-conduit
cd $GOPATH/src/github.com/alphagov/paas-cf-conduit
make install
```

## Usage

### General help

For help from command line:

```
cf conduit --help
```

### Creating tunnels

To tunnel a connection from your Cloud Foundry hosted service instance to your local machine:

```
cf conduit my-service-instance
```

You can configure multiple tunnels at the same time:

```
cf conduit service-1 service-2
```

Output from the command will report connection details for the tunnel(s) in the foreground, hit Ctrl+C to terminate the connections.

### Conduit apps

Traditionally `cf-conduit` creates an app to implement the tunnel. This app can be named with the `--app-name` option. The app `cf-conduit` created, will be deleted when the tunnel is closed. `--no-delete` option stops `cf-conduit` from deleting the app when the tunnel closes.

### Reusing existing app

An existing app can be reused, providing `--existing-app` flag with `--app-name`. `cf-conduit` will not delete an existing app while using this option. The existing app needs to be bound to the services we want cf-conduit to tunnel.

### Running database tools

There is limited support for some common database service tools. It works by detecting certain service types and setting up the environment so that the tools pickup the service binding details by default.

Currently only [GOV.UK PaaS RDS broker](https://github.com/alphagov/paas-rds-broker) provided `postgres` and `mysql` service types are supported.

Note: You should only specify a single service-instance when using this method and you must install any required tools on your machine for this to work.

#### Postgres

Launch a psql shell:

```
cf conduit pg-instance -- psql
```

Export a postgres database:

```
cf conduit pg-instance -- pg_dump -f backup.sql
```

Import a postgres dump

```
cf conduit pg-instance -- psql < backup.sql
```

Copy data from one instance to another

```
cf conduit --local-port 7001 pg-1 -- psql -c "COPY things TO STDOUT WITH CSV HEADER DELIMITER ','" | cf conduit --local-port 8001 pg-2 -- psql -c "COPY things FROM STDIN WITH CSV HEADER DELIMITER ','"
```

Launch a psql shell from Docker for Mac:

```
cf conduit pg-instance -- docker run --rm -ti -e PGUSER -e PGPASSWORD -e PGDATABASE -e PGPORT -e PGHOST=docker.for.mac.localhost postgres:9.5-alpine psql
```

#### MySQL

Launch a mysql shell:

```
cf conduit mysql-instance -- mysql
```

Export a mysql database:

```
cf conduit mysql-instance -- mysqldump --result-file backup.sql some_database_name
```

Import a mysql dump

```
cf conduit mysql-instance -- mysql < backup.sql
```

#### Redis

Launch a Redis shell:

```
cf conduit redis-instance -- redis-cli
```

Run a Redis command:

```
cf conduit redis-instance -- redis-cli get mykey
```

### Running local processes

A `VCAP_SERVICES` environment variable containing binding details for each service conduit is made available to any application given after the `--` on the command line.

For example, if your Ruby based application is located at `/home/myapp/app.rb` and requires access to your `app-db` service instance you could execute it via:

```
cf conduit app-db -- ruby /home/myapp/app.rb
```

Alternatively you could drop yourself into a `bash` shell and work from there:

```
cf conduit app-db -- bash
...
bash$
```

[logo]: logo.jpg

## Development


To run the tests, run `make test`


To release a new version:

1. Checkout latest main
1. Generate artefacts: `make dist`
1. Create a new tag: `git tag v0.0.xyz`
1. Push the new tag: `git push --tag`
1. Create a new release via the GitHub UI with release artefacts from `bin`
1. Raise a pull request via the
   [release repo](https://github.com/alphagov/paas-cf-cli-plugin-repo)
   using `make generate-release-yaml`
