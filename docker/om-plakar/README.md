# README.md

The plakar lab: a single node om3 in a container, with the om binary
built from this tree and plakar v1.1.7, to develop the plakar store
driver and run the tests T1 to T5 of the specification.

## Build and run

    make version
    docker buildx build -t opensvc/om-plakar:dev -f ./docker/om-plakar/Dockerfile .
    docker run -d --name om-plakar --hostname lab1 opensvc/om-plakar:dev
    docker exec om-plakar sh /lab/setup.sh

## Reset

    docker rm -f om-plakar

then run again.

## Tests

T1, driver detection:

    docker exec om-plakar om node capabilities scan
    docker exec om-plakar om node capabilities list | grep plakar

T2, first pass, then the snapshots in the store:

    docker exec om-plakar om test/svc/app1 sync update
    docker exec om-plakar om test/svc/app1 instance status -r
    docker exec om-plakar plakar \
        -configdir /var/lib/opensvc/store/lab/plakar/config \
        -cachedir /var/lib/opensvc/store/lab/plakar/cache \
        -datadir /var/lib/opensvc/store/lab/plakar/data \
        -keyfile /run/opensvc/store/lab/passphrase \
        at @lab ls -tags

One snapshot for fs#1 and one for the configuration, with the path, rid,
node, pass and sync_rid tags.

T4, restore aside and compare:

    docker exec om-plakar om test/svc/app1 sync restore --into /tmp/restore --rid fs#1
    docker exec om-plakar diff -r /tmp/restore/fs#1 /srv/app1
    docker exec om-plakar sh -c 'cd "/tmp/restore/fs#1" && sha256sum -c /lab/app1.sha256'

T5, schedule: the sync#1 schedule is @10m, wait two periods and check

    docker exec om-plakar om test/svc/app1 print schedule
