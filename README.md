# `conduit`

![alt text][logo]

The cloudfoundry cli plugin for that makes it easy to directly connect to your remote service instances.
 
## Features

* Create tunnels to remote service instances running on cloudfoundry to allow direct access.
* Enables running local application processes against live service instances by setting up a tunneled VCAP_SERVICES environment.
* Provides sugar for invocating cli tools for [supported service](#running-database-tools) types.

## Installation

Install from the plugin repository:

```
TODO
```

Install from released binary

```
// TODO
curl -o /tmp/cf-conduit-plugin http://github.com/alphagov/...release/xxx.darwin.amd64
cf install-plugin /tmp/cf-conduit-plugin
```

or build from source

```
git clone git@github.com:alphagov/paas-cf-plugin.git
cd paas-cf-plugin
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
