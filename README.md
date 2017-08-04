# redis_exporter
Simple server that scrapes Redis stats and exports them via HTTP for Prometheus consumption

## Build

It is as simple as:

    $ make

## Running

    $ ./redis_exporter

With default options, redis_export will listen at 0.0.0.0:9739 and
scrapes redis(127.0.0.1:6739).
To change default options, see:

    $ ./redis_exporter --help

## License

Copyright (C) 2017 Joyield, Inc. <joyield.com#gmail.com>

All rights reserved.

License under BSD 3-clause "New" or "Revised" License
