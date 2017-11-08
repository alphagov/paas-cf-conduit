# `conduit`

![alt text][logo]

The cloudfoundry cli plugin that makes it easy to directly connect to your remote service instances.
 
## Overview

* Create tunnels to remote service instances running on cloudfoundry to allow direct access.
* Provides a way to invoke cli tools such as `psql` or `myssqldump` for [supported service](#running-database-tools) types.
* [_experimental_] Enables running local cloudfoundry application processes against live service instances by setting up a tunneled VCAP_SERVICES environment.

## Installation

`cf-conduit` is a Cloudfoundry CLI Plugin. Cloudfoundry plugins are binaries that you download and install using the `cf install-plugin` command. For more general information on installing and using Cloudfoundry CLI Plugins please see [Using CF CLI Plugins](https://docs.cloudfoundry.org/cf-cli/use-cli-plugins.html#plugin-install)

To install `cf-conduit` from one of our released binaries:

1. Visit the [releases](https://github.com/alphagov/paas-cf-conduit/releases) page and download the latest binary for your platform. For example if you are on OSX you will liekly want to download the `cf-conduit.darwin.amd64` binary.

2. From a terminal console do:
 
    ```
    cf install-plugin ~/Downloads/cf-conduit.darwin.amd64
    ```

3. Your plugin should now be installed and you can use via:

    ```
    cf conduit --help
    ```

4. See the [usage](#usage) and [running database tools](#running-database-tools) section for examples.

## Building from source

Alternativly to build from source (you will need [dep](https://github.com/golang/dep) and [go-bindata](https://github.com/jteeuwen/go-bindata) installed to build from source)

```
go get -u -d github.com/alphagov/paas-cf-conduit
cd $GOPATH/github.com/alphagov/paas-cf-conduit
make install
```

## Usage

### General help

For help from command line:

```
cf conduit --help
```

### Creating tunnels

To tunnel a connection from your cloudfoundry hosted service instance to your local machine:

```
cf conduit my-service-instance
```

You can configure multiple tunnels at the same time:

```
cf conduit service-1 service-2
```

Output from the command will report connection details for the tunnel(s) in the foreground, hit Ctrl+C to terminate the connections.

### Running local processes

A `VCAP_SERVICES` environment variable containing binding details for each service conduit is made available to any application given after the `--` on the command line.

For example, if your Ruby based application is located at `/home/myapp/app.rb` and requires access to your `app-db` service instance you could execute it via:

```
cf conduit app-db -- ruby /home/myapp/app.rb
``` 

Alternativly you could drop yourself into a `bash` shell and work from there:

```
cf conduit app-db -- bash
...
bash$ 
```

### Running database tools

There is limited support for some common database service tools. It works by detecting certain service types and setting up the environment so that the tools pickup the service binding details by default.

Currently only [RDS broker](https://github.com/alphagov/paas-rds-broker) provided `postgres` and `mysql` service types are supported.

Note: You should only specify a single service-instance when using this method and you must install any required tools on your machine for this to work.

#### psql, pg_dump & friends

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
cf conduit pg-instance -- pgsql < backup.sql
```

Copy data from one instance to another

```
cf conduit --local-port 7001 pg-1 -- pgsql -c "COPY things TO STDOUT WITH CSV HEADER DELIMITER ','" | cf conduit --local-port 8001 pg-2 -- pgsql -c "COPY things FROM STDIN WITH CSV HEADER DELIMITER ','"
```


#### mysql, mysqldump & friends

Launch a mysql shell:

```
cf conduit mysql-instance -- mysql
```

Export mysql data:

```
cf conduit mysql-isntance -- mysqldump --all-databases
```


[logo]: logo.jpg
